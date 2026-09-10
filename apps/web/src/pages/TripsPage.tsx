import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { tripsApi } from '../services/api'
import type { Trip, TripStatus } from '../types'

type TabKey = 'all' | 'active' | 'planning' | 'done'
const TAB_KEYS: TabKey[] = ['all', 'active', 'planning', 'done']

const TAB_MATCH: Record<TabKey, (s: TripStatus) => boolean> = {
  all: () => true,
  active: (s) => s === 'ready',
  planning: (s) => s === 'draft' || s === 'planning',
  done: (s) => s === 'archived',
}

const STATUS_BADGE: Record<TripStatus, string> = {
  draft: 'bg-violet-100 text-violet-600',
  planning: 'bg-violet-100 text-violet-600',
  ready: 'bg-emerald-100 text-emerald-700',
  archived: 'bg-slate-200/90 text-slate-600',
}

// 封面占位插画：按 trip id 哈希确定性取色，避免每次渲染跳变
const COVER_PALETTES: Array<{ sky: [string, string]; sun: string; hillBack: string; hillFront: string }> = [
  { sky: ['#a5f3fc', '#0e7490'], sun: '#fef9c3', hillBack: '#22d3ee', hillFront: '#155e75' },
  { sky: ['#c7d2fe', '#4f46e5'], sun: '#fde68a', hillBack: '#818cf8', hillFront: '#3730a3' },
  { sky: ['#fde68a', '#ea580c'], sun: '#fff7ed', hillBack: '#fb923c', hillFront: '#7c2d12' },
  { sky: ['#bbf7d0', '#047857'], sun: '#fefce8', hillBack: '#34d399', hillFront: '#065f46' },
  { sky: ['#fbcfe8', '#be185d'], sun: '#fff1f2', hillBack: '#f472b6', hillFront: '#831843' },
  { sky: ['#e2e8f0', '#475569'], sun: '#f8fafc', hillBack: '#94a3b8', hillFront: '#334155' },
]

function hashSeed(s: string): number {
  let h = 2166136261
  for (let i = 0; i < s.length; i += 1) {
    h ^= s.charCodeAt(i)
    h = Math.imul(h, 16777619) >>> 0
  }
  return h >>> 0
}

function TripCover({ trip }: { trip: Trip }) {
  const seed = hashSeed(trip.id || trip.title)
  const p = COVER_PALETTES[seed % COVER_PALETTES.length]
  const flip = seed % 2 === 1
  const gid = `cover-${trip.id}`
  return (
    <div className="relative h-44 w-full overflow-hidden bg-slate-200">
      <svg
        viewBox="0 0 400 176"
        preserveAspectRatio="xMidYMid slice"
        className="h-full w-full transition-transform duration-500 group-hover:scale-105"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={p.sky[0]} />
            <stop offset="100%" stopColor={p.sky[1]} />
          </linearGradient>
        </defs>
        <rect width="400" height="176" fill={`url(#${gid})`} />
        <circle cx={flip ? 300 : 100} cy="52" r="26" fill={p.sun} opacity="0.9" />
        <path
          d={flip
            ? 'M0 132 L90 74 L170 128 L260 66 L400 140 V176 H0 Z'
            : 'M0 140 L110 70 L200 126 L300 78 L400 132 V176 H0 Z'}
          fill={p.hillBack}
          opacity="0.55"
        />
        <path
          d={flip
            ? 'M0 176 V150 L120 104 L220 152 L320 112 L400 156 V176 Z'
            : 'M0 176 V156 L100 112 L210 154 L310 108 L400 150 V176 Z'}
          fill={p.hillFront}
          opacity="0.85"
        />
      </svg>
    </div>
  )
}

function CalendarIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4 shrink-0">
      <rect x="3.5" y="5" width="17" height="15.5" rx="2.5" />
      <path d="M3.5 9.5h17M8 3v4M16 3v4" strokeLinecap="round" />
    </svg>
  )
}

function PlusIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-4 w-4">
      <path d="M12 5v14M5 12h14" strokeLinecap="round" />
    </svg>
  )
}

function PinIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-7 w-7">
      <path d="M12 21s-7-5.5-7-11a7 7 0 1114 0c0 5.5-7 11-7 11z" strokeLinejoin="round" />
      <circle cx="12" cy="10" r="2.6" />
    </svg>
  )
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function fmtDate(iso: string, withYear: boolean): string {
  const d = new Date(`${iso}T00:00:00`)
  if (Number.isNaN(d.getTime())) return iso
  return withYear ? `${d.getFullYear()}.${pad2(d.getMonth() + 1)}.${pad2(d.getDate())}` : `${pad2(d.getMonth() + 1)}.${pad2(d.getDate())}`
}

function EmptyHint() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <>
      <span className="flex h-16 w-16 items-center justify-center rounded-full bg-primary-100 text-primary-600">
        <PinIcon />
      </span>
      <p className="mt-2 text-base font-semibold text-slate-900">{t('trips.emptyTitle')}</p>
      <p className="text-sm text-slate-500">{t('trips.emptyDesc')}</p>
      <button
        type="button"
        onClick={() => navigate('/trip/new')}
        className="mt-1 text-sm font-semibold text-primary-600 transition hover:text-primary-700"
      >
        {t('trips.emptyCta')}
      </button>
    </>
  )
}

