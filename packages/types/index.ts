// TripWeave 前后端共享数据契约（与设计文档数据库实体对应）
// 注意：Go 后端仍有自己的 struct 与校验，TS 类型不替代后端模型。

export type TripStatus = 'draft' | 'planning' | 'ready' | 'archived' | 'deleted'

export interface Trip {
  id: string
  ownerId: string
  title: string
  startDate: string
  endDate: string
  status: TripStatus
  createdAt: string
  updatedAt: string
}

export interface TripDay {
  id: string
  tripId: string
  dayNumber: number
  date: string
  title: string
  createdAt: string
  updatedAt: string
}

export type ActivityType =
  | 'attraction'
  | 'restaurant'
  | 'cafe'
  | 'hotel'
  | 'transport'
  | 'free_time'
  | 'other'

export interface Activity {
  id: string
  tripDayId: string
  type: ActivityType
  title: string
  locationId: string | null
  startTime: string | null
  endTime: string | null
  sortOrder: number
  notes: string
  status: string
  createdAt: string
  updatedAt: string
}

export interface Location {
  id: string
  provider: string
  providerPlaceId: string
  name: string
  latitude: number
  longitude: number
  address: string
  city: string
  country: string
  timezone: string
  metadata: Record<string, unknown>
  createdAt: string
  updatedAt: string
}
