import React, { useEffect, useRef, useState } from 'react'
import { Download, Search } from 'lucide-react'
import { api, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { EmptyState, LoadingState } from '../components/Feedback'
import { Modal } from '../components/Modal'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import type { AuditLog } from '../types'

const ACTIONS = [
  'mail.send',
  'mail.web_test',
  'mail.auth_failed',
  'client.create',
  'client.update',
  'client.delete',
  'client.rotate_key',
  'client.reveal_secrets',
  'cron.execute',
  'cron.update',
  'cron.reveal_secrets',
  'settings.update',
  'settings.smtp_update',
  'settings.smtp_test',
  'migration.export',
  'migration.import',
  'system.setup_init',
  'admin.login',
  'admin.change_password',
  'admin.create',
  'admin.update',
  'auth.logout',
  'auth.session_revoke',
]

export const AuditPage: React.FC = () => {
  const toast = useToast()
  const { t, lang } = useI18n()
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [loading, setLoading] = useState(true)
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const limit = 50

  const [actorType, setActorType] = useState('')
  const [status, setStatus] = useState('')
  const [action, setAction] = useState('')
  const [q, setQ] = useState('')
  const [detail, setDetail] = useState<AuditLog | null>(null)
  const reqSeq = useRef(0)

  const buildParams = (includePage: boolean, targetOffset = offset) => {
    const p: Record<string, string> = {}
    if (includePage) {
      p.limit = String(limit)
      p.offset = String(targetOffset)
    }
    if (actorType) p.actor_type = actorType
    if (status) p.status = status
    if (action) p.action = action
    if (q.trim()) p.q = q.trim()
    return p
  }

  const load = async (targetOffset = offset) => {
    const seq = ++reqSeq.current
    setLoading(true)
    try {
      const res = await api.listAuditLogs(buildParams(true, targetOffset))
      if (reqSeq.current !== seq) return
      setLogs(res.items || [])
      setTotal(res.total || 0)
    } catch (e: any) {
      if (reqSeq.current === seq) toast.error(e.message)
    } finally {
      if (reqSeq.current === seq) setLoading(false)
    }
  }

  useEffect(() => {
    load(offset)
  }, [offset, actorType, status, action])

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    if (offset !== 0) setOffset(0)
    else load(0)
  }

  const exportParams = buildParams(false)

  const handleExport = async (format: 'csv' | 'json') => {
    try {
      await api.exportAudit(exportParams, format)
    } catch (e: any) {
      toast.error(e.message)
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('audit.title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('audit.subtitle')}</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            onClick={() => handleExport('csv')}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700"
          >
            <Download className="w-3.5 h-3.5" /> {t('common.exportCSV')}
          </button>
          <button
            type="button"
            onClick={() => handleExport('json')}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700"
          >
            <Download className="w-3.5 h-3.5" /> {t('common.exportJSON')}
          </button>
        </div>
      </div>

      <form onSubmit={handleSearch} className="filter-bar flex flex-wrap gap-2.5 bg-white dark:bg-slate-900 p-3.5 rounded-xl border border-slate-200 dark:border-slate-800">
        <div className="flex items-center gap-2 flex-1 min-w-[220px] px-3 py-1.5 border rounded-lg">
          <Search className="w-4 h-4 text-slate-400 shrink-0" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t('audit.searchPlaceholder')}
            className="w-full text-xs bg-transparent border-0 focus:outline-none focus:shadow-none"
          />
        </div>
        <select value={actorType} onChange={(e) => { setOffset(0); setActorType(e.target.value) }} className="px-3 py-1.5 border rounded-lg text-xs bg-white dark:bg-slate-900">
          <option value="">{t('audit.allActors')}</option>
          <option value="admin">{t('audit.actorAdmin')}</option>
          <option value="client">{t('audit.actorClient')}</option>
          <option value="system">{t('audit.actorSystem')}</option>
        </select>
        <select value={action} onChange={(e) => { setOffset(0); setAction(e.target.value) }} className="px-3 py-1.5 border rounded-lg text-xs bg-white dark:bg-slate-900">
          <option value="">{t('audit.allActions')}</option>
          {ACTIONS.map((a) => <option key={a} value={a}>{a}</option>)}
        </select>
        <select value={status} onChange={(e) => { setOffset(0); setStatus(e.target.value) }} className="px-3 py-1.5 border rounded-lg text-xs bg-white dark:bg-slate-900">
          <option value="">{t('audit.allStatus')}</option>
          <option value="success">{t('status.success')}</option>
          <option value="failure">{t('status.failure')}</option>
        </select>
        <button type="submit" className="px-3.5 py-1.5 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white">
          {t('common.search')}
        </button>
      </form>

      <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 overflow-x-auto">
        <table className="audit-table w-full text-sm">
          <thead className="text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200/80 dark:border-slate-800">
            <tr>
              <th className="px-4 py-3">{t('audit.colTime')}</th>
              <th className="px-4 py-3">{t('audit.colActor')}</th>
              <th className="px-4 py-3">{t('audit.colAction')}</th>
              <th className="px-4 py-3">{t('audit.colResource')}</th>
              <th className="px-4 py-3">{t('audit.colStatus')}</th>
              <th className="px-4 py-3">{t('audit.colIP')}</th>
              <th className="px-4 py-3 text-right">{t('audit.colDetail')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
            {loading && <tr><td colSpan={7}><LoadingState /></td></tr>}
            {!loading && logs.length === 0 && <tr><td colSpan={7}><EmptyState title={t('audit.emptyTitle')} /></td></tr>}
            {!loading && logs.map((l) => (
              <tr key={l.id}>
                <td className="px-4 py-3 text-slate-500">{formatTime(l.created_at, lang)}</td>
                <td className="px-4 py-3">
                  <span className="text-slate-400 mr-1">[{l.actor_type}]</span>
                  <span className="font-medium text-slate-800 dark:text-slate-200">{l.actor_name || '—'}</span>
                </td>
                <td className="px-4 py-3 font-mono">{l.action}</td>
                <td className="px-4 py-3 font-mono text-slate-500">{l.resource_type}{l.resource_id ? `:${l.resource_id.slice(0, 10)}` : ''}</td>
                <td className="px-4 py-3"><StatusBadge value={l.status} /></td>
                <td className="px-4 py-3 font-mono text-slate-500">{l.ip_address || '—'}</td>
                <td className="px-4 py-3 text-right">
                  <button
                    type="button"
                    onClick={() => setDetail(l)}
                    className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800"
                  >
                    {t('common.viewDetail')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {total > limit && (
        <div className="flex items-center justify-between text-xs text-slate-500 dark:text-slate-400">
          <span>{t('common.total', { total })}</span>
          <div className="flex items-center gap-2">
            <button type="button" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - limit))} className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 disabled:opacity-40">
              {t('common.prevPage')}
            </button>
            <button type="button" disabled={offset + limit >= total} onClick={() => setOffset(offset + limit)} className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 disabled:opacity-40">
              {t('common.nextPage')}
            </button>
          </div>
        </div>
      )}

      {detail && (
        <Modal title={t('audit.detailTitle', { id: detail.id })} onClose={() => setDetail(null)}>
          <div className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-3 text-xs">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('audit.detailTitle', { id: detail.id })}</h2>
            <div className="grid grid-cols-2 gap-3">
              <div><span className="text-slate-400 block">{t('audit.colTime')}</span><span className="font-mono">{formatTime(detail.created_at, lang)}</span></div>
              <div><span className="text-slate-400 block">{t('audit.colAction')}</span><span className="font-mono">{detail.action}</span></div>
              <div><span className="text-slate-400 block">{t('audit.colActor')}</span><span>[{detail.actor_type}] {detail.actor_name || detail.actor_id || '—'}</span></div>
              <div><span className="text-slate-400 block">{t('audit.colIP')}</span><span className="font-mono">{detail.ip_address || '—'}</span></div>
            </div>
            <pre className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 font-mono whitespace-pre-wrap break-all max-h-72 overflow-y-auto">
              {(() => {
                try {
                  return JSON.stringify(JSON.parse(detail.detail_json || '{}'), null, 2)
                } catch {
                  return detail.detail_json
                }
              })()}
            </pre>
          </div>
        </Modal>
      )}
    </div>
  )
}
