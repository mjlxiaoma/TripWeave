import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../features/auth/AuthProvider'
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
type ChipKey = (typeof CHIP_KEYS)[number]

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
  const [activeChip, setActiveChip] = useState<ChipKey>('popular')
  const [listening, setListening] = useState(false)

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

  const startPlanning = () => {
    const text = nl.trim()
    if (text) sessionStorage.setItem(DRAFT_NL_KEY, text)
    navigate(user ? '/trip/new' : '/register')
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
          onClick={startPlanning}
          className="mt-8 inline-flex items-center gap-2 rounded-xl bg-primary-600 px-8 py-3.5 text-base font-semibold text-white shadow-sm transition hover:bg-primary-700"
        >
          {t('home.startPlanning')}
          <ArrowRightIcon />
        </button>

        <div className="mt-20">
          <p className="text-sm font-medium text-slate-500">{t('home.popularTitle')}</p>
          <div className="mt-4 flex flex-wrap items-center justify-center gap-3">
            {CHIP_KEYS.map((key) => {
              const active = key === activeChip
              return (
                <button
                  key={key}
                  type="button"
                  onClick={() => {
                    setActiveChip(key)
                    setNl(t('home.chipSamples.' + key))
                  }}
                  className={(active
                    ? 'bg-primary-600 text-white '
                    : 'bg-white text-slate-600 hover:text-primary-700 ') +
                    'rounded-full px-4 py-2 text-sm font-medium shadow-sm transition'}
                >
                  {t('home.chips.' + key)}
                </button>
              )
            })}
          </div>
        </div>
      </div>
    </div>
  )
}
