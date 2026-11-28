import React, { useEffect, useRef, useState } from 'react'
import { api, setReauthHandler } from './api'
import { LoadingState } from './components/Feedback'
import { Layout, type NavTab } from './components/Layout'
import { SessionExpiredModal } from './components/SessionExpiredModal'
import { ToastProvider } from './components/Toast'
import { useI18n } from './contexts/I18nContext'
import { AccountsPage } from './pages/AccountsPage'
import { AuditPage } from './pages/AuditPage'
import { AuthPage } from './pages/AuthPages'
import { ClientsPage } from './pages/ClientsPage'
import { CronPage } from './pages/CronPage'
import { DashboardPage } from './pages/DashboardPage'
import { MailLogsPage } from './pages/MailLogsPage'
import { MigrationPage } from './pages/MigrationPage'
import { SettingsPage } from './pages/SettingsPage'
import type { Admin } from './types'
import { getBasePath, withBasePath } from './utils/basePath'

function initialTab(): NavTab {
  const base = getBasePath()
  let p = window.location.pathname
  if (base && (p === base || p.startsWith(`${base}/`))) {
    p = p.slice(base.length) || '/'
  }
  if (p.startsWith('/mail-logs')) return 'mail-logs'
  if (p.startsWith('/clients')) return 'clients'
  if (p.startsWith('/cron')) return 'cron'
  if (p.startsWith('/audit')) return 'audit'
  if (p.startsWith('/migration')) return 'migration'
  if (p.startsWith('/accounts')) return 'accounts'
  if (p.startsWith('/settings')) return 'settings'
  return 'dashboard'
}

