import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { tripsApi } from '../../services/api'
import type { Trip, TripStatus } from '../../types'

interface Props {
  trip: Trip
  onClose: () => void
  /** 保存成功后回写最新 trip（后端返回完整 DTO） */
  onSaved: (updated: Trip) => void
}

const STATUS_ORDER: TripStatus[] = ['draft', 'planning', 'ready', 'archived']
// 列表页只允许在 规划中/进行中/已完成 之间流转，draft 仅作展示
const STATUS_EDITABLE: TripStatus[] = ['planning', 'ready', 'archived']

const STATUS_ACTIVE: Record<TripStatus, string> = {
  draft: 'bg-violet-600 text-white',
  planning: 'bg-violet-600 text-white',
  ready: 'bg-emerald-600 text-white',
  archived: 'bg-slate-600 text-white',
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-xs font-semibold text-slate-500">{label}</span>
      {children}
    </label>
  )
}

const inputCls =
  'w-full rounded-xl border border-slate-300 bg-white px-3 py-2.5 text-sm text-slate-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20'

export default function TripEditDialog({ trip, onClose, onSaved }: Props) {
  const { t } = useTranslation()
  const [title, setTitle] = useState(trip.title)
  const [destination, setDestination] = useState(trip.destination ?? '')
  const [startDate, setStartDate] = useState(trip.start_date ?? '')
  const [endDate, setEndDate] = useState(trip.end_date ?? '')
  const [travelers, setTravelers] = useState(trip.travelers_count != null ? String(trip.travelers_count) : '')
  const [budget, setBudget] = useState(trip.preference?.budget != null ? String(trip.preference.budget) : '')
  const [status, setStatus] = useState<TripStatus>(trip.status)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose])

  const dateInvalid = startDate && endDate && endDate < startDate

  async function submit() {
    if (!title.trim() || saving || dateInvalid) return
    setSaving(true)
    setError(null)
    try {
      const updated = await tripsApi.update(trip.id, {
        title: title.trim(),
        destination: destination.trim() || null,
        start_date: startDate || null,
        end_date: endDate || null,
        travelers_count: travelers ? Math.max(1, parseInt(travelers, 10) || 1) : undefined,
        budget: budget ? Number(budget) : null,
        status,
      })
      onSaved(updated)
    } catch {
      setError(t('trips.edit.saveError'))
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/45 p-4"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div ref={dialogRef} className="w-full max-w-lg rounded-2xl bg-white p-6 shadow-2xl">
        <div className="mb-5 flex items-center justify-between">
          <h2 className="text-lg font-bold text-slate-900">{t('trips.edit.title')}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label={t('trips.edit.cancel')}
            className="rounded-lg p-1.5 text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
              <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        <div className="space-y-4">
          <Field label={t('trips.edit.name')}>
            <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} maxLength={60} />
          </Field>
          <Field label={t('trips.edit.destination')}>
            <input className={inputCls} value={destination} onChange={(e) => setDestination(e.target.value)} maxLength={50} />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field label={t('trips.edit.startDate')}>
              <input type="date" className={inputCls} value={startDate} onChange={(e) => setStartDate(e.target.value)} />
            </Field>
            <Field label={t('trips.edit.endDate')}>
              <input type="date" className={inputCls} value={endDate} onChange={(e) => setEndDate(e.target.value)} />
            </Field>
          </div>
          {dateInvalid && <p className="text-xs text-rose-600">{t('trips.edit.dateInvalid')}</p>}
          <div className="grid grid-cols-2 gap-3">
            <Field label={t('trips.edit.travelers')}>
              <input
                type="number"
                min={1}
                className={inputCls}
                value={travelers}
                onChange={(e) => setTravelers(e.target.value)}
              />
            </Field>
            <Field label={t('trips.edit.budget')}>
              <input
                type="number"
                min={0}
                className={inputCls}
                value={budget}
                onChange={(e) => setBudget(e.target.value)}
                placeholder="¥"
              />
            </Field>
          </div>
          <div>
            <span className="mb-1.5 block text-xs font-semibold text-slate-500">{t('trips.edit.status')}</span>
            <div className="flex gap-2">
              {STATUS_ORDER.filter((s) => STATUS_EDITABLE.includes(s) || s === trip.status).map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setStatus(s)}
                  className={`rounded-full px-4 py-1.5 text-xs font-medium transition-colors ${
                    status === s ? STATUS_ACTIVE[s] : 'border border-slate-300 bg-white text-slate-500 hover:border-slate-400'
                  }`}
                >
                  {t(`trips.status.${s}`)}
                </button>
              ))}
            </div>
          </div>
        </div>

        {error && <p className="mt-4 rounded-xl bg-rose-50 px-4 py-2 text-xs text-rose-700">{error}</p>}

        <div className="mt-6 flex justify-end gap-2.5">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl border border-slate-300 px-4 py-2.5 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-50"
          >
            {t('trips.edit.cancel')}
          </button>
          <button
            type="button"
            onClick={() => void submit()}
            disabled={!title.trim() || saving || Boolean(dateInvalid)}
            className="rounded-xl bg-primary-600 px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:opacity-50"
          >
            {saving ? t('trips.edit.saving') : t('trips.edit.save')}
          </button>
        </div>
      </div>
    </div>
  )
}
