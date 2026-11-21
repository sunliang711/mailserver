import React, { useEffect, useRef, useState } from 'react'
import { Download, Loader2, Mail, Search, Send } from 'lucide-react'
import { api, formatDuration, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { EmptyState, LoadingState } from '../components/Feedback'
import { Modal } from '../components/Modal'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import type { Client, MailLog } from '../types'

export const MailLogsPage: React.FC = () => {
  const toast = useToast()
  const { t, lang } = useI18n()
  const [logs, setLogs] = useState<MailLog[]>([])
  const [clients, setClients] = useState<Client[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [offset, setOffset] = useState(0)
  const limit = 30

  const [status, setStatus] = useState('')
  const [source, setSource] = useState('')
  const [clientId, setClientId] = useState('')
  const [q, setQ] = useState('')
  const [detail, setDetail] = useState<MailLog | null>(null)
  const reqSeq = useRef(0)

  // Test send modal
  const [testOpen, setTestOpen] = useState(false)
  const [testTo, setTestTo] = useState('')
  const [testSubject, setTestSubject] = useState('MailServer Test Email')
  const [testBody, setTestBody] = useState('Hello, this is a test email from MailServer Console.')
  const [sending, setSending] = useState(false)

  const buildParams = (includePage: boolean, targetOffset = offset) => {
    const p: Record<string, string> = {}
    if (includePage) {
      p.limit = String(limit)
      p.offset = String(targetOffset)
    }
    if (status) p.status = status
    if (source) p.source = source
    if (clientId) p.client_id = clientId
    if (q.trim()) p.q = q.trim()
    return p
  }

  const load = async (targetOffset = offset) => {
    const seq = ++reqSeq.current
    setLoading(true)
    try {
      const [res, cRes] = await Promise.all([api.listMailLogs(buildParams(true, targetOffset)), api.listClients()])
      if (reqSeq.current !== seq) return
      setLogs(res.items || [])
      setTotal(res.total || 0)
      setClients(cRes.items || [])
    } catch (e: any) {
      if (reqSeq.current === seq) toast.error(e.message)
    } finally {
      if (reqSeq.current === seq) setLoading(false)
    }
  }

  useEffect(() => {
    load(offset)
  }, [offset, status, source, clientId])

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    if (offset !== 0) setOffset(0)
    else load(0)
  }

  const handleTestSend = async (e: React.FormEvent) => {
    e.preventDefault()
    setSending(true)
    try {
      await api.testSendMail({ to: testTo, subject: testSubject, body: testBody })
      toast.success(t('mailLogs.testSuccess'))
      setTestOpen(false)
      load(0)
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setSending(false)
    }
  }

  const exportParams = buildParams(false)

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('mailLogs.title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('mailLogs.subtitle')}</p>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          <a
            href={api.exportMailLogsUrl(exportParams, 'csv')}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700"
          >
            <Download className="w-3.5 h-3.5" /> {t('common.exportCSV')}
          </a>
          <a
            href={api.exportMailLogsUrl(exportParams, 'json')}
            className="inline-flex items-center gap-1.5 px-3 py-2 text-xs font-medium rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-700"
          >
            <Download className="w-3.5 h-3.5" /> {t('common.exportJSON')}
          </a>
          <button type="button" onClick={() => setTestOpen(true)} className="primary-button">
            <Send className="w-4 h-4" />
            {t('mailLogs.testSendBtn')}
          </button>
        </div>
      </div>

      <form onSubmit={handleSearch} className="filter-bar flex flex-wrap gap-2.5 bg-white dark:bg-slate-900 p-3.5 rounded-xl border border-slate-200 dark:border-slate-800">
        <div className="flex items-center gap-2 flex-1 min-w-[220px] px-3 py-1.5 border rounded-lg">
          <Search className="w-4 h-4 text-slate-400 shrink-0" />
          <input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t('mailLogs.filter.searchPlaceholder')}
            className="w-full text-xs bg-transparent border-0 focus:outline-none focus:shadow-none"
          />
        </div>
        <select
          value={status}
          onChange={(e) => { setOffset(0); setStatus(e.target.value) }}
          className="px-3 py-1.5 border rounded-lg text-xs bg-white dark:bg-slate-900"
        >
          <option value="">{t('mailLogs.filter.allStatus')}</option>
          <option value="succeeded">{t('status.succeeded')}</option>
          <option value="failed">{t('status.failed')}</option>
        </select>
        <select
          value={source}
          onChange={(e) => { setOffset(0); setSource(e.target.value) }}
          className="px-3 py-1.5 border rounded-lg text-xs bg-white dark:bg-slate-900"
        >
          <option value="">{t('mailLogs.filter.allSource')}</option>
          <option value="api">{t('status.api')}</option>
          <option value="web_test">{t('status.web_test')}</option>
        </select>
        <select
          value={clientId}
          onChange={(e) => { setOffset(0); setClientId(e.target.value) }}
          className="px-3 py-1.5 border rounded-lg text-xs bg-white dark:bg-slate-900"
        >
          <option value="">{t('mailLogs.filter.allClients')}</option>
          {clients.map((c) => (
            <option key={c.id} value={c.id}>{c.name}</option>
          ))}
        </select>
        <button type="submit" className="px-3.5 py-1.5 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white">
          {t('common.search')}
        </button>
      </form>

      <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 overflow-x-auto">
        <table className="mail-table w-full text-sm">
          <thead className="text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200/80 dark:border-slate-800">
            <tr>
              <th className="px-4 py-3">{t('mailLogs.colTime')}</th>
              <th className="px-4 py-3">{t('mailLogs.colSource')}</th>
              <th className="px-4 py-3">{t('mailLogs.colClient')}</th>
              <th className="px-4 py-3">{t('mailLogs.colRecipient')}</th>
              <th className="px-4 py-3">{t('mailLogs.colSubject')}</th>
              <th className="px-4 py-3">{t('mailLogs.colStatus')}</th>
              <th className="px-4 py-3">{t('mailLogs.colDuration')}</th>
              <th className="px-4 py-3 text-right">{t('mailLogs.colActions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
            {loading && (
              <tr><td colSpan={8}><LoadingState /></td></tr>
            )}
            {!loading && logs.length === 0 && (
              <tr>
                <td colSpan={8}>
                  <EmptyState icon={<Mail className="w-6 h-6" />} title={t('mailLogs.emptyTitle')} description={t('mailLogs.emptyDesc')} />
                </td>
              </tr>
            )}
            {!loading && logs.map((m) => (
              <tr key={m.id}>
                <td className="px-4 py-3 whitespace-nowrap text-slate-500">{formatTime(m.created_at, lang)}</td>
                <td className="px-4 py-3"><StatusBadge value={m.source} /></td>
                <td className="px-4 py-3 font-medium text-slate-800 dark:text-slate-200">{m.client_name || '—'}</td>
                <td className="px-4 py-3 font-mono">{m.recipient}</td>
                <td className="px-4 py-3 max-w-[260px] truncate">{m.subject}</td>
                <td className="px-4 py-3"><StatusBadge value={m.status} /></td>
                <td className="px-4 py-3 font-mono text-slate-500">{formatDuration(m.duration_ms)}</td>
                <td className="px-4 py-3 text-right">
                  <button
                    type="button"
                    onClick={() => setDetail(m)}
                    className="px-2.5 py-1 text-xs rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800"
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
            <button
              type="button"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - limit))}
              className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 disabled:opacity-40"
            >
              {t('common.prevPage')}
            </button>
            <button
              type="button"
              disabled={offset + limit >= total}
              onClick={() => setOffset(offset + limit)}
              className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 disabled:opacity-40"
            >
              {t('common.nextPage')}
            </button>
          </div>
        </div>
      )}

      {detail && (
        <Modal title={t('mailLogs.detailTitle')} onClose={() => setDetail(null)}>
          <div className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-4 text-xs">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('mailLogs.detailTitle')}</h2>
            <div className="grid grid-cols-2 gap-3">
              <div><span className="text-slate-400 block">{t('mailLogs.colTime')}</span><span className="font-mono">{formatTime(detail.created_at, lang)}</span></div>
              <div><span className="text-slate-400 block">{t('mailLogs.colStatus')}</span><StatusBadge value={detail.status} /></div>
              <div><span className="text-slate-400 block">{t('mailLogs.colClient')}</span><span>{detail.client_name || '—'}</span></div>
              <div><span className="text-slate-400 block">{t('mailLogs.detailIP')}</span><span className="font-mono">{detail.client_ip || '—'}</span></div>
              <div><span className="text-slate-400 block">{t('mailLogs.colRecipient')}</span><span className="font-mono break-all">{detail.recipient}</span></div>
              <div><span className="text-slate-400 block">{t('mailLogs.colDuration')}</span><span className="font-mono">{formatDuration(detail.duration_ms)}</span></div>
            </div>
            <div>
              <span className="text-slate-400 block mb-1">{t('mailLogs.colSubject')}</span>
              <div className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700">{detail.subject}</div>
            </div>
            {detail.error_message && (
              <div>
                <span className="text-rose-500 block mb-1">{t('mailLogs.detailError')}</span>
                <pre className="p-2.5 rounded-lg bg-rose-50 dark:bg-rose-500/10 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300 whitespace-pre-wrap break-all font-mono">{detail.error_message}</pre>
              </div>
            )}
            <div>
              <span className="text-slate-400 block mb-1">{t('mailLogs.detailBody')}</span>
              {detail.body ? (
                <pre className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 whitespace-pre-wrap break-words max-h-60 overflow-y-auto">{detail.body}</pre>
              ) : (
                <div className="text-slate-400 italic">{t('mailLogs.detailNoBody')}</div>
              )}
            </div>
          </div>
        </Modal>
      )}

      {testOpen && (
        <Modal title={t('mailLogs.testModalTitle')} onClose={() => !sending && setTestOpen(false)} busy={sending} size="md">
          <form onSubmit={handleTestSend} className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-4 text-sm">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('mailLogs.testModalTitle')}</h2>
            <div>
              <label htmlFor="test-mail-to" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                {t('mailLogs.testTo')}
              </label>
              <input
                id="test-mail-to"
                type="email"
                required
                value={testTo}
                onChange={(e) => setTestTo(e.target.value)}
                placeholder="receiver@example.com"
                className="w-full px-3 py-2 text-xs border rounded-lg"
              />
            </div>
            <div>
              <label htmlFor="test-mail-subject" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                {t('mailLogs.testSubject')}
              </label>
              <input
                id="test-mail-subject"
                required
                value={testSubject}
                onChange={(e) => setTestSubject(e.target.value)}
                className="w-full px-3 py-2 text-xs border rounded-lg"
              />
            </div>
            <div>
              <label htmlFor="test-mail-body" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
                {t('mailLogs.testBody')}
              </label>
              <textarea
                id="test-mail-body"
                required
                rows={4}
                value={testBody}
                onChange={(e) => setTestBody(e.target.value)}
                className="w-full px-3 py-2 text-xs border rounded-lg"
              />
            </div>
            <div className="flex justify-end gap-2 pt-2">
              <button
                type="button"
                disabled={sending}
                onClick={() => setTestOpen(false)}
                className="px-3.5 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700"
              >
                {t('common.cancel')}
              </button>
              <button
                type="submit"
                disabled={sending}
                className="px-4 py-2 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white disabled:opacity-50 inline-flex items-center gap-1.5"
              >
                {sending && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
                {sending ? t('mailLogs.testSending') : t('mailLogs.testSendSubmit')}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
