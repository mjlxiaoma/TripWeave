import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router-dom'
import ShareDialog from '../share/ShareDialog'
import DateQuickSet from '../trips/DateQuickSet'
import { tripsApi } from '../../services/api'
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
  onDatesChanged?: (updated: Trip) => void
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

export default function TripHeader({ trip, onBack, onDatesChanged }: Props) {
  const { t } = useTranslation()
  const [shareOpen, setShareOpen] = useState(false)
  const [dateAnchor, setDateAnchor] = useState<{ x: number; y: number } | null>(null)
  const traveling = isTraveling(trip)
  const hasDates = Boolean(trip.start_date && trip.end_date)

  return (
    <header className="shrink-0 border-b border-slate-200 bg-white">
      <div className="flex w-full items-center gap-3 px-4 py-3">
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
            {[
              trip.destination,
              formatRange(trip),
              trip.travelers_count ? t('planner.metaTravelers', { count: trip.travelers_count }) : null,
              trip.preference?.budget != null
                ? t('planner.metaBudget', { amount: trip.preference.budget.toLocaleString() })
                : null,
            ]
              .filter(Boolean)
              .join(' · ')}
          </p>
        </div>
        {/* 无日期时显示设置入口 */}
        {!hasDates && (
          <button
            type="button"
            onClick={(e) => {
              const rect = e.currentTarget.getBoundingClientRect()
              setDateAnchor({ x: rect.left, y: rect.bottom })
            }}
            className="inline-flex items-center gap-1.5 rounded-full border border-dashed border-primary-400 px-3 py-1.5 text-xs font-semibold text-primary-600 transition-colors hover:bg-primary-50"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-3.5 w-3.5">
              <rect x="3.5" y="5" width="17" height="15.5" rx="2.5" />
              <path d="M3.5 9.5h17M8 3v4M16 3v4M12 12v4M10 14h4" strokeLinecap="round" />
            </svg>
            {t('trips.dateSet')}
          </button>
        )}
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
      <DateQuickSet
        anchor={dateAnchor}
        onClose={() => setDateAnchor(null)}
        onSave={async (start, end) => {
          if (onDatesChanged) {
            const updated = await tripsApi.update(trip.id, { start_date: start, end_date: end })
            onDatesChanged(updated)
          }
          setDateAnchor(null)
        }}
      />
    </header>
  )
}
