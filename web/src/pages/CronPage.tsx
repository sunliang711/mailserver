import React, { useEffect, useMemo, useRef, useState } from 'react'
import { CheckCircle2, Loader2, Plus, RefreshCw, X } from 'lucide-react'
import { api, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { Drawer } from '../components/Drawer'
import { LoadingState } from '../components/Feedback'
import { SecretField } from '../components/SecretField'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import type { TranslationKey } from '../i18n/zh'
import type { CronJob, CronJobRun, SafeKeepUpstream } from '../types'
import { describeCron } from '../utils/cron'

interface PresetOption {
  labelKey: TranslationKey
  value: string
}

const SCHEDULE_PRESETS: PresetOption[] = [
  { labelKey: 'cron.preset.every1m', value: '* * * * *' },
  { labelKey: 'cron.preset.every5m', value: '*/5 * * * *' },
  { labelKey: 'cron.preset.every10m', value: '*/10 * * * *' },
  { labelKey: 'cron.preset.hourly', value: '0 * * * *' },
  { labelKey: 'cron.preset.daily', value: '0 0 * * *' },
  { labelKey: 'cron.preset.weekly', value: '0 0 * * 1' },
  { labelKey: 'cron.preset.monthly', value: '0 0 1 * *' },
  { labelKey: 'cron.preset.custom', value: 'custom' },
]

const JOB_I18N: Record<string, { name: TranslationKey; desc: TranslationKey }> = {
  cleanup_audit_logs: { name: 'cron.job.cleanup_audit_logs.name', desc: 'cron.job.cleanup_audit_logs.desc' },
  cleanup_mail_records: { name: 'cron.job.cleanup_mail_records.name', desc: 'cron.job.cleanup_mail_records.desc' },
  smtp_health_check: { name: 'cron.job.smtp_health_check.name', desc: 'cron.job.smtp_health_check.desc' },
  migration_backup: { name: 'cron.job.migration_backup.name', desc: 'cron.job.migration_backup.desc' },
}

interface MigrationBackupForm {
  api_base_url: string
  api_key: string
  has_api_key: boolean
  clear_api_key: boolean
  targets: string[]
  path: string
  mode: 'sync' | 'async'
  rename: string
  passphrase: string
  has_passphrase: boolean
  clear_passphrase: boolean
  include_admins: boolean
  include_mail_logs: boolean
  include_audit_logs: boolean
}

type StructuredParams =
  | { kind: 'cleanup_audit_logs'; data: { retention_days: number } }
  | { kind: 'cleanup_mail_records'; data: { retention_days: number } }
  | { kind: 'smtp_health_check'; data: { probe_timeout_sec: number } }
  | { kind: 'migration_backup'; data: MigrationBackupForm }
  | { kind: 'raw'; data: string }

function safeParseJSON(raw?: string): Record<string, any> {
  try {
    return JSON.parse(raw || '{}') || {}
  } catch {
    return {}
  }
}

function parseMigrationBackupForm(raw?: string): MigrationBackupForm {
  const obj = safeParseJSON(raw)
  const rawTargets = typeof obj.targets === 'string' ? obj.targets : ''
  const targets = rawTargets.split(',').map((s: string) => s.trim()).filter(Boolean)
  return {
    api_base_url: obj.api_base_url || '',
    api_key: '',
    has_api_key: Boolean(obj.has_api_key),
    clear_api_key: false,
    targets,
    path: obj.path ?? 'mailserver-backups',
    mode: obj.mode === 'async' ? 'async' : 'sync',
    rename: obj.rename || '',
    passphrase: '',
    has_passphrase: Boolean(obj.has_passphrase),
    clear_passphrase: false,
    include_admins: obj.include_admins !== undefined ? Boolean(obj.include_admins) : true,
    include_mail_logs: Boolean(obj.include_mail_logs),
    include_audit_logs: Boolean(obj.include_audit_logs),
  }
}

function parseStructuredParams(job: CronJob): StructuredParams {
  const obj = safeParseJSON(job.config_json)
  switch (job.job_key) {
    case 'cleanup_audit_logs':
      return { kind: 'cleanup_audit_logs', data: { retention_days: Number(obj.retention_days) > 0 ? Number(obj.retention_days) : 90 } }
    case 'cleanup_mail_records':
      return { kind: 'cleanup_mail_records', data: { retention_days: Number(obj.retention_days) > 0 ? Number(obj.retention_days) : 90 } }
    case 'smtp_health_check':
      return { kind: 'smtp_health_check', data: { probe_timeout_sec: Number(obj.probe_timeout_sec) > 0 ? Number(obj.probe_timeout_sec) : 15 } }
    case 'migration_backup':
      return { kind: 'migration_backup', data: parseMigrationBackupForm(job.config_json) }
    default:
      return { kind: 'raw', data: job.config_json || '{}' }
  }
}

function formatDuration(startedAt: string, finishedAt?: string): string {
  if (!finishedAt) return ''
  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime()
  if (!Number.isFinite(ms) || ms < 0) return ''
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const m = Math.floor(ms / 60_000)
  const s = Math.round((ms % 60_000) / 1000)
  return `${m}m${s}s`
}

const Switch: React.FC<{
  checked: boolean
  disabled?: boolean
  label?: string
  onChange: (next: boolean) => void
}> = ({ checked, disabled, label, onChange }) => (
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    aria-label={label}
    disabled={disabled}
    onClick={() => onChange(!checked)}
    className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-indigo-500 disabled:opacity-50 ${
      checked ? 'bg-indigo-600' : 'bg-slate-200 hover:bg-slate-300 dark:bg-slate-700 dark:hover:bg-slate-600'
    }`}
  >
    <span
      className={`inline-block h-3.5 w-3.5 transform rounded-full bg-white shadow-sm transition-transform ${
        checked ? 'translate-x-[18px]' : 'translate-x-[3px]'
      }`}
    />
  </button>
)

export const CronPage: React.FC = () => {
  const toast = useToast()
  const { t, lang } = useI18n()
  const [loading, setLoading] = useState(true)
  const [jobs, setJobs] = useState<CronJob[]>([])
  const [runningId, setRunningId] = useState<string | null>(null)
  const [togglingId, setTogglingId] = useState<string | null>(null)

  const [editing, setEditing] = useState<CronJob | null>(null)
  const [preset, setPreset] = useState('custom')
  const [cronExpr, setCronExpr] = useState('')
  const [enabled, setEnabled] = useState(true)
  const [timeoutSec, setTimeoutSec] = useState(300)
  const [params, setParams] = useState<StructuredParams | null>(null)
  const [initialSnap, setInitialSnap] = useState('')
  const [previewRaw, setPreviewRaw] = useState<string[]>([])
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewError, setPreviewError] = useState('')
  const [formError, setFormError] = useState('')
  const [saving, setSaving] = useState(false)
  const [targetInput, setTargetInput] = useState('')
  const [testingSK, setTestingSK] = useState(false)
  const [skUpstreams, setSkUpstreams] = useState<SafeKeepUpstream[]>([])
  const previewSeq = useRef(0)

  const [runsJob, setRunsJob] = useState<CronJob | null>(null)
  const [runs, setRuns] = useState<CronJobRun[]>([])
  const [runsLoading, setRunsLoading] = useState(false)
  const [runsError, setRunsError] = useState('')

  const jobName = (job: CronJob) => (JOB_I18N[job.job_key] ? t(JOB_I18N[job.job_key].name) : job.name)
  const jobDesc = (job: CronJob) => (JOB_I18N[job.job_key] ? t(JOB_I18N[job.job_key].desc) : job.description)

  const load = async () => {
    try {
      const res = await api.listCronJobs()
      setJobs(res.items || [])
    } catch (err: any) {
      toast.error(t('cron.loadError', { error: err.message }))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const openEdit = (job: CronJob, overrideEnabled?: boolean) => {
    const matched = SCHEDULE_PRESETS.find((item) => item.value === job.cron_expr)?.value ?? 'custom'
    const parsedParams = parseStructuredParams(job)
    const nextEnabled = overrideEnabled ?? job.enabled
    const nextTimeout = job.timeout_sec > 0 ? job.timeout_sec : 300

    setEditing(job)
    setPreset(matched)
    setCronExpr(job.cron_expr)
    setEnabled(nextEnabled)
    setTimeoutSec(nextTimeout)
    setParams(parsedParams)
    setTargetInput('')
    setSkUpstreams([])
    setFormError('')
    setPreviewError('')
    setPreviewRaw(job.next_runs || [])
    setInitialSnap(
      JSON.stringify({
        cronExpr: job.cron_expr,
        enabled: job.enabled,
        timeoutSec: nextTimeout,
        params: parsedParams,
      })
    )
  }

  useEffect(() => {
    if (!editing) return
    const expr = cronExpr.trim()
    if (!expr) {
      setPreviewRaw([])
      setPreviewError(t('cron.errEmptyExpr'))
      return
    }
    const seq = ++previewSeq.current
    const timer = window.setTimeout(async () => {
      setPreviewLoading(true)
      try {
        const res = await api.previewCronSchedule(expr)
        if (previewSeq.current !== seq) return
        setPreviewRaw(res.next_runs || [])
        setPreviewError('')
      } catch (err: any) {
        if (previewSeq.current !== seq) return
        setPreviewRaw([])
        setPreviewError(err.message || t('cron.errInvalidExpr'))
      } finally {
        if (previewSeq.current === seq) setPreviewLoading(false)
      }
    }, 180)
    return () => window.clearTimeout(timer)
  }, [editing?.id, cronExpr])

  const currentSnap = useMemo(
    () => JSON.stringify({ cronExpr, enabled, timeoutSec, params }),
    [cronExpr, enabled, timeoutSec, params]
  )
  const isDirty = Boolean(editing && currentSnap !== initialSnap)

  const closeEdit = () => {
    if (saving) return
    if (isDirty && !confirm(t('cron.discardConfirm'))) return
    setEditing(null)
    setParams(null)
    setFormError('')
  }

  const handleToggle = async (job: CronJob, nextEnabled: boolean) => {
    if (togglingId) return
    if (nextEnabled && job.job_key === 'migration_backup') {
      const parsed = parseMigrationBackupForm(job.config_json)
      if (!parsed.api_base_url || !parsed.has_api_key) {
        toast.info(t('cron.needConfigBeforeEnable'))
        openEdit(job, true)
        return
      }
    }
    setTogglingId(job.id)
    try {
      await api.updateCronJob(job.id, {
        cron_expr: job.cron_expr,
        enabled: nextEnabled,
        timeout_sec: job.timeout_sec,
      })
      await load()
    } catch (err: any) {
      toast.error(err.message || t('cron.toggleError'))
    } finally {
      setTogglingId(null)
    }
  }

  const handleSave = async (e?: React.FormEvent) => {
    e?.preventDefault()
    if (!editing || !params || previewError) return
    setFormError('')

    const cleanTimeout = Number(timeoutSec)
    if (!Number.isInteger(cleanTimeout) || cleanTimeout < 1 || cleanTimeout > 86400) {
      setFormError(t('cron.errTimeout'))
      return
    }

    let cfg: any
    switch (params.kind) {
      case 'cleanup_audit_logs':
      case 'cleanup_mail_records': {
        const days = Number(params.data.retention_days)
        if (!Number.isInteger(days) || days < 1) {
          setFormError(t('cron.errRetentionDays'))
          return
        }
        cfg = { retention_days: days }
        break
      }
      case 'smtp_health_check': {
        const probeTimeout = Number(params.data.probe_timeout_sec)
        if (!Number.isInteger(probeTimeout) || probeTimeout < 1 || probeTimeout > 120) {
          setFormError(t('cron.errProbeTimeout'))
          return
        }
        cfg = { probe_timeout_sec: probeTimeout }
        break
      }
      case 'migration_backup': {
        const mb = params.data
        if (mb.passphrase && mb.passphrase.length < 6) {
          setFormError(t('cron.errPassphraseMin'))
          return
        }
        if (enabled && (!mb.api_base_url.trim() || (!mb.has_api_key && !mb.api_key.trim()))) {
          setFormError(t('cron.errNeedSafeKeep'))
          return
        }
        const pendingTag = targetInput.trim()
        const finalTargets = pendingTag && !mb.targets.includes(pendingTag) ? [...mb.targets, pendingTag] : mb.targets
        cfg = {
          api_base_url: mb.api_base_url.trim(),
          targets: finalTargets.join(','),
          path: mb.path.trim(),
          mode: mb.mode,
          rename: mb.rename.trim(),
          include_admins: mb.include_admins,
          include_mail_logs: mb.include_mail_logs,
          include_audit_logs: mb.include_audit_logs,
        }
        if (mb.clear_api_key) {
          cfg.api_key = ''
        } else if (mb.api_key.trim()) {
          cfg.api_key = mb.api_key.trim()
        }
        if (mb.clear_passphrase) {
          cfg.passphrase = ''
        } else if (mb.passphrase) {
          cfg.passphrase = mb.passphrase
        }
        break
      }
      case 'raw':
        try {
          cfg = JSON.parse(params.data || '{}')
        } catch (err: any) {
          setFormError(err.message)
          return
        }
        break
    }

    setSaving(true)
    try {
      await api.updateCronJob(editing.id, {
        cron_expr: cronExpr.trim(),
        enabled,
        timeout_sec: cleanTimeout,
        config: cfg,
      })
      setEditing(null)
      setParams(null)
      toast.success(t('common.saved'))
      await load()
    } catch (err: any) {
      setFormError(err.message || t('cron.saveError'))
    } finally {
      setSaving(false)
    }
  }

  const handleRunNow = async (job: CronJob) => {
    if (runningId) return
    const displayName = jobName(job)
    setRunningId(job.id)
    toast.info(t('cron.runningToast', { name: displayName }))
    try {
      const run = await api.runCronJob(job.id)
      const detail = run.output_log ? (lang === 'en' ? ` (${run.output_log})` : `（${run.output_log}）`) : ''
      if (run.status === 'succeeded') {
        toast.success(t('cron.runSucceededToast', { name: displayName, detail }))
      } else if (run.status === 'skipped') {
        toast.info(t('cron.runSkippedToast', { name: displayName, detail }))
      } else {
        toast.error(t('cron.runFailedToast', { name: displayName, detail }))
      }
      await load()
      if (runsJob?.id === job.id) {
        await loadRuns(job)
      }
    } catch (err: any) {
      toast.error(t('cron.runErrorToast', { error: err.message }))
    } finally {
      setRunningId(null)
    }
  }

  const loadRuns = async (job: CronJob) => {
    setRunsError('')
    setRunsLoading(true)
    try {
      const res = await api.listCronRuns(job.id)
      setRuns(res.items || [])
    } catch (err: any) {
      setRunsError(err.message || t('cron.loadRunsError'))
    } finally {
      setRunsLoading(false)
    }
  }

  const openRuns = (job: CronJob) => {
    setRunsJob(job)
    setRuns([])
    void loadRuns(job)
  }

  const mbForm = params?.kind === 'migration_backup' ? params.data : null
  const setMbForm = (next: MigrationBackupForm) => setParams({ kind: 'migration_backup', data: next })

  const handleTestSafeKeep = async () => {
    if (!editing || !mbForm || testingSK) return
    setTestingSK(true)
    setFormError('')
    try {
      const res = await api.testSafeKeep({
        job_id: editing.id,
        api_base_url: mbForm.api_base_url.trim(),
        api_key: mbForm.api_key.trim(),
      })
      const ups = res.upstreams || []
      setSkUpstreams(ups)
      toast.success(t('cron.mb.testOk', { count: ups.length }))
    } catch (err: any) {
      setFormError(err.message)
    } finally {
      setTestingSK(false)
    }
  }

  const addTargetTag = (raw: string) => {
    if (!mbForm) return
    const items = raw.split(',').map((s) => s.trim()).filter(Boolean)
    if (items.length === 0) return
    const next = [...mbForm.targets]
    for (const item of items) {
      if (!next.includes(item)) next.push(item)
    }
    setMbForm({ ...mbForm, targets: next })
    setTargetInput('')
  }

  const removeTargetTag = (name: string) => {
    if (!mbForm) return
    setMbForm({ ...mbForm, targets: mbForm.targets.filter((item) => item !== name) })
  }

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('cron.title')}</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400">{t('cron.subtitle')}</p>
      </div>

      <div className="bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 overflow-x-auto">
        <table className="cron-table w-full text-sm">
          <thead className="text-xs text-slate-500 dark:text-slate-400 border-b border-slate-200/80 dark:border-slate-800">
            <tr>
              <th className="px-4 py-3">{t('cron.colJob')}</th>
              <th className="px-4 py-3">{t('cron.colSchedule')}</th>
              <th className="px-4 py-3">{t('cron.colNextRun')}</th>
              <th className="px-4 py-3">{t('cron.colLastRun')}</th>
              <th className="px-4 py-3">{t('cron.colEnabled')}</th>
              <th className="px-4 py-3 text-right">{t('cron.colActions')}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
            {loading && <tr><td colSpan={6}><LoadingState label={t('cron.loading')} /></td></tr>}
            {!loading && jobs.length === 0 && (
              <tr><td colSpan={6} className="p-8 text-center text-slate-500">{t('cron.empty')}</td></tr>
            )}
            {jobs.map((j) => {
              const scheduleText = describeCron(j.cron_expr, lang)
              const nextTime = j.enabled ? j.next_runs?.[0] || j.next_run_at : undefined
              const isRunning = runningId === j.id
              const displayName = jobName(j)
              return (
                <tr key={j.id}>
                  <td className="px-4 py-3.5 align-top">
                    <div className="font-medium text-slate-900 dark:text-slate-100">{displayName}</div>
                    <div className="text-xs text-slate-400 mt-0.5 leading-relaxed">{jobDesc(j)}</div>
                  </td>
                  <td className="px-4 py-3.5 align-top">
                    {scheduleText && <div className="text-xs text-slate-700 dark:text-slate-300 mb-1">{scheduleText}</div>}
                    <code className="font-mono text-[11px] bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 px-1.5 py-0.5 rounded inline-block">
                      {j.cron_expr}
                    </code>
                  </td>
                  <td className="px-4 py-3.5 align-top text-xs text-slate-600 dark:text-slate-300 whitespace-nowrap">
                    {nextTime ? formatTime(nextTime, lang) : <span className="text-slate-400">—</span>}
                  </td>
                  <td className="px-4 py-3.5 align-top text-xs">
                    {!j.last_run_at ? (
                      <span className="text-slate-400">—</span>
                    ) : (
                      <div className="space-y-1">
                        <div className="flex items-center gap-2 whitespace-nowrap">
                          <StatusBadge value={j.last_status} />
                          <span className="text-slate-600 dark:text-slate-300">{formatTime(j.last_run_at, lang)}</span>
                        </div>
                        {j.last_summary && (
                          <div className="text-[11px] text-slate-400 max-w-[240px] truncate" title={j.last_summary}>
                            {j.last_summary}
                          </div>
                        )}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3.5 align-top">
                    <div className="pt-0.5">
                      <Switch checked={j.enabled} disabled={togglingId === j.id} label={t('cron.toggleAria', { name: displayName })} onChange={(next) => handleToggle(j, next)} />
                    </div>
                  </td>
                  <td className="px-4 py-3.5 align-top text-right whitespace-nowrap">
                    <div className="inline-flex items-center gap-1.5">
                      <button
                        type="button"
                        disabled={Boolean(runningId)}
                        onClick={() => handleRunNow(j)}
                        className="px-2.5 py-1 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700 disabled:opacity-45 inline-flex items-center gap-1"
                      >
                        {isRunning && <Loader2 className="w-3 h-3 animate-spin" />}
                        {isRunning ? t('cron.running') : t('cron.runNow')}
                      </button>
                      <button
                        type="button"
                        onClick={() => openEdit(j)}
                        className="px-2.5 py-1 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700"
                      >
                        {t('common.edit')}
                      </button>
                      <button
                        type="button"
                        onClick={() => openRuns(j)}
                        className="px-2.5 py-1 text-xs rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800 hover:bg-slate-50 dark:hover:bg-slate-700"
                      >
                        {t('cron.runsBtn')}
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>

      {editing && params && (
        <Drawer
          title={jobName(editing)}
          subtitle={jobDesc(editing)}
          onClose={closeEdit}
          busy={saving}
          width={editing.job_key === 'migration_backup' ? 'lg' : 'md'}
          footer={
            <>
              <button type="button" disabled={saving} onClick={closeEdit} className="px-3.5 py-1.5 text-xs font-medium rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-800">
                {t('common.cancel')}
              </button>
              <button type="button" disabled={saving || Boolean(previewError) || !isDirty} onClick={() => handleSave()} className="px-4 py-1.5 text-xs font-medium rounded-lg bg-indigo-600 hover:bg-indigo-700 text-white disabled:opacity-45">
                {saving ? t('common.saving') : t('common.save')}
              </button>
            </>
          }
        >
          <form onSubmit={handleSave} className="space-y-4 text-sm">
            {formError && (
              <div role="alert" className="rounded-lg border border-rose-200 dark:border-rose-500/30 bg-rose-50 dark:bg-rose-500/15 px-3 py-2 text-xs text-rose-700 dark:text-rose-300">
                {formError}
              </div>
            )}

            <div>
              <label htmlFor="cron-preset" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('cron.fieldSchedule')}</label>
              <select
                id="cron-preset"
                value={preset}
                onChange={(e) => {
                  setPreset(e.target.value)
                  if (e.target.value !== 'custom') setCronExpr(e.target.value)
                }}
                className="w-full px-3 py-2 text-xs border rounded-lg bg-white dark:bg-slate-900"
              >
                {SCHEDULE_PRESETS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {opt.value === 'custom' ? t(opt.labelKey) : `${t(opt.labelKey)} (${opt.value})`}
                  </option>
                ))}
              </select>
            </div>

            {preset === 'custom' && (
              <div>
                <label htmlFor="cron-expr" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('cron.fieldExpr')}</label>
                <input
                  id="cron-expr"
                  required
                  value={cronExpr}
                  onChange={(e) => setCronExpr(e.target.value)}
                  className={`w-full px-3 py-2 text-xs border rounded-lg font-mono ${previewError ? 'border-rose-400' : ''}`}
                />
                <p className="text-[11px] text-slate-400 mt-1">
                  {t('cron.fieldExprHint')} <span className="font-mono">{editing.default_schedule || editing.cron_expr}</span>
                </p>
              </div>
            )}

            <div>
              <div className="flex items-center justify-between mb-1.5">
                <span className="text-xs font-medium text-slate-700 dark:text-slate-300">{t('cron.nextRuns')}</span>
                {previewLoading && <Loader2 className="w-3 h-3 animate-spin text-slate-400" />}
              </div>
              {previewError ? (
                <div className="text-xs text-rose-600 dark:text-rose-400">{previewError}</div>
              ) : (
                <div className="rounded-lg bg-slate-50 dark:bg-slate-800/60 border border-slate-200/70 dark:border-slate-700 px-3 py-2 space-y-1">
                  {previewRaw.length > 0 ? previewRaw.map((rt) => (
                    <div key={rt} className="text-xs font-mono text-slate-500 dark:text-slate-400">{formatTime(rt, lang)}</div>
                  )) : <div className="text-xs text-slate-400">—</div>}
                </div>
              )}
            </div>

            <div className="grid grid-cols-2 gap-3 items-start">
              <div>
                <label htmlFor="cron-timeout" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('cron.fieldTimeout')}</label>
                <input
                  id="cron-timeout"
                  type="number"
                  min={1}
                  max={86400}
                  value={timeoutSec}
                  onChange={(e) => setTimeoutSec(Number(e.target.value))}
                  className="w-full px-3 py-2 text-xs border rounded-lg"
                />
              </div>
              <div>
                <span className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1.5">{t('cron.fieldEnabled')}</span>
                <div className="h-[34px] flex items-center gap-2.5">
                  <Switch checked={enabled} label={t('cron.toggleScheduleAria')} onChange={setEnabled} />
                  <span className="text-xs text-slate-600 dark:text-slate-300">{enabled ? t('common.enabled') : t('cron.statusPaused')}</span>
                </div>
              </div>
            </div>

            {(params.kind === 'cleanup_audit_logs' || params.kind === 'cleanup_mail_records') && (
              <div className="pt-3 border-t border-slate-100 dark:border-slate-800">
                <label htmlFor="param-retention-days" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                  {t('cron.fieldRetentionDays')}
                </label>
                <input
                  id="param-retention-days"
                  type="number"
                  min={1}
                  max={36500}
                  value={params.data.retention_days}
                  onChange={(e) => setParams({ kind: params.kind, data: { retention_days: Number(e.target.value) } })}
                  className="w-full px-3 py-2 text-xs border rounded-lg"
                />
                <p className="text-[11px] text-slate-400 mt-1">
                  {params.kind === 'cleanup_audit_logs' ? t('cron.auditRetentionHint') : t('cron.mailRetentionHint')}
                </p>
              </div>
            )}

            {params.kind === 'smtp_health_check' && (
              <div className="pt-3 border-t border-slate-100 dark:border-slate-800">
                <label htmlFor="param-probe-timeout" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                  {t('cron.fieldProbeTimeout')}
                </label>
                <input
                  id="param-probe-timeout"
                  type="number"
                  min={1}
                  max={120}
                  value={params.data.probe_timeout_sec}
                  onChange={(e) => setParams({ kind: 'smtp_health_check', data: { probe_timeout_sec: Number(e.target.value) } })}
                  className="w-full px-3 py-2 text-xs border rounded-lg"
                />
                <p className="text-[11px] text-slate-400 mt-1">{t('cron.probeTimeoutHint')}</p>
              </div>
            )}

            {mbForm && (
              <div className="pt-3 border-t border-slate-100 dark:border-slate-800 space-y-3.5 text-xs">
                <div>
                  <label htmlFor="mb-base-url" className="block font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                    {t('cron.mb.baseUrl')}
                  </label>
                  <input
                    id="mb-base-url"
                    value={mbForm.api_base_url}
                    onChange={(e) => setMbForm({ ...mbForm, api_base_url: e.target.value })}
                    placeholder="http://localhost:8333"
                    className="w-full px-3 py-2 border rounded-lg font-mono"
                  />
                  <p className="text-[11px] text-slate-400 mt-1">{t('cron.mb.baseUrlHint')}</p>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label htmlFor="mb-api-key" className="block font-medium text-slate-700 dark:text-slate-300">
                      {t('cron.mb.apiKey')}
                    </label>
                    {mbForm.has_api_key && (
                      <button
                        type="button"
                        onClick={() => setMbForm({ ...mbForm, api_key: '', has_api_key: false, clear_api_key: true })}
                        className="text-[11px] text-rose-600 dark:text-rose-400 hover:underline"
                      >
                        {t('common.clear')}
                      </button>
                    )}
                  </div>
                  <SecretField
                    id="mb-api-key"
                    hasSaved={mbForm.has_api_key}
                    value={mbForm.api_key}
                    onChange={(v) => setMbForm({ ...mbForm, api_key: v, clear_api_key: false })}
                    onReveal={async () => (await api.revealCronSecrets(editing.id)).api_key || ''}
                    placeholder={mbForm.has_api_key ? t('cron.mb.apiKeySavedPlaceholder') : t('cron.mb.apiKeyPlaceholder')}
                  />
                  <div className="mt-2">
                    <button
                      type="button"
                      disabled={testingSK || !mbForm.api_base_url.trim() || (!mbForm.api_key.trim() && !mbForm.has_api_key)}
                      onClick={handleTestSafeKeep}
                      className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-indigo-200 dark:border-indigo-500/40 bg-indigo-50/70 dark:bg-indigo-500/15 text-indigo-700 dark:text-indigo-300 hover:bg-indigo-100 dark:hover:bg-indigo-500/25 disabled:opacity-45"
                    >
                      {testingSK ? <Loader2 className="w-3.5 h-3.5 animate-spin" /> : <CheckCircle2 className="w-3.5 h-3.5" />}
                      {testingSK ? t('cron.mb.testing') : t('cron.mb.testBtn')}
                    </button>
                  </div>
                </div>

                <div>
                  <label htmlFor="mb-path" className="block font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                    {t('cron.mb.path')}
                  </label>
                  <input
                    id="mb-path"
                    value={mbForm.path}
                    onChange={(e) => setMbForm({ ...mbForm, path: e.target.value })}
                    placeholder="mailserver-backups"
                    className="w-full px-3 py-2 border rounded-lg"
                  />
                  <p className="text-[11px] text-slate-400 mt-1">{t('cron.mb.pathHint')}</p>
                </div>

                <div>
                  <label htmlFor="mb-targets-input" className="block font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                    {t('cron.mb.targets')}
                  </label>
                  <div className="flex flex-wrap items-center gap-1.5 mb-1.5">
                    {mbForm.targets.map((tag) => (
                      <span
                        key={tag}
                        className="inline-flex items-center gap-1 px-2 py-0.5 rounded-md bg-indigo-50 dark:bg-indigo-500/15 border border-indigo-200 dark:border-indigo-500/30 text-indigo-700 dark:text-indigo-300 font-mono text-[11px]"
                      >
                        {tag}
                        <button type="button" onClick={() => removeTargetTag(tag)} className="text-indigo-400 hover:text-indigo-700">
                          <X className="w-3 h-3" />
                        </button>
                      </span>
                    ))}
                  </div>
                  <div className="flex gap-1.5">
                    <input
                      id="mb-targets-input"
                      value={targetInput}
                      onChange={(e) => setTargetInput(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault()
                          addTargetTag(targetInput)
                        }
                      }}
                      placeholder={t('cron.mb.targetPlaceholder')}
                      className="flex-1 px-3 py-1.5 border rounded-lg font-mono"
                    />
                    <button
                      type="button"
                      disabled={!targetInput.trim()}
                      onClick={() => addTargetTag(targetInput)}
                      className="px-2.5 py-1.5 border border-slate-200 dark:border-slate-700 rounded-lg disabled:opacity-40 inline-flex items-center gap-1"
                    >
                      <Plus className="w-3.5 h-3.5" /> {t('common.add')}
                    </button>
                  </div>
                  {skUpstreams.length > 0 && (
                    <div className="flex flex-wrap gap-1.5 mt-2">
                      {skUpstreams.map((u) => {
                        const active = mbForm.targets.includes(u.name) || mbForm.targets.includes(u.id)
                        return (
                          <button
                            key={u.id}
                            type="button"
                            onClick={() => {
                              if (active) {
                                setMbForm({ ...mbForm, targets: mbForm.targets.filter((x) => x !== u.name && x !== u.id) })
                              } else {
                                setMbForm({ ...mbForm, targets: [...mbForm.targets, u.name] })
                              }
                            }}
                            className={`px-2 py-0.5 rounded-md border text-[11px] ${
                              active
                                ? 'bg-indigo-50 dark:bg-indigo-500/15 border-indigo-300 text-indigo-700 dark:text-indigo-300'
                                : 'bg-slate-50 dark:bg-slate-800 border-slate-200 dark:border-slate-700 text-slate-600 dark:text-slate-300'
                            }`}
                          >
                            {u.name} ({u.type?.toUpperCase()})
                          </button>
                        )
                      })}
                    </div>
                  )}
                  <p className="text-[11px] text-slate-400 mt-1">{t('cron.mb.targetsHint')}</p>
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <div>
                    <label htmlFor="mb-rename" className="block font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                      {t('cron.mb.rename')}
                    </label>
                    <input
                      id="mb-rename"
                      value={mbForm.rename}
                      onChange={(e) => setMbForm({ ...mbForm, rename: e.target.value })}
                      placeholder={t('cron.mb.renamePlaceholder')}
                      className="w-full px-3 py-2 border rounded-lg"
                    />
                  </div>
                  <div>
                    <label htmlFor="mb-mode" className="block font-medium text-slate-700 dark:text-slate-300 mb-1.5">
                      {t('cron.mb.mode')}
                    </label>
                    <select
                      id="mb-mode"
                      value={mbForm.mode}
                      onChange={(e) => setMbForm({ ...mbForm, mode: e.target.value as 'sync' | 'async' })}
                      className="w-full px-3 py-2 border rounded-lg bg-white dark:bg-slate-900"
                    >
                      <option value="sync">{t('cron.mb.modeSync')}</option>
                      <option value="async">{t('cron.mb.modeAsync')}</option>
                    </select>
                  </div>
                </div>

                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <label htmlFor="mb-passphrase" className="block font-medium text-slate-700 dark:text-slate-300">
                      {t('cron.mb.passphrase')}
                    </label>
                    {mbForm.has_passphrase && (
                      <button
                        type="button"
                        onClick={() => setMbForm({ ...mbForm, passphrase: '', has_passphrase: false, clear_passphrase: true })}
                        className="text-[11px] text-rose-600 dark:text-rose-400 hover:underline"
                      >
                        {t('common.clear')}
                      </button>
                    )}
                  </div>
                  <SecretField
                    id="mb-passphrase"
                    hasSaved={mbForm.has_passphrase}
                    value={mbForm.passphrase}
                    onChange={(v) => setMbForm({ ...mbForm, passphrase: v, clear_passphrase: false })}
                    onReveal={async () => (await api.revealCronSecrets(editing.id)).passphrase || ''}
                    placeholder={mbForm.has_passphrase ? t('cron.mb.apiKeySavedPlaceholder') : t('cron.mb.passphrasePlaceholder')}
                  />
                  <p className="text-[11px] text-slate-400 mt-1">{t('cron.mb.passphraseHint')}</p>
                </div>

                <div className="pt-1 space-y-2">
                  <div className="font-medium text-slate-700 dark:text-slate-300">{t('cron.mb.exportScope')}</div>
                  <p className="text-[11px] text-slate-400">{t('cron.mb.exportScopeHint')}</p>
                  <div className="flex flex-wrap gap-x-4 gap-y-2 pt-0.5">
                    <label className="inline-flex items-center gap-1.5">
                      <input type="checkbox" checked={mbForm.include_admins} onChange={(e) => setMbForm({ ...mbForm, include_admins: e.target.checked })} />
                      <span>{t('cron.mb.includeAdmins')}</span>
                    </label>
                    <label className="inline-flex items-center gap-1.5">
                      <input type="checkbox" checked={mbForm.include_mail_logs} onChange={(e) => setMbForm({ ...mbForm, include_mail_logs: e.target.checked })} />
                      <span>{t('cron.mb.includeMailLogs')}</span>
                    </label>
                    <label className="inline-flex items-center gap-1.5">
                      <input type="checkbox" checked={mbForm.include_audit_logs} onChange={(e) => setMbForm({ ...mbForm, include_audit_logs: e.target.checked })} />
                      <span>{t('cron.mb.includeAuditLogs')}</span>
                    </label>
                  </div>
                </div>
              </div>
            )}
          </form>
        </Drawer>
      )}

      {runsJob && (
        <Drawer
          title={t('cron.runsTitle', { name: jobName(runsJob) })}
          onClose={() => setRunsJob(null)}
          width="lg"
          headerExtra={
            <button type="button" onClick={() => loadRuns(runsJob)} className="p-1.5 rounded-lg text-slate-400 hover:text-slate-700">
              <RefreshCw className={`w-4 h-4 ${runsLoading ? 'animate-spin' : ''}`} />
            </button>
          }
        >
          {runsError && <div role="alert" className="p-3 rounded-lg bg-rose-50 text-rose-700 text-xs">{runsError}</div>}
          {runsLoading && runs.length === 0 && <LoadingState />}
          {!runsLoading && runs.length === 0 && <div className="text-center text-xs text-slate-400 py-10">{t('cron.runsEmpty')}</div>}
          <div className="space-y-3">
            {runs.map((r) => (
              <div key={r.id} className="p-3.5 rounded-xl border border-slate-200 dark:border-slate-800 space-y-1.5 text-xs">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <StatusBadge value={r.status} />
                    <span className="text-slate-500">{r.trigger_type === 'manual' ? t('cron.trigger.manual') : t('cron.trigger.schedule')}</span>
                    {r.triggered_by && <span className="font-mono text-slate-400">({r.triggered_by})</span>}
                  </div>
                  <span className="font-mono text-slate-400">{formatDuration(r.started_at, r.finished_at)}</span>
                </div>
                <div className="text-[11px] text-slate-400">{formatTime(r.started_at, lang)}</div>
                {r.output_log && (
                  <pre className="p-2.5 rounded-lg bg-slate-50 dark:bg-slate-800/70 font-mono text-[11px] whitespace-pre-wrap break-all">{r.output_log}</pre>
                )}
              </div>
            ))}
          </div>
        </Drawer>
      )}
    </div>
  )
}
