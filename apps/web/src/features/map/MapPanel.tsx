import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Activity, Day, DayRoute, Location, RouteMode } from '../../types'
import { loadAMap, type AMapMap, type AMapNS, type AMapOverlay } from './amap'
import { mapApi } from './api'

// PlacedPoint 是地图上可渲染的一个点：precise=已绑定 location（参与后端路线），
// false=前端按标题实时搜索的兜底结果（仅画灰色 marker，不进路线）。
interface PlacedPoint {
  activity: Activity
  lng: number
  lat: number
  precise: boolean
}

// fallbackCache 跨渲染记住「activityId → 兜底搜索结果」，null 表示搜过但无结果，
// 避免时间轴每次刷新都重新烧搜索配额。
type FallbackCache = Map<string, Location | null>

// 兜底搜索只对「地点型」活动做：transport 的标题是「成都出发经都汶高速前往卧龙」
// 这类行程描述，搜 POI 得到的是噪声；与后端自动定位白名单保持一致。
const SEARCHABLE_TYPES = new Set(['attraction', 'restaurant', 'cafe', 'hotel'])

interface Props {
  destination: string | null
  days: Day[]
  selectedDayId: string | null
  onSelectDay: (dayId: string) => void
  selectedActivityId: string | null
  onSelectActivity: (activityId: string) => void
  /** 公开分享模式：fallback 搜索与路线都走无需登录的分享端点 */
  publicToken?: string
}

function fmtDistance(m: number): string {
  if (m >= 1000) return `${(m / 1000).toFixed(1)} km`
  return `${m} m`
}

function fmtDuration(s: number): string {
  return `${Math.round(s / 60)}`
}

// haversineKm：两点大圆距离（km），fallback 锚过滤用。
function haversineKm(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const toRad = (d: number) => (d * Math.PI) / 180
  const dLat = toRad(lat2 - lat1)
  const dLng = toRad(lng2 - lng1)
  const a =
    Math.sin(dLat / 2) ** 2 +
    Math.cos(toRad(lat1)) * Math.cos(toRad(lat2)) * Math.sin(dLng / 2) ** 2
  return 2 * 6371 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a))
}

// fallback 搜索同样会全国错绑（"日隆镇晚餐"→ 丽江藏餐吧）。与后端锚链同理：
// 灰点距任一精确定位锚超过此距离时丢弃。阈值与后端 maxAnchorKm 一致——
// 一天行程半径很少超 300km，而典型错绑（阿坝→丽江 518km、阿坝→北京 1500km）都在 400km 外。
const FALLBACK_MAX_ANCHOR_KM = 400

// markerHTML 生成编号圆点：选中放大，未定位的灰色。
function markerHTML(n: number, precise: boolean, selected: boolean): string {
  const bg = selected ? '#1d4ed8' : precise ? '#2563eb' : '#94a3b8'
  const size = selected ? 32 : 28
  const border = selected ? 'box-shadow:0 0 0 3px rgba(29,78,216,.3);' : ''
  return `<div style="width:${size}px;height:${size}px;border-radius:50%;background:${bg};color:#fff;` +
    `display:flex;align-items:center;justify-content:center;font-size:12px;font-weight:700;` +
    `border:2px solid #fff;box-sizing:border-box;${border}cursor:pointer">${n}</div>`
}

