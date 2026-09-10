import { useTranslation } from 'react-i18next'

export default function LanguageSwitch() {
  const { i18n } = useTranslation()
  const current = i18n.language.startsWith('en') ? 'en' : 'zh'
  const base = 'rounded-full px-2.5 py-1 transition-colors'
  return (
    <div className="flex items-center rounded-full bg-slate-100 p-0.5 text-xs font-semibold">
      <button
        type="button"
        aria-label="切换到中文"
        onClick={() => void i18n.changeLanguage('zh-CN')}
        className={`${base} ${current === 'zh' ? 'bg-primary-600 text-white' : 'text-slate-500 hover:text-slate-700'}`}
      >
        中
      </button>
      <button
        type="button"
        aria-label="Switch to English"
        onClick={() => void i18n.changeLanguage('en-US')}
        className={`${base} ${current === 'en' ? 'bg-primary-600 text-white' : 'text-slate-500 hover:text-slate-700'}`}
      >
        EN
      </button>
    </div>
  )
}
