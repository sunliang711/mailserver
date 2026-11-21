import React, { useEffect, useState } from 'react'
import { api } from './api'
import { LoadingState } from './components/Feedback'
import { Layout, type NavTab } from './components/Layout'
import { ToastProvider } from './components/Toast'
import { useI18n } from './contexts/I18nContext'
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
  if (p.startsWith('/settings')) return 'settings'
  return 'dashboard'
}

export function App() {
  const { t } = useI18n()
  const [booting, setBooting] = useState(true)
  const [bootFailed, setBootFailed] = useState(false)
  const [initialized, setInitialized] = useState(true)
  const [admin, setAdmin] = useState<Admin | null>(null)
  const [version, setVersion] = useState('dev')
  const [tab, setTab] = useState<NavTab>(initialTab())

  useEffect(() => {
    const onPopState = () => setTab(initialTab())
    window.addEventListener('popstate', onPopState)
    ;(async () => {
      try {
        const st = await api.setupStatus()
        setInitialized(st.initialized)
        if (st.version) setVersion(st.version)
        if (st.initialized) {
          try {
            const me = await api.me()
            setAdmin(me.admin)
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
        onSuccess={(a) => {
          setInitialized(true)
          setAdmin(a)
        }}
      />
    )
  }

  if (!admin) {
    return <AuthPage mode="login" version={version} onSuccess={(a) => setAdmin(a)} />
  }

  return (
    <ToastProvider>
      <Layout
        active={tab}
        onNavigate={navigate}
        admin={admin}
        version={version}
        onLogout={async () => {
          await api.logout()
          setAdmin(null)
        }}
      >
        {tab === 'dashboard' && <DashboardPage onNavigate={navigate} />}
        {tab === 'mail-logs' && <MailLogsPage />}
        {tab === 'clients' && <ClientsPage />}
        {tab === 'cron' && <CronPage />}
        {tab === 'audit' && <AuditPage />}
        {tab === 'migration' && <MigrationPage />}
        {tab === 'settings' && <SettingsPage />}
      </Layout>
    </ToastProvider>
  )
}

export default App
