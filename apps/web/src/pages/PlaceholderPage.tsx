import { useTranslation } from 'react-i18next'

export default function PlaceholderPage() {
  const { t } = useTranslation()
  return (
    <div className="flex min-h-[calc(100vh-4rem)] flex-col items-center justify-center px-6 text-center">
      <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-slate-100">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className="h-7 w-7 text-slate-400" aria-hidden="true">
          <circle cx="12" cy="12" r="10" />
          <path d="M12 6v6l4 2" />
        </svg>
      </div>
      <h1 className="mt-5 text-2xl font-bold text-slate-900">{t('placeholder.title')}</h1>
      <p className="mt-2 text-sm text-slate-500">{t('placeholder.desc')}</p>
    </div>
  )
}
