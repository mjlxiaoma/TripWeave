import { useCallback, useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { usePlannerChat } from '../ai/usePlannerChat'
import AiComposer from '../ai/AiComposer'
import AiMessageList from '../ai/AiMessageList'
import SummaryCard from '../ai/SummaryCard'
import DayTimeline from './DayTimeline'
import TripHeader from './TripHeader'
import { plannerApi } from './api'
import type { Day, Trip } from '../../types'

export default function PlannerPage() {
  const { t } = useTranslation()
  const { tripId = '' } = useParams<{ tripId: string }>()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()

  const [trip, setTrip] = useState<Trip | null>(null)
  const [days, setDays] = useState<Day[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)

  const refreshDays = useCallback(async () => {
    if (!tripId) return
    try {
      const d = await plannerApi.fetchDays(tripId)
      setDays(d)
    } catch {
      /* 静默:时间轴刷新失败不打断对话 */
    }
  }, [tripId])

  const { state, send, stop } = usePlannerChat(tripId, refreshDays)

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

  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col">
      <TripHeader trip={trip} onBack={() => navigate('/trips')} />

      <div className="mx-auto grid w-full max-w-6xl flex-1 grid-cols-1 gap-6 px-4 py-6 lg:grid-cols-3">
        {/* 左:时间轴(占 2/3) */}
        <div className="lg:col-span-2">
          <DayTimeline days={days} generating={generating} />
        </div>

        {/* 右:AI 摘要 + 上下文 */}
        <aside className="space-y-4 lg:col-span-1">
          <SummaryCard lines={state.summaryLines} changes={state.summary} />
          <div className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <h3 className="mb-3 text-sm font-semibold text-slate-900">{t('planner.contextTitle')}</h3>
            <dl className="space-y-2 text-xs text-slate-600">
              {trip.destination && (
                <div className="flex justify-between">
                  <dt className="text-slate-400">{t('planner.destination')}</dt>
                  <dd className="font-medium">{trip.destination}</dd>
                </div>
              )}
              {trip.travelers_count != null && (
                <div className="flex justify-between">
                  <dt className="text-slate-400">{t('planner.travelers')}</dt>
                  <dd className="font-medium">{trip.travelers_count}</dd>
                </div>
              )}
              {trip.preference?.budget != null && (
                <div className="flex justify-between">
                  <dt className="text-slate-400">{t('planner.budget')}</dt>
                  <dd className="font-medium">¥{trip.preference.budget.toLocaleString()}</dd>
                </div>
              )}
            </dl>
            {trip.preference?.preferences && trip.preference.preferences.length > 0 && (
              <div className="mt-3 flex flex-wrap gap-1.5">
                {trip.preference.preferences.map((p) => (
                  <span key={p} className="rounded-full bg-primary-50 px-2.5 py-0.5 text-[11px] font-medium text-primary-700">
                    {t(`wizard.prefTags.${p}`, { defaultValue: p })}
                  </span>
                ))}
              </div>
            )}
          </div>
        </aside>
      </div>

      {/* 底部:对话区 */}
      <div className="sticky bottom-0 border-t border-slate-200 bg-slate-50/95 backdrop-blur">
        <div className="mx-auto max-w-6xl">
          <div className="max-h-64 overflow-y-auto">
            <AiMessageList messages={state.messages} streaming={state.streaming} />
          </div>
          {state.error && (
            <p className="mx-4 mb-2 rounded-xl bg-rose-50 px-4 py-2 text-xs text-rose-700">{state.error}</p>
          )}
          <AiComposer streaming={state.streaming} onSend={send} onStop={stop} />
        </div>
      </div>
    </div>
  )
}
