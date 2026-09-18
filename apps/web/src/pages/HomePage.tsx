import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../features/auth/AuthProvider'
import { api, tripsApi } from '../services/api'
import { DRAFT_NL_KEY } from '../utils/draftNl'

type SpeechAlt = { transcript: string }
type SpeechEventLike = { results: ArrayLike<ArrayLike<SpeechAlt>> }
type SpeechRecognitionLike = {
  lang: string
  continuous: boolean
  interimResults: boolean
  onresult: ((event: SpeechEventLike) => void) | null
  onend: (() => void) | null
  onerror: (() => void) | null
  start: () => void
  stop: () => void
}
type SpeechWindow = Window & {
  SpeechRecognition?: new () => SpeechRecognitionLike
  webkitSpeechRecognition?: new () => SpeechRecognitionLike
}

const CHIP_KEYS = ['popular', 'niche', 'photo', 'weekend', 'drive'] as const

// AI 生成的灵感标签（/inspiration 返回）；null = 未加载或失败（走静态兜底）
interface AiChip {
  label: string
  sample: string
}

function SparkleIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4">
      <path d="M12 2.5l1.9 5.6 5.6 1.9-5.6 1.9L12 17.5l-1.9-5.6-5.6-1.9 5.6-1.9L12 2.5z" />
      <path d="M19 14.5l.9 2.6 2.6.9-2.6.9-.9 2.6-.9-2.6-2.6-.9 2.6-.9.9-2.6z" />
    </svg>
  )
}

function ArrowRightIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5">
      <path d="M5 12h14" strokeLinecap="round" />
      <path d="M13 6l6 6-6 6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function MicIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-5 w-5">
      <rect x="9" y="3" width="6" height="11" rx="3" />
      <path d="M5.5 11a6.5 6.5 0 0013 0" strokeLinecap="round" />
      <path d="M12 17.5V21" strokeLinecap="round" />
    </svg>
  )
}

