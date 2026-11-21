import React, { useEffect, useRef, useState } from 'react'
import {
  Activity,
  ArrowLeftRight,
  ChevronRight,
  Clock,
  KeyRound,
  LogOut,
  Mail,
  Menu,
  ScrollText,
  Settings,
  X,
} from 'lucide-react'
import { useI18n } from '../contexts/I18nContext'
import type { TranslationKey } from '../i18n/zh'
import { PreferenceControls } from './PreferenceControls'
import type { Admin } from '../types'

export type NavTab = 'dashboard' | 'mail-logs' | 'clients' | 'cron' | 'audit' | 'migration' | 'settings'

interface Props {
  active: NavTab
  onNavigate: (tab: NavTab) => void
  admin: Admin
  version?: string
  onLogout: () => void
  children: React.ReactNode
}

const navItems: { id: NavTab; labelKey: TranslationKey; icon: React.ReactNode }[] = [
  { id: 'dashboard', labelKey: 'nav.dashboard', icon: <Activity className="w-4 h-4" /> },
  { id: 'mail-logs', labelKey: 'nav.mailLogs', icon: <Mail className="w-4 h-4" /> },
  { id: 'clients', labelKey: 'nav.clients', icon: <KeyRound className="w-4 h-4" /> },
  { id: 'cron', labelKey: 'nav.cron', icon: <Clock className="w-4 h-4" /> },
  { id: 'audit', labelKey: 'nav.audit', icon: <ScrollText className="w-4 h-4" /> },
  { id: 'migration', labelKey: 'nav.migration', icon: <ArrowLeftRight className="w-4 h-4" /> },
  { id: 'settings', labelKey: 'nav.settings', icon: <Settings className="w-4 h-4" /> },
]

