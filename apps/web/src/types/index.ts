// 与后端 API 契约对应（apps/api）
export interface User {
  id: string
  email: string
  display_name: string
  avatar_url: string | null
}

export interface RegisterResult {
  needs_verification: boolean
  email: string
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

// --- trips ---

export type TripStatus = 'draft' | 'planning' | 'ready' | 'archived'

export interface TripConstraints {
  max_drive_hours_per_day: number
  max_walk_km_per_day: number
  earliest_start: string
  latest_end: string
  allow_hotel_change: boolean
  budget_range: [number, number]
}

export interface TripPreference {
  budget: number | null
  transport_mode: string | null
  travel_style: string | null
  constraints: TripConstraints
  preferences: string[]
  natural_language: string | null
}

export interface Trip {
  id: string
  title: string
  destination: string | null
  start_date: string | null
  end_date: string | null
  travelers_count: number | null
  status: TripStatus
  created_at: string
  updated_at: string
  role?: string
  preference?: TripPreference
}

export interface CreateTripPayload {
  title?: string
  destination?: string | null
  start_date?: string | null
  end_date?: string | null
  travelers_count?: number
  status?: TripStatus
  budget?: number | null
  transport_mode?: string | null
  travel_style?: string | null
  constraints?: TripConstraints
  preferences?: string[]
  natural_language?: string | null
}

// --- planner (days / activities) ---

export type ActivityType =
  | 'attraction'
  | 'restaurant'
  | 'cafe'
  | 'hotel'
  | 'transport'
  | 'free_time'
  | 'other'

export type ActivityStatus = 'planned' | 'done' | 'skipped'

export interface Activity {
  id: string
  day_id: string
  type: ActivityType
  title: string
  start_time: string | null
  end_time: string | null
  sort_order: number
  notes: string | null
  status: ActivityStatus
  created_at: string
  updated_at: string
}

export interface Day {
  id: string
  trip_id: string
  day_number: number
  date: string | null
  title: string | null
  created_at: string
  updated_at: string
  activities: Activity[]
}