export default function MapPanel({
  destination,
  days,
  selectedDayId,
  onSelectDay,
  selectedActivityId,
  onSelectActivity,
  publicToken,
}: Props) {
  const { t } = useTranslation()
  const containerRef = useRef<HTMLDivElement>(null)
  const amapRef = useRef<AMapNS>(null)
  const mapRef = useRef<AMapMap>(null)
  const markersRef = useRef<AMapOverlay[]>([])
  const polylinesRef = useRef<AMapOverlay[]>([])
  const infoRef = useRef<AMapOverlay>(null)
  const markerById = useRef<Map<string, AMapOverlay>>(new Map())
  const fallbackCache = useRef<FallbackCache>(new Map())
  // 上次渲染的天：只有切换 day 才 setFitView，点选 marker 时保持视野不动。
  const fittedDayRef = useRef<string | null>(null)

  const [ready, setReady] = useState(false)
  const [loadFailed, setLoadFailed] = useState(false)
  const [mode, setMode] = useState<RouteMode>('driving')
  const [route, setRoute] = useState<DayRoute | null>(null)
  const [routeLoading, setRouteLoading] = useState(false)
  const [points, setPoints] = useState<PlacedPoint[]>([])

  const mapEnabled = useMemo(() => Boolean(import.meta.env.VITE_AMAP_KEY), [])

  // 当前展示的天：外部选中的优先，否则第一天。
  // useMemo 稳定引用——否则 days.find() 每次渲染都返回新数组里看似相同实则
  // 新引用的对象，effect 因引用漂移反复重跑（cleanup 取消搜索→搜索永不完→
  // 永远重发，最终触发 Maximum update depth 与限流 429/502）。
  const activeDay: Day | undefined = useMemo(
    () => days.find((d) => d.id === selectedDayId) ?? days[0],
    [days, selectedDayId],
  )
  const city = destination ?? ''

  // --- 初始化地图（mount 一次） ---
  useEffect(() => {
    if (!mapEnabled || !containerRef.current) return
    let cancelled = false
    let map: AMapMap = null
    const p = loadAMap()
    if (!p) return
    p.then((AMap: AMapNS) => {
      if (cancelled || !containerRef.current) return
      amapRef.current = AMap
      map = new AMap.Map(containerRef.current, {
        zoom: 11,
        center: [116.397, 39.909], // 默认北京，首个点位渲染后 setFitView 覆盖
        viewMode: '2D',
      })
      map.addControl(new AMap.Scale())
      mapRef.current = map
      infoRef.current = new AMap.InfoWindow({ offset: new AMap.Pixel(0, -34), closeWhenClickMap: true })
      setReady(true)
    }).catch(() => {
      if (!cancelled) setLoadFailed(true)
    })
    return () => {
      cancelled = true
      markersRef.current = []
      polylinesRef.current = []
      markerById.current.clear()
      if (map) {
        map.destroy()
        mapRef.current = null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mapEnabled])

  // --- 点位计算：已绑定坐标直接画，未绑定的走实时搜索兜底（带缓存 + 锚过滤） ---
  // 无 cacheVersion 自增：搜索完成后直接局部 setPoints，effect 只在
  // activeDay/city 变化时运行——从结构上消除 setState→重渲染→effect 的循环。
  useEffect(() => {
    const day = activeDay
    if (!day) {
      setPoints([])
      return
    }
    const cache = fallbackCache.current
    const precise = day.activities
      .filter((a) => a.location)
      .map((a) => ({
        activity: a,
        lng: a.location!.longitude,
        lat: a.location!.latitude,
        precise: true,
      }))
    // 锚过滤：灰点离所有精点太远（跨省级错绑）即丢弃
    const accept = (loc: Location): boolean =>
      precise.length === 0 ||
      precise.some(
        (p) => haversineKm(p.lat, p.lng, loc.latitude, loc.longitude) <= FALLBACK_MAX_ANCHOR_KM,
      )
    const extra: PlacedPoint[] = []
    const pending: Activity[] = []
    for (const a of day.activities) {
      if (a.location || !SEARCHABLE_TYPES.has(a.type)) continue
      if (!cache.has(a.id)) {
        // 公开分享模式不做实时搜索（该端点需登录），只画已绑定坐标
        if (!publicToken) pending.push(a)
        continue
      }
      const loc = cache.get(a.id)
      if (loc && accept(loc)) {
        extra.push({ activity: a, lng: loc.longitude, lat: loc.latitude, precise: false })
      }
    }
    const byOrder = (x: PlacedPoint, y: PlacedPoint) => x.activity.sort_order - y.activity.sort_order
    setPoints([...precise, ...extra].sort(byOrder))

    if (pending.length === 0) return
    let cancelled = false
    Promise.all(
      pending.map((a) =>
        mapApi
          .searchLocations(a.title, city)
          .then((locs) => ({ a, loc: locs[0] ?? null }))
          .catch(() => ({ a, loc: null as Location | null })),
      ),
    ).then((results) => {
      if (cancelled) return
      const newExtra = [...extra]
      for (const { a, loc } of results) {
        cache.set(a.id, loc)
        if (loc && accept(loc)) {
          newExtra.push({ activity: a, lng: loc.longitude, lat: loc.latitude, precise: false })
        }
      }
      setPoints([...precise, ...newExtra].sort(byOrder))
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeDay, city])

  // --- 路线：仅基于已绑定坐标的活动（后端路线数据源），>=2 点才请求 ---
  const preciseCount = useMemo(
    () => (activeDay ? activeDay.activities.filter((a) => a.location).length : 0),
    [activeDay],
  )
  useEffect(() => {
    const day = activeDay
    if (!day || preciseCount < 2) {
      setRoute(null)
      return
    }
    let cancelled = false
    setRouteLoading(true)
    const req = publicToken
      ? mapApi.getShareRoute(publicToken, day.id, mode)
      : mapApi.getDayRoute(day.id, mode)
    req
      .then((r) => {
        if (!cancelled) setRoute(r)
      })
      .catch(() => {
        // 路线失败（配额/网络）静默降级：markers 仍然可用。
        if (!cancelled) setRoute(null)
      })
      .finally(() => {
        if (!cancelled) setRouteLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [activeDay, preciseCount, mode, publicToken])

  // --- 绘制 markers + 路线 ---
  useEffect(() => {
    const map = mapRef.current
    const AMap = amapRef.current
    if (!map || !AMap || !ready) return

    for (const m of markersRef.current) map.remove(m)
    for (const pl of polylinesRef.current) map.remove(pl)
    markersRef.current = []
    polylinesRef.current = []
    markerById.current.clear()
    infoRef.current?.close()

    // 路线在 marker 之下
    for (const leg of route?.legs ?? []) {
      const path = leg.polyline
        .split(';')
        .map((pair) => pair.split(',').map(Number))
        .filter((c) => c.length === 2 && c.every((n) => Number.isFinite(n)))
      if (path.length < 2) continue
      const pl = new AMap.Polyline({
        path,
        strokeColor: '#2563eb',
        strokeWeight: 5,
        strokeOpacity: 0.75,
        lineJoin: 'round',
        showDir: true,
      })
      map.add(pl)
      polylinesRef.current.push(pl)
    }

    points.forEach((p, i) => {
      const selected = p.activity.id === selectedActivityId
      const marker = new AMap.Marker({
        position: [p.lng, p.lat],
        content: markerHTML(i + 1, p.precise, selected),
        offset: new AMap.Pixel(-14, -14),
        zIndex: selected ? 120 : 100,
        extData: p.activity.id,
      })
      marker.on('click', () => onSelectActivity(p.activity.id))
      map.add(marker)
      markersRef.current.push(marker)
      markerById.current.set(p.activity.id, marker)
    })

    // 仅在切换天且 points 已换成该天的数据时自适应视野。
    // points 更新比 activeDay 晚一个 commit，若只看 day 变化会在旧 points 上
    // fitView 一次、随后正确 points 到达时又不再 fit（视野卡在旧天）。
    const pointsMatchDay = points.length > 0 && points[0].activity.day_id === activeDay?.id
    if (pointsMatchDay && fittedDayRef.current !== activeDay?.id) {
      // 显式传 overlays：setFitView(null) 读「全部覆盖物」对刚 add 的 marker
      // 有 bounds 时序坑（首次加载静默不生效）；immediately=true 同步跳转。
      const overlays = [...markersRef.current, ...polylinesRef.current]
      if (overlays.length > 0) {
        map.setFitView(overlays, true, [40, 40, 40, 40])
        fittedDayRef.current = activeDay?.id ?? null
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [points, route, ready, activeDay?.id])

  // --- 外部选中（点时间轴卡片）→ 定位 + 信息窗 ---
  useEffect(() => {
    const map = mapRef.current
    const AMap = amapRef.current
    if (!map || !AMap || !selectedActivityId) return
    const marker = markerById.current.get(selectedActivityId)
    const idx = points.findIndex((p) => p.activity.id === selectedActivityId)
    if (!marker || idx < 0) {
      // 选中的活动不在当前展示的天（切 day 后）：关掉残留的信息窗
      infoRef.current?.close()
      return
    }
    const p = points[idx]
    map.panTo([p.lng, p.lat])
    const a = p.activity
    const timeRange = [a.start_time, a.end_time].filter(Boolean).join(' – ')
    infoRef.current?.setContent(
      `<div style="padding:6px 10px;font-size:13px;line-height:1.5">` +
        `<strong>${escapeHtml(a.title)}</strong>` +
        (timeRange ? `<div style="color:#64748b;font-size:12px">${escapeHtml(timeRange)}</div>` : '') +
        (p.precise
          ? ''
          : `<div style="color:#b45309;font-size:11px">${escapeHtml(t('map.approximate'))}</div>`) +
        `</div>`,
    )
    infoRef.current?.open(map, [p.lng, p.lat])
  }, [selectedActivityId, points, t])

  if (!mapEnabled) {
    return (
      <div className="rounded-2xl border border-dashed border-slate-300 bg-white/60 p-4 text-center text-xs text-slate-400">
        {t('map.noKey')}
      </div>
    )
  }
  if (loadFailed) {
    return (
      <div className="rounded-2xl border border-slate-200 bg-white p-4 text-center text-xs text-slate-500">
        {t('map.loadError')}
      </div>
    )
  }
  if (days.length === 0) return null

  return (
    <div className="rounded-2xl border border-slate-200 bg-white p-3 shadow-sm">
      {/* Day tabs */}
      <div className="mb-2 flex items-center gap-1.5 overflow-x-auto pb-0.5">
        {days.map((d, idx) => {
          const active = d.id === activeDay?.id
          return (
            <button
              key={d.id}
              type="button"
              onClick={() => onSelectDay(d.id)}
              className={`shrink-0 rounded-lg px-2.5 py-1 text-xs font-medium transition-colors ${
                active
                  ? 'bg-primary-600 text-white'
                  : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
              }`}
            >
              {t('planner.dayLabel', { n: idx + 1 })}
            </button>
          )
        })}
      </div>

      {/* 模式切换 */}
      <div className="mb-2 flex items-center justify-between">
        <span className="text-xs font-medium text-slate-500">{t('map.title')}</span>
        <div className="flex gap-1">
          {(['driving', 'walking'] as const).map((m) => (
            <button
              key={m}
              type="button"
              onClick={() => setMode(m)}
              className={`rounded-md px-2 py-0.5 text-[11px] font-medium transition-colors ${
                mode === m
                  ? 'bg-primary-100 text-primary-700'
                  : 'text-slate-500 hover:bg-slate-100'
              }`}
            >
              {t(`map.mode.${m}`)}
            </button>
          ))}
        </div>
      </div>

      {/* 地图容器 */}
      <div ref={containerRef} className="h-72 w-full overflow-hidden rounded-xl bg-slate-100" />

      {/* 汇总行 */}
      <div className="mt-2 flex items-center justify-between text-xs text-slate-500">
        <span>
          {route && route.legs.length > 0
            ? t('map.summary', {
                distance: fmtDistance(route.total_distance_m),
                duration: fmtDuration(route.total_duration_s),
                mode: t(`map.mode.${mode}`),
              })
            : routeLoading
              ? t('map.routeLoading')
              : preciseCount < 2
                ? t('map.tooFewPoints')
                : t('map.noRoute')}
        </span>
        {points.some((p) => !p.precise) && (
          <span className="text-[11px] text-amber-600">{t('map.hasApproximate')}</span>
        )}
      </div>
    </div>
  )
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}
