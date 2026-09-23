import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { ApiError, tripsApi } from '../services/api'
import type { CreateTripPayload } from '../types'
import { DRAFT_NL_KEY } from '../utils/draftNl'
import { compressCover } from '../features/trips/coverImage'

const STEP_KEYS = ['basic', 'extra'] as const
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

// 快捷约束 chip → 合并进自然语言(约束数值滑块从向导移除,交给 AI 对话/默认)
const QUICK_CHIP_TEXT: Record<'relaxed' | 'lessWalk' | 'shortDrive', string> = {
  relaxed: '节奏别太赶',
  lessWalk: '尽量少走路',
  shortDrive: '每天开车不超过 3 小时',
}

function buildAutoPrompt(draft: DraftState, t: (key: string, opt?: Record<string, unknown>) => string): string {
  const parts: string[] = []
  const dest = draft.destination.trim()
  if (dest) {
    parts.push(`帮我规划去${dest}的行程`)
  }
  if (draft.startDate && draft.endDate) {
    const s = new Date(draft.startDate)
    const e = new Date(draft.endDate)
    const diff = Math.max(1, Math.round((e.getTime() - s.getTime()) / 86400000) + 1)
    parts.push(`时间为 ${draft.startDate} 至 ${draft.endDate}（共 ${diff} 天）`)
  }
  if (draft.travelers > 0) {
    parts.push(`${draft.travelers}人出行`)
  }
  if (draft.budget !== '') {
    parts.push(`预算约 ${draft.budget} 元/人`)
  }
  if (draft.transport.trim()) {
    parts.push(`交通方式偏好：${draft.transport.trim()}`)
  }
  if (draft.prefTags.length > 0) {
    const tags = draft.prefTags
      .map((k) => t(`wizard.prefTags.${k}`, { defaultValue: k }))
      .join('、')
    parts.push(`偏好：${tags}`)
  }
  const chips = draft.quickChips
    .map((k) => QUICK_CHIP_TEXT[k as keyof typeof QUICK_CHIP_TEXT])
    .filter(Boolean)
  if (chips.length > 0) {
    parts.push(`要求：${chips.join('、')}`)
  }
  if (draft.extra.trim()) {
    parts.push(`补充需求：${draft.extra.trim()}`)
  }
  if (parts.length === 0) return ''
  return parts.join('，') + '。请为我们设计合理的每日行程安排。'
}

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
  quickChips: string[]
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
  quickChips: [],
}

