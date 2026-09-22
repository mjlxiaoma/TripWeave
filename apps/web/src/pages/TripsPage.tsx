import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { tripsApi } from '../services/api'
import TripEditDialog from '../features/trips/TripEditDialog'
import DateQuickSet from '../features/trips/DateQuickSet'
import { compressCover } from '../features/trips/coverImage'
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

// 状态流转：点击徽标切到下一个状态（draft 与 planning 在 UI 上同属「规划中」，
// 从 draft 起步先落到 planning，之后 ready → archived 单向流转，便于撤销误点时回退）
const NEXT_STATUS: Record<TripStatus, TripStatus> = {
  draft: 'planning',
  planning: 'ready',
  ready: 'archived',
  archived: 'planning',
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
  const { t } = useTranslation()
  const [coverUrl, setCoverUrl] = useState<string | null>(null)

  // 自定义封面:按需拉取二进制 → objectURL(鉴权走 api 层,<img> 无法带 header)
  useEffect(() => {
    let objectUrl: string | null = null
    let cancelled = false
    if (trip.has_cover) {
      tripsApi
        .fetchCover(trip.id)
        .then((blob) => {
          if (cancelled || !blob) return
          objectUrl = URL.createObjectURL(blob)
          setCoverUrl(objectUrl)
        })
        .catch(() => {})
    } else {
      setCoverUrl(null)
    }
    return () => {
      cancelled = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [trip.id, trip.has_cover])

  if (coverUrl) {
    return (
      <div className="relative h-44 w-full overflow-hidden bg-slate-200">
        <img
          src={coverUrl}
          alt={trip.title}
          className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
        />
      </div>
    )
  }

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
      {/* 默认封面语义化:目的地名 + 标识,不再是“随机色块” */}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 bg-gradient-to-t from-slate-900/60 to-transparent px-5 pb-3 pt-10">
        <p className="text-xl font-bold text-white drop-shadow">{trip.destination || trip.title}</p>
        <p className="mt-0.5 text-[10px] font-medium text-white/70">{t('trips.coverDefault')}</p>
      </div>
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

// 单个旅行卡片：承载 ⋯ 菜单（编辑/删除）、状态徽标切换、删除确认条
function TripCard({
  trip,
  onOpen,
  onEdit,
  onStatusChange,
  onDelete,
  onCoverChanged,
  onDatesChanged,
}: {
  trip: Trip
  onOpen: () => void
  onEdit: () => void
  onStatusChange: (s: TripStatus) => void
  onDelete: () => Promise<void>
  onCoverChanged: (hasCover: boolean) => void
  onDatesChanged: (updated: Trip) => void
}) {
  const { t } = useTranslation()
  const [menuOpen, setMenuOpen] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [delError, setDelError] = useState(false)
  const [statusBusy, setStatusBusy] = useState(false)
  const [dateAnchor, setDateAnchor] = useState<{ x: number; y: number } | null>(null)
  const [coverBusy, setCoverBusy] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const fileRef = useRef<HTMLInputElement>(null)
  const dateChipRef = useRef<HTMLButtonElement>(null)

  async function pickCover(file: File | undefined) {
    if (!file || coverBusy) return
    setCoverBusy(true)
    try {
      const { blob } = await compressCover(file)
      await tripsApi.uploadCover(trip.id, blob)
      onCoverChanged(true)
    } catch {
      /* 静默:压缩/上传失败保持原封面 */
    } finally {
      setCoverBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  async function removeCover() {
    if (coverBusy) return
    setCoverBusy(true)
    try {
      await tripsApi.removeCover(trip.id)
      onCoverChanged(false)
    } catch {
      /* 静默 */
    } finally {
      setCoverBusy(false)
    }
  }

  function openDatePop(e: React.MouseEvent) {
    e.stopPropagation()
    const rect = dateChipRef.current?.getBoundingClientRect()
    if (rect) setDateAnchor({ x: rect.left, y: rect.bottom })
  }

  useEffect(() => {
    if (!menuOpen) return
    const onDown = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setMenuOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    return () => document.removeEventListener('mousedown', onDown)
  }, [menuOpen])

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

  async function cycleStatus(e: React.MouseEvent) {
    e.stopPropagation()
    if (statusBusy) return
    setStatusBusy(true)
    try {
      await onStatusChange(NEXT_STATUS[trip.status])
    } finally {
      setStatusBusy(false)
    }
  }

  async function confirmDelete() {
    if (deleting) return
    setDeleting(true)
    setDelError(false)
    try {
      await onDelete()
    } catch {
      setDelError(true)
      setDeleting(false)
    }
  }

  if (confirming) {
    return (
      <div className="flex min-h-[17rem] flex-col justify-center gap-2.5 rounded-2xl bg-rose-50 p-5 shadow-sm">
        <p className="flex items-center gap-2 text-sm font-semibold text-rose-700">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4 shrink-0">
            <path d="M12 9v4m0 4h.01M10.3 3.9L1.8 18a2 2 0 001.7 3h17a2 2 0 001.7-3L13.7 3.9a2 2 0 00-3.4 0z" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
          {t('trips.del.confirm', { title: trip.title })}
        </p>
        <p className="text-xs text-rose-600/80">{t('trips.del.hint')}</p>
        {delError && <p className="text-xs text-rose-700">{t('trips.del.error')}</p>}
        <div className="mt-1 flex justify-end gap-2">
          <button
            type="button"
            onClick={() => { setConfirming(false); setDelError(false) }}
            className="rounded-lg border border-slate-300 bg-white px-3.5 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50"
          >
            {t('trips.del.cancel')}
          </button>
          <button
            type="button"
            onClick={() => void confirmDelete()}
            disabled={deleting}
            className="rounded-lg bg-rose-600 px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-rose-700 disabled:opacity-60"
          >
            {deleting ? t('trips.del.deleting') : t('trips.del.confirmBtn')}
          </button>
        </div>
      </div>
    )
  }

  const hasDates = Boolean(trip.start_date && trip.end_date)

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => { if (e.key === 'Enter') onOpen() }}
      className="group cursor-pointer overflow-hidden rounded-2xl bg-white text-left shadow-sm transition hover:-translate-y-0.5 hover:shadow-md"
    >
      <input
        ref={fileRef}
        type="file"
        accept="image/jpeg,image/png,image/webp"
        className="hidden"
        onClick={(e) => e.stopPropagation()}
        onChange={(e) => void pickCover(e.target.files?.[0])}
      />
      <div className="relative">
        <TripCover trip={trip} />

        {/* hover 上传入口(未上传时) */}
        {!trip.has_cover && (
          <button
            type="button"
            onClick={(e) => { e.stopPropagation(); fileRef.current?.click() }}
            disabled={coverBusy}
            className="absolute inset-0 hidden items-center justify-center bg-slate-900/40 opacity-0 transition-opacity group-hover:flex group-hover:opacity-100"
          >
            <span className="flex items-center gap-2 rounded-full bg-white/95 px-4 py-2 text-sm font-semibold text-slate-800 shadow">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
                <path d="M4 16l4.6-6 3.4 4 3-3 5 5M4 16v3h16v-3M4 16v-9h16v9" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
              {coverBusy ? t('wizard.coverUploading') : t('wizard.coverUpload')}
            </span>
          </button>
        )}

        <button
          type="button"
          onClick={cycleStatus}
          disabled={statusBusy}
          title={t(`trips.status.${trip.status}`)}
          className={`absolute left-4 top-4 inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-medium transition-opacity disabled:opacity-60 ${STATUS_BADGE[trip.status]}`}
        >
          {t(`trips.status.${trip.status}`)}
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="h-3 w-3 opacity-70">
            <path d="M6 9l6 6 6-6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
        <div ref={menuRef} className="absolute right-3 top-3">
          <button
            type="button"
            aria-label={t('trips.menu.edit')}
            onClick={(e) => { e.stopPropagation(); setMenuOpen((v) => !v) }}
            className={`flex h-8 w-8 items-center justify-center rounded-full bg-white/90 text-slate-500 shadow-sm transition-all hover:text-slate-800 ${
              menuOpen ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
            }`}
          >
            <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4">
              <circle cx="5" cy="12" r="1.6" />
              <circle cx="12" cy="12" r="1.6" />
              <circle cx="19" cy="12" r="1.6" />
            </svg>
          </button>
          {menuOpen && (
            <div className="absolute right-0 top-9 z-20 w-40 overflow-hidden rounded-xl border border-slate-200 bg-white py-1 shadow-lg">
              <button
                type="button"
                onClick={(e) => { e.stopPropagation(); setMenuOpen(false); fileRef.current?.click() }}
                className="flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-slate-700 transition-colors hover:bg-slate-50"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-3.5 w-3.5">
                  <path d="M4 16l4.6-6 3.4 4 3-3 5 5M4 16v3h16v-3M4 16v-9h16v9" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                {t('wizard.coverChange')}
              </button>
              {trip.has_cover && (
                <button
                  type="button"
                  onClick={(e) => { e.stopPropagation(); setMenuOpen(false); void removeCover() }}
                  className="flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-slate-700 transition-colors hover:bg-slate-50"
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-3.5 w-3.5">
                    <path d="M4 7l16 10M5 5h14a2 2 0 012 2v10a2 2 0 01-2 2H5a2 2 0 01-2-2V7a2 2 0 012-2z" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                  {t('wizard.coverRemove')}
                </button>
              )}
              <button
                type="button"
                onClick={(e) => { e.stopPropagation(); setMenuOpen(false); onEdit() }}
                className="flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-slate-700 transition-colors hover:bg-slate-50"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-3.5 w-3.5">
                  <path d="M17 3a2.8 2.8 0 114 4L7.5 20.5 2 22l1.5-5.5L17 3z" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                {t('trips.menu.edit')}
              </button>
              <button
                type="button"
                onClick={(e) => { e.stopPropagation(); setMenuOpen(false); setConfirming(true) }}
                className="flex w-full items-center gap-2.5 px-3.5 py-2 text-left text-sm text-rose-600 transition-colors hover:bg-rose-50"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-3.5 w-3.5">
                  <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m3 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6h14z" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
                {t('trips.menu.delete')}
              </button>
            </div>
          )}
        </div>
      </div>
      <div className="p-5">
        <h2 className="truncate text-lg font-semibold text-slate-900">{trip.title}</h2>
        {hasDates ? (
          <button
            ref={dateChipRef}
            type="button"
            onClick={openDatePop}
            className="mt-3 inline-flex items-center gap-2 text-sm text-slate-500 transition-colors hover:text-primary-700"
          >
            <CalendarIcon />
            {dateLabel(trip)}
          </button>
        ) : (
          <button
            ref={dateChipRef}
            type="button"
            onClick={openDatePop}
            className="mt-3 inline-flex items-center gap-2 rounded-lg border border-dashed border-primary-400 px-3 py-1.5 text-xs font-semibold text-primary-600 transition-colors hover:bg-primary-50"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-3.5 w-3.5">
              <rect x="3.5" y="5" width="17" height="15.5" rx="2.5" />
              <path d="M3.5 9.5h17M8 3v4M16 3v4M12 12v4M10 14h4" strokeLinecap="round" />
            </svg>
            {t('trips.dateSet')}
            <span className="font-normal text-slate-400">· {t('trips.dateSetHint')}</span>
          </button>
        )}
      </div>

      <DateQuickSet
        anchor={dateAnchor}
        onClose={() => setDateAnchor(null)}
        onSave={async (start, end) => {
          const updated = await tripsApi.update(trip.id, { start_date: start, end_date: end })
          onDatesChanged(updated)
          setDateAnchor(null)
        }}
      />
    </div>
  )
}

export default function TripsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [trips, setTrips] = useState<Trip[]>([])
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [tab, setTab] = useState<TabKey>('all')
  const [editingTrip, setEditingTrip] = useState<Trip | null>(null)

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
              <TripCard
                key={trip.id}
                trip={trip}
                onOpen={() => navigate(`/trip/${trip.id}`)}
                onEdit={() => setEditingTrip(trip)}
                onStatusChange={async (status) => {
                  const updated = await tripsApi.update(trip.id, { status })
                  setTrips((prev) => prev.map((x) => (x.id === trip.id ? { ...x, ...updated } : x)))
                }}
                onDelete={async () => {
                  await tripsApi.remove(trip.id)
                  setTrips((prev) => prev.filter((x) => x.id !== trip.id))
                }}
                onCoverChanged={(hasCover) =>
                  setTrips((prev) => prev.map((x) => (x.id === trip.id ? { ...x, has_cover: hasCover } : x)))
                }
                onDatesChanged={(updated) =>
                  setTrips((prev) => prev.map((x) => (x.id === updated.id ? { ...x, ...updated } : x)))
                }
              />
            ))}
            <div className="flex min-h-[17rem] flex-col items-center justify-center gap-3 rounded-2xl bg-white p-6 text-center shadow-sm">
              <EmptyHint />
            </div>
          </div>
        )}
        {editingTrip && (
          <TripEditDialog
            trip={editingTrip}
            onClose={() => setEditingTrip(null)}
            onSaved={(updated) => {
              setTrips((prev) => prev.map((x) => (x.id === updated.id ? { ...x, ...updated } : x)))
              setEditingTrip(null)
            }}
          />
        )}
      </div>
    </div>
  )
}
