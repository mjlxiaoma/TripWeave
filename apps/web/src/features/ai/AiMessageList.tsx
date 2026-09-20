import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { ChatMessage } from './types'

function Spinner() {
  return (
    <span className="inline-block h-3.5 w-3.5 animate-spin rounded-full border-2 border-ai-600 border-t-transparent" />
  )
}

function ToolLine({ label, status }: { label: string; status: 'running' | 'success' | 'error' }) {
  return (
    <div className="mt-1.5 flex items-center gap-2 text-xs text-slate-500">
      {status === 'running' && <Spinner />}
      {status === 'success' && <span className="text-emerald-600">✓</span>}
      {status === 'error' && <span className="text-rose-600">✗</span>}
      <span>{label}</span>
    </div>
  )
}

interface Props {
  messages: ChatMessage[]
  streaming: boolean
}

export default function AiMessageList({ messages, streaming }: Props) {
  const { t } = useTranslation()
  const endRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' })
  }, [messages, streaming])

  if (messages.length === 0 && !streaming) {
    return (
      <div className="px-6 py-12 text-center">
        <p className="text-sm text-slate-400">{t('ai.emptyHint')}</p>
      </div>
    )
  }

  // 滚动由父级面板容器承担，消息流自身只负责排列
  return (
    <div className="space-y-4 px-4 py-4">
      {messages.map((m) =>
        m.role === 'user' ? (
          <div key={m.id} className="flex justify-end">
            <div className="max-w-[80%] whitespace-pre-wrap rounded-2xl rounded-br-sm bg-primary-600 px-4 py-2.5 text-sm text-white">
              {m.text}
            </div>
          </div>
        ) : (
          <div key={m.id} className="flex justify-start">
            <div className="max-w-[85%] rounded-2xl rounded-bl-sm border border-slate-200 bg-white px-4 py-2.5 text-sm text-slate-800 shadow-sm">
              {m.text ? <p className="whitespace-pre-wrap">{m.text}</p> : null}
              {m.tools && m.tools.length > 0 && (
                <div className={m.text ? 'mt-2 border-t border-slate-100 pt-2' : ''}>
                  {m.tools.map((tool) => (
                    <ToolLine key={tool.id} label={tool.label} status={tool.status} />
                  ))}
                </div>
              )}
              {m.pending && !m.text && (!m.tools || m.tools.length === 0) && (
                <div className="flex items-center gap-2 text-slate-400">
                  <Spinner />
                  <span className="text-xs">{t('ai.thinking')}</span>
                </div>
              )}
            </div>
          </div>
        ),
      )}
      <div ref={endRef} />
    </div>
  )
}
