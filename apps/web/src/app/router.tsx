import { createBrowserRouter, Navigate, Outlet } from 'react-router-dom'
import type { ReactElement } from 'react'
import AppHeader from '../components/AppHeader'
import { useAuth } from '../features/auth/AuthProvider'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import VerifyEmailPage from '../features/auth/VerifyEmailPage'
import HomePage from '../pages/HomePage'
import TripsPage from '../pages/TripsPage'
import TripNewPage from '../pages/TripNewPage'
import PlannerPage from '../features/planner/PlannerPage'
import TodayPage from '../features/today/TodayPage'
import SharePage from '../features/share/SharePage'
import ExplorePage from '../features/explore/ExplorePage'

function FullPageLoading() {
  return (
    <div className="flex min-h-[calc(100vh-4rem)] items-center justify-center">
      <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary-600 border-t-transparent" />
    </div>
  )
}

function GuestOnly({ children }: { children: ReactElement }) {
  const { user, loading } = useAuth()
  if (loading) return <FullPageLoading />
  if (user) return <Navigate to="/" replace />
  return children
}

function RequireAuth({ children }: { children: ReactElement }) {
  const { user, loading } = useAuth()
  if (loading) return <FullPageLoading />
  if (!user) return <Navigate to="/login" replace />
  return children
}

function Layout() {
  return (
    <div className="min-h-screen bg-slate-50 font-sans text-slate-900">
      <AppHeader />
      <main>
        <Outlet />
      </main>
    </div>
  )
}

export const router = createBrowserRouter([
  {
    element: <Layout />,
    children: [
      { path: '/', element: <HomePage /> },
      { path: '/explore', element: <ExplorePage /> },
      {
        path: '/trips',
        element: (
          <RequireAuth>
            <TripsPage />
          </RequireAuth>
        ),
      },
      {
        path: '/trip/new',
        element: (
          <RequireAuth>
            <TripNewPage />
          </RequireAuth>
        ),
      },
      {
        path: '/trip/:tripId',
        element: (
          <RequireAuth>
            <PlannerPage />
          </RequireAuth>
        ),
      },
      {
        path: '/trip/:tripId/today',
        element: (
          <RequireAuth>
            <TodayPage />
          </RequireAuth>
        ),
      },
      // 公开分享视图：无需登录
      { path: '/share/:token', element: <SharePage /> },
      {
        path: '/login',
        element: (
          <GuestOnly>
            <LoginPage />
          </GuestOnly>
        ),
      },
      {
        path: '/register',
        element: (
          <GuestOnly>
            <RegisterPage />
          </GuestOnly>
        ),
      },
      {
        path: '/verify-email',
        element: (
          <GuestOnly>
            <VerifyEmailPage />
          </GuestOnly>
        ),
      },
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
