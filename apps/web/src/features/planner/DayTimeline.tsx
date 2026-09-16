import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { Activity, ActivityType, Day } from '../../types'
import { plannerApi } from './api'
import ActivityEditor from './ActivityEditor'

const TYPE_COLOR: Record<ActivityType, string> = {
  attraction: 'text-primary-600 bg-primary-50',
  restaurant: 'text-amber-600 bg-amber-50',
  cafe: 'text-orange-500 bg-orange-50',
  hotel: 'text-violet-600 bg-violet-50',
  transport: 'text-slate-600 bg-slate-100',
  free_time: 'text-emerald-600 bg-emerald-50',
  other: 'text-slate-500 bg-slate-50',
}

function TypeIcon({ type }: { type: ActivityType }) {
  const cls = 'h-4 w-4'
  switch (type) {
    case 'attraction':
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <path d="M12 21s7-5.5 7-11a7 7 0 10-14 0c0 5.5 7 11 7 11z" />
          <circle cx="12" cy="10" r="2.5" />
        </svg>
      )
    case 'restaurant':
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <path d="M5 3v7a2 2 0 002 2h0a2 2 0 002-2V3M7 12v9M17 3v18M17 3a3 3 0 013 3v4h-3" strokeLinecap="round" />
        </svg>
      )
    case 'cafe':
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <path d="M4 8h12v5a4 4 0 01-4 4H8a4 4 0 01-4-4V8zM16 9h2a2 2 0 010 4h-2M7 4v2M11 4v2" strokeLinecap="round" />
        </svg>
      )
    case 'hotel':
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <path d="M3 21V7l9-4 9 4v14M3 21h18M9 21v-6h6v6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      )
    case 'transport':
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <path d="M5 17h14M5 17a2 2 0 11-4 0M19 17a2 2 0 104 0M5 17l2-8h8l2 8M7 9V5h10v4" strokeLinecap="round" />
        </svg>
      )
    case 'free_time':
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <circle cx="12" cy="12" r="8" />
          <path d="M12 8v4l3 2" strokeLinecap="round" />
        </svg>
      )
    default:
      return (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className={cls}>
          <circle cx="12" cy="12" r="2" />
          <circle cx="5" cy="12" r="2" />
          <circle cx="19" cy="12" r="2" />
        </svg>
      )
  }
}

// DB 的 time 类型会带秒（"09:00:00"）,展示截到 HH:MM。
function shortTime(t?: string | null): string {
  return t ? t.slice(0, 5) : ''
}

function timeLabel(a: Activity): string {
  const s = shortTime(a.start_time)
  const e = shortTime(a.end_time)
  if (s && e) return `${s} – ${e}`
  return s
}

function GripIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="currentColor" className="h-4 w-4">
      <circle cx="9" cy="6" r="1.5" />
      <circle cx="15" cy="6" r="1.5" />
      <circle cx="9" cy="12" r="1.5" />
      <circle cx="15" cy="12" r="1.5" />
      <circle cx="9" cy="18" r="1.5" />
      <circle cx="15" cy="18" r="1.5" />
    </svg>
  )
}

interface CardProps {
  activity: Activity
  selected: boolean
  onSelect?: (id: string) => void
  onEdit: () => void
  onDelete: () => void
  /** dnd-kit 把手属性（attributes + listeners），仅把手可拖避免与点击编辑冲突 */
  handleProps?: Record<string, unknown>
}

