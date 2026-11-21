import React, { useState } from 'react'
import { Download, ShieldAlert, Upload } from 'lucide-react'
import { api, formatTime } from '../api'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import type { InspectSummary } from '../types'

export const MigrationPage: React.FC = () => {
  const toast = useToast()
  const { t, lang } = useI18n()

  const [expPass, setExpPass] = useState('')
  const [expMailLogs, setExpMailLogs] = useState(false)
  const [expAdmins, setExpAdmins] = useState(true)
  const [expAudit, setExpAudit] = useState(false)
  const [exporting, setExporting] = useState(false)

  const [impFile, setImpFile] = useState<File | null>(null)
  const [impPass, setImpPass] = useState('')
  const [impMode, setImpMode] = useState<'merge' | 'replace'>('merge')
  const [impConflict, setImpConflict] = useState<'skip' | 'overwrite'>('skip')
  const [impMailLogs, setImpMailLogs] = useState(false)
  const [impAdmins, setImpAdmins] = useState(false)
  const [impAudit, setImpAudit] = useState(false)
  const [inspectRes, setInspectRes] = useState<InspectSummary | null>(null)
  const [inspecting, setInspecting] = useState(false)
  const [importing, setImporting] = useState(false)

  const handleExport = async (e: React.FormEvent) => {
    e.preventDefault()
    if (expPass !== '' && expPass.length < 6) {
      toast.error(t('migration.exportPassError'))
      return
    }
    setExporting(true)
    try {
      await api.exportMigration({
        include_mail_logs: expMailLogs,
        include_admins: expAdmins,
        include_audit_logs: expAudit,
        passphrase: expPass,
      })
      toast.success(t('migration.exportSuccess'))
    } catch (e: any) {
      toast.error(e.message || t('migration.exportError'))
    } finally {
      setExporting(false)
    }
  }

  const handleInspect = async () => {
    if (!impFile) return
    setInspecting(true)
    const fd = new FormData()
    fd.append('bundle', impFile)
    fd.append('passphrase', impPass)
    try {
      const summary = await api.inspectMigration(fd)
      setInspectRes(summary)
      setImpMailLogs(Boolean(summary.includes?.mail_logs))
      setImpAdmins(Boolean(summary.includes?.admins))
      setImpAudit(Boolean(summary.includes?.audit_logs))
      toast.info(t('migration.inspectDone'))
    } catch (e: any) {
      setInspectRes(null)
      toast.error(e.message || t('migration.inspectError'))
    } finally {
      setInspecting(false)
    }
  }

  const handleImport = async () => {
    if (!impFile || !inspectRes || importing) return
    if (impMode === 'replace' && !confirm(t('migration.replaceConfirm'))) return
    setImporting(true)
    const fd = new FormData()
    fd.append('bundle', impFile)
    fd.append('passphrase', impPass)
    fd.append('mode', impMode)
    fd.append('conflict_strategy', impConflict)
    fd.append('include_mail_logs', String(impMailLogs))
    fd.append('include_admins', String(impAdmins))
    fd.append('include_audit_logs', String(impAudit))
    try {
      const res = await api.importMigration(fd)
      toast.success(
        t('migration.importSuccess', {
          settings: res.counts.system_settings,
          clients: res.counts.clients,
          cron: res.counts.cron_jobs,
          admins: res.counts.admins,
          mails: res.counts.mail_logs,
        })
      )
      setInspectRes(null)
    } catch (e: any) {
      toast.error(e.message || t('migration.importError'))
    } finally {
      setImporting(false)
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('migration.title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400">{t('migration.subtitle')}</p>
      </div>

      <div className="flex flex-wrap items-center gap-3 rounded-xl border border-indigo-100 dark:border-indigo-500/30 bg-indigo-50/60 dark:bg-indigo-500/10 p-4 text-xs text-indigo-800 dark:text-indigo-200">
        <span className="font-semibold">{t('migration.flowLabel')}</span>
        <span>{t('migration.flowStep1')}</span>
        <span aria-hidden="true">→</span>
        <span>{t('migration.flowStep2')}</span>
        <span aria-hidden="true">→</span>
        <span>{t('migration.flowStep3')}</span>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-4">
          <div className="flex items-center gap-2 font-semibold text-slate-900 dark:text-slate-100">
            <Download className="w-4 h-4 text-indigo-600 dark:text-indigo-400" />
            <span>{t('migration.exportTitle')}</span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400">{t('migration.exportDesc')}</p>
          <form onSubmit={handleExport} className="space-y-3 text-sm">
            <div className="space-y-1.5 text-xs text-slate-700 dark:text-slate-300">
              <div className="font-medium text-slate-700 dark:text-slate-200">{t('migration.exportRequiredNote')}</div>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={expMailLogs} onChange={(e) => setExpMailLogs(e.target.checked)} />
                <span>{t('migration.exportIncludeMailLogs')}</span>
              </label>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={expAdmins} onChange={(e) => setExpAdmins(e.target.checked)} />
                <span>{t('migration.exportIncludeAdmins')}</span>
              </label>
              <label className="flex items-center gap-2">
                <input type="checkbox" checked={expAudit} onChange={(e) => setExpAudit(e.target.checked)} />
                <span>{t('migration.exportIncludeAudit')}</span>
              </label>
            </div>
            <div>
              <label htmlFor="mig-exp-pass" className="block text-xs text-slate-600 dark:text-slate-300 mb-1">
                {t('migration.exportPassLabel')}
              </label>
              <input
                id="mig-exp-pass"
                type="password"
                value={expPass}
                onChange={(e) => setExpPass(e.target.value)}
                placeholder={t('migration.exportPassPlaceholder')}
                className="w-full px-3 py-2 border rounded-lg text-xs"
              />
            </div>
            <button type="submit" disabled={exporting} className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white rounded-lg font-medium text-xs">
              {exporting ? t('migration.exporting') : t('migration.exportBtn')}
            </button>
          </form>
        </div>

        <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 space-y-4">
          <div className="flex items-center gap-2 font-semibold text-slate-900 dark:text-slate-100">
            <Upload className="w-4 h-4 text-emerald-600 dark:text-emerald-400" />
            <span>{t('migration.importTitle')}</span>
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400">{t('migration.importDesc')}</p>
          <div className="space-y-3 text-sm">
            <div>
              <label htmlFor="mig-imp-file" className="block text-xs text-slate-600 dark:text-slate-300 mb-1">
                {t('migration.importFileLabel')}
              </label>
              <input
                id="mig-imp-file"
                type="file"
                accept=".msbundle"
                disabled={inspecting || importing}
                onChange={(e) => { setImpFile(e.target.files?.[0] || null); setInspectRes(null) }}
                className="w-full text-xs"
              />
            </div>
            <div>
              <label htmlFor="mig-imp-pass" className="block text-xs text-slate-600 dark:text-slate-300 mb-1">
                {t('migration.importPassLabel')}
              </label>
              <input
                id="mig-imp-pass"
                type="password"
                autoComplete="off"
                disabled={inspecting || importing}
                value={impPass}
                onChange={(e) => { setImpPass(e.target.value); setInspectRes(null) }}
                placeholder={t('migration.importPassPlaceholder')}
                className="w-full px-3 py-2 border rounded-lg text-xs"
              />
            </div>
            <div className="grid grid-cols-2 gap-3 text-xs">
              <div>
                <label htmlFor="mig-imp-mode" className="block text-slate-600 dark:text-slate-300 mb-1">{t('migration.importModeLabel')}</label>
                <select id="mig-imp-mode" value={impMode} onChange={(e) => setImpMode(e.target.value as any)} className="w-full px-2.5 py-1.5 border rounded-lg bg-white dark:bg-slate-900">
                  <option value="merge">{t('migration.importModeMerge')}</option>
                  <option value="replace">{t('migration.importModeReplace')}</option>
                </select>
              </div>
              {impMode === 'merge' && (
                <div>
                  <label htmlFor="mig-imp-conflict" className="block text-slate-600 dark:text-slate-300 mb-1">{t('migration.importConflictLabel')}</label>
                  <select id="mig-imp-conflict" value={impConflict} onChange={(e) => setImpConflict(e.target.value as any)} className="w-full px-2.5 py-1.5 border rounded-lg bg-white dark:bg-slate-900">
                    <option value="skip">{t('migration.importConflictSkip')}</option>
                    <option value="overwrite">{t('migration.importConflictOverwrite')}</option>
                  </select>
                </div>
              )}
            </div>
            <div className="flex flex-wrap gap-4 text-xs text-slate-700 dark:text-slate-300">
              <label className={`flex items-center gap-1.5 ${inspectRes && !inspectRes.includes?.mail_logs ? 'opacity-45' : ''}`}>
                <input
                  type="checkbox"
                  checked={impMailLogs}
                  disabled={Boolean(inspectRes && !inspectRes.includes?.mail_logs)}
                  onChange={(e) => setImpMailLogs(e.target.checked)}
                />
                {t('migration.importIncludeMailLogs')}
              </label>
              <label className={`flex items-center gap-1.5 ${inspectRes && !inspectRes.includes?.admins ? 'opacity-45' : ''}`}>
                <input
                  type="checkbox"
                  checked={impAdmins}
                  disabled={Boolean(inspectRes && !inspectRes.includes?.admins)}
                  onChange={(e) => setImpAdmins(e.target.checked)}
                />
                {t('migration.importIncludeAdmins')}
              </label>
              <label className={`flex items-center gap-1.5 ${inspectRes && !inspectRes.includes?.audit_logs ? 'opacity-45' : ''}`}>
                <input
                  type="checkbox"
                  checked={impAudit}
                  disabled={Boolean(inspectRes && !inspectRes.includes?.audit_logs)}
                  onChange={(e) => setImpAudit(e.target.checked)}
                />
                {t('migration.importIncludeAudit')}
              </label>
            </div>
            <p className="text-xs text-slate-500 dark:text-slate-400">
              {inspectRes ? t('migration.importHintReady') : t('migration.importHintPending')}
            </p>
            <div className="flex flex-wrap gap-2 text-xs">
              <button type="button" onClick={handleInspect} disabled={!impFile || inspecting || importing} className="px-4 py-2 border border-slate-200 dark:border-slate-700 rounded-lg hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-50">
                {inspecting ? t('migration.inspecting') : t('migration.inspectBtn')}
              </button>
              <button type="button" onClick={handleImport} disabled={!impFile || !inspectRes || inspecting || importing} className="px-4 py-2 bg-emerald-600 hover:bg-emerald-700 text-white rounded-lg font-medium disabled:opacity-50">
                {importing ? t('migration.importing') : t('migration.importBtn')}
              </button>
            </div>
          </div>

          {inspectRes && (
            <div className="p-3 rounded-lg bg-slate-50 dark:bg-slate-800/70 border border-slate-200 dark:border-slate-700 text-xs space-y-1.5">
              <div className="font-semibold">{t('migration.inspectSummaryTitle', { time: formatTime(inspectRes.exported_at, lang) })}</div>
              <div>
                {t('migration.inspectCounts', {
                  settings: inspectRes.counts.system_settings,
                  clients: inspectRes.counts.clients,
                  cron: inspectRes.counts.cron_jobs,
                  admins: inspectRes.counts.admins,
                  mails: inspectRes.counts.mail_logs,
                  audit: inspectRes.counts.audit_logs,
                })}
              </div>
              {inspectRes.client_conflicts?.length || inspectRes.admin_conflicts?.length ? (
                <div className="text-amber-700 dark:text-amber-300 flex items-start gap-1">
                  <ShieldAlert className="w-4 h-4 shrink-0 mt-0.5" />
                  <span>
                    {t('migration.conflictsDetected')}
                    {[
                      inspectRes.client_conflicts?.length ? `Clients: ${inspectRes.client_conflicts.join(', ')}` : '',
                      inspectRes.admin_conflicts?.length ? `Admins: ${inspectRes.admin_conflicts.join(', ')}` : '',
                    ].filter(Boolean).join(' · ')}
                  </span>
                </div>
              ) : (
                <div className="text-emerald-700 dark:text-emerald-400">{t('migration.noConflicts')}</div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
