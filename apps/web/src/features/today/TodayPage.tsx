import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { usePlannerChat } from '../ai/usePlannerChat'
import AiComposer from '../ai/AiComposer'
import AiMessageList from '../ai/AiMessageList'
import { plannerApi } from '../planner/api'
import type { Activity, ActivityStatus, Day, Trip } from '../../types'

// --- 时间工具 ---

function todayStr(): string {
  const d = new Date()
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${mm}-${dd}`
}

function nowHHMM(): string {
  const d = new Date()
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

function shortTime(t?: string | null): string {
  return t ? t.slice(0, 5) : ''
}

function timeLabel(a: Activity): string {
  const s = shortTime(a.start_time)
  const e = shortTime(a.end_time)
  if (s && e) return `${s} – ${e}`
  return s || '—'
}

// addDays：YYYY-MM-DD ± n 天（本地时区，行程日期推算用）。
function addDays(dateStr: string, n: number): string {
  const d = new Date(dateStr + 'T00:00:00')
  d.setDate(d.getDate() + n)
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${mm}-${dd}`
}

// 高德深链：position 为 GCJ-02（与 locations 表一致），callnative 尝试唤起 App。
function amapUri(a: Activity): string {
  if (!a.location) return ''
  const { longitude, latitude } = a.location
  return `https://uri.amap.com/marker?position=${longitude},${latitude}&name=${encodeURIComponent(a.title)}&src=tripweave&coordinate=gaode&callnative=1`
}

// nextStatus：done/skipped 再点对应按钮恢复 planned。
function nextStatus(current: ActivityStatus, action: 'done' | 'skipped'): ActivityStatus {
  return current === action ? 'planned' : action
}

// 当前活动：now 落在 [start, end] 内的优先；否则第一个 planned（即「下一个」）。
function currentIndex(activities: Activity[], now: string): number {
  const inWindow = activities.findIndex((a) => {
    const s = shortTime(a.start_time)
    const e = shortTime(a.end_time)
    return s && e && s <= now && now <= e && a.status === 'planned'
  })
  if (inWindow >= 0) return inWindow
  return activities.findIndex((a) => a.status === 'planned')
}

