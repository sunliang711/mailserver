import React, { useEffect, useState } from 'react'
import { CheckCircle2, Loader2, Mail, SlidersHorizontal, Users } from 'lucide-react'
import { api, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { LoadingState } from '../components/Feedback'
import { Modal } from '../components/Modal'
import { SecretField } from '../components/SecretField'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import type { Admin, SMTPConfig, SystemSettings } from '../types'

type SubTab = 'smtp' | 'general' | 'admins'

function smtpSnapshot(cfg: { host: string; port: number; user: string }): string {
  return JSON.stringify({
    host: cfg.host.trim(),
    port: Number(cfg.port),
    user: cfg.user.trim(),
  })
}

export const SettingsPage: React.FC = () => {
  const toast = useToast()
  const { t, lang } = useI18n()
  const [subTab, setSubTab] = useState<SubTab>('smtp')
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState('')

  // SMTP
  const [smtpHost, setSmtpHost] = useState('')
  const [smtpPort, setSmtpPort] = useState(465)
  const [smtpUser, setSmtpUser] = useState('')
  const [smtpPassword, setSmtpPassword] = useState('')
  const [smtpSavedPassword, setSmtpSavedPassword] = useState('')
  const [smtpHasPassword, setSmtpHasPassword] = useState(false)
  const [smtpClearPassword, setSmtpClearPassword] = useState(false)
  const [smtpStatus, setSmtpStatus] = useState('untested')
  const [smtpTestError, setSmtpTestError] = useState('')
  const [smtpTestTo, setSmtpTestTo] = useState('')
  const [savedSmtpSnap, setSavedSmtpSnap] = useState('')
  const [savingSmtp, setSavingSmtp] = useState(false)
  const [testingSmtp, setTestingSmtp] = useState(false)

  // General
  const [settings, setSettings] = useState<SystemSettings | null>(null)
  const [savedGeneralSnap, setSavedGeneralSnap] = useState('')
  const [savingGeneral, setSavingGeneral] = useState(false)

  // Admins
  const [admins, setAdmins] = useState<Admin[]>([])
  const [oldPwd, setOldPwd] = useState('')
  const [newPwd, setNewPwd] = useState('')
  const [changingPwd, setChangingPwd] = useState(false)
  const [newAdminUser, setNewAdminUser] = useState('')
  const [newAdminPass, setNewAdminPass] = useState('')
  const [creatingAdmin, setCreatingAdmin] = useState(false)
  const [resetAdmin, setResetAdmin] = useState<Admin | null>(null)
  const [resetPwd, setResetPwd] = useState('')
  const [resetting, setResetting] = useState(false)

  const applySmtp = (cfg: SMTPConfig) => {
    setSmtpHost(cfg.host || '')
    setSmtpPort(cfg.port || 465)
    setSmtpUser(cfg.user || '')
    setSmtpHasPassword(cfg.has_password)
    setSmtpSavedPassword(cfg.password || '')
    setSmtpPassword('')
    setSmtpClearPassword(false)
    setSmtpStatus(cfg.test_status || 'untested')
    setSmtpTestError(cfg.test_error || '')
    setSavedSmtpSnap(smtpSnapshot({ host: cfg.host || '', port: cfg.port || 465, user: cfg.user || '' }))
  }

  const load = async () => {
    setLoading(true)
    setLoadError('')
    try {
      const [smtp, gen, adm] = await Promise.all([api.getSMTP(), api.getSettings(), api.listAdmins()])
      applySmtp(smtp)
      setSettings(gen)
      setSavedGeneralSnap(JSON.stringify(gen))
      setAdmins(adm.items || [])
    } catch (e: any) {
      setLoadError(e.message || t('settings.loadError'))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const smtpPayload = () => ({
    host: smtpHost.trim(),
    port: Number(smtpPort),
    user: smtpUser.trim(),
    ...(smtpClearPassword ? { clear_password: true } : smtpPassword !== '' ? { password: smtpPassword } : {}),
  })

  const saveSmtp = async (e: React.FormEvent) => {
    e.preventDefault()
    setSavingSmtp(true)
    try {
      const updated = await api.updateSMTP(smtpPayload())
      applySmtp(updated)
      toast.success(t('settings.smtp.saveSuccess'))
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setSavingSmtp(false)
    }
  }

  const testSmtp = async () => {
    setTestingSmtp(true)
    setSmtpTestError('')
    try {
      await api.testSMTP({ ...smtpPayload(), test_to: smtpTestTo.trim() })
      setSmtpStatus('ok')
      toast.success(t('settings.smtp.testSuccess'))
    } catch (e: any) {
      setSmtpStatus('failed')
      setSmtpTestError(e.message)
      toast.error(e.message)
    } finally {
      setTestingSmtp(false)
    }
  }

  const saveGeneral = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!settings) return
    setSavingGeneral(true)
    try {
      const updated = await api.updateSettings(settings)
      setSettings(updated)
      setSavedGeneralSnap(JSON.stringify(updated))
      toast.success(t('settings.general.saveSuccess'))
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setSavingGeneral(false)
    }
  }

  const currentSmtpSnap = smtpSnapshot({ host: smtpHost, port: smtpPort, user: smtpUser })
  const smtpDirty =
    savedSmtpSnap !== '' &&
    (currentSmtpSnap !== savedSmtpSnap || smtpClearPassword || smtpPassword !== '')
  const generalDirty = settings ? JSON.stringify(settings) !== savedGeneralSnap : false

  useEffect(() => {
    if (!smtpDirty && !generalDirty) return
    const beforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', beforeUnload)
    return () => window.removeEventListener('beforeunload', beforeUnload)
  }, [smtpDirty, generalDirty])

  const changeOwnPassword = async (e: React.FormEvent) => {
    e.preventDefault()
    setChangingPwd(true)
    try {
      await api.changePassword(oldPwd, newPwd)
      setOldPwd('')
      setNewPwd('')
      toast.success(t('settings.admins.changePwdSuccess'))
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setChangingPwd(false)
    }
  }

  const createAdmin = async (e: React.FormEvent) => {
    e.preventDefault()
    setCreatingAdmin(true)
    try {
      await api.createAdmin(newAdminUser, newAdminPass)
      setNewAdminUser('')
      setNewAdminPass('')
      toast.success(t('settings.admins.createSuccess'))
      await load()
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setCreatingAdmin(false)
    }
  }

  const toggleAdminActive = async (a: Admin) => {
    try {
      await api.updateAdmin(a.id, { is_active: !a.is_active })
      await load()
    } catch (e: any) {
      toast.error(e.message)
    }
  }

  const submitResetPassword = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!resetAdmin) return
    setResetting(true)
    try {
      await api.updateAdmin(resetAdmin.id, { password: resetPwd })
      setResetAdmin(null)
      setResetPwd('')
      toast.success(t('settings.admins.changePwdSuccess'))
    } catch (e: any) {
      toast.error(e.message)
    } finally {
      setResetting(false)
    }
  }

  if (loading) return <LoadingState />
  if (loadError) return <div role="alert" className="p-4 rounded-xl bg-rose-50 text-rose-700 text-sm">{loadError}</div>

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('settings.title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400">{t('settings.subtitle')}</p>
      </div>

      <div className="flex flex-wrap gap-2 border-b border-slate-200 dark:border-slate-800 pb-3">
        {[
          { id: 'smtp' as SubTab, label: t('settings.tabSmtp'), icon: <Mail className="w-4 h-4" /> },
          { id: 'general' as SubTab, label: t('settings.tabGeneral'), icon: <SlidersHorizontal className="w-4 h-4" /> },
          { id: 'admins' as SubTab, label: t('settings.tabAdmins'), icon: <Users className="w-4 h-4" /> },
        ].map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setSubTab(tab.id)}
            className={`inline-flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-medium transition ${
              subTab === tab.id
                ? 'bg-indigo-600 text-white'
                : 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800'
            }`}
          >
            {tab.icon}
            <span>{tab.label}</span>
          </button>
        ))}
      </div>

      {subTab === 'smtp' && (
        <form onSubmit={saveSmtp} className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-6 space-y-4 max-w-2xl">
          <div className="flex items-center justify-between gap-2">
            <div>
              <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('settings.smtp.title')}</h2>
              <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">{t('settings.smtp.desc')}</p>
            </div>
            <StatusBadge value={smtpStatus} />
          </div>

          {smtpTestError && (
            <div role="alert" className="p-3 rounded-lg bg-rose-50 dark:bg-rose-500/15 border border-rose-200 dark:border-rose-500/30 text-xs text-rose-700 dark:text-rose-300">
              {smtpTestError}
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div className="sm:col-span-2">
              <label htmlFor="smtp-host" className="block text-xs font-medium mb-1">{t('settings.smtp.host')}</label>
              <input id="smtp-host" required value={smtpHost} onChange={(e) => setSmtpHost(e.target.value)} placeholder="smtp.host.com" className="w-full px-3 py-2 text-xs border rounded-lg font-mono" />
            </div>
            <div>
              <label htmlFor="smtp-port" className="block text-xs font-medium mb-1">{t('settings.smtp.port')}</label>
              <input id="smtp-port" type="number" min={1} max={65535} required value={smtpPort} onChange={(e) => setSmtpPort(Number(e.target.value))} className="w-full px-3 py-2 text-xs border rounded-lg font-mono" />
            </div>
          </div>

          <div>
            <label htmlFor="smtp-user" className="block text-xs font-medium mb-1">{t('settings.smtp.user')}</label>
            <input id="smtp-user" required value={smtpUser} onChange={(e) => setSmtpUser(e.target.value)} placeholder="user@host.com" className="w-full px-3 py-2 text-xs border rounded-lg font-mono" />
          </div>

          <div>
            <label htmlFor="smtp-pass" className="block text-xs font-medium mb-1">{t('settings.smtp.password')}</label>
            <SecretField
              id="smtp-pass"
              hasSaved={smtpHasPassword}
              value={smtpPassword}
              onChange={(v) => { setSmtpPassword(v); setSmtpClearPassword(false) }}
              onReveal={async () => smtpSavedPassword}
            />
          </div>

          <div className="pt-2 border-t border-slate-100 dark:border-slate-800">
            <label htmlFor="smtp-test-to" className="block text-xs font-medium mb-1">{t('settings.smtp.testTo')}</label>
            <input
              id="smtp-test-to"
              type="email"
              value={smtpTestTo}
              onChange={(e) => setSmtpTestTo(e.target.value)}
              placeholder={t('settings.smtp.testToPlaceholder')}
              className="w-full px-3 py-2 text-xs border rounded-lg"
            />
          </div>

          <div className="flex flex-wrap gap-2 pt-2">
            <button type="submit" disabled={savingSmtp || !smtpDirty} className="primary-button">
              {savingSmtp && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
              {savingSmtp ? t('common.saving') : t('settings.smtp.saveBtn')}
            </button>
            <button
              type="button"
              disabled={testingSmtp || !smtpHost.trim() || !smtpUser.trim()}
              onClick={testSmtp}
              className="inline-flex items-center gap-1.5 px-4 py-2 text-xs font-medium rounded-lg border border-slate-200 dark:border-slate-700 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-45"
            >
              {testingSmtp ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <CheckCircle2 className="w-3.5 h-3.5 text-emerald-500" />}
              {testingSmtp ? t('settings.smtp.testing') : t('settings.smtp.testBtn')}
            </button>
          </div>
        </form>
      )}

      {subTab === 'general' && settings && (
        <form onSubmit={saveGeneral} className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-6 space-y-4 max-w-2xl">
          <div>
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('settings.general.title')}</h2>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-1">{t('settings.general.desc')}</p>
          </div>
          <div>
            <label htmlFor="set-pub-url" className="block text-xs font-medium mb-1">{t('settings.general.publicBaseUrl')}</label>
            <input
              id="set-pub-url"
              value={settings.public_base_url}
              onChange={(e) => setSettings({ ...settings, public_base_url: e.target.value })}
              className="w-full px-3 py-2 text-xs border rounded-lg font-mono"
            />
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
            <div>
              <label htmlFor="set-timeout" className="block text-xs font-medium mb-1">{t('settings.general.sendTimeout')}</label>
              <input
                id="set-timeout"
                type="number"
                min={1}
                value={settings.send_timeout_sec}
                onChange={(e) => setSettings({ ...settings, send_timeout_sec: Number(e.target.value) })}
                className="w-full px-3 py-2 text-xs border rounded-lg"
              />
            </div>
            <div>
              <label htmlFor="set-rpm" className="block text-xs font-medium mb-1">{t('settings.general.defaultRpm')}</label>
              <input
                id="set-rpm"
                type="number"
                min={0}
                value={settings.default_rate_limit_rpm}
                onChange={(e) => setSettings({ ...settings, default_rate_limit_rpm: Number(e.target.value) })}
                className="w-full px-3 py-2 text-xs border rounded-lg"
              />
            </div>
            <div>
              <label htmlFor="set-body-max" className="block text-xs font-medium mb-1">{t('settings.general.maxBodyBytes')}</label>
              <input
                id="set-body-max"
                type="number"
                min={0}
                value={settings.max_body_bytes}
                onChange={(e) => setSettings({ ...settings, max_body_bytes: Number(e.target.value) })}
                className="w-full px-3 py-2 text-xs border rounded-lg"
              />
            </div>
          </div>
          <label className="flex items-center gap-2 text-xs">
            <input
              type="checkbox"
              checked={settings.store_mail_body}
              onChange={(e) => setSettings({ ...settings, store_mail_body: e.target.checked })}
            />
            <span>{t('settings.general.storeBody')}</span>
          </label>
          <button type="submit" disabled={savingGeneral || !generalDirty} className="primary-button">
            {savingGeneral ? t('common.saving') : t('common.save')}
          </button>
        </form>
      )}

      {subTab === 'admins' && (
        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div className="space-y-6">
            <form onSubmit={changeOwnPassword} className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-3">
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('settings.admins.ownPasswordTitle')}</h2>
              <div>
                <label htmlFor="own-old" className="block text-xs mb-1">{t('settings.admins.oldPassword')}</label>
                <input id="own-old" type="password" required value={oldPwd} onChange={(e) => setOldPwd(e.target.value)} className="w-full px-3 py-2 text-xs border rounded-lg" />
              </div>
              <div>
                <label htmlFor="own-new" className="block text-xs mb-1">{t('settings.admins.newPassword')}</label>
                <input id="own-new" type="password" required minLength={8} value={newPwd} onChange={(e) => setNewPwd(e.target.value)} className="w-full px-3 py-2 text-xs border rounded-lg" />
              </div>
              <button type="submit" disabled={changingPwd || !oldPwd || !newPwd} className="primary-button">
                {t('settings.admins.changePwdBtn')}
              </button>
            </form>

            <form onSubmit={createAdmin} className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-3">
              <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('settings.admins.createTitle')}</h2>
              <div>
                <label htmlFor="new-adm-u" className="block text-xs mb-1">{t('settings.admins.username')}</label>
                <input id="new-adm-u" required minLength={3} value={newAdminUser} onChange={(e) => setNewAdminUser(e.target.value)} className="w-full px-3 py-2 text-xs border rounded-lg" />
              </div>
              <div>
                <label htmlFor="new-adm-p" className="block text-xs mb-1">{t('settings.admins.password')}</label>
                <input id="new-adm-p" type="password" required minLength={8} value={newAdminPass} onChange={(e) => setNewAdminPass(e.target.value)} className="w-full px-3 py-2 text-xs border rounded-lg" />
              </div>
              <button type="submit" disabled={creatingAdmin || !newAdminUser.trim() || !newAdminPass} className="primary-button">
                {t('settings.admins.createBtn')}
              </button>
            </form>
          </div>

          <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-3">
            <h2 className="text-sm font-semibold text-slate-900 dark:text-slate-100">{t('settings.admins.listTitle')}</h2>
            <div className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
              {admins.map((a) => (
                <div key={a.id} className="py-3 flex items-center justify-between gap-2">
                  <div>
                    <div className="font-medium text-slate-900 dark:text-slate-100">{a.username}</div>
                    <div className="text-[11px] text-slate-400">{formatTime(a.last_login_at, lang)}</div>
                  </div>
                  <div className="flex items-center gap-2">
                    <StatusBadge value={a.is_active ? 'enabled' : 'disabled'} />
                    <button type="button" onClick={() => { setResetAdmin(a); setResetPwd('') }} className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700">
                      {t('settings.admins.resetPwdBtn')}
                    </button>
                    <button type="button" onClick={() => toggleAdminActive(a)} className="px-2.5 py-1 rounded-lg border border-slate-200 dark:border-slate-700">
                      {a.is_active ? t('settings.admins.disable') : t('settings.admins.enable')}
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {resetAdmin && (
        <Modal title={t('settings.admins.resetPwdTitle', { name: resetAdmin.username })} onClose={() => !resetting && setResetAdmin(null)} busy={resetting} size="md">
          <form onSubmit={submitResetPassword} className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-4 text-xs">
            <h2 className="text-base font-semibold">{t('settings.admins.resetPwdTitle', { name: resetAdmin.username })}</h2>
            <div>
              <label htmlFor="reset-adm-pwd" className="block mb-1">{t('settings.admins.newPassword')}</label>
              <input id="reset-adm-pwd" type="password" required minLength={8} value={resetPwd} onChange={(e) => setResetPwd(e.target.value)} className="w-full px-3 py-2 border rounded-lg" />
            </div>
            <div className="flex justify-end gap-2">
              <button type="button" onClick={() => setResetAdmin(null)} className="px-3.5 py-2 rounded-lg border border-slate-200 dark:border-slate-700">{t('common.cancel')}</button>
              <button type="submit" disabled={resetting || !resetPwd} className="px-4 py-2 rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white disabled:opacity-45">{t('common.save')}</button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
