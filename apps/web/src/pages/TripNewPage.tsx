import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ApiError, tripsApi } from '../services/api'
import type { CreateTripPayload } from '../types'
import { DRAFT_NL_KEY } from '../utils/draftNl'

const STEP_KEYS = ['basic', 'prefs', 'constraints', 'extra'] as const
const PREF_TAG_KEYS = [
  'nature',
  'photo',
  'food',
  'coffee',
  'museum',
  'history',
  'shopping',
  'nightlife',
  'hiking',
  'family',
] as const

const BUDGET_MIN = 0
const BUDGET_MAX = 50000
const BUDGET_STEP = 500

interface DraftState {
  destination: string
  startDate: string
  endDate: string
  travelers: number
  budget: string
  transport: string
  prefTags: string[]
  maxDrive: number
  maxWalk: number
  earliest: string
  latest: string
  allowHotelChange: boolean
  budgetRange: [number, number]
  extra: string
}

const initialDraft: DraftState = {
  destination: '',
  startDate: '',
  endDate: '',
  travelers: 2,
  budget: '',
  transport: '',
  prefTags: [],
  maxDrive: 4,
  maxWalk: 10,
  earliest: '09:00',
  latest: '21:00',
  allowHotelChange: true,
  budgetRange: [2000, 10000],
  extra: '',
}

function StepIcon({ n, active, done }: { n: number; active: boolean; done: boolean }) {
  const cls = done
    ? 'border-primary-600 bg-primary-600 text-white'
    : active
      ? 'border-primary-600 bg-white text-primary-600'
      : 'border-slate-300 bg-white text-slate-400'
  return (
    <span
      className={`flex h-9 w-9 items-center justify-center rounded-full border-2 text-sm font-semibold transition-colors ${cls}`}
    >
      {done ? (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3" className="h-4 w-4">
          <path d="M5 13l4 4L19 7" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      ) : (
        n
      )}
    </span>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-sm font-medium text-slate-700">{label}</span>
      {children}
    </label>
  )
}

const inputCls =
  'w-full rounded-xl border border-slate-300 bg-white px-3.5 py-2.5 text-sm text-slate-900 placeholder:text-slate-400 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20'

