import React, { useCallback, useEffect, useState } from 'react'
import { Monitor, Plus, RefreshCw, Shield, UserRound } from 'lucide-react'
import { api, ApiError, formatTime } from '../api'
import { StatusBadge } from '../components/Badge'
import { EmptyState, LoadingState } from '../components/Feedback'
import { Modal } from '../components/Modal'
import { useToast } from '../components/Toast'
import { useI18n } from '../contexts/I18nContext'
import type { Admin, AdminSession } from '../types'

type AccountAction = { kind: 'create' | 'role' | 'active' | 'password'; target?: Admin }

interface Props {
  admin: Admin
  onAdminChange: (admin: Admin) => void
  onSignOut: () => Promise<void>
}

export function AccountsPage({ admin, onAdminChange, onSignOut }: Props) {
  const { t, lang } = useI18n()
  const toast = useToast()
  const isAdmin = admin.role === 'admin'
  const [sessions, setSessions] = useState<AdminSession[]>([])
  const [accounts, setAccounts] = useState<Admin[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [action, setAction] = useState<AccountAction | null>(null)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<Admin['role']>('viewer')
  const [actorPassword, setActorPassword] = useState('')
  const [actionError, setActionError] = useState('')
  const [revokeTarget, setRevokeTarget] = useState<AdminSession | null>(null)

  const errorText = (e: unknown) => {
    if (e instanceof ApiError) {
      if (e.code === 'invalid_password') return t('accounts.error.password')
      if (e.code === 'last_admin') return t('accounts.error.lastAdmin')
      if (e.code === 'username_taken') return t('accounts.error.usernameTaken')
      if (e.code === 'cannot_disable_self') return t('accounts.error.disableSelf')
      if (e.code === 'validation_error') return t('accounts.error.validation')
    }
    return e instanceof Error ? e.message : t('accounts.error.failed')
  }

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [sessionResult, accountResult] = await Promise.all([
        api.listSessions(),
        isAdmin ? api.listAdmins() : Promise.resolve({ items: [] as Admin[] }),
      ])
      setSessions(sessionResult.items)
      setAccounts(accountResult.items)
    } catch (e) {
      setError(e instanceof Error ? e.message : t('accounts.error.failed'))
    } finally {
      setLoading(false)
    }
  }, [isAdmin, t])

  useEffect(() => { void load() }, [load])

  const openAction = (next: AccountAction) => {
    setAction(next)
    setUsername('')
    setPassword('')
    setRole('viewer')
    setActorPassword('')
    setActionError('')
  }

  const changePassword = async (event: React.FormEvent) => {
    event.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      const result = await api.changePassword(oldPassword, newPassword)
      if (result.admin) onAdminChange(result.admin)
      setOldPassword('')
      setNewPassword('')
      toast.success(t('accounts.passwordChanged'))
      await load()
    } catch (e) {
      toast.error(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  const submitAction = async (event: React.FormEvent) => {
    event.preventDefault()
    if (!action || busy) return
    setBusy(true)
    setActionError('')
    try {
      const target = action.target
      if (action.kind === 'create') {
        await api.createAdmin({ username, password, role, current_password: actorPassword })
      } else if (target) {
        const update = action.kind === 'role' ? { role: target.role === 'admin' ? 'viewer' as const : 'admin' as const }
          : action.kind === 'active' ? { is_active: !target.is_active } : { password }
        const result = await api.updateAdmin(target.id, { ...update, current_password: actorPassword })
        if (target.id === admin.id) {
          if (action.kind === 'role') {
            setAction(null)
            setActorPassword('')
            await onSignOut()
            return
          }
          onAdminChange(result)
        }
      }
      setAction(null)
      setPassword('')
      setActorPassword('')
      toast.success(t('accounts.saved'))
      await load()
    } catch (e) {
      setActionError(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  const revokeSession = async () => {
    if (!revokeTarget || busy) return
    setBusy(true)
    try {
      await api.revokeSession(revokeTarget.id)
      const current = revokeTarget.current
      setRevokeTarget(null)
      if (current) {
        await onSignOut()
        return
      }
      toast.success(t('accounts.revoked'))
      await load()
    } catch (e) {
      toast.error(errorText(e))
    } finally {
      setBusy(false)
    }
  }

  const panel = 'bg-white dark:bg-slate-900 rounded-xl border border-slate-200 dark:border-slate-800 p-5 sm:p-6'
  const field = 'w-full px-3 py-2 text-sm border rounded-lg'
  const secondary = 'px-3 py-1.5 text-xs rounded-lg border border-slate-200 dark:border-slate-700 disabled:opacity-45'
  const actionTitle = action?.kind === 'create' ? t('accounts.create')
    : action?.kind === 'role' ? t('accounts.changeRole', { name: action.target?.username || '' })
    : action?.kind === 'active' ? t(action.target?.is_active ? 'accounts.disableAccount' : 'accounts.enableAccount', { name: action.target?.username || '' })
    : t('accounts.resetPassword', { name: action?.target?.username || '' })
  const activeAdmins = accounts.filter((a) => a.is_active && a.role === 'admin').length

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-slate-100">{t('accounts.title')}</h1>
          <p className="text-sm text-slate-500 dark:text-slate-400 mt-1">{t('accounts.subtitle')}</p>
        </div>
        <button type="button" className={secondary + ' inline-flex items-center gap-2'} disabled={loading || busy} onClick={() => void load()}>
          <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />{t('common.refresh')}
        </button>
      </div>
      {error && <div role="alert" className="p-4 rounded-xl bg-rose-50 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300 text-sm">{error}</div>}
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
        <form onSubmit={changePassword} className={panel + ' space-y-4 self-start'}>
          <h2 className="text-sm font-semibold flex items-center gap-2"><Shield className="w-4 h-4 text-indigo-500" />{t('accounts.ownPassword', { name: admin.username })}</h2>
          <div>
            <label htmlFor="account-old-password" className="block text-xs mb-1.5">{t('settings.admins.oldPassword')}</label>
            <input id="account-old-password" type="password" autoComplete="current-password" required value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} className={field} />
          </div>
          <div>
            <label htmlFor="account-new-password" className="block text-xs mb-1.5">{t('accounts.newPassword')}</label>
            <input id="account-new-password" type="password" autoComplete="new-password" minLength={8} maxLength={72} required value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className={field} />
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400">{t('accounts.passwordHint')}</p>
          <button type="submit" disabled={busy} className="primary-button">{t('settings.admins.changePwdBtn')}</button>
        </form>

        <section className={panel} aria-labelledby="account-sessions-title">
          <h2 id="account-sessions-title" className="text-sm font-semibold flex items-center gap-2 mb-2"><Monitor className="w-4 h-4 text-indigo-500" />{t('accounts.sessions')}</h2>
          <p className="text-xs text-slate-500 dark:text-slate-400 mb-4">{t('accounts.sessionHint')}</p>
          {loading ? <LoadingState /> : sessions.length === 0 ? <EmptyState title={t('accounts.noSessions')} /> : (
            <div className="space-y-3 max-h-[520px] overflow-y-auto">
              {sessions.map((session) => (
                <div key={session.id} className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50/50 dark:bg-slate-800/40 p-3 space-y-2">
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex flex-wrap items-center gap-2 min-w-0">
                      <span className="font-mono text-xs break-all">{session.client_ip || '—'}</span>
                      {session.current && <StatusBadge value="enabled" label={t('accounts.current')} />}
                    </div>
                    <button type="button" className={secondary + ' shrink-0'} disabled={busy} onClick={() => setRevokeTarget(session)}>{t('accounts.revoke')}</button>
                  </div>
                  <p className="text-xs text-slate-500 dark:text-slate-400 break-all" title={session.user_agent}>{session.user_agent || t('accounts.unknownDevice')}</p>
                  <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-[11px] text-slate-500 dark:text-slate-400">
                    <dt>{t('accounts.createdAt')}</dt><dd>{formatTime(session.created_at, lang)}</dd>
                    <dt>{t('accounts.lastSeen')}</dt><dd>{formatTime(session.last_seen_at, lang)}</dd>
                    <dt>{t('accounts.expiresAt')}</dt><dd>{formatTime(session.expires_at, lang)}</dd>
                  </dl>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

      {isAdmin && (
        <section className={panel} aria-labelledby="account-management-title">
          <div className="flex flex-wrap items-center justify-between gap-3 mb-4">
            <h2 id="account-management-title" className="text-sm font-semibold flex items-center gap-2"><UserRound className="w-4 h-4 text-indigo-500" />{t('accounts.management')}</h2>
            <button type="button" className="primary-button" disabled={busy} onClick={() => openAction({ kind: 'create' })}><Plus className="w-4 h-4" />{t('accounts.create')}</button>
          </div>
          {loading ? <LoadingState /> : (
            <div className="overflow-x-auto">
              <table className="w-full text-xs text-left">
                <thead className="text-slate-500 dark:text-slate-400 border-b border-slate-200 dark:border-slate-800">
                  <tr><th className="py-3 pr-4">{t('settings.admins.username')}</th><th className="py-3 pr-4">{t('accounts.role')}</th><th className="py-3 pr-4">{t('accounts.status')}</th><th className="py-3 pr-4">{t('accounts.lastLogin')}</th><th className="py-3 text-right">{t('accounts.actions')}</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {accounts.map((account) => {
                    const lastAdmin = account.is_active && account.role === 'admin' && activeAdmins <= 1
                    return (
                      <tr key={account.id}>
                        <td className="py-3 pr-4 font-medium">{account.username}{account.id === admin.id && <span className="text-slate-400 ml-2">{t('accounts.you')}</span>}</td>
                        <td className="py-3 pr-4 whitespace-nowrap">{t(account.role === 'admin' ? 'app.roleAdmin' : 'app.roleViewer')}</td>
                        <td className="py-3 pr-4"><StatusBadge value={account.is_active ? 'enabled' : 'disabled'} /></td>
                        <td className="py-3 pr-4 whitespace-nowrap text-slate-500">{formatTime(account.last_login_at, lang)}</td>
                        <td className="py-3 text-right">
                          <div className="flex flex-wrap justify-end gap-2 min-w-[260px]">
                            <button type="button" className={secondary} disabled={busy || lastAdmin} title={lastAdmin ? t('accounts.error.lastAdmin') : undefined} onClick={() => openAction({ kind: 'role', target: account })}>{t(account.role === 'admin' ? 'accounts.setViewer' : 'accounts.setAdmin')}</button>
                            <button type="button" className={secondary} disabled={busy} onClick={() => openAction({ kind: 'password', target: account })}>{t('settings.admins.resetPwdBtn')}</button>
                            <button type="button" className={secondary} disabled={busy || account.is_active && (account.id === admin.id || lastAdmin)} onClick={() => openAction({ kind: 'active', target: account })}>{t(account.is_active ? 'settings.admins.disable' : 'settings.admins.enable')}</button>
                          </div>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          )}
        </section>
      )}

      {action && (
        <Modal title={actionTitle} onClose={() => setAction(null)} busy={busy} size="md">
          <form onSubmit={submitAction} className="p-6 space-y-4">
            <h2 className="text-base font-semibold pr-6">{actionTitle}</h2>
            {actionError && <p role="alert" className="text-sm text-rose-600 dark:text-rose-400">{actionError}</p>}
            {action.kind === 'create' && <>
              <div><label htmlFor="account-username" className="block text-xs mb-1.5">{t('settings.admins.username')}</label><input id="account-username" autoComplete="off" required minLength={3} maxLength={64} value={username} onChange={(e) => setUsername(e.target.value)} className={field} /></div>
              <div><label htmlFor="account-role" className="block text-xs mb-1.5">{t('accounts.role')}</label><select id="account-role" value={role} onChange={(e) => setRole(e.target.value as Admin['role'])} className={field}><option value="viewer">{t('app.roleViewer')}</option><option value="admin">{t('app.roleAdmin')}</option></select></div>
            </>}
            {(action.kind === 'create' || action.kind === 'password') && <div><label htmlFor="account-password" className="block text-xs mb-1.5">{t('accounts.newPassword')}</label><input id="account-password" type="password" autoComplete="new-password" required minLength={8} maxLength={72} value={password} onChange={(e) => setPassword(e.target.value)} className={field} /></div>}
            {action.kind === 'role' && <p className="text-sm text-slate-500 dark:text-slate-400">{t('accounts.roleChangeHint', { role: t(action.target?.role === 'admin' ? 'app.roleViewer' : 'app.roleAdmin') })}</p>}
            {action.kind === 'role' && action.target?.id === admin.id && <p className="text-xs text-amber-600 dark:text-amber-400">{t('accounts.selfRoleHint')}</p>}
            {action.kind === 'password' && <p className="text-xs text-slate-500 dark:text-slate-400">{t('accounts.resetHint')}</p>}
            <div><label htmlFor="account-actor-password" className="block text-xs mb-1.5">{t('accounts.actorPassword')}</label><input id="account-actor-password" type="password" autoComplete="current-password" required value={actorPassword} onChange={(e) => setActorPassword(e.target.value)} className={field} /></div>
            <div className="flex justify-end gap-2"><button type="button" disabled={busy} className={secondary} onClick={() => setAction(null)}>{t('common.cancel')}</button><button type="submit" disabled={busy} className="primary-button">{t(busy ? 'common.saving' : 'common.confirm')}</button></div>
          </form>
        </Modal>
      )}
      {revokeTarget && (
        <Modal title={t('accounts.revokeTitle')} onClose={() => setRevokeTarget(null)} busy={busy} size="md">
          <div className="p-6 space-y-4">
            <h2 className="text-base font-semibold">{t('accounts.revokeTitle')}</h2>
            <p className="text-sm text-slate-500 dark:text-slate-400">{t(revokeTarget.current ? 'accounts.revokeCurrentHint' : 'accounts.revokeHint')}</p>
            <p className="font-mono text-xs break-all">{revokeTarget.client_ip || '—'}</p>
            <div className="flex justify-end gap-2"><button type="button" disabled={busy} className={secondary} onClick={() => setRevokeTarget(null)}>{t('common.cancel')}</button><button type="button" disabled={busy} onClick={() => void revokeSession()} className="px-4 py-2 text-xs rounded-lg bg-rose-600 hover:bg-rose-700 text-white disabled:opacity-45">{t('accounts.revoke')}</button></div>
          </div>
        </Modal>
      )}
    </div>
  )
}
