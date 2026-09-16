import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { Activity, ActivityType, Location } from '../../types'
import { mapApi } from '../map/api'
import { plannerApi, type ActivityPayload } from './api'

const TYPES: ActivityType[] = [
  'attraction',
  'restaurant',
  'cafe',
  'hotel',
  'transport',
  'free_time',
  'other',
]

interface Props {
  dayId: string
  destination: string | null
  /** 传入则为编辑模式，否则为新增模式 */
  activity?: Activity
  onSaved: () => void
  onCancel: () => void
}

const inputCls =
  'w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-primary-500'
const labelCls = 'mb-1 block text-xs font-medium text-slate-500'

export default function ActivityEditor({ dayId, destination, activity, onSaved, onCancel }: Props) {
  const { t } = useTranslation()
  const editing = Boolean(activity)
  const [title, setTitle] = useState(activity?.title ?? '')
  const [type, setType] = useState<ActivityType>(activity?.type ?? 'attraction')
  const [startTime, setStartTime] = useState(activity?.start_time ?? '')
  const [endTime, setEndTime] = useState(activity?.end_time ?? '')
  const [notes, setNotes] = useState(activity?.notes ?? '')
  const [location, setLocation] = useState<Location | null>(
    activity?.location ? { ...activity.location, city: null } : null,
  )
  const [clearLoc, setClearLoc] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // 地点搜索
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Location[]>([])
  const [searching, setSearching] = useState(false)
  const [open, setOpen] = useState(false)
  const debounceRef = useRef<number | undefined>(undefined)
  const boxRef = useRef<HTMLDivElement>(null)

  // 防抖 300ms 搜索
  useEffect(() => {
    if (!query.trim()) {
      setResults([])
      return
    }
    window.clearTimeout(debounceRef.current)
    debounceRef.current = window.setTimeout(() => {
      setSearching(true)
      mapApi
        .searchLocations(query.trim(), destination ?? '')
        .then((locs) => {
          setResults(locs)
          setOpen(true)
        })
        .catch(() => setResults([]))
        .finally(() => setSearching(false))
    }, 300)
    return () => window.clearTimeout(debounceRef.current)
  }, [query, destination])

  // 点击外部关闭下拉
  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (boxRef.current && !boxRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    return () => document.removeEventListener('mousedown', onDoc)
  }, [])

  async function save() {
    if (!title.trim()) {
      setError(t('planner.editor.titleRequired'))
      return
    }
    setSaving(true)
    setError(null)
    const payload: ActivityPayload = {
      title: title.trim(),
      type,
      start_time: startTime || undefined,
      end_time: endTime || undefined,
      notes: notes.trim() || undefined,
    }
    if (location) payload.location_id = location.id
    else if (clearLoc) payload.clear_location = true
    try {
      if (editing && activity) {
        await plannerApi.updateActivity(activity.id, payload)
      } else {
        await plannerApi.createActivity(dayId, payload)
      }
      onSaved()
    } catch {
      setError(t('planner.editor.saveError'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-3 rounded-xl border border-primary-200 bg-primary-50/40 p-3">
      <div>
        <label className={labelCls}>{t('planner.editor.title')}</label>
        <input className={inputCls} value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t('planner.editor.titlePh')} />
      </div>

      <div className="grid grid-cols-3 gap-2">
        <div>
          <label className={labelCls}>{t('planner.editor.type')}</label>
          <select className={inputCls} value={type} onChange={(e) => setType(e.target.value as ActivityType)}>
            {TYPES.map((ty) => (
              <option key={ty} value={ty}>
                {t(`planner.activityType.${ty}`)}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label className={labelCls}>{t('planner.editor.start')}</label>
          <input className={inputCls} type="time" value={startTime} onChange={(e) => setStartTime(e.target.value)} />
        </div>
        <div>
          <label className={labelCls}>{t('planner.editor.end')}</label>
          <input className={inputCls} type="time" value={endTime} onChange={(e) => setEndTime(e.target.value)} />
        </div>
      </div>

      {/* 地点：搜索重绑 */}
      <div ref={boxRef} className="relative">
        <label className={labelCls}>{t('planner.editor.location')}</label>
        {location && !clearLoc ? (
          <div className="flex items-center justify-between rounded-lg border border-slate-200 bg-white px-3 py-2">
            <span className="truncate text-sm text-slate-700">📍 {location.name}</span>
            <button type="button" onClick={() => { setLocation(null); setClearLoc(true) }} className="text-xs text-slate-400 hover:text-rose-600">
              {t('planner.editor.locationClear')}
            </button>
          </div>
        ) : (
          <input
            className={inputCls}
            value={query}
            onChange={(e) => { setQuery(e.target.value); setClearLoc(false) }}
            onFocus={() => results.length > 0 && setOpen(true)}
            placeholder={t('planner.editor.locationPh')}
          />
        )}
        {open && (searching || results.length > 0) && (
          <ul className="absolute z-20 mt-1 max-h-48 w-full overflow-y-auto rounded-lg border border-slate-200 bg-white shadow-lg">
            {searching && <li className="px-3 py-2 text-xs text-slate-400">{t('planner.editor.searching')}</li>}
            {!searching && results.map((l) => (
              <li key={l.id}>
                <button
                  type="button"
                  className="w-full px-3 py-2 text-left text-sm hover:bg-primary-50"
                  onClick={() => { setLocation(l); setQuery(''); setOpen(false) }}
                >
                  <span className="block truncate font-medium text-slate-800">{l.name}</span>
                  {l.address && <span className="block truncate text-xs text-slate-400">{l.address}</span>}
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <label className={labelCls}>{t('planner.editor.notes')}</label>
        <textarea className={inputCls} rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} placeholder={t('planner.editor.notesPh')} />
      </div>

      {error && <p className="text-xs text-rose-600">{error}</p>}

      <div className="flex justify-end gap-2">
        <button type="button" onClick={onCancel} className="rounded-lg px-3 py-1.5 text-sm text-slate-500 hover:bg-slate-100">
          {t('planner.editor.cancel')}
        </button>
        <button type="button" onClick={save} disabled={saving} className="rounded-lg bg-primary-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-primary-700 disabled:opacity-50">
          {saving ? t('planner.editor.saving') : t('planner.editor.save')}
        </button>
      </div>
    </div>
  )
}
