import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../features/auth/AuthProvider'
import CompassIcon from '../components/CompassIcon'

export default function HomePage() {
  const { t } = useTranslation()
  const { user } = useAuth()

  return (
    <div className="mx-auto flex max-w-3xl flex-col items-center px-6 py-24 text-center">
      <div className="flex h-16 w-16 items-center justify-center rounded-2xl bg-primary-50">
        <CompassIcon className="h-10 w-10 text-primary-600" />
      </div>
      {user ? (
        <>
          <h1 className="mt-6 text-3xl font-bold text-slate-900">
            {t('home.welcomeBack', { name: user.display_name })}
          </h1>
          <p className="mt-3 text-slate-500">{t('home.welcomeSubtitle')}</p>
          <Link
            to="/trips"
            className="mt-8 rounded-lg bg-primary-600 px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-primary-700"
          >
            {t('home.goTrips')}
          </Link>
          <p className="mt-10 text-xs text-slate-400">{t('home.building')}</p>
        </>
      ) : (
        <>
          <h1 className="mt-6 text-4xl font-bold leading-tight text-slate-900">{t('home.heroTitle')}</h1>
          <p className="mt-4 max-w-xl text-slate-500">{t('home.heroSubtitle')}</p>
          <div className="mt-8 flex items-center gap-4">
            <Link
              to="/register"
              className="rounded-lg bg-primary-600 px-6 py-3 text-sm font-semibold text-white transition-colors hover:bg-primary-700"
            >
              {t('home.startNow')}
            </Link>
            <Link
              to="/login"
              className="rounded-lg border border-slate-300 px-6 py-3 text-sm font-semibold text-slate-700 transition-colors hover:border-slate-400 hover:text-slate-900"
            >
              {t('home.goLogin')}
            </Link>
          </div>
        </>
      )}
    </div>
  )
}
