import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '../auth/AuthProvider'
import { tripsApi } from '../../services/api'
import { DESTINATIONS, TEMPLATES, type TripTemplate } from './data'

// ExplorePage：目的地灵感 + 行程模板一键生成。
export default function ExplorePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { user } = useAuth()
  const [creatingId, setCreatingId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // 目的地卡：跳新建旅行页并预填目的地
  function pickDestination(name: string) {
    navigate(`/trip/new?destination=${encodeURIComponent(name)}`)
  }

  // 模板卡：一键创建 + autostart 让 AI 直接生成行程
  async function applyTemplate(tpl: TripTemplate) {
    if (creatingId) return
    if (!user) {
      navigate('/login')
      return
    }
    setCreatingId(tpl.id)
    setError(null)
    try {
      const trip = await tripsApi.create({
        title: tpl.title,
        destination: tpl.destination,
        status: 'planning',
        natural_language: tpl.prompt,
      })
      navigate(`/trip/${trip.id}?autostart=1`, { replace: true })
    } catch {
      setError(t('explore.createFailed'))
      setCreatingId(null)
    }
  }

  return (
    <div className="mx-auto max-w-6xl px-4 pb-20 pt-10">
      {/* Hero */}
      <div className="mb-10 text-center">
        <h1 className="text-3xl font-bold text-slate-900">{t('explore.title')}</h1>
        <p className="mt-2 text-sm text-slate-500">{t('explore.subtitle')}</p>
      </div>

      {/* 热门目的地 */}
      <section className="mb-12">
        <h2 className="mb-4 text-lg font-semibold text-slate-900">{t('explore.destinations')}</h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {DESTINATIONS.map((d) => (
            <button
              key={d.name}
              type="button"
              onClick={() => pickDestination(d.name)}
              className="group flex items-center gap-3 rounded-2xl border border-slate-200 bg-white p-4 text-left shadow-sm transition-all hover:-translate-y-0.5 hover:border-primary-300 hover:shadow-md"
            >
              <span className="text-3xl">{d.emoji}</span>
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold text-slate-900 group-hover:text-primary-700">
                  {d.name} <span className="text-xs font-normal text-slate-400">{d.nameEn}</span>
                </p>
                <p className="truncate text-xs text-slate-500">{d.tagline}</p>
              </div>
            </button>
          ))}
        </div>
      </section>

      {/* 行程模板 */}
      <section>
        <div className="mb-4 flex items-baseline justify-between">
          <h2 className="text-lg font-semibold text-slate-900">{t('explore.templates')}</h2>
          <p className="text-xs text-slate-400">{t('explore.templatesHint')}</p>
        </div>
        {error && (
          <p className="mb-3 rounded-xl bg-rose-50 px-4 py-2.5 text-sm text-rose-700">{error}</p>
        )}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {TEMPLATES.map((tpl) => (
            <div
              key={tpl.id}
              className="group flex flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm transition-shadow hover:shadow-md"
            >
              {/* 封面：渐变 + 大 emoji */}
              <div className="flex h-28 items-center justify-center bg-gradient-to-br from-primary-50 via-slate-50 to-ai-100 text-5xl">
                {tpl.emoji}
              </div>
              <div className="flex flex-1 flex-col p-4">
                <h3 className="text-base font-semibold text-slate-900">{tpl.title}</h3>
                <p className="mt-0.5 text-xs text-slate-500">
                  {tpl.destination} · {t('explore.days', { count: tpl.days })}
                </p>
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {tpl.tags.map((tag) => (
                    <span key={tag} className="rounded-full bg-slate-100 px-2 py-0.5 text-[11px] text-slate-600">
                      {tag}
                    </span>
                  ))}
                </div>
                <button
                  type="button"
                  onClick={() => applyTemplate(tpl)}
                  disabled={creatingId === tpl.id}
                  className="mt-4 w-full rounded-xl bg-primary-600 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-700 disabled:opacity-50"
                >
                  {creatingId === tpl.id ? t('explore.generating') : t('explore.useTemplate')}
                </button>
              </div>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}
