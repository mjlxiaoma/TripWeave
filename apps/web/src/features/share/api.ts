import { api } from '../../services/api'
import type { Day, Trip } from '../../types'

// ShareData 是公开视图返回的精简 trip（无 owner/status）+ 完整行程。
export interface ShareData {
  trip: Pick<Trip, 'id' | 'title' | 'destination' | 'start_date' | 'end_date' | 'travelers_count'>
  days: Day[]
}

export const shareApi = {
  // 公开：无需登录
  get: (token: string) => api<ShareData>(`/share/${token}`),
  // 需登录（owner/editor）
  create: (tripId: string) =>
    api<{ token: string }>(`/trips/${tripId}/share`, { method: 'POST' }),
  // 需登录（owner）
  revoke: (tripId: string) =>
    api<{ revoked: boolean }>(`/trips/${tripId}/share`, { method: 'DELETE' }),
}
