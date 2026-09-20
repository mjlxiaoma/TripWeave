import { useState } from 'react'
import { useTranslation } from 'react-i18next'

function SendIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-4 w-4">
      <path d="M4 12l16-8-6 16-2.5-6.5L4 12z" strokeLinejoin="round" />
    </svg>
  )
}

function StopIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4">
      <rect x="7" y="7" width="10" height="10" rx="1.5" />
    </svg>
  )
}

interface Props {
  streaming: boolean
  onSend: (text: string) => void
  onStop: () => void
}

export default function AiComposer({ streaming, onSend, onStop }: Props) {
  const { t } = useTranslation()
  const [text, setText] = useState('')

  const submit = () => {
    const v = text.trim()
    if (!v || streaming) return
    onSend(v)
    setText('')
  }

  return (
    <div className="border-t border-slate-200 bg-white/90 backdrop-blur">
      <div className="flex items-end gap-3 px-4 py-4">
        <textarea
          rows={1}
          value={text}
          disabled={streaming}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
              e.preventDefault()
              submit()
            }
          }}
          placeholder={t('ai.composerPlaceholder')}
          className="max-h-32 min-h-[44px] flex-1 resize-none rounded-xl border border-slate-300 bg-white px-4 py-2.5 text-sm text-slate-900 placeholder:text-slate-400 focus:border-ai-500 focus:outline-none focus:ring-2 focus:ring-ai-500/20 disabled:opacity-60 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
        />
        {streaming ? (
          <button
            type="button"
            onClick={onStop}
            className="inline-flex h-11 items-center gap-2 rounded-xl border border-slate-300 px-4 text-sm font-medium text-slate-600 transition-colors hover:bg-slate-100"
          >
            <StopIcon />
            {t('ai.stop')}
          </button>
        ) : (
          <button
            type="button"
            onClick={submit}
            disabled={!text.trim()}
            className="inline-flex h-11 items-center gap-2 rounded-xl bg-ai-600 px-5 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-ai-700 disabled:opacity-40"
          >
            <SendIcon />
            {t('ai.send')}
          </button>
        )}
      </div>
    </div>
  )
}
