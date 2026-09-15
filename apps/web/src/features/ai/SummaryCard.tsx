import { useTranslation } from 'react-i18next'
import type { SummaryChange } from './types'

function SparkleIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4">
      <path d="M12 2.5l1.9 5.6 5.6 1.9-5.6 1.9L12 17.5l-1.9-5.6-5.6-1.9 5.6-1.9L12 2.5z" />
    </svg>
  )
}

interface Props {
  lines: string[]
  changes: SummaryChange[] | null
}

export default function SummaryCard({ lines, changes }: Props) {
  const { t } = useTranslation()
  if (!changes || changes.length === 0 || lines.length === 0) return null

  return (
    <div className="rounded-2xl border border-ai-200 bg-ai-50 p-4">
      <div className="mb-2 flex items-center gap-2 text-sm font-semibold text-ai-700">
        <SparkleIcon />
        {t('ai.summaryTitle')}
      </div>
      <ul className="space-y-1 text-xs leading-relaxed text-ai-900">
        {lines.map((line, i) => (
          <li key={i} className={line.endsWith('：') ? 'font-medium' : 'pl-3'}>
            {line}
          </li>
        ))}
      </ul>
    </div>
  )
}
