import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Link, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from './AuthProvider'
import { authErrorMessage } from './errorMessage'
import { authApi, ApiError } from '../../services/api'

const CODE_LENGTH = 6
// 与后端 verifyResendCool 一致
const RESEND_COOLDOWN = 60

interface LocationState {
  email?: string
}

export default function VerifyEmailPage() {
  const { t } = useTranslation()
  const { verifyEmail } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const email = (location.state as LocationState | null)?.email ?? ''

  const [code, setCode] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [resending, setResending] = useState(false)
  // 进入页面时刚发过一封（注册/登录跳转），直接进入冷却
  const [cooldown, setCooldown] = useState(RESEND_COOLDOWN)
  const submittingRef = useRef(false)

  useEffect(() => {
    if (cooldown <= 0) return
    const id = setInterval(() => setCooldown((c) => Math.max(0, c - 1)), 1000)
    return () => clearInterval(id)
  }, [cooldown])

  if (!email) {
    return <Navigate to="/register" replace />
  }

  const submit = async (value: string) => {
    if (submittingRef.current || value.length !== CODE_LENGTH) return
    submittingRef.current = true
    setSubmitting(true)
    setError('')
    setNotice('')
    try {
      await verifyEmail(email, value)
      navigate('/', { replace: true })
    } catch (err) {
      if (err instanceof ApiError && err.code === 'ALREADY_VERIFIED') {
        navigate('/login', { replace: true })
        return
      }
      setError(authErrorMessage(err))
      setCode('')
    } finally {
      submittingRef.current = false
      setSubmitting(false)
    }
  }

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    void submit(code)
  }

  const onCodeChange = (value: string) => {
    const digits = value.replace(/\D/g, '').slice(0, CODE_LENGTH)
    setCode(digits)
    if (digits.length === CODE_LENGTH) void submit(digits)
  }

  const onResend = async () => {
    if (cooldown > 0 || resending) return
    setResending(true)
    setError('')
    setNotice('')
    try {
      await authApi.resendVerification(email)
      setNotice(t('auth.resent'))
      setCooldown(RESEND_COOLDOWN)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'RESEND_TOO_SOON') {
        setCooldown(RESEND_COOLDOWN)
      }
      setError(authErrorMessage(err))
    } finally {
      setResending(false)
    }
  }

  return (
    <div className="flex min-h-[calc(100vh-4rem)] items-center justify-center px-4 py-12">
      <div className="w-full max-w-md rounded-2xl bg-white p-10 shadow-sm">
        <div className="flex flex-col items-center">
          <div className="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-50">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="h-8 w-8 text-primary-600" aria-hidden="true">
              <rect x="2" y="4" width="20" height="16" rx="2" />
              <path d="m22 7-10 6L2 7" />
            </svg>
          </div>
          <h1 className="mt-5 text-2xl font-bold text-slate-900">{t('auth.verifyTitle')}</h1>
          <p className="mt-2 text-center text-sm text-slate-500">
            {t('auth.verifySubtitle', { email })}
          </p>
        </div>

        <form onSubmit={onSubmit} className="mt-8 space-y-5" noValidate>
          <div>
            <label htmlFor="code" className="mb-1.5 block text-sm font-semibold text-slate-700">
              {t('auth.code')}
            </label>
            <input
              id="code"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              autoFocus
              required
              maxLength={CODE_LENGTH}
              value={code}
              onChange={(e) => onCodeChange(e.target.value)}
              placeholder={t('auth.codePlaceholder')}
              className="w-full rounded-lg border-0 bg-slate-100 px-4 py-3 text-center text-lg font-semibold tracking-[0.5em] text-slate-900 placeholder:text-sm placeholder:font-normal placeholder:tracking-normal placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
            />
            <p className="mt-1.5 text-xs text-slate-400">{t('auth.codeHint')}</p>
          </div>

          {error && (
            <p role="alert" className="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600">
              {error}
            </p>
          )}
          {notice && (
            <p role="status" className="rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-600">
              {notice}
            </p>
          )}

          <button
            type="submit"
            disabled={submitting || code.length !== CODE_LENGTH}
            className="w-full rounded-lg bg-primary-600 py-3 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {submitting ? t('auth.verifying') : t('auth.verify')}
          </button>
        </form>

        <p className="mt-6 text-center text-sm text-slate-500">
          {t('auth.notReceived')}{' '}
          {cooldown > 0 ? (
            <span className="text-slate-400">
              {t('auth.resendIn', { s: cooldown })}
            </span>
          ) : (
            <button
              type="button"
              onClick={() => void onResend()}
              disabled={resending}
              className="font-semibold text-primary-600 hover:text-primary-700 disabled:opacity-60"
            >
              {resending ? t('auth.resending') : t('auth.resend')}
            </button>
          )}
        </p>

        <p className="mt-4 text-center text-sm text-slate-500">
          <Link to="/login" className="font-semibold text-primary-600 hover:text-primary-700">
            {t('auth.backToLogin')}
          </Link>
        </p>
      </div>
    </div>
  )
}