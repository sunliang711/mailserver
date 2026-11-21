import React from 'react'
import { useI18n } from '../contexts/I18nContext'
import type { TranslationKey } from '../i18n/zh'

const styles: Record<string, string> = {
  succeeded: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-500/15 dark:text-emerald-300 dark:border-emerald-500/30',
  ok: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-500/15 dark:text-emerald-300 dark:border-emerald-500/30',
  success: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-500/15 dark:text-emerald-300 dark:border-emerald-500/30',
  enabled: 'bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-500/15 dark:text-emerald-300 dark:border-emerald-500/30',
  running: 'bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-500/15 dark:text-blue-300 dark:border-blue-500/30',
  pending: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-500/15 dark:text-amber-300 dark:border-amber-500/30',
  timeout: 'bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-500/15 dark:text-amber-300 dark:border-amber-500/30',
  skipped: 'bg-slate-100 text-slate-600 border-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700',
  untested: 'bg-slate-100 text-slate-600 border-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700',
  never: 'bg-slate-100 text-slate-600 border-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700',
  disabled: 'bg-slate-100 text-slate-500 border-slate-200 dark:bg-slate-800 dark:text-slate-400 dark:border-slate-700',
  failed: 'bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-500/15 dark:text-rose-300 dark:border-rose-500/30',
  failure: 'bg-rose-50 text-rose-700 border-rose-200 dark:bg-rose-500/15 dark:text-rose-300 dark:border-rose-500/30',
  api: 'bg-indigo-50 text-indigo-700 border-indigo-200 dark:bg-indigo-500/15 dark:text-indigo-300 dark:border-indigo-500/30',
  web_test: 'bg-cyan-50 text-cyan-700 border-cyan-200 dark:bg-cyan-500/15 dark:text-cyan-300 dark:border-cyan-500/30',
}

const labelKeys: Record<string, TranslationKey> = {
  succeeded: 'status.succeeded',
  ok: 'status.ok',
  success: 'status.success',
  enabled: 'status.enabled',
  running: 'status.running',
  pending: 'status.pending',
  timeout: 'status.timeout',
  skipped: 'status.skipped',
  untested: 'status.untested',
  never: 'status.never',
  disabled: 'status.disabled',
  failed: 'status.failed',
  failure: 'status.failure',
  api: 'status.api',
  web_test: 'status.web_test',
}

export const StatusBadge: React.FC<{ value: string; label?: string }> = ({ value, label }) => {
  const { t } = useI18n()
  const cls = styles[value] || 'bg-slate-100 text-slate-700 border-slate-200 dark:bg-slate-800 dark:text-slate-300 dark:border-slate-700'
  const key = labelKeys[value]
  return (
    <span className={`inline-flex items-center gap-1.5 whitespace-nowrap px-2 py-0.5 rounded-full text-[11px] font-medium border ${cls}`}>
      <span className={`w-1.5 h-1.5 rounded-full bg-current ${value === 'running' ? 'animate-pulse' : ''}`} aria-hidden="true" />
      {label || (key ? t(key) : value || t('common.unknown'))}
    </span>
  )
}
