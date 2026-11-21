import React, { useState } from 'react'
import { ArrowRight, Eye, EyeOff, Loader2, LockKeyhole, Mail } from 'lucide-react'
import { api } from '../api'
import { PreferenceControls } from '../components/PreferenceControls'
import { useI18n } from '../contexts/I18nContext'
import type { Admin } from '../types'

interface Props {
  mode: 'setup' | 'login'
  version?: string
  onSuccess: (admin: Admin) => void
}

export const AuthPage: React.FC<Props> = ({ mode, version = 'dev', onSuccess }) => {
  const { t } = useI18n()
  const versionLabel = version && version !== 'dev' && !version.startsWith('v') ? `v${version}` : version || 'dev'
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    if (mode === 'setup' && password !== confirm) {
      setError(t('auth.passwordMismatch'))
      return
    }
    setLoading(true)
    try {
      const res = mode === 'setup' ? await api.setupInit(username, password) : await api.login(username, password)
      onSuccess(res.admin)
    } catch (err: any) {
      setError(err.message === 'invalid username or password' ? t('auth.invalidCredentials') : err.message || t('auth.actionFailed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen grid lg:grid-cols-2 bg-[#f7f9fc] dark:bg-[#0b1120] transition-colors">
      <div className="hidden lg:flex flex-col justify-between relative overflow-hidden bg-[#111b34] dark:bg-[#090e1a] border-r border-transparent dark:border-slate-800 text-white p-12 xl:p-16">
        <div className="absolute -right-32 top-1/4 w-96 h-96 bg-indigo-500/10 rounded-full blur-3xl" />
        <div className="flex items-center gap-3 font-semibold text-2xl"><Mail className="w-8 h-8 text-indigo-400" />MailServer<span className="text-indigo-400">.</span></div>
        <div className="relative py-16 max-w-lg">
          <div className="w-16 h-16 bg-indigo-400/10 border border-indigo-400/20 rounded-2xl grid place-items-center mb-8"><Mail className="w-8 h-8 text-indigo-300" /></div>
          <div className="text-[40px] xl:text-5xl font-semibold leading-tight tracking-tight">
            {t('auth.heroLine1')}<br /><span className="text-indigo-300">{t('auth.heroLine2')}</span>
          </div>
          <p className="text-slate-400 text-sm leading-7 mt-6">{t('auth.heroDesc')}</p>
          <div className="flex gap-5 text-xs text-slate-300 mt-8">
            <span>{t('auth.feature1')}</span>
            <span className="text-slate-600">/</span>
            <span>{t('auth.feature2')}</span>
            <span className="text-slate-600">/</span>
            <span>{t('auth.feature3')}</span>
          </div>
        </div>
        <div className="text-xs text-slate-500">{t('auth.footerBrand')} · <span className="font-mono">{versionLabel}</span></div>
      </div>

      <div className="relative flex flex-col items-center justify-center p-6 sm:p-12">
        <div className="absolute top-5 right-5 sm:top-6 sm:right-6">
          <PreferenceControls />
        </div>

        <div className="w-full max-w-md bg-white dark:bg-slate-900 rounded-2xl border border-slate-200/80 dark:border-slate-800 shadow-xl shadow-slate-200/30 dark:shadow-black/40 p-6 sm:p-9">
          <div className="flex items-center gap-3 mb-6">
            <div className="w-10 h-10 rounded-xl bg-indigo-600 text-white flex items-center justify-center">
              <Mail className="w-5 h-5" />
            </div>
            <div>
              <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">
                {mode === 'setup' ? t('auth.setupTitle') : t('auth.loginTitle')}
              </h1>
              <p className="text-xs text-slate-500 dark:text-slate-400">
                {mode === 'setup' ? t('auth.setupSubtitle') : t('auth.loginSubtitle')}
              </p>
            </div>
          </div>

          {error && (
            <div role="alert" className="mb-4 p-3 rounded-lg bg-rose-50 dark:bg-rose-500/15 border border-rose-200 dark:border-rose-500/30 text-rose-700 dark:text-rose-300 text-sm">
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label htmlFor="auth-username" className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                {t('auth.username')}
              </label>
              <input
                id="auth-username"
                autoComplete="username"
                autoFocus
                type="text"
                required
                minLength={3}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
                className="w-full px-3 py-2 border border-slate-300 dark:border-slate-700 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
            </div>
            <div>
              <label htmlFor="auth-password" className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                {t('auth.password')}{mode === 'setup' ? t('auth.passwordMinHint') : ''}
              </label>
              <div className="relative">
                <input
                  id="auth-password"
                  autoComplete={mode === 'setup' ? 'new-password' : 'current-password'}
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
            {mode === 'setup' && (
              <div>
                <label htmlFor="auth-confirm" className="block text-sm font-medium text-slate-700 dark:text-slate-300 mb-1">
                  {t('auth.confirmPassword')}
                </label>
                <input
                  id="auth-confirm"
                  autoComplete="new-password"
                  type={showPassword ? 'text' : 'password'}
                  required
                  minLength={8}
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                  placeholder="••••••••"
                  className="w-full px-3 py-2 border border-slate-300 dark:border-slate-700 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
                />
              </div>
            )}
            <button
              type="submit"
              disabled={loading}
              className="w-full py-2.5 bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50 text-white rounded-lg text-sm font-medium flex items-center justify-center gap-2 transition"
            >
              {loading ? <Loader2 className="w-4 h-4 animate-spin" /> : <LockKeyhole className="w-4 h-4" />}
              {loading ? t('auth.submitting') : mode === 'setup' ? t('auth.setupBtn') : t('auth.loginBtn')}
              {!loading && <ArrowRight className="w-4 h-4" />}
            </button>
          </form>
        </div>
      </div>
    </div>
  )
}
