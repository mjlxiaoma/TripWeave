import { api } from '../../services/api'
import type { DayRoute, Location, RouteMode } from '../../types'

export const mapApi = {
  searchLocations: (q: string, city: string) =>
    api<Location[]>(
      `/locations/search?q=${encodeURIComponent(q)}&city=${encodeURIComponent(city)}`,
    ),
  getDayRoute: (dayId: string, mode: RouteMode) =>
    api<DayRoute>(`/days/${dayId}/route?mode=${mode}`),
}
