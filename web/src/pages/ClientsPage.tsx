import React, { useEffect, useState } from 'react'
import { Check, Copy, Eye, EyeOff, KeyRound, Plus, RefreshCw, Send, Terminal, Trash2 } from 'lucide-react'
import { api, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { EmptyState, LoadingState } from '../components/Feedback'
import { Modal } from '../components/Modal'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import { copyText } from '../utils/clipboard'
import { withBasePath } from '../utils/basePath'
import type { Client } from '../types'

interface ClientForm {
  id?: string
  name: string
  description: string
  enabled: boolean
  rate_limit_rpm: number
  expires_at: string
}

interface TestSendState {
  client: Client
  authKey: string
  loadingKey: boolean
  keyError: string
  showKey: boolean
  to: string
  subject: string
  body: string
  sending: boolean
  result: { ok: boolean; status: number; payload: unknown; msg: string } | null
}

const shellSingleQuote = (value: string) => `'${value.replace(/'/g, `'\\''`)}'`

const buildSendCurlCommand = (params: {
  url: string
  to: string
  subject: string
  body: string
  authKey: string
}) => {
  const payload = JSON.stringify(
    {
      to: params.to,
      subject: params.subject,
      body: params.body,
      auth_key: params.authKey,
    },
    null,
    2,
  )
  return `curl -X POST ${params.url} \\\n  -H "Content-Type: application/json" \\\n  -d ${shellSingleQuote(payload)}`
}

export const ClientsPage: React.FC = () => {
  const toast = useToast()
  const { t, lang } = useI18n()
  const [clients, setClients] = useState<Client[]>([])
  const [loading, setLoading] = useState(true)
  const [modalOpen, setModalOpen] = useState(false)
  const [form, setForm] = useState<ClientForm | null>(null)
  const [initialSnap, setInitialSnap] = useState('')
  const [saving, setSaving] = useState(false)
  const [revealedKey, setRevealedKey] = useState<{ key: string; name: string; isNew?: boolean } | null>(null)
  const [revealingId, setRevealingId] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const [copiedCurl, setCopiedCurl] = useState(false)
  const [testSend, setTestSend] = useState<TestSendState | null>(null)
  const [error, setError] = useState('')

  const load = async () => {
    setLoading(true)
    try {
      const res = await api.listClients()
      setClients(res.items || [])
    } catch (e: any) {
      toast.error(e.message || t('clients.loadError'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const openNew = () => {
    setError('')
    const next: ClientForm = { name: '', description: '', enabled: true, rate_limit_rpm: 0, expires_at: '' }
    setForm(next)
    setInitialSnap(JSON.stringify(next))
    setModalOpen(true)
  }

  const openEdit = (c: Client) => {
    setError('')
    const next: ClientForm = {
      id: c.id,
      name: c.name,
      description: c.description,
      enabled: c.enabled,
      rate_limit_rpm: c.rate_limit_rpm,
      expires_at: c.expires_at ? new Date(c.expires_at).toISOString().slice(0, 16) : '',
    }
    setForm(next)
    setInitialSnap(JSON.stringify(next))
    setModalOpen(true)
  }

  const closeModal = () => {
    if (JSON.stringify(form) !== initialSnap && !confirm(t('clients.discardConfirm'))) return
    setModalOpen(false)
  }

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!form || saving) return
    setSaving(true)
    setError('')
    const payload = {
      name: form.name,
      description: form.description,
      enabled: form.enabled,
      rate_limit_rpm: Number(form.rate_limit_rpm) || 0,
      expires_at: form.expires_at ? new Date(form.expires_at).toISOString() : null,
    }
    try {
      if (form.id) {
        await api.updateClient(form.id, payload)
        toast.success(t('common.saved'))
      } else {
        const res = await api.createClient(payload)
        setRevealedKey({ key: res.auth_key, name: res.client.name, isNew: true })
      }
      setModalOpen(false)
      await load()
    } catch (e: any) {
      setError(e.message)
    } finally {
      setSaving(false)
    }
  }

  const handleReveal = async (c: Client) => {
    setRevealingId(c.id)
    try {
      const res = await api.revealClientSecrets(c.id)
      setRevealedKey({ key: res.auth_key, name: c.name, isNew: false })
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setRevealingId(null)
    }
  }

  const handleRotate = async (c: Client) => {
    if (!confirm(t('clients.rotateConfirm', { name: c.name }))) return
    try {
      const res = await api.rotateClientKey(c.id)
      setRevealedKey({ key: res.auth_key, name: c.name, isNew: true })
      await load()
    } catch (e: any) {
      toast.error(e.message)
    }
  }

  const handleDelete = async (c: Client) => {
    if (!confirm(t('clients.deleteConfirm', { name: c.name }))) return
    try {
      await api.deleteClient(c.id)
      await load()
    } catch (e: any) {
      toast.error(e.message)
    }
  }

  const openTestSend = async (c: Client) => {
    setCopiedCurl(false)
    setTestSend({
      client: c,
      authKey: '',
      loadingKey: true,
      keyError: '',
      showKey: false,
      to: '',
      subject: t('clients.testDefaultSubject'),
      body: t('clients.testDefaultBody', { name: c.name }),
      sending: false,
      result: null,
    })
    try {
      const res = await api.revealClientSecrets(c.id)
      setTestSend((prev) =>
        prev && prev.client.id === c.id ? { ...prev, authKey: res.auth_key, loadingKey: false } : prev,
      )
    } catch (e: any) {
      setTestSend((prev) =>
        prev && prev.client.id === c.id
          ? { ...prev, keyError: e.message || t('clients.loadError'), loadingKey: false }
          : prev,
      )
    }
  }

  const handleTestSend = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!testSend || testSend.sending || testSend.loadingKey || !testSend.authKey) return
    setTestSend((prev) => (prev ? { ...prev, sending: true, result: null } : prev))
    try {
      const res = await fetch(withBasePath('/send'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          to: testSend.to.trim(),
          subject: testSend.subject.trim(),
          body: testSend.body,
          auth_key: testSend.authKey,
        }),
      })
      const rawText = await res.text()
      let payload: any = rawText
      try {
        payload = rawText ? JSON.parse(rawText) : {}
      } catch {
        // non-JSON response body
      }
      const msg =
        typeof payload === 'object' && payload !== null && 'msg' in payload
          ? String(payload.msg)
          : rawText || res.statusText
      const ok = res.ok && typeof payload === 'object' && payload !== null && payload.code === 0
      setTestSend((prev) =>
        prev ? { ...prev, sending: false, result: { ok, status: res.status, payload, msg } } : prev,
      )
      if (ok) {
        toast.success(t('clients.testSuccess', { status: res.status }))
      } else {
        toast.error(t('clients.testFailed', { status: res.status, msg }))
      }
      await load()
    } catch (err: any) {
      const msg = err?.message || 'Network error'
      setTestSend((prev) =>
        prev ? { ...prev, sending: false, result: { ok: false, status: 0, payload: { error: msg }, msg } } : prev,
      )
      toast.error(t('clients.testFailed', { status: 0, msg }))
    }
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('clients.title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400">{t('clients.subtitle')}</p>
        </div>
        <button type="button" onClick={openNew} className="primary-button">
          <Plus className="w-4 h-4" />
          {t('clients.createBtn')}
        </button>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 overflow-x-auto">
        <table className="clients-table w-full text-sm">
          <thead className="text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200/80 dark:border-slate-800">
            <tr>
              <th className="px-4 py-3">{t('clients.colName')}</th>
              <th className="px-4 py-3">{t('clients.colPrefix')}</th>
              <th className="px-4 py-3">{t('clients.colRateLimit')}</th>
              <th className="px-4 py-3">{t('clients.colUsage')}</th>
              <th className="px-4 py-3">{t('clients.colLastUsed')}</th>
              <th className="px-4 py-3">{t('clients.colStatus')}</th>
              <th className="px-4 py-3 text-right">{t('clients.colActions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
            {loading && <tr><td colSpan={7}><LoadingState /></td></tr>}
            {!loading && clients.length === 0 && (
              <tr>
                <td colSpan={7}>
                  <EmptyState icon={<KeyRound className="w-6 h-6" />} title={t('clients.emptyTitle')} description={t('clients.emptyDesc')} />
                </td>
              </tr>
            )}
            {!loading && clients.map((c) => (
              <tr key={c.id}>
                <td className="px-4 py-3">
                  <div className="font-medium text-slate-900 dark:text-slate-100">{c.name}</div>
                  {c.description && <div className="text-[11px] text-slate-400 mt-0.5">{c.description}</div>}
                </td>
                <td className="px-4 py-3 font-mono text-slate-600 dark:text-slate-300">{c.key_prefix}…</td>
                <td className="px-4 py-3">{c.rate_limit_rpm > 0 ? `${c.rate_limit_rpm}/min` : t('common.unlimited')}</td>
                <td className="px-4 py-3 font-mono">
                  <span className="text-emerald-600 dark:text-emerald-400">{c.total_sent}</span>
                  {' / '}
                  <span className="text-rose-600 dark:text-rose-400">{c.total_failed}</span>
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-slate-500">{formatTime(c.last_used_at, lang)}</td>
                <td className="px-4 py-3"><StatusBadge value={c.enabled ? 'enabled' : 'disabled'} /></td>
                <td className="px-4 py-3 text-right whitespace-nowrap">
                  <div className="inline-flex items-center gap-1.5">
                    <button
                      type="button"
                      onClick={() => openTestSend(c)}
                      className="px-2.5 py-1 rounded-lg border border-indigo-200 dark:border-indigo-500/30 text-indigo-600 dark:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-500/10 inline-flex items-center gap-1"
                    >
                      <Send className="w-3.5 h-3.5" /> {t('clients.testBtn')}
                    </button>
                    <button
                      type="button"
                      disabled={revealingId === c.id}
                      onClick={() => handleReveal(c)}
                      className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 inline-flex items-center gap-1"
                    >
                      <Eye className="w-3.5 h-3.5" /> {t('clients.revealBtn')}
                    </button>
                    <button
                      type="button"
                      onClick={() => handleRotate(c)}
                      className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 inline-flex items-center gap-1"
                    >
                      <RefreshCw className="w-3.5 h-3.5" /> {t('clients.rotateBtn')}
                    </button>
                    <button
                      type="button"
                      onClick={() => openEdit(c)}
                      className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800"
                    >
                      {t('common.edit')}
                    </button>
                    <button
                      type="button"
                      onClick={() => handleDelete(c)}
                      className="p-1.5 rounded-lg border border-rose-200 dark:border-rose-500/30 text-rose-600 dark:text-rose-400 hover:bg-rose-50 dark:hover:bg-rose-500/10"
                      aria-label={t('common.delete')}
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {modalOpen && form && (
        <Modal title={form.id ? t('clients.modalEditTitle', { name: form.name }) : t('clients.modalCreateTitle')} onClose={closeModal} busy={saving} size="md">
          <form onSubmit={handleSave} className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-4 text-sm">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
              {form.id ? t('clients.modalEditTitle', { name: form.name }) : t('clients.modalCreateTitle')}
            </h2>
            {error && <div role="alert" className="p-2.5 rounded-lg bg-rose-50 dark:bg-rose-500/15 text-rose-700 dark:text-rose-300 text-xs">{error}</div>}
            <div>
              <label htmlFor="cli-name" className="block text-xs font-medium mb-1">{t('clients.fieldName')}</label>
              <input id="cli-name" required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className="w-full px-3 py-2 text-xs border rounded-lg" />
            </div>
            <div>
              <label htmlFor="cli-desc" className="block text-xs font-medium mb-1">{t('clients.fieldDesc')}</label>
              <input id="cli-desc" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className="w-full px-3 py-2 text-xs border rounded-lg" />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label htmlFor="cli-rpm" className="block text-xs font-medium mb-1">{t('clients.fieldRateLimit')}</label>
                <input id="cli-rpm" type="number" min={0} value={form.rate_limit_rpm} onChange={(e) => setForm({ ...form, rate_limit_rpm: Number(e.target.value) })} className="w-full px-3 py-2 text-xs border rounded-lg" />
              </div>
              <div>
                <label htmlFor="cli-exp" className="block text-xs font-medium mb-1">{t('clients.fieldExpires')}</label>
                <input id="cli-exp" type="datetime-local" value={form.expires_at} onChange={(e) => setForm({ ...form, expires_at: e.target.value })} className="w-full px-3 py-2 text-xs border rounded-lg" />
              </div>
            </div>
            <label className="flex items-center gap-2 text-xs">
              <input type="checkbox" checked={form.enabled} onChange={(e) => setForm({ ...form, enabled: e.target.checked })} />
              <span>{t('clients.fieldEnabled')}</span>
            </label>
            <div className="flex justify-end gap-2 pt-2">
              <button type="button" disabled={saving} onClick={closeModal} className="px-3.5 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700">{t('common.cancel')}</button>
              <button type="submit" disabled={saving || (form.id ? JSON.stringify(form) === initialSnap : !form.name.trim())} className="px-4 py-2 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white disabled:opacity-45">
                {saving ? t('common.saving') : t('common.save')}
              </button>
            </div>
          </form>
        </Modal>
      )}

      {revealedKey && (
        <Modal title={t('clients.keyDialogTitle')} onClose={() => { setRevealedKey(null); setCopied(false) }} size="md">
          <div className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-4 text-xs">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('clients.keyDialogTitle')} · {revealedKey.name}</h2>
            <p className="text-slate-500 dark:text-slate-400">
              {revealedKey.isNew ? t('clients.keyDialogNewHint') : t('clients.keyDialogViewHint')}
            </p>
            <div className="p-3 rounded-xl bg-slate-50 dark:bg-slate-800 border border-slate-200 dark:border-slate-700 font-mono text-xs break-all select-all">
              {revealedKey.key}
            </div>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={async () => {
                  await copyText(revealedKey.key)
                  setCopied(true)
                  toast.success(t('secret.copied'))
                  setTimeout(() => setCopied(false), 1500)
                }}
                className="inline-flex items-center gap-1.5 px-3.5 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white font-medium"
              >
                {copied ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                {copied ? t('common.copied') : t('common.copy')}
              </button>
              <button type="button" onClick={() => setRevealedKey(null)} className="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700">
                {t('common.close')}
              </button>
            </div>
          </div>
        </Modal>
      )}

      {testSend && (() => {
        const sendEndpoint = `${window.location.origin}${withBasePath('/send')}`
        const liveTo = testSend.to.trim() || 'receiver@example.com'
        const liveSubject = testSend.subject.trim()
        const maskedAuthKey = testSend.loadingKey
          ? '...'
          : testSend.authKey
            ? `${testSend.client.key_prefix}••••••••`
            : 'ms_your_auth_key'
        const previewCurl = buildSendCurlCommand({
          url: sendEndpoint,
          to: liveTo,
          subject: liveSubject,
          body: testSend.body,
          authKey: testSend.showKey && testSend.authKey ? testSend.authKey : maskedAuthKey,
        })
        const copyableCurl = buildSendCurlCommand({
          url: sendEndpoint,
          to: liveTo,
          subject: liveSubject,
          body: testSend.body,
          authKey: testSend.authKey || 'ms_your_auth_key',
        })

        return (
          <Modal
            title={t('clients.testModalTitle', { name: testSend.client.name })}
            onClose={() => { if (!testSend.sending) setTestSend(null) }}
            size="2xl"
          >
            <form onSubmit={handleTestSend} className="bg-white dark:bg-slate-900 rounded-2xl p-6 sm:p-7 space-y-5 text-xs">
              <div>
                <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">
                  {t('clients.testModalTitle', { name: testSend.client.name })}
                </h2>
                <p className="text-slate-500 dark:text-slate-400 mt-1">{t('clients.testModalDesc')}</p>
              </div>

              {testSend.keyError && (
                <div className="p-3 rounded-lg bg-rose-50 dark:bg-rose-500/15 text-rose-600 dark:text-rose-300">
                  {testSend.keyError}
                </div>
              )}

              <div className="grid lg:grid-cols-[minmax(0,1fr)_minmax(0,1.25fr)] gap-6 items-start">
                <div className="space-y-4 min-w-0">
                  <div>
                    <label className="block text-slate-600 dark:text-slate-300 mb-1.5">{t('clients.testKey')}</label>
                    <div className="flex items-center gap-2">
                      <input
                        type={testSend.showKey ? 'text' : 'password'}
                        readOnly
                        value={testSend.loadingKey ? t('clients.testLoadingKey') : testSend.authKey}
                        className="min-w-0 flex-1 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-800/80 font-mono text-xs text-slate-700 dark:text-slate-200"
                      />
                      <button
                        type="button"
                        disabled={testSend.loadingKey || !testSend.authKey}
                        onClick={() => setTestSend((prev) => (prev ? { ...prev, showKey: !prev.showKey } : prev))}
                        className="shrink-0 p-2 rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-45"
                        aria-label={t('clients.revealBtn')}
                      >
                        {testSend.showKey ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                      </button>
                    </div>
                  </div>

                  <div>
                    <label className="block text-slate-600 dark:text-slate-300 mb-1.5">{t('clients.testTo')}</label>
                    <input
                      type="email"
                      required
                      placeholder="ops@example.com"
                      value={testSend.to}
                      onChange={(e) => setTestSend((prev) => (prev ? { ...prev, to: e.target.value } : prev))}
                      className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800"
                    />
                  </div>

                  <div>
                    <label className="block text-slate-600 dark:text-slate-300 mb-1.5">{t('clients.testSubject')}</label>
                    <input
                      required
                      value={testSend.subject}
                      onChange={(e) => setTestSend((prev) => (prev ? { ...prev, subject: e.target.value } : prev))}
                      className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800"
                    />
                  </div>

                  <div>
                    <label className="block text-slate-600 dark:text-slate-300 mb-1.5">{t('clients.testBody')}</label>
                    <textarea
                      rows={4}
                      required
                      value={testSend.body}
                      onChange={(e) => setTestSend((prev) => (prev ? { ...prev, body: e.target.value } : prev))}
                      className="w-full px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800"
                    />
                  </div>
                </div>

                <div className="space-y-4 min-w-0">
                  <div className="rounded-xl border border-slate-200 dark:border-slate-700/80 bg-slate-50/70 dark:bg-slate-800/40 p-4 space-y-3">
                    <div className="flex items-center justify-between gap-3">
                      <div className="flex items-center gap-2 min-w-0">
                        <Terminal className="w-4 h-4 text-indigo-500 shrink-0" />
                        <span className="font-medium text-slate-700 dark:text-slate-200 truncate">{t('clients.testCurlTitle')}</span>
                        <span className="shrink-0 font-mono text-[10px] px-1.5 py-0.5 rounded bg-indigo-50 dark:bg-indigo-500/15 text-indigo-600 dark:text-indigo-300">
                          POST /send
                        </span>
                      </div>
                      <button
                        type="button"
                        disabled={testSend.loadingKey}
                        onClick={async () => {
                          await copyText(copyableCurl)
                          setCopiedCurl(true)
                          toast.success(t('secret.copied'))
                          setTimeout(() => setCopiedCurl(false), 1500)
                        }}
                        className="shrink-0 whitespace-nowrap inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-100 dark:hover:bg-slate-700 text-slate-700 dark:text-slate-200 disabled:opacity-45"
                      >
                        {copiedCurl ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                        {copiedCurl ? t('common.copied') : t('clients.testCurlCopy')}
                      </button>
                    </div>
                    <p className="text-[11px] leading-relaxed text-slate-500 dark:text-slate-400">
                      {t('clients.testCurlDesc')}
                    </p>
                    <pre className="p-3.5 rounded-lg bg-slate-950 text-slate-100 font-mono text-[11px] leading-relaxed overflow-x-auto max-h-64 whitespace-pre">{previewCurl}</pre>
                  </div>

                  {testSend.result && (
                    <div
                      className={`p-3.5 rounded-xl border space-y-1.5 ${
                        testSend.result.ok
                          ? 'bg-emerald-50 dark:bg-emerald-500/10 border-emerald-200 dark:border-emerald-500/30 text-emerald-700 dark:text-emerald-300'
                          : 'bg-rose-50 dark:bg-rose-500/10 border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300'
                      }`}
                    >
                      <div className="font-medium flex items-center justify-between gap-2">
                        <span>{t('clients.testResponse')}</span>
                        <span className="font-mono">HTTP {testSend.result.status || 'ERR'}</span>
                      </div>
                      <pre className="text-[11px] font-mono whitespace-pre-wrap break-all opacity-90">
                        {typeof testSend.result.payload === 'string'
                          ? testSend.result.payload
                          : JSON.stringify(testSend.result.payload, null, 2)}
                      </pre>
                    </div>
                  )}
                </div>
              </div>

              <div className="flex justify-end gap-2 pt-2 border-t border-slate-100 dark:border-slate-800">
                <button
                  type="button"
                  disabled={testSend.sending}
                  onClick={() => setTestSend(null)}
                  className="px-4 py-2 text-xs rounded-lg border border-slate-200 dark:border-slate-700 disabled:opacity-45"
                >
                  {t('common.close')}
                </button>
                <button
                  type="submit"
                  disabled={
                    testSend.sending ||
                    testSend.loadingKey ||
                    !testSend.authKey ||
                    !testSend.to.trim() ||
                    !testSend.subject.trim() ||
                    !testSend.body
                  }
                  className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white disabled:opacity-45"
                >
                  <Send className="w-3.5 h-3.5" />
                  {testSend.sending ? t('clients.testSending') : t('clients.testSubmit')}
                </button>
              </div>
            </form>
          </Modal>
        )
      })()}
    </div>
  )
}
