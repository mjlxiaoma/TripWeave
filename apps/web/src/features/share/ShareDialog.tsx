import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { shareApi } from './api'

interface Props {
  tripId: string
  onClose: () => void
}

// ShareDialog：生成/复制/撤销行程的公开只读链接。
export default function ShareDialog({ tripId, onClose }: Props) {
  const { t } = useTranslation()
  const [token, setToken] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const boxRef = useRef<HTMLDivElement>(null)

  const shareUrl = token ? `${window.location.origin}/share/${token}` : null

  // 打开时生成（幂等：已有 token 直接返回）
  useEffect(() => {
    let cancelled = false
    shareApi
      .create(tripId)
      .then((r) => {
        if (!cancelled) setToken(r.token)
      })
      .catch(() => {
        if (!cancelled) setError(t('share.error'))
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [tripId, t])

  // 点击外部关闭
  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) onClose()
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [onClose])

  async function copy() {
    if (!shareUrl) return
    try {
      await navigator.clipboard.writeText(shareUrl)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      setError(t('share.copyError'))
    }
  }

  async function revoke() {
    if (!window.confirm(t('share.revokeConfirm'))) return
    try {
      await shareApi.revoke(tripId)
      setToken(null)
      onClose()
    } catch {
      setError(t('share.error'))
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4 backdrop-blur-sm">
      <div ref={boxRef} className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-5 shadow-xl">
        <div className="flex items-center justify-between">
          <h3 className="text-base font-semibold text-slate-900">{t('share.title')}</h3>
          <button type="button" onClick={onClose} className="rounded-md p-1 text-slate-400 hover:bg-slate-100" aria-label={t('share.close')}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="h-5 w-5">
              <path d="M18 6L6 18M6 6l12 12" strokeLinecap="round" />
            </svg>
          </button>
        </div>
        <p className="mt-1 text-xs text-slate-500">{t('share.hint')}</p>

        {loading ? (
          <div className="mt-4 flex justify-center py-4">
            <span className="h-6 w-6 animate-spin rounded-full border-2 border-primary-600 border-t-transparent" />
          </div>
        ) : error ? (
          <p className="mt-4 rounded-xl bg-rose-50 px-4 py-3 text-sm text-rose-700">{error}</p>
        ) : shareUrl ? (
          <>
            <div className="mt-4 flex items-center gap-2">
              <input
                readOnly
                value={shareUrl}
                onFocus={(e) => e.target.select()}
                className="min-w-0 flex-1 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-700 focus:outline-none"
              />
              <button
                type="button"
                onClick={copy}
                className={`shrink-0 rounded-lg px-4 py-2 text-sm font-semibold transition-colors ${
                  copied ? 'bg-emerald-600 text-white' : 'bg-primary-600 text-white hover:bg-primary-700'
                }`}
              >
                {copied ? `✓ ${t('share.copied')}` : t('share.copy')}
              </button>
            </div>
            <div className="mt-4 flex items-center justify-between">
              <a href={shareUrl} target="_blank" rel="noreferrer" className="text-xs font-medium text-primary-600 hover:underline">
                {t('share.preview')} ↗
              </a>
              <button type="button" onClick={revoke} className="text-xs font-medium text-rose-600 hover:underline">
                {t('share.revoke')}
              </button>
            </div>
          </>
        ) : null}
      </div>
    </div>
  )
}