export default function TripNewPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [step, setStep] = useState(0)
  const [draft, setDraft] = useState<DraftState>(initialDraft)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // 从首页「开始规划」带过来的自然语言草稿
  useEffect(() => {
    const nl = sessionStorage.getItem(DRAFT_NL_KEY)
    if (nl) {
      setDraft((d) => ({ ...d, extra: nl }))
      sessionStorage.removeItem(DRAFT_NL_KEY)
    }
  }, [])

  // 从探索页目的地卡带过来的 ?destination= 预填
  useEffect(() => {
    const dest = searchParams.get('destination')
    if (dest) setDraft((d) => ({ ...d, destination: dest }))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const set = <K extends keyof DraftState>(key: K, value: DraftState[K]) => {
    setDraft((d) => ({ ...d, [key]: value }))
    setErrors((e) => {
      if (!e[key]) return e
      const next = { ...e }
      delete next[key]
      return next
    })
  }

  const toggleTag = (tag: string) => {
    setDraft((d) => ({
      ...d,
      prefTags: d.prefTags.includes(tag) ? d.prefTags.filter((x) => x !== tag) : [...d.prefTags, tag],
    }))
  }

  function validateStep(s: number): boolean {
    const e: Record<string, string> = {}
    if (s === 0) {
      if (!draft.destination.trim()) e.destination = t('wizard.errors.destination')
      if (!draft.startDate || !draft.endDate) e.dates = t('wizard.errors.dates')
      else if (draft.endDate < draft.startDate) e.dates = t('wizard.errors.dateOrder')
      if (draft.travelers < 1 || draft.travelers > 100) e.travelers = t('wizard.errors.travelers')
      if (draft.budget !== '' && Number(draft.budget) < 0) e.budget = t('wizard.errors.budget')
    }
    setErrors(e)
    return Object.keys(e).length === 0
  }

  const canNext = useMemo(() => step < STEP_KEYS.length - 1, [step])

  async function submit(saveAsDraft: boolean) {
    if (submitting) return
    if (!validateStep(0)) {
      setStep(0)
      return
    }
    setSubmitting(true)
    setSubmitError(null)
    const payload: CreateTripPayload = {
      title: draft.destination.trim() || t('wizard.untitled'),
      destination: draft.destination.trim(),
      start_date: draft.startDate,
      end_date: draft.endDate,
      travelers_count: draft.travelers,
      status: saveAsDraft ? 'draft' : 'planning',
      budget: draft.budget === '' ? null : Number(draft.budget),
      transport_mode: draft.transport.trim() || null,
      constraints: {
        max_drive_hours_per_day: draft.maxDrive,
        max_walk_km_per_day: draft.maxWalk,
        earliest_start: draft.earliest,
        latest_end: draft.latest,
        allow_hotel_change: draft.allowHotelChange,
        budget_range: draft.budgetRange,
      },
      preferences: draft.prefTags,
      natural_language: draft.extra.trim() || null,
    }
    try {
      const trip = await tripsApi.create(payload)
      navigate(`/trip/${trip.id}`, { replace: true })
    } catch (err) {
      const code = err instanceof ApiError ? err.code : 'UNKNOWN'
      setSubmitError(t(`errors.${code}`, { defaultValue: t('errors.UNKNOWN') }))
      setSubmitting(false)
    }
  }

  const stepTitle = t(`wizard.steps.${STEP_KEYS[step]}`)

  return (
    <div className="mx-auto max-w-3xl px-4 py-10">
      {/* 头部 */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-slate-900">{t('wizard.title')}</h1>
        <p className="mt-1 text-sm text-slate-500">{t('wizard.subtitle')}</p>
      </div>

      {/* 步骤条 */}
      <ol className="mb-10 flex items-center">
        {STEP_KEYS.map((key, i) => (
          <li key={key} className={`flex items-center ${i < STEP_KEYS.length - 1 ? 'flex-1' : ''}`}>
            <button
              type="button"
              onClick={() => i < step && setStep(i)}
              className="flex items-center gap-2.5"
              disabled={i > step}
            >
              <StepIcon n={i + 1} active={i === step} done={i < step} />
              <span
                className={`hidden text-sm font-medium sm:block ${
                  i === step ? 'text-slate-900' : 'text-slate-400'
                }`}
              >
                {t(`wizard.steps.${key}`)}
              </span>
            </button>
            {i < STEP_KEYS.length - 1 && (
              <span className={`mx-3 h-0.5 flex-1 rounded ${i < step ? 'bg-primary-600' : 'bg-slate-200'}`} />
            )}
          </li>
        ))}
      </ol>
      <p className="mb-4 text-xs font-medium uppercase tracking-wide text-slate-400">
        {t('wizard.stepOf', { current: step + 1, total: STEP_KEYS.length })} · {stepTitle}
      </p>

      {/* 步骤① 基本信息 */}
      {step === 0 && (
        <div className="space-y-5 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <p className="text-sm text-slate-500">{t('wizard.basicDesc')}</p>
          <Field label={t('wizard.destination')}>
            <input
              className={inputCls}
              value={draft.destination}
              placeholder={t('wizard.destinationPh')}
              onChange={(e) => set('destination', e.target.value)}
            />
            {errors.destination && <p className="mt-1 text-xs text-rose-600">{errors.destination}</p>}
          </Field>
          <div className="grid grid-cols-2 gap-4">
            <Field label={t('wizard.startDate')}>
              <input
                type="date"
                className={inputCls}
                value={draft.startDate}
                onChange={(e) => set('startDate', e.target.value)}
              />
            </Field>
            <Field label={t('wizard.endDate')}>
              <input
                type="date"
                className={inputCls}
                value={draft.endDate}
                min={draft.startDate || undefined}
                onChange={(e) => set('endDate', e.target.value)}
              />
            </Field>
          </div>
          {errors.dates && <p className="text-xs text-rose-600">{errors.dates}</p>}
          <div className="grid grid-cols-2 gap-4">
            <Field label={`${t('wizard.travelers')}（${t('wizard.travelersUnit')}）`}>
              <input
                type="number"
                min={1}
                max={100}
                className={inputCls}
                value={draft.travelers}
                onChange={(e) => set('travelers', Number(e.target.value))}
              />
              {errors.travelers && <p className="mt-1 text-xs text-rose-600">{errors.travelers}</p>}
            </Field>
            <Field label={`${t('wizard.budget')}（${t('wizard.budgetUnit')}）`}>
              <input
                type="number"
                min={0}
                className={inputCls}
                value={draft.budget}
                placeholder="5000"
                onChange={(e) => set('budget', e.target.value)}
              />
              {errors.budget && <p className="mt-1 text-xs text-rose-600">{errors.budget}</p>}
            </Field>
          </div>
          <Field label={t('wizard.transport')}>
            <input
              className={inputCls}
              value={draft.transport}
              placeholder={t('wizard.transportPh')}
              onChange={(e) => set('transport', e.target.value)}
            />
          </Field>
        </div>
      )}

      {/* 步骤② 偏好标签 */}
      {step === 1 && (
        <div className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <p className="mb-5 text-sm text-slate-500">{t('wizard.prefsDesc')}</p>
          <div className="flex flex-wrap gap-2.5">
            {PREF_TAG_KEYS.map((key) => {
              const on = draft.prefTags.includes(key)
              return (
                <button
                  key={key}
                  type="button"
                  onClick={() => toggleTag(key)}
                  className={`rounded-full border px-4 py-2 text-sm font-medium transition-colors ${
                    on
                      ? 'border-primary-600 bg-primary-50 text-primary-700'
                      : 'border-slate-300 bg-white text-slate-600 hover:border-slate-400'
                  }`}
                >
                  {t(`wizard.prefTags.${key}`)}
                </button>
              )
            })}
          </div>
        </div>
      )}

      {/* 步骤③ 约束 */}
      {step === 2 && (
        <div className="space-y-6 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <p className="text-sm text-slate-500">{t('wizard.constraintsDesc')}</p>
          <div className="grid grid-cols-2 gap-4">
            <Field label={`${t('wizard.maxDrive')}（${t('wizard.maxDriveUnit')}）`}>
              <div className="flex items-center gap-3">
                <input
                  type="range"
                  min={0}
                  max={12}
                  step={0.5}
                  value={draft.maxDrive}
                  onChange={(e) => set('maxDrive', Number(e.target.value))}
                  className="flex-1 accent-primary-600"
                />
                <span className="w-14 text-right text-sm font-semibold text-slate-900">{draft.maxDrive}h</span>
              </div>
            </Field>
            <Field label={`${t('wizard.maxWalk')}（${t('wizard.maxWalkUnit')}）`}>
              <div className="flex items-center gap-3">
                <input
                  type="range"
                  min={0}
                  max={30}
                  step={1}
                  value={draft.maxWalk}
                  onChange={(e) => set('maxWalk', Number(e.target.value))}
                  className="flex-1 accent-primary-600"
                />
                <span className="w-14 text-right text-sm font-semibold text-slate-900">{draft.maxWalk}km</span>
              </div>
            </Field>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <Field label={t('wizard.earliest')}>
              <input
                type="time"
                className={inputCls}
                value={draft.earliest}
                onChange={(e) => set('earliest', e.target.value)}
              />
            </Field>
            <Field label={t('wizard.latest')}>
              <input
                type="time"
                className={inputCls}
                value={draft.latest}
                onChange={(e) => set('latest', e.target.value)}
              />
            </Field>
          </div>
          <Field label={t('wizard.allowHotelChange')}>
            <div className="flex gap-2">
              {[true, false].map((v) => (
                <button
                  key={String(v)}
                  type="button"
                  onClick={() => set('allowHotelChange', v)}
                  className={`rounded-xl border px-4 py-2 text-sm font-medium transition-colors ${
                    draft.allowHotelChange === v
                      ? 'border-primary-600 bg-primary-50 text-primary-700'
                      : 'border-slate-300 bg-white text-slate-600 hover:border-slate-400'
                  }`}
                >
                  {v ? t('wizard.allow') : t('wizard.disallow')}
                </button>
              ))}
            </div>
          </Field>
          {/* 预算范围双滑块 */}
          <div>
            <span className="mb-1.5 block text-sm font-medium text-slate-700">{t('wizard.budgetRange')}</span>
            <div className="relative h-6">
              <div className="absolute inset-y-0 my-auto h-1.5 w-full rounded-full bg-slate-200" />
              <div
                className="absolute inset-y-0 my-auto h-1.5 rounded-full bg-primary-500"
                style={{
                  left: `${((draft.budgetRange[0] - BUDGET_MIN) / (BUDGET_MAX - BUDGET_MIN)) * 100}%`,
                  right: `${100 - ((draft.budgetRange[1] - BUDGET_MIN) / (BUDGET_MAX - BUDGET_MIN)) * 100}%`,
                }}
              />
              <input
                type="range"
                min={BUDGET_MIN}
                max={BUDGET_MAX}
                step={BUDGET_STEP}
                value={draft.budgetRange[0]}
                onChange={(e) => {
                  const v = Math.min(Number(e.target.value), draft.budgetRange[1] - BUDGET_STEP)
                  set('budgetRange', [v, draft.budgetRange[1]])
                }}
                className="dual-range"
              />
              <input
                type="range"
                min={BUDGET_MIN}
                max={BUDGET_MAX}
                step={BUDGET_STEP}
                value={draft.budgetRange[1]}
                onChange={(e) => {
                  const v = Math.max(Number(e.target.value), draft.budgetRange[0] + BUDGET_STEP)
                  set('budgetRange', [draft.budgetRange[0], v])
                }}
                className="dual-range"
              />
            </div>
            <div className="mt-1 flex justify-between text-sm text-slate-600">
              <span className="font-semibold text-slate-900">¥{draft.budgetRange[0].toLocaleString()}</span>
              <span className="font-semibold text-slate-900">¥{draft.budgetRange[1].toLocaleString()}</span>
            </div>
          </div>
        </div>
      )}

      {/* 步骤④ 自然语言补充 */}
      {step === 3 && (
        <div className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <h2 className="text-base font-semibold text-slate-900">{t('wizard.extraTitle')}</h2>
          <p className="mt-1 text-sm text-slate-500">{t('wizard.extraDesc')}</p>
          <textarea
            rows={6}
            className={`${inputCls} mt-4 resize-none`}
            placeholder={t('wizard.extraPh')}
            value={draft.extra}
            maxLength={2000}
            onChange={(e) => set('extra', e.target.value)}
          />
        </div>
      )}

      {submitError && (
        <p className="mt-4 rounded-xl bg-rose-50 px-4 py-3 text-sm text-rose-700">{submitError}</p>
      )}

      {/* 底部操作 */}
      <div className="mt-8 flex items-center justify-between">
        <button
          type="button"
          onClick={() => setStep((s) => Math.max(0, s - 1))}
          disabled={step === 0 || submitting}
          className="rounded-xl border border-slate-300 px-5 py-2.5 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-40"
        >
          {t('wizard.prev')}
        </button>
        <div className="flex items-center gap-3">
          {step === STEP_KEYS.length - 1 && (
            <button
              type="button"
              onClick={() => void submit(true)}
              disabled={submitting}
              className="rounded-xl border border-slate-300 px-5 py-2.5 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100 disabled:opacity-50"
            >
              {submitting ? t('wizard.savingDraft') : t('wizard.saveDraft')}
            </button>
          )}
          {canNext ? (
            <button
              type="button"
              onClick={() => {
                if (validateStep(step)) setStep((s) => s + 1)
              }}
              className="rounded-xl bg-primary-600 px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-primary-700"
            >
              {t('wizard.next')}
            </button>
          ) : (
            <button
              type="button"
              onClick={() => void submit(false)}
              disabled={submitting}
              className="rounded-xl bg-ai-600 px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-ai-700 disabled:opacity-50"
            >
              {submitting ? t('wizard.creating') : t('wizard.create')}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
