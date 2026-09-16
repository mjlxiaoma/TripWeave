import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import ShareDialog from '../share/ShareDialog'
import type { Trip, TripStatus } from '../../types'

function BackIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5">
      <path d="M15 6l-6 6 6 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

const STATUS_BADGE: Record<TripStatus, string> = {
  draft: 'bg-slate-100 text-slate-600',
  planning: 'bg-violet-100 text-violet-700',
  ready: 'bg-emerald-100 text-emerald-700',
  archived: 'bg-slate-100 text-slate-500',
}

interface Props {
  trip: Trip
  onBack: () => void
}

function formatRange(trip: Trip): string {
  if (!trip.start_date || !trip.end_date) return ''
  return `${trip.start_date} ~ ${trip.end_date}`
}

// isTraveling：今天是否落在行程日期范围内（用于「旅行模式」入口高亮）。
function isTraveling(trip: Trip): boolean {
  if (!trip.start_date || !trip.end_date) return false
  const today = new Date()
  const str = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}-${String(today.getDate()).padStart(2, '0')}`
  return str >= trip.start_date && str <= trip.end_date
}

export default function TripHeader({ trip, onBack }: Props) {
  const { t } = useTranslation()
  const [shareOpen, setShareOpen] = useState(false)
  const traveling = isTraveling(trip)
  return (
    <header className="sticky top-16 z-10 border-b border-slate-200 bg-white/95 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center gap-3 px-4 py-3">
        <button
          type="button"
          onClick={onBack}
          aria-label={t('planner.back')}
          className="flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100"
        >
          <BackIcon />
        </button>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-base font-semibold text-slate-900">{trip.title}</h1>
          <p className="text-xs text-slate-500">
            {[trip.destination, formatRange(trip)].filter(Boolean).join(' · ')}
          </p>
        </div>
        <Link
          to={`/trip/${trip.id}/today`}
          className={`rounded-full px-3 py-1.5 text-xs font-semibold transition-colors ${
            traveling
              ? 'bg-primary-600 text-white hover:bg-primary-700'
              : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
          }`}
        >
          {traveling ? `● ${t('today.enterTraveling')}` : t('today.enter')}
        </Link>
        <button
          type="button"
          onClick={() => setShareOpen(true)}
          className="rounded-full bg-slate-100 px-3 py-1.5 text-xs font-semibold text-slate-600 transition-colors hover:bg-slate-200"
        >
          {t('share.open')}
        </button>
        <span className={`rounded-full px-3 py-1 text-xs font-medium ${STATUS_BADGE[trip.status]}`}>
          {t(`trips.status.${trip.status}`)}
        </span>
      </div>
      {shareOpen && <ShareDialog tripId={trip.id} onClose={() => setShareOpen(false)} />}
    </header>
  )
}