// 默认封面预览:与列表卡同源的生成插画 + 目的地名(随输入联动)
function CoverPreview({ destination, previewUrl }: { destination: string; previewUrl: string | null }) {
  const { t } = useTranslation()
  if (previewUrl) {
    return (
      <img
        src={previewUrl}
        alt={t('wizard.cover')}
        className="h-24 w-40 rounded-lg object-cover"
      />
    )
  }
  let h = 0
  for (const ch of destination || 'trip') h = (h + ch.charCodeAt(0)) % 360
  const gid = 'wiz-cover'
  return (
    <svg viewBox="0 0 160 96" className="h-24 w-40 rounded-lg" aria-hidden="true">
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor={`hsl(${h}, 45%, 72%)`} />
          <stop offset="100%" stopColor={`hsl(${h}, 50%, 42%)`} />
        </linearGradient>
      </defs>
      <rect width="160" height="96" fill={`url(#${gid})`} />
      <circle cx="118" cy="24" r="11" fill="#FFFFFF" opacity="0.85" />
      <path d="M0 70 L30 38 L60 66 L95 30 L130 58 L160 46 V96 H0 Z" fill="#1E293B" opacity="0.35" />
      <path d="M0 96 V78 L42 52 L88 82 L128 60 L160 74 V96 Z" fill="#0F172A" opacity="0.55" />
      {destination.trim() && (
        <text x="10" y="78" fill="#FFFFFF" fontSize="15" fontWeight="700" fontFamily="Inter, sans-serif">
          {destination.trim().slice(0, 8)}
        </text>
      )}
    </svg>
  )
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
  const [coverFile, setCoverFile] = useState<{ blob: Blob; previewUrl: string } | null>(null)
  const [coverError, setCoverError] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  async function onPickCover(file: File | undefined) {
    if (!file) return
    try {
      setCoverError(false)
      setCoverFile(await compressCover(file))
    } catch {
      setCoverError(true)
    } finally {
      if (fileRef.current) fileRef.current.value = ''
    }
  }

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

  const toggleQuick = (key: string) => {
    setDraft((d) => ({
      ...d,
      quickChips: d.quickChips.includes(key) ? d.quickChips.filter((x) => x !== key) : [...d.quickChips, key],
    }))
  }

  function validateStep(s: number): boolean {
    const e: Record<string, string> = {}
    if (s === 0) {
      if (!draft.destination.trim()) e.destination = t('wizard.errors.destination')
      // 日期可选:填则须成对且顺序正确,不填则后续在卡片/规划器里补
      if (draft.startDate || draft.endDate) {
        if (!draft.startDate || !draft.endDate) e.dates = t('wizard.errors.dates')
        else if (draft.endDate < draft.startDate) e.dates = t('wizard.errors.dateOrder')
      }
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
    const extraText = [
      draft.extra.trim(),
      ...draft.quickChips.map((k) => QUICK_CHIP_TEXT[k as keyof typeof QUICK_CHIP_TEXT]).filter(Boolean),
    ]
      .filter(Boolean)
      .join('\n')
    const autoPrompt = buildAutoPrompt(draft, t)
    const payload: CreateTripPayload = {
      title: draft.destination.trim() || t('wizard.untitled'),
      destination: draft.destination.trim(),
      start_date: draft.startDate || null,
      end_date: draft.endDate || null,
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
      natural_language: autoPrompt || extraText || null,
    }
    try {
      const trip = await tripsApi.create(payload)
      // 封面补传:best-effort,失败不阻塞进入规划器(卡片上仍可重传)
      if (coverFile) {
        await tripsApi.uploadCover(trip.id, coverFile.blob).catch(() => {})
      }
      // 正常创建进入规划器时自动启动 AI 规划；保存草稿则静默进入
      const target = saveAsDraft ? `/trip/${trip.id}` : `/trip/${trip.id}?autostart=1`
      navigate(target, { replace: true })
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

          {/* 封面:可选,默认自动生成目的地语义封面 */}
          <div className="rounded-xl border border-slate-200 bg-slate-50/60 p-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium text-slate-700">{t('wizard.cover')}</span>
              <span className="text-xs text-slate-400">{t('wizard.coverOptional')}</span>
            </div>
            <div className="mt-3 flex items-center gap-4">
              <CoverPreview destination={draft.destination} previewUrl={coverFile?.previewUrl ?? null} />
              <div className="flex flex-col gap-2">
                <input
                  ref={fileRef}
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  className="hidden"
                  onChange={(e) => void onPickCover(e.target.files?.[0])}
                />
                <button
                  type="button"
                  onClick={() => fileRef.current?.click()}
                  className="inline-flex items-center gap-2 rounded-xl border border-primary-500 px-4 py-2 text-sm font-semibold text-primary-600 transition-colors hover:bg-primary-50"
                >
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
                    <path d="M4 16l4.6-6 3.4 4 3-3 5 5M4 16v3h16v-3M4 16v-9h16v9" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                  {coverFile ? t('wizard.coverChange') : t('wizard.coverUpload')}
                </button>
                {coverFile && (
                  <button
                    type="button"
                    onClick={() => setCoverFile(null)}
                    className="text-left text-xs text-slate-500 underline-offset-2 hover:text-rose-600 hover:underline"
                  >
                    {t('wizard.coverRemove')}
                  </button>
                )}
              </div>
            </div>
            {coverError && <p className="mt-2 text-xs text-rose-600">{t('wizard.coverError')}</p>}
          </div>
        </div>
      )}

      {/* 步骤② 补充说明(偏好 + 约束快捷项 + 自然语言,全部可跳过) */}
      {step === 1 && (
        <div className="space-y-6 rounded-2xl border border-slate-200 bg-white p-6 shadow-sm">
          <div>
            <p className="mb-3 text-sm font-medium text-slate-700">{t('wizard.prefsDesc')}</p>
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

          <div>
            <p className="mb-3 text-sm font-medium text-slate-700">{t('wizard.constraintsDesc')}</p>
            <div className="flex flex-wrap gap-2.5">
              {(['relaxed', 'lessWalk', 'shortDrive'] as const).map((key) => {
                const on = draft.quickChips.includes(key)
                return (
                  <button
                    key={key}
                    type="button"
                    onClick={() => toggleQuick(key)}
                    className={`rounded-full border px-4 py-2 text-sm font-medium transition-colors ${
                      on
                        ? 'border-ai-600 bg-ai-50 text-ai-700'
                        : 'border-slate-300 bg-white text-slate-600 hover:border-slate-400'
                    }`}
                  >
                    {t(`wizard.quickChips.${key}`)}
                  </button>
                )
              })}
            </div>
          </div>

          <div>
            <p className="text-sm font-medium text-slate-700">{t('wizard.extraTitle')}</p>
            <p className="mt-1 text-xs text-slate-500">{t('wizard.extraDesc')}</p>
            <textarea
              rows={5}
              className={`${inputCls} mt-3 resize-none`}
              placeholder={t('wizard.extraPh')}
              value={draft.extra}
              maxLength={2000}
              onChange={(e) => set('extra', e.target.value)}
            />
          </div>
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
