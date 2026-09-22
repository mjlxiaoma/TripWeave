import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { createPortal } from 'react-dom'
import { useTranslation } from 'react-i18next'

interface Props {
  /** 锚点(芯片按钮的 viewport 坐标);null = 不渲染 */
  anchor: { x: number; y: number } | null
  onClose: () => void
  onSave: (start: string, end: string) => Promise<void> | void
}

function fmt(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

function addDays(d: Date, n: number): Date {
  const x = new Date(d)
  x.setDate(x.getDate() + n)
  return x
}

// 快捷区间:本周末(六日)/ 下周(下周一~日)/ 国庆(10.1–10.7,过了则明年)
function quickRanges(): { key: 'weekend' | 'nextWeek' | 'nationalDay'; start: string; end: string }[] {
  const today = new Date()
  const day = today.getDay()
  const sat = addDays(today, (6 - day + 7) % 7)
  const weekend = { key: 'weekend' as const, start: fmt(sat), end: fmt(addDays(sat, 1)) }
  const nextMon = addDays(today, ((8 - day) % 7) || 7)
  const nextWeek = { key: 'nextWeek' as const, start: fmt(nextMon), end: fmt(addDays(nextMon, 6)) }
  const y = today.getFullYear()
  const ndYear = today > new Date(y, 9, 7) ? y + 1 : y
  const nationalDay = { key: 'nationalDay' as const, start: `${ndYear}-10-01`, end: `${ndYear}-10-07` }
  return [weekend, nextWeek, nationalDay]
}

// DateQuickSet: 轻量日期弹层 —— 锚定在触发芯片下方,支持快捷区间与手选,
// 选定即 PATCH 保存;卡片/规划器子头部共用。
export default function DateQuickSet({ anchor, onClose, onSave }: Props) {
  const { t } = useTranslation()
  const [start, setStart] = useState('')
  const [end, setEnd] = useState('')
  const [busy, setBusy] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    window.addEventListener('scroll', onClose, true)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
      window.removeEventListener('scroll', onClose, true)
    }
  }, [onClose])

  if (!anchor) return null

  const invalid = Boolean(start && end && end < start)
  const style: CSSProperties = {
    position: 'fixed',
    left: Math.max(12, Math.min(anchor.x, window.innerWidth - 376)),
    top: anchor.y + 8,
    zIndex: 60,
  }

  async function save() {
    if (!start || !end || invalid || busy) return
    setBusy(true)
    try {
      await onSave(start, end)
    } finally {
      setBusy(false)
    }
  }

  return createPortal(
    <div
      ref={ref}
      style={style}
      className="w-[360px] rounded-2xl border border-slate-200 bg-white p-4 shadow-2xl"
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
    >
      <p className="text-sm font-semibold text-slate-900">{t('trips.quick.title')}</p>
      <p className="mt-0.5 text-xs text-slate-400">{t('trips.quick.sub')}</p>

      <div className="mt-3 grid grid-cols-2 gap-2.5">
        <label className="block">
          <span className="mb-1 block text-[11px] font-semibold text-slate-500">{t('trips.quick.start')}</span>
          <input
            type="date"
            value={start}
            onChange={(e) => setStart(e.target.value)}
            className="w-full rounded-lg border border-slate-300 px-2.5 py-2 text-xs text-slate-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
          />
        </label>
        <label className="block">
          <span className="mb-1 block text-[11px] font-semibold text-slate-500">{t('trips.quick.end')}</span>
          <input
            type="date"
            value={end}
            min={start || undefined}
            onChange={(e) => setEnd(e.target.value)}
            className="w-full rounded-lg border border-slate-300 px-2.5 py-2 text-xs text-slate-900 focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/20"
          />
        </label>
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        {quickRanges().map((q) => (
          <button
            key={q.key}
            type="button"
            onClick={() => {
              setStart(q.start)
              setEnd(q.end)
            }}
            className="rounded-full bg-slate-100 px-3 py-1.5 text-xs font-medium text-slate-600 transition-colors hover:bg-primary-50 hover:text-primary-700"
          >
            {t(`trips.quick.${q.key}`)}
          </button>
        ))}
      </div>

      {invalid && <p className="mt-2 text-xs text-rose-600">{t('trips.quick.invalid')}</p>}

      <div className="mt-4 flex justify-end gap-2">
        <button
          type="button"
          onClick={onClose}
          className="rounded-lg border border-slate-300 px-3.5 py-2 text-xs font-medium text-slate-600 transition-colors hover:bg-slate-50"
        >
          {t('trips.quick.cancel')}
        </button>
        <button
          type="button"
          onClick={() => void save()}
          disabled={!start || !end || invalid || busy}
          className="rounded-lg bg-primary-600 px-4 py-2 text-xs font-semibold text-white transition-colors hover:bg-primary-700 disabled:opacity-50"
        >
          {t('trips.quick.ok')}
        </button>
      </div>
    </div>,
    document.body,
  )
}