export default function TodayPage() {
  const { t } = useTranslation()
  const { tripId = '' } = useParams<{ tripId: string }>()
  const [trip, setTrip] = useState<Trip | null>(null)
  const [days, setDays] = useState<Day[]>([])
  const [loading, setLoading] = useState(true)
  const [now, setNow] = useState(nowHHMM())

  const refresh = useCallback(async () => {
    if (!tripId) return
    try {
      setDays(await plannerApi.fetchDays(tripId))
    } catch {
      /* 静默：保留旧数据 */
    }
  }, [tripId])

  const { state, send, stop } = usePlannerChat(tripId, refresh)

  useEffect(() => {
    let cancelled = false
    if (!tripId) return
    Promise.all([plannerApi.getTrip(tripId), plannerApi.fetchDays(tripId)])
      .then(([tr, d]) => {
        if (cancelled) return
        setTrip(tr)
        setDays(d)
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [tripId])

  // 每分钟刷新一次「当前活动」高亮
  useEffect(() => {
    const timer = window.setInterval(() => setNow(nowHHMM()), 60_000)
    return () => window.clearInterval(timer)
  }, [])

  // 今日所在天：day.date 精确匹配优先，否则按 start_date + day_number 推算
  const today = todayStr()
  const activeDay = useMemo(() => {
    const byDate = days.find((d) => d.date === today)
    if (byDate) return byDate
    if (!trip?.start_date || days.length === 0) return undefined
    const idx = days.findIndex((d) => {
      const expected = addDays(trip.start_date!, d.day_number - 1)
      return expected === today
    })
    return idx >= 0 ? days[idx] : undefined
  }, [days, today, trip?.start_date])

  const dayIndex = activeDay ? days.findIndex((d) => d.id === activeDay.id) : -1
  const activities = activeDay?.activities ?? []
  const doneCount = activities.filter((a) => a.status === 'done').length
  const skippedCount = activities.filter((a) => a.status === 'skipped').length
  const currentIdx = currentIndex(activities, now)

  async function toggleStatus(activity: Activity, action: 'done' | 'skipped') {
    const next = nextStatus(activity.status, action)
    // 乐观更新
    setDays((prev) =>
      prev.map((d) =>
        d.id === activeDay?.id
          ? { ...d, activities: d.activities.map((a) => (a.id === activity.id ? { ...a, status: next } : a)) }
          : d,
      ),
    )
    try {
      await plannerApi.updateActivity(activity.id, { status: next })
    } catch {
      refresh() // 失败回滚
    }
  }

  if (loading) {
    return (
      <div className="flex h-[60vh] items-center justify-center">
        <span className="h-8 w-8 animate-spin rounded-full border-2 border-primary-600 border-t-transparent" />
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-xl px-4 pb-32 pt-4">
      {/* 头部 */}
      <div className="mb-4 flex items-center gap-3">
        <Link
          to={`/trip/${tripId}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100"
          aria-label={t('today.backToPlanner')}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5">
            <path d="M15 6l-6 6 6 6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </Link>
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium text-primary-600">{t('today.label')}</p>
          <h1 className="truncate text-lg font-bold text-slate-900">
            {activeDay
              ? t('today.dayTitle', { n: dayIndex + 1, date: activeDay.date ?? '' })
              : t('today.outOfRange')}
          </h1>
        </div>
        {activeDay && activities.length > 0 && (
          <span className="rounded-full bg-primary-50 px-3 py-1 text-xs font-semibold text-primary-700">
            {t('today.progress', { done: doneCount, total: activities.length })}
          </span>
        )}
      </div>

      {/* 非行程期 */}
      {!activeDay && (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white/60 px-6 py-12 text-center">
          <p className="text-sm text-slate-500">{t('today.outOfRangeHint')}</p>
          <Link to={`/trip/${tripId}`} className="mt-3 inline-block text-sm font-medium text-primary-600 hover:underline">
            {t('today.backToPlanner')} →
          </Link>
        </div>
      )}

      {/* 活动卡片 */}
      <div className="space-y-3">
        {activeDay && activities.length === 0 && (
          <p className="py-10 text-center text-sm text-slate-400">{t('today.empty')}</p>
        )}
        {activities.map((a, i) => {
          const isCurrent = i === currentIdx && a.status === 'planned'
          const done = a.status === 'done'
          const skipped = a.status === 'skipped'
          return (
            <div
              key={a.id}
              className={`rounded-2xl border p-4 transition-all ${
                done
                  ? 'border-emerald-200 bg-emerald-50/50'
                  : skipped
                    ? 'border-slate-200 bg-slate-50 opacity-60'
                    : isCurrent
                      ? 'border-primary-500 bg-white shadow-md ring-2 ring-primary-100'
                      : 'border-slate-200 bg-white shadow-sm'
              }`}
            >
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-slate-400">{timeLabel(a)}</span>
                {isCurrent && (
                  <span className="rounded-full bg-primary-600 px-2.5 py-0.5 text-[11px] font-bold text-white">
                    {t('today.current')}
                  </span>
                )}
                {done && <span className="text-xs font-semibold text-emerald-600">✓ {t('today.done')}</span>}
                {skipped && <span className="text-xs font-semibold text-slate-400">{t('today.skipped')}</span>}
              </div>

              <h3 className={`mt-1.5 text-base font-semibold ${done ? 'text-slate-500 line-through' : 'text-slate-900'}`}>
                {a.title}
              </h3>
              {a.location && <p className="mt-0.5 text-xs text-slate-500">📍 {a.location.name}</p>}
              {a.notes && <p className="mt-1 text-xs text-slate-400">{a.notes}</p>}

              <div className="mt-3 flex gap-2">
                <button
                  type="button"
                  onClick={() => toggleStatus(a, 'done')}
                  className={`flex-1 rounded-xl py-2 text-sm font-semibold transition-colors ${
                    done
                      ? 'bg-emerald-600 text-white'
                      : 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100'
                  }`}
                >
                  {done ? `✓ ${t('today.done')}` : t('today.markDone')}
                </button>
                <button
                  type="button"
                  onClick={() => toggleStatus(a, 'skipped')}
                  className={`rounded-xl px-4 py-2 text-sm font-medium transition-colors ${
                    skipped
                      ? 'bg-slate-500 text-white'
                      : 'bg-slate-100 text-slate-500 hover:bg-slate-200'
                  }`}
                >
                  {t('today.skip')}
                </button>
                {a.location && (
                  <a
                    href={amapUri(a)}
                    target="_blank"
                    rel="noreferrer"
                    className="rounded-xl bg-primary-600 px-4 py-2 text-sm font-semibold text-white hover:bg-primary-700"
                  >
                    {t('today.navigate')}
                  </a>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {activeDay && skippedCount > 0 && (
        <p className="mt-3 text-center text-xs text-slate-400">
          {t('today.skippedCount', { count: skippedCount })}
        </p>
      )}

      {/* 底部：AI 现场调整 */}
      <div className="fixed inset-x-0 bottom-0 border-t border-slate-200 bg-white/95 backdrop-blur">
        <div className="mx-auto max-w-xl">
          {state.messages.length > 0 && (
            <div className="max-h-40 overflow-y-auto">
              <AiMessageList messages={state.messages.slice(-4)} streaming={state.streaming} />
            </div>
          )}
          {state.error && (
            <p className="mx-4 mb-2 rounded-xl bg-rose-50 px-4 py-2 text-xs text-rose-700">{state.error}</p>
          )}
          <AiComposer streaming={state.streaming} onSend={send} onStop={stop} />
        </div>
      </div>
    </div>
  )
}
