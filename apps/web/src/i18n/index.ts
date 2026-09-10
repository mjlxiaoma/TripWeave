import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import LanguageDetector from 'i18next-browser-languagedetector'
import zhCN from './zh-CN'
import enUS from './en-US'

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      'zh-CN': { translation: zhCN },
      'en-US': { translation: enUS },
    },
    supportedLngs: ['zh-CN', 'en-US'],
    fallbackLng: 'zh-CN',
    detection: {
      order: ['localStorage', 'navigator'],
      lookupLocalStorage: 'tw_lang',
      caches: ['localStorage'],
    },
    interpolation: { escapeValue: false },
  })

export default i18n