export default function TripsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [trips, setTrips] = useState<Trip[]>([])
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [tab, setTab] = useState<TabKey>('all')

  const load = useCallback(async () => {
    setLoading(true)
    setFailed(false)
    try {
      setTrips(await tripsApi.list())
    } catch {
      setFailed(true)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const filtered = useMemo(() => trips.filter((trip) => TAB_MATCH[tab](trip.status)), [trips, tab])
  const activeCount = useMemo(() => trips.filter((trip) => trip.status === 'ready').length, [trips])

  const dateLabel = (trip: Trip): string => {
    if (trip.start_date && trip.end_date) {
      const s = new Date(`${trip.start_date}T00:00:00`)
      const e = new Date(`${trip.end_date}T00:00:00`)
      const days = Math.round((e.getTime() - s.getTime()) / 86400000) + 1
      const sameYear = s.getFullYear() === e.getFullYear()
      const range = `${fmtDate(trip.start_date, true)} – ${fmtDate(trip.end_date, !sameYear)}`
      return Number.isFinite(days) && days > 0 ? `${range} · ${t('trips.days', { n: days })}` : range
    }
    if (trip.start_date) return fmtDate(trip.start_date, true)
    if (trip.end_date) return fmtDate(trip.end_date, true)
    return t('trips.noDates')
  }

  return (
    <div className="bg-slate-100/70 px-6 pb-24 pt-12">
      <div className="mx-auto max-w-6xl">
        <div className="flex flex-wrap items-start justify-between gap-6">
          <div>
            <h1 className="text-3xl font-bold tracking-tight text-slate-900">{t('trips.title')}</h1>
            <p className="mt-3 text-sm text-slate-500">
              {t('trips.summary', { total: trips.length, active: activeCount })}
            </p>
          </div>
          <button
            type="button"
            onClick={() => navigate('/trip/new')}
            className="inline-flex items-center gap-2 rounded-xl bg-primary-600 px-5 py-3 text-sm font-semibold text-white shadow-sm transition hover:bg-primary-700"
          >
            <PlusIcon />
            {t('trips.newTrip')}
          </button>
        </div>

        <div className="mt-8 flex flex-wrap items-center gap-3">
          {TAB_KEYS.map((key) => {
            const active = key === tab
            return (
              <button
                key={key}
                type="button"
                onClick={() => setTab(key)}
                className={(active
                  ? 'bg-primary-600 text-white shadow-sm '
                  : 'bg-white text-slate-600 shadow-sm hover:text-primary-700 ') +
                  'rounded-full px-5 py-2 text-sm font-medium transition'}
              >
                {t(`trips.tabs.${key}`)}
              </button>
            )
          })}
        </div>

        {loading ? (
          <div className="mt-8 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <div key={i} className="overflow-hidden rounded-2xl bg-white shadow-sm">
                <div className="h-44 animate-pulse bg-slate-200" />
                <div className="space-y-3 p-5">
                  <div className="h-5 w-2/3 animate-pulse rounded bg-slate-200" />
                  <div className="h-4 w-1/2 animate-pulse rounded bg-slate-100" />
                </div>
              </div>
            ))}
          </div>
        ) : failed ? (
          <div className="mt-24 flex flex-col items-center gap-4 text-center">
            <p className="text-sm text-slate-500">{t('trips.loadError')}</p>
            <button
              type="button"
              onClick={() => void load()}
              className="rounded-full bg-white px-5 py-2 text-sm font-medium text-primary-600 shadow-sm transition hover:text-primary-700"
            >
              {t('trips.retry')}
            </button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="mt-24 flex flex-col items-center gap-3 text-center">
            <EmptyHint />
          </div>
        ) : (
          <div className="mt-8 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {filtered.map((trip) => (
              <button
                key={trip.id}
                type="button"
                onClick={() => navigate(`/trip/${trip.id}`)}
                className="group overflow-hidden rounded-2xl bg-white text-left shadow-sm transition hover:-translate-y-0.5 hover:shadow-md"
              >
                <div className="relative">
                  <TripCover trip={trip} />
                  <span
                    className={`absolute left-4 top-4 rounded-full px-3 py-1 text-xs font-medium ${STATUS_BADGE[trip.status]}`}
                  >
                    {t(`trips.status.${trip.status}`)}
                  </span>
                </div>
                <div className="p-5">
                  <h2 className="truncate text-lg font-semibold text-slate-900">{trip.title}</h2>
                  <p className="mt-3 flex items-center gap-2 text-sm text-slate-500">
                    <CalendarIcon />
                    {dateLabel(trip)}
                  </p>
                </div>
              </button>
            ))}
            <div className="flex min-h-[17rem] flex-col items-center justify-center gap-3 rounded-2xl bg-white p-6 text-center shadow-sm">
              <EmptyHint />
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
