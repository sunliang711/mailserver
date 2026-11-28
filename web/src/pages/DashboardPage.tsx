import React, { useEffect, useState } from 'react'
import { Activity, ArrowRight, CheckCircle2, KeyRound, Mail, RefreshCw, Send, Settings } from 'lucide-react'
import { api, formatDuration, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { EmptyState, LoadingState } from '../components/Feedback'
import type { NavTab } from '../components/Layout'
import { useI18n } from '../contexts/I18nContext'
import type { DashboardSummary } from '../types'

export const DashboardPage: React.FC<{ onNavigate: (tab: NavTab) => void; readOnly?: boolean }> = ({ onNavigate, readOnly = false }) => {
  const { t, lang } = useI18n()
  const [data, setData] = useState<DashboardSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      setData(await api.dashboard())
    } catch (e: any) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  if (loading && !data) return <LoadingState label={t('dashboard.loading')} />
  if (error && !data) {
    return (
      <div role="alert" className="p-4 bg-rose-50 dark:bg-rose-500/15 text-rose-700 dark:text-rose-300 border border-rose-200 dark:border-rose-500/30 rounded-lg text-sm">
        {error}
        <button onClick={load} className="ml-4 underline">{t('common.reload')}</button>
      </div>
    )
  }
  if (!data) return null

  const cards = [
    {
      icon: <Send className="w-5 h-5" />,
      label: t('dashboard.card.todaySent'),
      value: data.today_sent,
      sub: t('dashboard.card.todaySub', { succeeded: data.today_succeeded, failed: data.today_failed }),
    },
    {
      icon: <Mail className="w-5 h-5" />,
      label: t('dashboard.card.totalSent'),
      value: data.total_sent,
      sub: t('dashboard.card.totalSub', { succeeded: data.succeeded_count, failed: data.failed_count }),
    },
    {
      icon: <KeyRound className="w-5 h-5" />,
      label: t('dashboard.card.clients'),
      value: data.enabled_clients,
      sub: t('dashboard.card.clientsSub', { total: data.total_clients }),
    },
    {
      icon: <CheckCircle2 className="w-5 h-5" />,
      label: t('dashboard.card.smtp'),
      value: data.smtp_configured ? (
        <StatusBadge value={data.smtp_test_status || 'untested'} />
      ) : (
        <span className="text-base text-amber-600 dark:text-amber-400">{t('dashboard.card.smtpUnconfigured')}</span>
      ),
      sub: data.smtp_configured ? `${data.smtp_host}:${data.smtp_port}` : t('dashboard.card.smtpSubUnconfigured'),
    },
  ]

  const maxDay = Math.max(1, ...data.daily_series.map((d) => d.succeeded + d.failed))

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('dashboard.title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('dashboard.subtitle')}</p>
        </div>
        <button
          onClick={load}
          disabled={loading}
          className="inline-flex items-center gap-1.5 px-3 py-1.5 bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 text-slate-700 dark:text-slate-200 rounded-lg text-sm hover:bg-slate-50 dark:hover:bg-slate-700"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
          {t('common.refresh')}
        </button>
      </div>

      {error && (
        <div role="alert" className="p-3 rounded-xl bg-rose-50 dark:bg-rose-500/15 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300 text-sm">
          {error}
        </div>
      )}

      <div className="rounded-2xl border border-indigo-100 dark:border-indigo-500/25 bg-gradient-to-r from-indigo-50 via-white to-white dark:from-indigo-950/60 dark:via-slate-900 dark:to-slate-900 p-6 sm:p-7 flex flex-wrap items-center justify-between gap-5">
        <div className="flex gap-4 items-center">
          <div className="hidden sm:grid w-12 h-12 place-items-center rounded-2xl bg-white dark:bg-indigo-500/15 border border-indigo-100 dark:border-indigo-500/30 text-indigo-500 dark:text-indigo-300">
            <Activity className="w-6 h-6" />
          </div>
          <div>
            <div className="text-xs font-medium text-indigo-500 dark:text-indigo-400 mb-1.5">{t('dashboard.hero.eyebrow')}</div>
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
              {data.smtp_configured ? t('dashboard.hero.titleReady') : t('dashboard.hero.titleEmpty')}
            </h2>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-2">
              {data.smtp_configured ? t('dashboard.hero.descReady') : t('dashboard.hero.descEmpty')}
            </p>
          </div>
        </div>
        {(!readOnly || data.smtp_configured) && <button className="primary-button" onClick={() => onNavigate(data.smtp_configured ? 'mail-logs' : 'settings')}>
          {data.smtp_configured ? <Send className="w-4 h-4" /> : <Settings className="w-4 h-4" />}
          {data.smtp_configured ? t('dashboard.hero.btnReady') : t('dashboard.hero.btnEmpty')}
          <ArrowRight className="w-3.5 h-3.5" />
        </button>}
      </div>

      <div className="grid grid-cols-1 min-[380px]:grid-cols-2 lg:grid-cols-4 gap-4">
        {cards.map((c) => (
          <div key={c.label} className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5">
            <div className="flex items-center justify-between gap-2 text-xs text-slate-500 dark:text-slate-400">
              {c.label}
              <span className="text-indigo-400 bg-indigo-50 dark:bg-indigo-500/15 p-2 rounded-lg">{c.icon}</span>
            </div>
            <div className="text-2xl font-semibold tracking-tight text-slate-900 dark:text-slate-100 mt-2 tabular-nums">{c.value}</div>
            <div className="text-xs text-slate-400 dark:text-slate-400 mt-1 truncate" title={c.sub}>{c.sub}</div>
          </div>
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('dashboard.series.title')}</h3>
            <div className="flex items-center gap-3 text-xs text-slate-500 dark:text-slate-400">
              <span className="inline-flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm bg-emerald-500" />{t('dashboard.series.succeeded')}</span>
              <span className="inline-flex items-center gap-1.5"><span className="w-2.5 h-2.5 rounded-sm bg-rose-500" />{t('dashboard.series.failed')}</span>
            </div>
          </div>
          <div className="grid grid-cols-7 gap-3 items-end h-40 pt-4">
            {data.daily_series.map((d) => {
              const total = d.succeeded + d.failed
              const okPct = Math.round((d.succeeded / maxDay) * 100)
              const failPct = Math.round((d.failed / maxDay) * 100)
              return (
                <div key={d.date} className="flex flex-col items-center gap-1.5 h-full justify-end">
                  <div className="text-[11px] font-mono text-slate-500 dark:text-slate-400">{total}</div>
                  <div className="w-full max-w-[36px] flex-1 flex flex-col justify-end gap-0.5 bg-slate-100 dark:bg-slate-800/60 rounded-lg p-1 overflow-hidden">
                    {d.failed > 0 && (
                      <div className="w-full bg-rose-500 rounded-sm" style={{ height: `${Math.max(4, failPct)}%` }} />
                    )}
                    {d.succeeded > 0 && (
                      <div className="w-full bg-emerald-500 rounded-sm" style={{ height: `${Math.max(4, okPct)}%` }} />
                    )}
                  </div>
                  <div className="text-[11px] font-mono text-slate-400">{d.date.slice(5)}</div>
                </div>
              )
            })}
          </div>
        </div>

        <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-3">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('dashboard.cron.title')}</h3>
            <button onClick={() => onNavigate('cron')} className="text-xs text-indigo-600 dark:text-indigo-400 hover:underline">
              {t('dashboard.cron.manage')}
            </button>
          </div>
          <div className="space-y-2.5">
            {(data.cron_jobs || []).map((j) => (
              <div key={j.id} className="flex items-center justify-between gap-2 text-xs py-1.5 border-b last:border-0 border-slate-100 dark:border-slate-800">
                <div className="min-w-0">
                  <div className="font-medium text-slate-800 dark:text-slate-200 truncate">{j.name}</div>
                  <div className="text-[11px] font-mono text-slate-400">{j.cron_expr}</div>
                </div>
                <div className="flex items-center gap-1.5 shrink-0">
                  <StatusBadge value={j.enabled ? j.last_status : 'disabled'} />
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
        <div className="px-5 py-4 border-b border-slate-100 dark:border-slate-800 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('dashboard.recent.title')}</h3>
          <button onClick={() => onNavigate('mail-logs')} className="text-xs text-indigo-600 dark:text-indigo-400 hover:underline">
            {t('dashboard.recent.viewAll')}
          </button>
        </div>
        {data.recent_mails.length === 0 ? (
          <EmptyState title={t('dashboard.recent.empty')} />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead className="text-xs text-slate-500 dark:text-slate-400 border-b border-slate-100 dark:border-slate-800">
                <tr>
                  <th className="px-4 py-2.5">{t('mailLogs.colTime')}</th>
                  <th className="px-4 py-2.5">{t('mailLogs.colClient')}</th>
                  <th className="px-4 py-2.5">{t('mailLogs.colRecipient')}</th>
                  <th className="px-4 py-2.5">{t('mailLogs.colSubject')}</th>
                  <th className="px-4 py-2.5">{t('mailLogs.colStatus')}</th>
                  <th className="px-4 py-2.5">{t('mailLogs.colDuration')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
                {data.recent_mails.map((m) => (
                  <tr key={m.id}>
                    <td className="px-4 py-2.5 whitespace-nowrap text-slate-500">{formatTime(m.created_at, lang)}</td>
                    <td className="px-4 py-2.5 whitespace-nowrap font-medium text-slate-800 dark:text-slate-200">{m.client_name || '—'}</td>
                    <td className="px-4 py-2.5 font-mono">{m.recipient}</td>
                    <td className="px-4 py-2.5 max-w-[260px] truncate">{m.subject}</td>
                    <td className="px-4 py-2.5"><StatusBadge value={m.status} /></td>
                    <td className="px-4 py-2.5 font-mono text-slate-500">{formatDuration(m.duration_ms)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