export default function HomePage() {
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const { user } = useAuth()
  const [nl, setNl] = useState('')
  const [activeChip, setActiveChip] = useState(0)
  const [listening, setListening] = useState(false)
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string | null>(null)
  const [aiChips, setAiChips] = useState<AiChip[] | null>(null)
  const [chipsLoading, setChipsLoading] = useState(true)

  // 登录/注册回来自动恢复上次输入的草稿。
  useEffect(() => {
    const draft = sessionStorage.getItem(DRAFT_NL_KEY)
    if (draft) setNl(draft)
  }, [])

  // 拉取 AI 生成的灵感标签；失败/为空则保持 null，渲染时走静态兜底。
  useEffect(() => {
    let cancelled = false
    setChipsLoading(true)
    api<{ chips: AiChip[]; source: string }>(`/inspiration?locale=${i18n.language}`)
      .then((res) => {
        if (!cancelled && res.chips.length > 0) setAiChips(res.chips)
      })
      .catch(() => {
        /* 静默：AI 不可用时用静态标签兜底 */
      })
      .finally(() => {
        if (!cancelled) setChipsLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [i18n.language])

  // 渲染用的标签：AI 的优先，否则静态兜底
  const chips: AiChip[] =
    aiChips ??
    CHIP_KEYS.map((k) => ({
      label: t('home.chips.' + k),
      sample: t('home.chipSamples.' + k),
    }))

  const speechSupported = useMemo(() => {
    if (typeof window === 'undefined') return false
    const w = window as SpeechWindow
    return Boolean(w.SpeechRecognition ?? w.webkitSpeechRecognition)
  }, [])

  const startListening = () => {
    const w = window as SpeechWindow
    const Ctor = w.SpeechRecognition ?? w.webkitSpeechRecognition
    if (!Ctor) return
    const rec = new Ctor()
    rec.lang = i18n.language === 'en-US' ? 'en-US' : 'zh-CN'
    rec.continuous = false
    rec.interimResults = false
    rec.onresult = (event) => {
      let text = ''
      for (let i = 0; i < event.results.length; i += 1) {
        const alt = event.results[i]?.[0]
        if (alt) text += alt.transcript
      }
      const transcript = text.trim()
      if (transcript) setNl((prev) => (prev ? prev + ' ' + transcript : transcript))
    }
    rec.onend = () => setListening(false)
    rec.onerror = () => setListening(false)
    setListening(true)
    rec.start()
  }

  const startPlanning = async () => {
    const text = nl.trim()
    if (!text || creating) return
    // 未登录:存草稿,注册/登录后回来恢复。
    if (!user) {
      sessionStorage.setItem(DRAFT_NL_KEY, text)
      navigate('/register')
      return
    }
    // 已登录:创建最小骨架 trip,进规划器由 AI 接管生成。
    setCreating(true)
    setCreateError(null)
    try {
      const trip = await tripsApi.create({
        title: text.slice(0, 24) || t('home.untitledTrip'),
        status: 'planning',
        natural_language: text,
      })
      sessionStorage.removeItem(DRAFT_NL_KEY)
      navigate(`/trip/${trip.id}?autostart=1`, { replace: true })
    } catch {
      setCreateError(t('home.createFailed'))
      setCreating(false)
    }
  }

  return (
    <div className="bg-slate-100/70 px-6 pb-24 pt-20">
      <div className="mx-auto flex max-w-3xl flex-col items-center text-center">
        <span className="inline-flex items-center gap-2 rounded-full bg-primary-100 px-4 py-1.5 text-sm font-medium text-primary-700">
          <SparkleIcon />
          {t('home.badge')}
        </span>
        <h1 className="mt-8 text-5xl font-bold tracking-tight text-slate-900">{t('home.heroTitle')}</h1>
        <p className="mt-6 text-lg text-slate-500">{t('home.heroSubtitle')}</p>

        <div className="relative mt-10 w-full rounded-2xl bg-slate-50 p-6 text-left shadow-sm">
          <textarea
            rows={3}
            value={nl}
            onChange={(e) => setNl(e.target.value)}
            placeholder={t('home.nlPlaceholder')}
            className="w-full resize-none bg-transparent pr-14 text-base text-slate-800 outline-none placeholder:text-slate-400"
          />
          <p className="mt-2 text-xs text-slate-400">
            {listening ? t('home.listening') : t('home.nlHint')}
          </p>
          {speechSupported && (
            <button
              type="button"
              onClick={startListening}
              aria-label={t('home.listening')}
              className={(listening
                ? 'bg-primary-600 text-white '
                : 'bg-white text-slate-500 hover:text-primary-600 ') +
                'absolute right-5 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full shadow-sm transition'}
            >
              <MicIcon />
            </button>
          )}
        </div>

        <button
          type="button"
          onClick={() => void startPlanning()}
          disabled={creating}
          className="mt-8 inline-flex items-center gap-2 rounded-xl bg-primary-600 px-8 py-3.5 text-base font-semibold text-white shadow-sm transition hover:bg-primary-700 disabled:opacity-50"
        >
          {creating ? t('home.creating') : t('home.startPlanning')}
          {!creating && <ArrowRightIcon />}
        </button>
        {createError && (
          <p className="mt-3 rounded-xl bg-rose-50 px-4 py-2 text-sm text-rose-700">{createError}</p>
        )}

        <div className="mt-20">
          <p className="flex items-center justify-center gap-2 text-sm font-medium text-slate-500">
            {t('home.popularTitle')}
            {aiChips && (
              <span className="inline-flex items-center gap-1 rounded-full bg-ai-100 px-2 py-0.5 text-[11px] font-semibold text-ai-700">
                <SparkleIcon />
                {t('home.aiBadge')}
              </span>
            )}
          </p>
          <div className="mt-4 flex flex-wrap items-center justify-center gap-3">
            {chipsLoading
              ? // 骨架：5 个 pulse 圆块
                Array.from({ length: 5 }).map((_, i) => (
                  <span
                    key={i}
                    className="h-9 w-24 animate-pulse rounded-full bg-slate-200"
                  />
                ))
              : chips.map((chip, idx) => {
                  const active = idx === activeChip
                  return (
                    <button
                      key={idx}
                      type="button"
                      onClick={() => {
                        setActiveChip(idx)
                        setNl(chip.sample)
                      }}
                      className={(active
                        ? 'bg-primary-600 text-white '
                        : 'bg-white text-slate-600 hover:text-primary-700 ') +
                        'rounded-full px-4 py-2 text-sm font-medium shadow-sm transition'}
                    >
                      {chip.label}
                    </button>
                  )
                })}
          </div>
        </div>
      </div>
    </div>
  )
}
