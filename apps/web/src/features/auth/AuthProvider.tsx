import { createContext, useContext, useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { authApi, clearTokens, getAccessToken, setTokens } from '../../services/api'
import type { User } from '../../types'

interface AuthContextValue {
  user: User | null
  loading: boolean
  login: (email: string, password: string) => Promise<void>
  register: (email: string, password: string, displayName: string) => Promise<void>
  verifyEmail: (email: string, code: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    async function bootstrap() {
      if (!getAccessToken()) {
        setLoading(false)
        return
      }
      try {
        const me = await authApi.me()
        if (!cancelled) setUser(me)
      } catch {
        if (!cancelled) clearTokens()
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void bootstrap()
    return () => {
      cancelled = true
    }
  }, [])

  const login = async (email: string, password: string) => {
    const t = await authApi.login(email, password)
    setTokens(t)
    setUser(t.user)
  }

  // 注册不直接登录：后端发送邮箱验证码，验证通过后才签发 token
  const register = async (email: string, password: string, displayName: string) => {
    await authApi.register(email, password, displayName)
  }

  const verifyEmail = async (email: string, code: string) => {
    const t = await authApi.verifyEmail(email, code)
    setTokens(t)
    setUser(t.user)
  }

  const logout = async () => {
    try {
      await authApi.logout()
    } catch {
      // 本地登出优先，忽略后端错误
    }
    clearTokens()
    setUser(null)
  }

  return (
    <AuthContext.Provider value={{ user, loading, login, register, verifyEmail, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
