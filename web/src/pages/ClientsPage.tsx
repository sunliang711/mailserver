import React, { useEffect, useState } from 'react'
import { Check, ChevronDown, ChevronRight, Copy, Eye, KeyRound, Plus, RefreshCw, Terminal, Trash2 } from 'lucide-react'
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
  const [guideOpen, setGuideOpen] = useState(false)
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

  const curlExample = `curl -X POST ${window.location.origin}${withBasePath('/send')} \\
  -H "Content-Type: application/json" \\
  -d '{
    "to": "receiver@example.com",
    "subject": "Test Mail",
    "body": "Hello from mailserver",
    "auth_key": "ms_your_auth_key"
  }'`

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

      <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 overflow-hidden">
        <button
          type="button"
          onClick={() => setGuideOpen(!guideOpen)}
          className="w-full px-5 py-3.5 flex items-center justify-between text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-slate-800/50"
        >
          <span className="inline-flex items-center gap-2"><Terminal className="w-4 h-4 text-indigo-500" />{t('clients.guideTitle')}</span>
          {guideOpen ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />}
        </button>
        {guideOpen && (
          <div className="px-5 pb-5 space-y-3 border-t border-slate-100 dark:border-slate-800 pt-3">
            <p className="text-xs text-slate-500 dark:text-slate-400">{t('clients.guideDesc')}</p>
            <pre className="p-3.5 rounded-xl bg-slate-950 text-slate-100 font-mono text-xs overflow-x-auto">{curlExample}</pre>
          </div>
        )}
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
              <button type="submit" disabled={saving} className="px-4 py-2 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white disabled:opacity-50">
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
    </div>
  )
}
