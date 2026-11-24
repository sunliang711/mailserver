import React, { useState } from 'react'
import { Eye, EyeOff, Loader2, LockKeyhole, LogOut } from 'lucide-react'
import { api } from '../api'
import { useI18n } from '../contexts/I18nContext'
import type { Admin } from '../types'
import { Modal } from './Modal'
import { useToast } from './Toast'

interface Props {
  defaultUsername: string
  onSuccess: (admin: Admin, resetAdminEnvActive?: boolean) => void
  onDismiss: () => void
  onLogout: () => void
}

export const SessionExpiredModal: React.FC<Props> = ({ defaultUsername, onSuccess, onDismiss, onLogout }) => {
  const { t } = useI18n()
  const toast = useToast()
  const [username, setUsername] = useState(defaultUsername)
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await api.login(username.trim(), password)
      toast.success(t('auth.reloginSuccess'))
      onSuccess(res.admin, res.reset_admin_env_active)
    } catch (err: any) {
      setError(err.message === 'invalid username or password' ? t('auth.invalidCredentials') : err.message || t('auth.actionFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal title={t('auth.sessionExpiredTitle')} onClose={onDismiss} busy={loading} size="md" zIndexClass="z-[70]">
      <div className="bg-white dark:bg-slate-900 rounded-2xl p-6 space-y-5">
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 rounded-xl bg-amber-50 dark:bg-amber-500/15 border border-amber-200/80 dark:border-amber-500/30 text-amber-600 dark:text-amber-400 flex items-center justify-center shrink-0">
            <LockKeyhole className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">{t('auth.sessionExpiredTitle')}</h2>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-1 leading-relaxed">{t('auth.sessionExpiredDesc')}</p>
          </div>
        </div>

        {error && (
          <div role="alert" className="p-3 rounded-lg bg-rose-50 dark:bg-rose-500/15 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300 text-xs">
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="reauth-username" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
              {t('auth.username')}
            </label>
            <input
              id="reauth-username"
              autoComplete="username"
              type="text"
              required
              minLength={3}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              className="w-full px-3 py-2 border border-slate-300 dark:border-slate-700 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>

          <div>
            <label htmlFor="reauth-password" className="block text-xs font-medium text-slate-700 dark:text-slate-300 mb-1">
              {t('auth.password')}
            </label>
            <div className="relative">
              <input
                id="reauth-password"
                autoComplete="current-password"
                autoFocus
                type={showPassword ? 'text' : 'password'}
                required
                minLength={8}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t('auth.passwordPlaceholder')}
                className="w-full pl-3 pr-11 py-2 border border-slate-300 dark:border-slate-700 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
              <button
                type="button"
                aria-label={showPassword ? t('auth.hidePassword') : t('auth.showPassword')}
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-2 top-1/2 -translate-y-1/2 p-1.5 text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400"
              >
                {showPassword ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
              </button>
            </div>
          </div>

          <div className="flex items-center justify-between gap-2 pt-1">
            <button
              type="button"
              disabled={loading}
              onClick={onLogout}
              className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg border border-slate-200 dark:border-slate-700 text-xs font-medium text-slate-600 dark:text-slate-300 hover:bg-slate-50 dark:hover:bg-slate-800 disabled:opacity-45"
            >
              <LogOut className="w-3.5 h-3.5" />
              {t('auth.logoutToLogin')}
            </button>
            <button type="submit" disabled={loading} className="primary-button">
              {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <LockKeyhole className="w-4 h-4" />}
              {loading ? t('auth.submitting') : t('auth.reloginBtn')}
            </button>
          </div>
        </form>
      </div>
    </Modal>
  )
}
