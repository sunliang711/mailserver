import React from 'react'
import { AlertTriangle } from 'lucide-react'
import { useI18n } from '../contexts/I18nContext'

interface Props {
  active?: boolean
}

export const ResetAdminBanner: React.FC<Props> = ({ active }) => {
  const { t } = useI18n()
  if (!active) return null

  return (
    <div
      role="alert"
      className="w-full bg-amber-50 dark:bg-amber-950/70 border-b border-amber-200 dark:border-amber-800/70 text-amber-900 dark:text-amber-200 px-4 sm:px-6 py-2.5 text-xs flex items-center justify-center gap-2"
    >
      <AlertTriangle className="w-4 h-4 shrink-0 text-amber-600 dark:text-amber-400" />
      <span className="leading-relaxed">{t('auth.resetEnvBanner')}</span>
    </div>
  )
}