export function App() {
  const { t } = useI18n()
  const [booting, setBooting] = useState(true)
  const [bootFailed, setBootFailed] = useState(false)
  const [initialized, setInitialized] = useState(true)
  const [admin, setAdmin] = useState<Admin | null>(null)
  const [resetAdminEnvActive, setResetAdminEnvActive] = useState(false)
  const [sessionExpired, setSessionExpired] = useState(false)
  const [version, setVersion] = useState('dev')
  const [tab, setTab] = useState<NavTab>(initialTab())
  const reauthResolverRef = useRef<((ok: boolean) => void) | null>(null)
  const isAuthenticated = Boolean(admin)

  useEffect(() => {
    const onPopState = () => setTab(initialTab())
    window.addEventListener('popstate', onPopState)
    ;(async () => {
      try {
        const st = await api.setupStatus()
        setInitialized(st.initialized)
        setResetAdminEnvActive(Boolean(st.reset_admin_env_active))
        if (st.version) setVersion(st.version)
        if (st.initialized) {
          try {
            const me = await api.me()
            setAdmin(me.admin)
            if (me.reset_admin_env_active !== undefined) {
              setResetAdminEnvActive(Boolean(me.reset_admin_env_active))
            }
            if (me.version) setVersion(me.version)
          } catch {
            setAdmin(null)
          }
        }
      } catch {
        setBootFailed(true)
      } finally {
        setBooting(false)
      }
    })()
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  useEffect(() => {
    if (!isAuthenticated) return
    setReauthHandler(() => {
      if (reauthResolverRef.current) {
        reauthResolverRef.current(false)
      }
      setSessionExpired(true)
      return new Promise<boolean>((resolve) => {
        reauthResolverRef.current = resolve
      })
    })
    return () => {
      setReauthHandler(null)
      setSessionExpired(false)
      const resolve = reauthResolverRef.current
      reauthResolverRef.current = null
      resolve?.(false)
    }
  }, [isAuthenticated])

  const handleReauthSuccess = (nextAdmin: Admin, resetActive?: boolean) => {
    setAdmin(nextAdmin)
    if (resetActive !== undefined) setResetAdminEnvActive(Boolean(resetActive))
    setSessionExpired(false)
    const resolve = reauthResolverRef.current
    reauthResolverRef.current = null
    resolve?.(true)
  }

  const handleReauthDismiss = () => {
    setSessionExpired(false)
    const resolve = reauthResolverRef.current
    reauthResolverRef.current = null
    resolve?.(false)
  }

  const handleLogout = async () => {
    setSessionExpired(false)
    const resolve = reauthResolverRef.current
    reauthResolverRef.current = null
    resolve?.(false)
    setReauthHandler(null)
    try {
      await api.logout()
    } catch {
      // ignore network/session errors during logout
    } finally {
      setAdmin(null)
    }
  }

  const navigate = (next: NavTab) => {
    if (next === tab) return
    setTab(next)
    const path = next === 'dashboard' ? '/' : `/${next}`
    window.history.pushState({}, '', withBasePath(path))
    window.scrollTo({ top: 0 })
  }

  if (booting) {
    return (
      <div className="min-h-screen grid place-items-center bg-[#f5f7fb] dark:bg-[#0b1120]">
        <LoadingState label={t('app.loading')} />
      </div>
    )
  }

  if (bootFailed) {
    return (
      <div className="min-h-screen flex flex-col items-center justify-center gap-4 p-6 text-center bg-[#f5f7fb] dark:bg-[#0b1120]">
        <p role="alert" className="text-sm text-slate-600 dark:text-slate-300">
          {t('app.connectError')}
        </p>
        <button className="primary-button" onClick={() => window.location.reload()}>
          {t('app.reconnect')}
        </button>
      </div>
    )
  }

  if (!initialized) {
    return (
      <AuthPage
        mode="setup"
        version={version}
        resetAdminEnvActive={resetAdminEnvActive}
        onSuccess={(a, resetActive) => {
          setInitialized(true)
          setAdmin(a)
          if (resetActive !== undefined) setResetAdminEnvActive(Boolean(resetActive))
        }}
      />
    )
  }

  if (!admin) {
    return (
      <AuthPage
        mode="login"
        version={version}
        resetAdminEnvActive={resetAdminEnvActive}
        onSuccess={(a, resetActive) => {
          setAdmin(a)
          if (resetActive !== undefined) setResetAdminEnvActive(Boolean(resetActive))
        }}
      />
    )
  }

  if (admin.must_change_password) {
    return (
      <AuthPage
        mode="force-reset"
        username={admin.username}
        version={version}
        resetAdminEnvActive={resetAdminEnvActive}
        onSuccess={(a, resetActive) => {
          setAdmin(a)
          if (resetActive !== undefined) setResetAdminEnvActive(Boolean(resetActive))
        }}
        onLogout={handleLogout}
      />
    )
  }

  const visibleTab = admin.role === 'viewer' && (tab === 'settings' || tab === 'migration') ? 'accounts' : tab

  return (
    <ToastProvider>
      <Layout
        active={visibleTab}
        onNavigate={navigate}
        admin={admin}
        version={version}
        resetAdminEnvActive={resetAdminEnvActive}
        onLogout={handleLogout}
      >
        {visibleTab === 'dashboard' && <DashboardPage key={admin.id + admin.role} onNavigate={navigate} readOnly={admin.role !== 'admin'} />}
        {visibleTab === 'mail-logs' && <MailLogsPage key={admin.id + admin.role} readOnly={admin.role !== 'admin'} />}
        {visibleTab === 'clients' && <ClientsPage key={admin.id + admin.role} readOnly={admin.role !== 'admin'} />}
        {visibleTab === 'cron' && <CronPage key={admin.id + admin.role} readOnly={admin.role !== 'admin'} />}
        {visibleTab === 'audit' && <AuditPage key={admin.id + admin.role} />}
        {visibleTab === 'migration' && <MigrationPage key={admin.id + admin.role} />}
        {visibleTab === 'accounts' && <AccountsPage key={admin.id + admin.role} admin={admin} onAdminChange={setAdmin} onSignOut={handleLogout} />}
        {visibleTab === 'settings' && <SettingsPage key={admin.id + admin.role} />}
      </Layout>
      {sessionExpired && (
        <SessionExpiredModal
          defaultUsername={admin.username}
          onSuccess={handleReauthSuccess}
          onDismiss={handleReauthDismiss}
          onLogout={handleLogout}
        />
      )}
    </ToastProvider>
  )
}

export default App
