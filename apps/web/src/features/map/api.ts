import { api } from '../../services/api'
import type { DayRoute, Location, RouteMode } from '../../types'

export const mapApi = {
  searchLocations: (q: string, city: string) =>
    api<Location[]>(
      `/locations/search?q=${encodeURIComponent(q)}&city=${encodeURIComponent(city)}`,
    ),
  getDayRoute: (dayId: string, mode: RouteMode) =>
    api<DayRoute>(`/days/${dayId}/route?mode=${mode}`),
  // 公开分享视图：路线走 token 端点（无需登录）
  getShareRoute: (token: string, dayId: string, mode: RouteMode) =>
    api<DayRoute>(`/share/${token}/days/${dayId}/route?mode=${mode}`),
}
