import React from 'react'
import { Inbox, Loader2 } from 'lucide-react'
import { useI18n } from '../contexts/I18nContext'

export function EmptyState({ title, description, action, icon }: {
  title: string
  description?: string
  action?: React.ReactNode
  icon?: React.ReactNode
}) {
  return (
    <div className="empty-state">
      <div className="empty-icon">{icon || <Inbox className="w-6 h-6" />}</div>
      <div className="text-sm font-semibold text-slate-800 dark:text-slate-100">{title}</div>
      {description && <p className="text-xs text-slate-500 dark:text-slate-400 max-w-sm leading-6">{description}</p>}
      {action && <div className="mt-2">{action}</div>}
    </div>
  )
}

export function LoadingState({ label }: { label?: string }) {
  const { t } = useI18n()
  return (
    <div role="status" className="flex items-center justify-center gap-2 p-10 text-sm text-slate-500 dark:text-slate-400">
      <Loader2 className="w-4 h-4 animate-spin text-indigo-500 dark:text-indigo-400" />
      {label || t('common.loading')}
    </div>
  )
}
