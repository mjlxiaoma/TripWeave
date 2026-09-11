import { createBrowserRouter, Navigate, Outlet } from 'react-router-dom'
import type { ReactElement } from 'react'
import AppHeader from '../components/AppHeader'
import { useAuth } from '../features/auth/AuthProvider'
import LoginPage from '../features/auth/LoginPage'
import RegisterPage from '../features/auth/RegisterPage'
import HomePage from '../pages/HomePage'
import TripsPage from '../pages/TripsPage'
import PlaceholderPage from '../pages/PlaceholderPage'
import TripNewPage from '../pages/TripNewPage'

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
      { path: '/explore', element: <PlaceholderPage /> },
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
            <PlaceholderPage />
          </RequireAuth>
        ),
      },
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
      { path: '*', element: <Navigate to="/" replace /> },
    ],
  },
])
