import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { usePlannerChat } from '../ai/usePlannerChat'
import AiComposer from '../ai/AiComposer'
import AiMessageList from '../ai/AiMessageList'
import SummaryCard from '../ai/SummaryCard'
import MapPanel from '../map/MapPanel'
import DayTimeline from './DayTimeline'
import TripHeader from './TripHeader'
import { plannerApi } from './api'
import type { Day, Trip } from '../../types'

// 后端自动定位的活动类型（与 planner/locate.go 的 locatableTypes 一致）；
// transport/free_time/other 不定位，轮询对它们无意义。
const LOCATABLE_TYPES = new Set(['attraction', 'restaurant', 'cafe', 'hotel'])

export default function PlannerPage() {
  const { t } = useTranslation()
  const { tripId = '' } = useParams<{ tripId: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const [trip, setTrip] = useState<Trip | null>(null)
  const [days, setDays] = useState<Day[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  // 地图 ↔ 时间轴双向联动的选中态（null = 未选中，地图默认展示第一天）
  const [selectedDayId, setSelectedDayId] = useState<string | null>(null)
  const [selectedActivityId, setSelectedActivityId] = useState<string | null>(null)

  // 时间轴卡片点击 → 地图定位（再次点击取消选中）
  const selectActivity = useCallback((activityId: string) => {
    setSelectedActivityId((prev) => (prev === activityId ? null : activityId))
  }, [])

  // 地图 marker 点击 → 切到该活动所在的 Day、高亮时间轴卡片并滚入视野
  const handleMapSelectActivity = useCallback(
    (activityId: string) => {
      const day = days.find((d) => d.activities.some((a) => a.id === activityId))
      if (day && day.id !== selectedDayId) setSelectedDayId(day.id)
      setSelectedActivityId(activityId)
      // 等切 Day 后的时间轴渲染完成再滚动
      window.setTimeout(() => {
        document
          .getElementById(`activity-${activityId}`)
          ?.scrollIntoView({ behavior: 'smooth', block: 'center' })
      }, 60)
    },
    [days, selectedDayId],
  )

  // 后端自动定位是后台串行任务(节流+重试,数秒),AI 回合结束时的那次 refresh
  // 拿到的常是「尚未定位」的快照。这里做有限轮询补齐坐标,避免地图停在
  // 「坐标点不足」。发送新消息时重置计数。
  const locatePollsRef = useRef(0)
  const locateTimerRef = useRef<number | undefined>(undefined)
  useEffect(() => () => window.clearTimeout(locateTimerRef.current), [])

  const refreshDays = useCallback(async () => {
    if (!tripId) return
    try {
      const d = await plannerApi.fetchDays(tripId)
      setDays(d)
      pollLocate(d)
    } catch {
      /* 静默:时间轴刷新失败不打断对话 */
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tripId])

  // pollLocate: 若还有白名单类型活动未定位,3s 后再拉一次(最多 3 次)。
  function pollLocate(d: Day[]) {
    const pending = d.some((day) =>
      day.activities.some((a) => !a.location && LOCATABLE_TYPES.has(a.type)),
    )
    if (!pending || locatePollsRef.current >= 3) return
    locatePollsRef.current += 1
    locateTimerRef.current = window.setTimeout(() => {
      plannerApi
        .fetchDays(tripId)
        .then((fresh) => {
          setDays(fresh)
          pollLocate(fresh)
        })
        .catch(() => {})
    }, 3000)
  }

  const { state, send, stop } = usePlannerChat(tripId, refreshDays)

  // 发送消息时重置轮询计数(新一轮 AI 可能又产生待定位活动)
  const sendMessage = useCallback(
    (msg: string) => {
      locatePollsRef.current = 0
      send(msg)
    },
    [send],
  )

  // 初始加载 trip + days。
  useEffect(() => {
    let cancelled = false
    if (!tripId) return
    setLoading(true)
    Promise.all([plannerApi.getTrip(tripId), plannerApi.fetchDays(tripId)])
      .then(([tr, d]) => {
        if (cancelled) return
        setTrip(tr)
        setDays(d)
        setLoadError(null)
      })
      .catch(() => {
        if (!cancelled) setLoadError(t('planner.loadError'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [tripId, t])

  // autostart:首页「开始规划」带来的自然语言,作为第一条消息自动发送。
  const autostartedRef = useRef(false)
  useEffect(() => {
    if (autostartedRef.current) return
    if (searchParams.get('autostart') !== '1') return
    if (loading || !state.loaded) return
    const nl = trip?.preference?.natural_language
    const shouldStart = days.length === 0 && state.messages.length === 0 && nl && nl.trim()
    autostartedRef.current = true
    // 清掉 query,防刷新重发。
    setSearchParams({}, { replace: true })
    if (shouldStart) {
      send(nl.trim())
    }
  }, [searchParams, loading, state.loaded, state.messages.length, days.length, trip, send, setSearchParams])

  if (loading) {
    return (
      <div className="flex h-[60vh] items-center justify-center">
        <span className="h-8 w-8 animate-spin rounded-full border-2 border-primary-600 border-t-transparent" />
      </div>
    )
  }
  if (loadError || !trip) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-16 text-center">
        <p className="rounded-xl bg-rose-50 px-4 py-3 text-sm text-rose-700">{loadError ?? t('planner.loadError')}</p>
      </div>
    )
  }

  const generating = state.streaming && days.length === 0

  // 三栏工作区：lg 下页面锁定视口高度、不滚动，滚动只发生在三个面板内部；
  // 小屏回退为纵向堆叠 + 页面滚动。
  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col lg:h-[calc(100vh-4rem)] lg:overflow-hidden">
      <TripHeader trip={trip} onBack={() => navigate('/trips')} />

      <div className="flex min-h-0 flex-1 flex-col gap-3 p-3 lg:flex-row">
        {/* 左栏:行程面板(Day 标签 + 时间轴,独立滚动) */}
        <section className="flex min-h-0 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm lg:w-[420px] lg:shrink-0">
          <DayTimeline
            tripId={tripId}
            days={days}
            generating={generating}
            destination={trip.destination}
            selectedActivityId={selectedActivityId}
            onSelectActivity={selectActivity}
            onDaysChange={setDays}
            onRefetch={refreshDays}
            activeDayId={selectedDayId}
            onSelectDay={setSelectedDayId}
          />
        </section>

        {/* 中栏:地图(始终填满,摘要浮层在图内) */}
        <section className="h-[340px] shrink-0 lg:h-auto lg:min-h-0 lg:flex-1">
          <MapPanel
            destination={trip.destination}
            days={days}
            selectedDayId={selectedDayId}
            onSelectDay={setSelectedDayId}
            selectedActivityId={selectedActivityId}
            onSelectActivity={handleMapSelectActivity}
            hideDayTabs
            fillHeight
          />
        </section>

        {/* 右栏:AI 助手面板(面板头固定 / 消息流滚动 / 输入框固定底部) */}
        <aside className="flex h-[440px] shrink-0 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm lg:h-auto lg:min-h-0 lg:w-[400px]">
          <div className="flex shrink-0 items-center gap-2 border-b border-slate-100 px-4 py-3">
            <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4 text-ai-600">
              <path d="M12 2.5l1.9 5.6 5.6 1.9-5.6 1.9L12 17.5l-1.9-5.6-5.6-1.9 5.6-1.9L12 2.5z" />
            </svg>
            <h2 className="text-sm font-semibold text-slate-900">{t('ai.panelTitle')}</h2>
            <span className="ml-auto rounded-full bg-ai-50 px-2.5 py-0.5 text-[11px] font-medium text-ai-700">
              {state.streaming
                ? t('ai.statusWorking')
                : days.length > 0
                  ? t('ai.statusReady')
                  : t('ai.statusIdle')}
            </span>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto">
            <AiMessageList messages={state.messages} streaming={state.streaming} />
            {/* 变更摘要卡内嵌在对话流末尾,随消息上下文滚动 */}
            <div className="px-4 pb-4">
              <SummaryCard lines={state.summaryLines} changes={state.summary} />
            </div>
          </div>

          {state.error && (
            <p className="mx-4 mb-2 shrink-0 rounded-xl bg-rose-50 px-4 py-2 text-xs text-rose-700">{state.error}</p>
          )}
          <div className="shrink-0">
            <AiComposer streaming={state.streaming} onSend={sendMessage} onStop={stop} />
          </div>
        </aside>
      </div>
    </div>
  )
}