function ActivityCard({ activity, selected, onSelect, onEdit, onDelete, handleProps }: CardProps) {
  const { t } = useTranslation()
  return (
    <div
      id={`activity-${activity.id}`}
      className={`group flex items-center gap-2 rounded-xl border bg-white p-3 shadow-sm transition-colors ${
        selected ? 'border-primary-500 ring-2 ring-primary-100' : 'border-slate-200'
      }`}
    >
      {/* 拖拽把手：仅把手可拖，避免与点击编辑冲突 */}
      <button
        type="button"
        {...handleProps}
        aria-label={t('planner.drag')}
        className="cursor-grab touch-none text-slate-300 opacity-0 transition-opacity hover:text-slate-500 focus:opacity-100 group-hover:opacity-100 active:cursor-grabbing"
      >
        <GripIcon />
      </button>

      <div
        className="min-w-0 flex-1 cursor-pointer"
        onClick={() => onSelect?.(activity.id)}
      >
        <div className="flex items-center gap-3">
          <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg ${TYPE_COLOR[activity.type]}`}>
            <TypeIcon type={activity.type} />
          </span>
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium text-slate-900">{activity.title}</p>
            <p className="text-xs text-slate-500">
              {timeLabel(activity)}
              {activity.location ? ` · ${activity.location.name}` : ''}
              {activity.notes ? ` · ${activity.notes}` : ''}
            </p>
          </div>
          <span className="shrink-0 text-[10px] uppercase tracking-wide text-slate-400">
            {t(`planner.activityType.${activity.type}`)}
          </span>
        </div>
      </div>

      {/* 操作按钮：hover 显示 */}
      <div className="flex shrink-0 gap-1 opacity-0 transition-opacity group-hover:opacity-100">
        <button
          type="button"
          onClick={onEdit}
          title={t('planner.edit')}
          className="rounded-md p-1.5 text-slate-400 hover:bg-slate-100 hover:text-primary-600"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4">
            <path d="M17 3a2.8 2.8 0 114 4L7.5 20.5 2 22l1.5-5.5L17 3z" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
        <button
          type="button"
          onClick={onDelete}
          title={t('planner.delete')}
          className="rounded-md p-1.5 text-slate-400 hover:bg-rose-50 hover:text-rose-600"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4">
            <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m3 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6h14zM10 11v6M14 11v6" strokeLinecap="round" strokeLinejoin="round" />
          </svg>
        </button>
      </div>
    </div>
  )
}

// SortableCard 包装 dnd-kit 的 useSortable，把手监听器挂在卡片内 aria-label 的按钮上。
function SortableCard({ children, id }: { children: (handleProps: Record<string, unknown>) => React.ReactNode; id: string }) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  return (
    <div
      ref={setNodeRef}
      style={{ transform: CSS.Transform.toString(transform), transition }}
      className={isDragging ? 'relative z-10 opacity-60' : undefined}
    >
      {children({ ...attributes, ...listeners })}
    </div>
  )
}

interface Props {
  tripId: string
  days: Day[]
  generating: boolean
  destination: string | null
  selectedActivityId?: string | null
  onSelectActivity?: (activityId: string) => void
  onDaysChange: (days: Day[]) => void
  onRefetch: () => void
}

export default function DayTimeline({
  tripId,
  days,
  generating,
  destination,
  selectedActivityId,
  onSelectActivity,
  onDaysChange,
  onRefetch,
}: Props) {
  const { t } = useTranslation()
  const [editingId, setEditingId] = useState<string | null>(null)
  const [addingForDay, setAddingForDay] = useState<string | null>(null)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
  )

  function handleDragEnd(event: DragEndEvent) {
    const { active, over } = event
    if (!over || active.id === over.id) return
    const day = days.find((d) => d.activities.some((a) => a.id === active.id))
    if (!day || !day.activities.some((a) => a.id === over.id)) return // 只允许天内排序
    const oldIdx = day.activities.findIndex((a) => a.id === active.id)
    const newIdx = day.activities.findIndex((a) => a.id === over.id)
    const reordered = arrayMove(day.activities, oldIdx, newIdx)
    onDaysChange(days.map((d) => (d.id === day.id ? { ...d, activities: reordered } : d)))
    plannerApi
      .reorderActivities(day.id, reordered.map((a) => a.id))
      .catch(() => onRefetch()) // 失败回滚
  }

  async function deleteActivity(dayId: string, activityId: string) {
    if (!window.confirm(t('planner.confirmDeleteActivity'))) return
    onDaysChange(
      days.map((d) =>
        d.id === dayId ? { ...d, activities: d.activities.filter((a) => a.id !== activityId) } : d,
      ),
    )
    try {
      await plannerApi.deleteActivity(activityId)
    } catch {
      onRefetch()
    }
  }

  async function addDay() {
    try {
      const day = await plannerApi.createDay(tripId, {})
      onDaysChange([...days, day])
    } catch {
      onRefetch()
    }
  }

  async function deleteDay(dayId: string) {
    if (!window.confirm(t('planner.confirmDeleteDay'))) return
    onDaysChange(days.filter((d) => d.id !== dayId))
    try {
      await plannerApi.deleteDay(tripId, dayId)
    } catch {
      onRefetch()
    }
  }

  if (days.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-slate-300 bg-white/60 px-6 py-16 text-center">
        {generating ? (
          <>
            <span className="mb-3 inline-block h-7 w-7 animate-spin rounded-full border-2 border-ai-600 border-t-transparent" />
            <p className="text-sm font-medium text-slate-700">{t('planner.generating')}</p>
            <p className="mt-1 text-xs text-slate-400">{t('planner.generatingHint')}</p>
          </>
        ) : (
          <p className="text-sm text-slate-400">{t('planner.empty')}</p>
        )}
      </div>
    )
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
      <div className="space-y-5">
        {days.map((day, idx) => (
          <section key={day.id} className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm">
            <div className="group flex items-center gap-3 border-b border-slate-100 bg-slate-50/60 px-4 py-3">
              <span className="flex h-7 w-7 items-center justify-center rounded-full bg-primary-600 text-xs font-bold text-white">
                {idx + 1}
              </span>
              <div className="flex-1">
                <p className="text-sm font-semibold text-slate-900">
                  {t('planner.dayLabel', { n: idx + 1 })}
                  {day.title ? ` · ${day.title}` : ''}
                </p>
                {day.date && <p className="text-xs text-slate-500">{day.date}</p>}
              </div>
              <button
                type="button"
                onClick={() => deleteDay(day.id)}
                title={t('planner.deleteDay')}
                className="rounded-md p-1.5 text-slate-400 opacity-0 transition-opacity hover:bg-rose-50 hover:text-rose-600 group-hover:opacity-100"
              >
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-4 w-4">
                  <path d="M3 6h18M8 6V4a2 2 0 012-2h4a2 2 0 012 2v2m3 0v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6h14z" strokeLinecap="round" strokeLinejoin="round" />
                </svg>
              </button>
            </div>

            <SortableContext items={day.activities.map((a) => a.id)} strategy={verticalListSortingStrategy}>
              <div className="space-y-2 p-3">
                {day.activities.length === 0 && addingForDay !== day.id ? (
                  <p className="px-2 py-3 text-center text-xs text-slate-400">{t('planner.noActivities')}</p>
                ) : (
                  day.activities.map((a) =>
                    editingId === a.id ? (
                      <ActivityEditor
                        key={a.id}
                        dayId={day.id}
                        destination={destination}
                        activity={a}
                        onSaved={() => { setEditingId(null); onRefetch() }}
                        onCancel={() => setEditingId(null)}
                      />
                    ) : (
                      <SortableCard key={a.id} id={a.id}>
                        {(handleProps) => (
                          <ActivityCard
                            activity={a}
                            selected={a.id === selectedActivityId}
                            onSelect={onSelectActivity}
                            onEdit={() => setEditingId(a.id)}
                            onDelete={() => deleteActivity(day.id, a.id)}
                            handleProps={handleProps}
                          />
                        )}
                      </SortableCard>
                    ),
                  )
                )}

                {/* 添加活动 */}
                {addingForDay === day.id ? (
                  <ActivityEditor
                    dayId={day.id}
                    destination={destination}
                    onSaved={() => { setAddingForDay(null); onRefetch() }}
                    onCancel={() => setAddingForDay(null)}
                  />
                ) : (
                  <button
                    type="button"
                    onClick={() => setAddingForDay(day.id)}
                    className="w-full rounded-xl border border-dashed border-slate-300 px-3 py-2 text-xs font-medium text-slate-500 transition-colors hover:border-primary-400 hover:text-primary-600"
                  >
                    + {t('planner.addActivity')}
                  </button>
                )}
              </div>
            </SortableContext>
          </section>
        ))}

        <button
          type="button"
          onClick={addDay}
          className="w-full rounded-2xl border border-dashed border-slate-300 bg-white/60 px-4 py-3 text-sm font-medium text-slate-500 transition-colors hover:border-primary-400 hover:text-primary-600"
        >
          + {t('planner.addDay')}
        </button>
      </div>
    </DndContext>
  )
}
