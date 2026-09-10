import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../features/auth/AuthProvider'
import LanguageSwitch from './LanguageSwitch'
import CompassIcon from './CompassIcon'

export default function AppHeader() {
  const { t } = useTranslation()
  const { user, logout } = useAuth()
  const navigate = useNavigate()

  const navClass = ({ isActive }: { isActive: boolean }) =>
    `transition-colors ${isActive ? 'text-primary-600 font-semibold' : 'text-slate-600 hover:text-slate-900'}`

  const onLogout = async () => {
    await logout()
    navigate('/')
  }

  return (
    <header className="sticky top-0 z-10 border-b border-slate-200 bg-white">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
        <Link to="/" className="flex items-center gap-2">
          <CompassIcon className="h-7 w-7 text-primary-600" />
          <span className="text-xl font-bold text-slate-900">{t('brand.name')}</span>
          <span className="mt-0.5 text-sm text-slate-400">{t('brand.tag')}</span>
        </Link>

        <nav className="hidden items-center gap-8 text-sm md:flex">
          <NavLink to="/" end className={navClass}>
            {t('nav.home')}
          </NavLink>
          <NavLink to="/explore" className={navClass}>
            {t('nav.explore')}
          </NavLink>
          <NavLink to="/trips" className={navClass}>
            {t('nav.myTrips')}
          </NavLink>
        </nav>

        <div className="flex items-center gap-3">
          <LanguageSwitch />
          {user ? (
            <>
              <span
                className="flex h-8 w-8 items-center justify-center rounded-full bg-primary-600 text-sm font-semibold text-white"
                title={user.email}
              >
                {user.display_name.slice(0, 1).toUpperCase()}
              </span>
              <button
                type="button"
                onClick={() => void onLogout()}
                className="text-sm text-slate-500 transition-colors hover:text-slate-800"
              >
                {t('nav.logout')}
              </button>
            </>
          ) : (
            <Link
              to="/login"
              className="rounded-lg bg-primary-600 px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-700"
            >
              {t('nav.login')}
            </Link>
          )}
        </div>
      </div>
    </header>
  )
}