export const Layout: React.FC<Props> = ({ active, onNavigate, admin, version = 'dev', onLogout, children }) => {
  const { t } = useI18n()
  const [menuOpen, setMenuOpen] = useState(false)
  const navigation = useRef<HTMLElement>(null)
  const current = navItems.find((item) => item.id === active)!
  const versionLabel = version && version !== 'dev' && !version.startsWith('v') ? `v${version}` : version || 'dev'

  useEffect(() => {
    if (!menuOpen) return
    const previous = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    const focusable = () => Array.from(navigation.current?.querySelectorAll<HTMLElement>('button, a[href]') || []).filter((el) => el.getClientRects().length > 0)
    focusable()[0]?.focus()
    const onEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false)
      if (event.key === 'Tab') {
        const elements = focusable()
        const first = elements[0]
        const last = elements[elements.length - 1]
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault()
          last?.focus()
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault()
          first?.focus()
        }
      }
    }
    document.addEventListener('keydown', onEscape)
    return () => {
      document.body.style.overflow = overflow
      document.removeEventListener('keydown', onEscape)
      previous?.focus()
    }
  }, [menuOpen])

  return (
    <div className="min-h-screen bg-[#f5f7fb] dark:bg-[#0b1120] transition-colors">
      <a href="#main-content" className="sr-only focus:not-sr-only focus:fixed focus:top-4 focus:left-4 focus:z-[80] bg-white dark:bg-slate-900 text-slate-900 dark:text-slate-100 p-3 rounded-lg">
        {t('app.skipToContent')}
      </a>
      {menuOpen && (
        <button
          className="fixed inset-0 bg-slate-950/50 backdrop-blur-sm z-30 lg:hidden"
          aria-label={t('nav.closeMenu')}
          onClick={() => setMenuOpen(false)}
        />
      )}
      <aside ref={navigation} id="app-navigation" className={`app-sidebar fixed inset-y-0 left-0 w-64 text-slate-200 flex-col z-40 transition-transform duration-200 ${menuOpen ? 'flex translate-x-0' : 'hidden -translate-x-full'} lg:flex lg:translate-x-0`}>
        <div className="px-6 h-[88px] flex items-center gap-3 shrink-0">
          <div className="w-10 h-10 rounded-xl bg-indigo-500 flex items-center justify-center text-white shadow-lg shadow-indigo-950/30">
            <Mail className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <div className="font-semibold text-xl text-white tracking-tight leading-none flex items-center gap-2">
              <span>MailServer<span className="text-indigo-400">.</span></span>
              <span className="text-[10px] font-mono font-normal px-1.5 py-0.5 rounded bg-white/10 text-slate-300">{versionLabel}</span>
            </div>
            <div className="text-[11px] text-slate-400 mt-2 tracking-wider truncate">{t('app.subtitle')}</div>
          </div>
          <button onClick={() => setMenuOpen(false)} className="lg:hidden ml-auto p-1 text-slate-400" aria-label={t('nav.collapseMenu')}>
            <X className="w-5 h-5" />
          </button>
        </div>

        <nav aria-label={t('nav.mainAria')} className="flex-1 px-4 py-4 overflow-y-auto">
          {navItems.map((item, index) => {
            const isActive = active === item.id
            return (
              <React.Fragment key={item.id}>
                {(index === 0 || index === 3) && (
                  <div className="text-[10px] text-slate-500 tracking-widest px-3 mb-3 mt-2">
                    {index === 0 ? t('nav.group.workspace') : t('nav.group.admin')}
                  </div>
                )}
                <a
                  href={item.id === 'dashboard' ? '/' : `/${item.id}`}
                  aria-current={isActive ? 'page' : undefined}
                  onClick={(event) => {
                    if (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
                    event.preventDefault()
                    setMenuOpen(false)
                    onNavigate(item.id)
                  }}
                  className={`w-full flex items-center gap-3 px-3 py-3 mb-1 rounded-xl text-[13px] transition ${isActive ? 'bg-indigo-500 text-white font-medium shadow-md shadow-indigo-950/20' : 'text-slate-400 hover:bg-white/5 hover:text-white'} ${index === 2 ? 'mb-7' : ''}`}
                >
                  {item.icon}<span>{t(item.labelKey)}</span>
                  {isActive && <ChevronRight className="w-3.5 h-3.5 ml-auto opacity-70" />}
                </a>
              </React.Fragment>
            )
          })}
        </nav>

        <div className="mx-4 mb-4 rounded-xl bg-white/[0.04] p-3 flex items-center gap-3 text-xs">
          <div className="w-9 h-9 rounded-full bg-indigo-400/15 text-indigo-200 flex items-center justify-center font-semibold text-sm shrink-0">
            {admin.username.slice(0, 1).toUpperCase()}
          </div>
          <div className="truncate flex-1">
            <div className="text-white font-medium truncate">{admin.username}</div>
            <div className="text-slate-500 mt-1">{t('app.roleAdmin')}</div>
          </div>
          <button
            onClick={onLogout}
            title={t('nav.logout')}
            aria-label={t('nav.logout')}
            className="p-2 rounded-lg text-slate-400 hover:text-white hover:bg-white/10"
          >
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </aside>

      <div className="lg:pl-64">
        <header className="h-[72px] border-b border-slate-200/70 dark:border-slate-800 bg-white/90 dark:bg-slate-900/85 backdrop-blur flex items-center justify-between gap-3 px-5 sm:px-8 sticky top-0 z-20">
          <div className="flex items-center gap-3 text-xs text-slate-400 dark:text-slate-500 min-w-0">
            <button
              onClick={() => setMenuOpen(true)}
              aria-label={t('nav.openMenu')}
              aria-controls="app-navigation"
              aria-expanded={menuOpen}
              className="lg:hidden p-2 -ml-2 text-slate-600 dark:text-slate-300 rounded-lg hover:bg-slate-100 dark:hover:bg-slate-800"
            >
              <Menu className="w-5 h-5" />
            </button>
            <span className="hidden sm:inline">{t('app.workspace')}</span>
            <ChevronRight className="hidden sm:block w-3 h-3 shrink-0" />
            <span className="font-medium text-slate-700 dark:text-slate-200 truncate">{t(current.labelKey)}</span>
          </div>
          <div className="flex items-center gap-3 shrink-0">
            <div className="hidden md:flex items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
              <Mail className="w-4 h-4 text-indigo-500 dark:text-indigo-400" />
              <span>{t('app.console')}</span>
              <span className="font-mono text-[11px] px-1.5 py-0.5 rounded bg-slate-100 dark:bg-slate-800 text-slate-600 dark:text-slate-300">
                {versionLabel}
              </span>
            </div>
            <PreferenceControls />
          </div>
        </header>
        <main id="main-content" tabIndex={-1} className="min-w-0">
          <div className="page-content max-w-[1440px] mx-auto px-4 py-6 sm:p-8">{children}</div>
          <footer className="px-8 py-6 text-[11px] text-slate-400 dark:text-slate-500 flex flex-wrap gap-2 justify-between">
            <span>{t('app.footerLeft', { version: versionLabel })}</span>
            <span>{t('app.footerRight')}</span>
          </footer>
        </main>
      </div>
    </div>
  )
}
