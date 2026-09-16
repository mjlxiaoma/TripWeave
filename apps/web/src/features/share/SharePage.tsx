import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { toPng } from 'html-to-image'
import DayTimeline from '../planner/DayTimeline'
import MapPanel from '../map/MapPanel'
import { shareApi, type ShareData } from './api'

// SharePage 是无需登录的行程只读视图（/share/:token）。
// 时间轴与地图复用 planner 组件的 readonly/public 模式。
export default function SharePage() {
  const { t } = useTranslation()
  const { token = '' } = useParams<{ token: string }>()
  const [data, setData] = useState<ShareData | null>(null)
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [selectedDayId, setSelectedDayId] = useState<string | null>(null)
  const [selectedActivityId, setSelectedActivityId] = useState<string | null>(null)
  const [exporting, setExporting] = useState(false)
  const contentRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let cancelled = false
    shareApi
      .get(token)
      .then((d) => {
        if (!cancelled) setData(d)
      })
      .catch(() => {
        if (!cancelled) setNotFound(true)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [token])

  async function exportImage() {
    const el = contentRef.current
    if (!el || exporting) return
    setExporting(true)
    try {
      // 等 React 重渲染（地图面板隐藏、时间轴全宽）后再截图。
      // 高德底图 canvas/tile 跨域不可读，整块地图在导出图里是空白，所以导出时
      // 整个地图面板不参与——导出图 = 全宽时间轴 + 水印。
      await new Promise((r) => setTimeout(r, 60))
      const url = await toPng(el, { backgroundColor: '#f8fafc', pixelRatio: 2 })
      const a = document.createElement('a')
      a.href = url
      a.download = `${data?.trip.title ?? 'trip'}.png`
      a.click()
    } catch {
      /* 导出失败静默（canvas 受污染等极端情况） */
    } finally {
      setExporting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex h-[60vh] items-center justify-center">
        <span className="h-8 w-8 animate-spin rounded-full border-2 border-primary-600 border-t-transparent" />
      </div>
    )
  }
  if (notFound || !data) {
    return (
      <div className="mx-auto max-w-md px-4 py-20 text-center">
        <p className="text-4xl">🧭</p>
        <h1 className="mt-3 text-lg font-semibold text-slate-900">{t('share.notFound')}</h1>
        <p className="mt-1 text-sm text-slate-500">{t('share.notFoundHint')}</p>
      </div>
    )
  }

  const { trip, days } = data
  const range = [trip.start_date, trip.end_date].filter(Boolean).join(' ~ ')

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      {/* 头部（不进导出图） */}
      <div className="mb-4 flex items-start justify-between gap-3">
        <div>
          <span className="rounded-full bg-primary-50 px-2.5 py-0.5 text-[11px] font-semibold text-primary-700">
            {t('share.badge')}
          </span>
          <h1 className="mt-1.5 text-xl font-bold text-slate-900">{trip.title}</h1>
          <p className="text-sm text-slate-500">
            {[trip.destination, range, trip.travelers_count ? t('share.travelers', { count: trip.travelers_count }) : null]
              .filter(Boolean)
              .join(' · ')}
          </p>
        </div>
        <button
          type="button"
          onClick={exportImage}
          disabled={exporting}
          className="shrink-0 rounded-xl bg-primary-600 px-4 py-2 text-sm font-semibold text-white hover:bg-primary-700 disabled:opacity-50"
        >
          {exporting ? t('share.exporting') : t('share.export')}
        </button>
      </div>

      {/* 导出区域：时间轴 + 地图 + 水印。导出时地图面板隐藏、时间轴全宽。 */}
      <div ref={contentRef} className="rounded-2xl bg-slate-50 p-4">
        <div className={`grid gap-6 ${exporting ? 'grid-cols-1' : 'grid-cols-1 lg:grid-cols-3'}`}>
          <div className={exporting ? '' : 'lg:col-span-2'}>
            <DayTimeline
              tripId={trip.id}
              days={days}
              generating={false}
              destination={trip.destination}
              selectedActivityId={selectedActivityId}
              onSelectActivity={(id) => setSelectedActivityId((p) => (p === id ? null : id))}
              onDaysChange={() => {}}
              onRefetch={() => {}}
              readonly
            />
          </div>
          {!exporting && (
            <aside className="lg:col-span-1">
              <div className="lg:sticky lg:top-4">
                <MapPanel
                  destination={trip.destination}
                  days={days}
                  selectedDayId={selectedDayId}
                  onSelectDay={setSelectedDayId}
                  selectedActivityId={selectedActivityId}
                  onSelectActivity={setSelectedActivityId}
                  publicToken={token}
                />
              </div>
            </aside>
          )}
        </div>
        <p className="mt-6 text-center text-xs text-slate-400">
          {t('share.watermark', { title: trip.title })} · TripWeave 旅迹编织
        </p>
      </div>
    </div>
  )
}
