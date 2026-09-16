import { api, tripsApi } from '../../services/api'
import type { Activity, ActivityStatus, ActivityType, Day, Trip } from '../../types'

// ActivityPayload 与后端 activityRequest 对齐：全部可选，nil 不变更。
// clear_location=true 显式解绑地点（传 null 无法区分「不变更」与「清除」）。
export interface ActivityPayload {
  type?: ActivityType
  title?: string
  start_time?: string | null
  end_time?: string | null
  notes?: string | null
  status?: ActivityStatus
  location_id?: string | null
  clear_location?: boolean
}

export const plannerApi = {
  getTrip: (tripId: string) => tripsApi.get(tripId),
  fetchDays: (tripId: string) => tripsApi.getDays(tripId),
  updateTitle: (tripId: string, title: string) =>
    api<Trip>(`/trips/${tripId}`, { method: 'PATCH', body: JSON.stringify({ title }) }),

  // --- days ---
  createDay: (tripId: string, payload: { date?: string; title?: string }) =>
    api<Day>(`/trips/${tripId}/days`, { method: 'POST', body: JSON.stringify(payload) }),
  deleteDay: (tripId: string, dayId: string) =>
    api<{ deleted: boolean }>(`/trips/${tripId}/days/${dayId}`, { method: 'DELETE' }),

  // --- activities ---
  createActivity: (dayId: string, payload: ActivityPayload) =>
    api<Activity>(`/days/${dayId}/activities`, { method: 'POST', body: JSON.stringify(payload) }),
  updateActivity: (activityId: string, payload: ActivityPayload) =>
    api<Activity>(`/activities/${activityId}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteActivity: (activityId: string) =>
    api<{ deleted: boolean }>(`/activities/${activityId}`, { method: 'DELETE' }),
  reorderActivities: (dayId: string, activityIds: string[]) =>
    api<Activity[]>(`/days/${dayId}/activities/reorder`, {
      method: 'POST',
      body: JSON.stringify({ activity_ids: activityIds }),
    }),
}

export type { Activity, Day, Trip }
