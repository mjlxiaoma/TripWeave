import { api, tripsApi } from '../../services/api'
import type { Day, Trip } from '../../types'

export const plannerApi = {
  getTrip: (tripId: string) => tripsApi.get(tripId),
  fetchDays: (tripId: string) => tripsApi.getDays(tripId),
  updateTitle: (tripId: string, title: string) =>
    api<Trip>(`/trips/${tripId}`, { method: 'PATCH', body: JSON.stringify({ title }) }),
}

export type { Day, Trip }
