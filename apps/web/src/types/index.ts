// 与后端 API 契约对应（apps/api）
export interface User {
  id: string
  email: string
  display_name: string
  avatar_url: string | null
}

export interface AuthTokens {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
  user: User
}

export interface ApiErrorDetail {
  code: string
  message: string
}

export interface Envelope<T> {
  data: T | null
  error: ApiErrorDetail | null
}
