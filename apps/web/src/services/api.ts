import type { AuthTokens, CreateTripPayload, Day, Envelope, RegisterResult, Trip, User } from '../types'

const BASE: string = import.meta.env.VITE_API_BASE ?? '/api/v1'

const ACCESS_KEY = 'tw_access_token'
const REFRESH_KEY = 'tw_refresh_token'

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_KEY)
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_KEY)
}

export function setTokens(t: AuthTokens): void {
  localStorage.setItem(ACCESS_KEY, t.access_token)
  localStorage.setItem(REFRESH_KEY, t.refresh_token)
}

export function clearTokens(): void {
  localStorage.removeItem(ACCESS_KEY)
  localStorage.removeItem(REFRESH_KEY)
}

async function rawRequest<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, {
      ...init,
      headers: { 'Content-Type': 'application/json', ...init?.headers },
    })
  } catch {
    throw new ApiError(0, 'NETWORK', 'network error')
  }
  let env: Envelope<T> | null = null
  try {
    env = (await res.json()) as Envelope<T>
  } catch {
    env = null
  }
  if (!res.ok || !env || env.error) {
    throw new ApiError(
      res.status,
      env?.error?.code ?? 'UNKNOWN',
      env?.error?.message ?? res.statusText,
    )
  }
  return env.data as T
}

// 单飞刷新：并发 401 只触发一次 refresh
let refreshing: Promise<boolean> | null = null
// refreshSession 导出给 SSE 流式请求复用（流式请求不走 api<T> 的 JSON 解包）。
export function refreshSession(): Promise<boolean> {
  return tryRefresh()
}
function tryRefresh(): Promise<boolean> {
  const rt = getRefreshToken()
  if (!rt) return Promise.resolve(false)
  refreshing ??= rawRequest<AuthTokens>('/auth/refresh', {
    method: 'POST',
    body: JSON.stringify({ refresh_token: rt }),
  })
    .then((t) => {
      setTokens(t)
      return true
    })
    .catch(() => {
      clearTokens()
      return false
    })
    .finally(() => {
      refreshing = null
    })
  return refreshing
}

export async function api<T>(path: string, init?: RequestInit, retry = true): Promise<T> {
  const token = getAccessToken()
  const authInit: RequestInit = token
    ? { ...init, headers: { Authorization: `Bearer ${token}`, ...init?.headers } }
    : (init ?? {})
  try {
    return await rawRequest<T>(path, authInit)
  } catch (err) {
    if (err instanceof ApiError && err.status === 401 && retry && !path.startsWith('/auth/')) {
      if (await tryRefresh()) return api<T>(path, init, false)
    }
    throw err
  }
}

export const authApi = {
  register: (email: string, password: string, displayName: string) =>
    api<RegisterResult>('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, display_name: displayName }),
    }),
  verifyEmail: (email: string, code: string) =>
    api<AuthTokens>('/auth/verify-email', {
      method: 'POST',
      body: JSON.stringify({ email, code }),
    }),
  resendVerification: (email: string) =>
    api<{ sent: boolean }>('/auth/resend-verification', {
      method: 'POST',
      body: JSON.stringify({ email }),
    }),
  login: (email: string, password: string) =>
    api<AuthTokens>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    }),
  logout: () =>
    api<Record<string, never>>('/auth/logout', {
      method: 'POST',
      body: JSON.stringify({ refresh_token: getRefreshToken() ?? '' }),
    }),
  me: () => api<User>('/me'),
}

// authedFetch: 二进制端点(封面)专用 —— 带鉴权与 401 单飞刷新,但不走 JSON envelope。
async function authedFetch(path: string, init?: RequestInit, retry = true): Promise<Response> {
  const token = getAccessToken()
  const headers: Record<string, string> = { ...((init?.headers as Record<string, string>) ?? {}) }
  if (token) headers.Authorization = `Bearer ${token}`
  const res = await fetch(`${BASE}${path}`, { ...init, headers })
  if (res.status === 401 && retry && !path.startsWith('/auth/')) {
    if (await tryRefresh()) return authedFetch(path, init, false)
  }
  return res
}

export const tripsApi = {
  list: () => api<Trip[]>('/trips'),
  get: (id: string) => api<Trip>('/trips/' + id),
  create: (payload: CreateTripPayload) =>
    api<Trip>('/trips', { method: 'POST', body: JSON.stringify(payload) }),
  update: (id: string, patch: CreateTripPayload) =>
    api<Trip>('/trips/' + id, { method: 'PATCH', body: JSON.stringify(patch) }),
  remove: (id: string) =>
    api<{ deleted: boolean }>('/trips/' + id, { method: 'DELETE' }),
  getDays: (id: string) => api<Day[]>('/trips/' + id + '/days'),

  // 封面:上传(二进制 PUT,响应为 JSON envelope)/ 拉取(二进制)/ 移除
  uploadCover: async (id: string, blob: Blob) => {
    const res = await authedFetch(`/trips/${id}/cover`, {
      method: 'PUT',
      body: blob,
      headers: { 'Content-Type': 'image/jpeg' },
    })
    if (!res.ok) throw new ApiError(res.status, 'COVER_UPLOAD', 'cover upload failed')
    const env = (await res.json()) as Envelope<{ has_cover: boolean; bytes: number }>
    return env.data
  },
  fetchCover: async (id: string): Promise<Blob | null> => {
    const res = await authedFetch(`/trips/${id}/cover`)
    if (res.status === 404) return null
    if (!res.ok) throw new ApiError(res.status, 'COVER_LOAD', 'cover load failed')
    return res.blob()
  },
  removeCover: async (id: string) => {
    const res = await authedFetch(`/trips/${id}/cover`, { method: 'DELETE' })
    if (!res.ok) throw new ApiError(res.status, 'COVER_DELETE', 'cover delete failed')
    const env = (await res.json()) as Envelope<{ has_cover: boolean }>
    return env.data
  },
}
