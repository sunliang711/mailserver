import { getCurrentLang, translate } from './contexts/I18nContext'
import type {
  Admin,
  AuditLog,
  Client,
  CronJob,
  CronJobRun,
  DashboardSummary,
  InspectSummary,
  MailLog,
  SafeKeepUpstream,
  SMTPConfig,
  SystemSettings,
} from './types'

const BASE = '/admin/api/v1'

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const isForm = init.body instanceof FormData
  const headers: Record<string, string> = isForm ? {} : { 'Content-Type': 'application/json' }
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    credentials: 'include',
    headers: { ...headers, ...(init.headers as Record<string, string> | undefined) },
  })
  if (!res.ok) {
    let code = 'http_error'
    let msg = res.statusText
    try {
      const data = await res.json()
      if (data?.error) {
        code = data.error.code || code
        msg = data.error.message || msg
      }
    } catch {
      // ignore non-JSON error
    }
    throw new ApiError(res.status, code, msg)
  }
  return res.json() as Promise<T>
}

export const api = {
  setupStatus: () => request<{ initialized: boolean; version?: string }>('/setup/status'),
  setupInit: (username: string, password: string) =>
    request<{ admin: Admin }>('/setup/init', { method: 'POST', body: JSON.stringify({ username, password }) }),
  login: (username: string, password: string) =>
    request<{ admin: Admin }>('/auth/login', { method: 'POST', body: JSON.stringify({ username, password }) }),
  logout: () => request('/auth/logout', { method: 'POST' }),
  me: () => request<{ admin: Admin; version?: string }>('/auth/me'),
  changePassword: (old_password: string, new_password: string) =>
    request('/auth/password', { method: 'PUT', body: JSON.stringify({ old_password, new_password }) }),

  dashboard: () => request<DashboardSummary>('/dashboard/summary'),

  listMailLogs: (params: Record<string, string>) => {
    const qs = new URLSearchParams(params).toString()
    return request<{ items: MailLog[]; total: number }>(`/mail-logs${qs ? '?' + qs : ''}`)
  },
  getMailLog: (id: string) => request<MailLog>(`/mail-logs/${encodeURIComponent(id)}`),
  exportMailLogsUrl: (params: Record<string, string>, format: 'csv' | 'json') => {
    const qs = new URLSearchParams({ ...params, format }).toString()
    return `${BASE}/mail-logs/export?${qs}`
  },
  testSendMail: (payload: { to: string; subject: string; body: string }) =>
    request<{ status: string }>('/mail/test-send', { method: 'POST', body: JSON.stringify(payload) }),

  listClients: () => request<{ items: Client[] }>('/clients'),
  createClient: (payload: {
    name: string
    description: string
    enabled: boolean
    rate_limit_rpm: number
    expires_at?: string | null
  }) => request<{ client: Client; auth_key: string }>('/clients', { method: 'POST', body: JSON.stringify(payload) }),
  revealClientSecrets: (id: string) =>
    request<{ auth_key: string }>(`/clients/${encodeURIComponent(id)}/secrets`),
  updateClient: (
    id: string,
    payload: {
      name: string
      description: string
      enabled: boolean
      rate_limit_rpm: number
      expires_at?: string | null
    }
  ) => request<Client>(`/clients/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteClient: (id: string) => request(`/clients/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  rotateClientKey: (id: string) =>
    request<{ auth_key: string; key_prefix: string }>(`/clients/${encodeURIComponent(id)}/rotate-key`, {
      method: 'POST',
    }),

  listCronJobs: () => request<{ items: CronJob[] }>('/cron-jobs'),
  previewCronSchedule: (cronExpr: string) =>
    request<{ next_runs: string[] }>('/cron-jobs/preview', {
      method: 'POST',
      body: JSON.stringify({ cron_expr: cronExpr }),
    }),
  testSafeKeep: (payload: { job_id?: string; api_base_url: string; api_key: string }) =>
    request<{ status: string; upstreams: SafeKeepUpstream[] }>('/cron-jobs/test-safekeep', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  revealCronSecrets: (id: string) =>
    request<{ api_key: string; passphrase: string }>(`/cron-jobs/${encodeURIComponent(id)}/secrets`),
  updateCronJob: (id: string, payload: { cron_expr: string; enabled: boolean; timeout_sec?: number; config?: any }) =>
    request<CronJob>(`/cron-jobs/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload) }),
  runCronJob: (id: string) => request<CronJobRun>(`/cron-jobs/${encodeURIComponent(id)}/run`, { method: 'POST' }),
  listCronRuns: (id: string) => request<{ items: CronJobRun[] }>(`/cron-jobs/${encodeURIComponent(id)}/runs`),

  listAuditLogs: (params: Record<string, string>) => {
    const qs = new URLSearchParams(params).toString()
    return request<{ items: AuditLog[]; total: number }>(`/audit-logs${qs ? '?' + qs : ''}`)
  },
  exportAuditUrl: (params: Record<string, string>, format: 'csv' | 'json') => {
    const qs = new URLSearchParams({ ...params, format }).toString()
    return `${BASE}/audit-logs/export?${qs}`
  },

  getSettings: () => request<SystemSettings>('/settings'),
  updateSettings: (payload: SystemSettings) =>
    request<SystemSettings>('/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  getSMTP: () => request<SMTPConfig>('/settings/smtp'),
  updateSMTP: (payload: {
    host: string
    port: number
    user: string
    password?: string
    clear_password?: boolean
  }) => request<SMTPConfig>('/settings/smtp', { method: 'PUT', body: JSON.stringify(payload) }),
  testSMTP: (payload: {
    host?: string
    port?: number
    user?: string
    password?: string
    clear_password?: boolean
    test_to?: string
  }) => request<{ status: string; host: string; port: number }>('/settings/smtp/test', {
    method: 'POST',
    body: JSON.stringify(payload),
  }),

  listAdmins: () => request<{ items: Admin[] }>('/admins'),
  createAdmin: (username: string, password: string) =>
    request<Admin>('/admins', { method: 'POST', body: JSON.stringify({ username, password }) }),
  updateAdmin: (id: string, payload: { is_active?: boolean; password?: string }) =>
    request<Admin>(`/admins/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(payload) }),

  exportMigration: async (payload: {
    include_mail_logs: boolean
    include_admins: boolean
    include_audit_logs: boolean
    passphrase: string
  }) => {
    const res = await fetch(`${BASE}/migration/export`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
    if (!res.ok) {
      let msg = res.statusText
      try {
        const d = await res.json()
        msg = d?.error?.message || msg
      } catch {
        // ignore
      }
      throw new Error(msg)
    }
    const blob = await res.blob()
    const cd = res.headers.get('Content-Disposition') || ''
    const match = cd.match(/filename="([^"]+)"/)
    const filename = match ? match[1] : 'mailserver-migration.msbundle'
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
  },

  inspectMigration: (formData: FormData) =>
    request<InspectSummary>('/migration/inspect', { method: 'POST', body: formData }),
  importMigration: (formData: FormData) =>
    request<{ counts: InspectSummary['counts'] }>('/migration/import', { method: 'POST', body: formData }),
}

export function formatTime(iso?: string | null, lang = getCurrentLang()): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString(lang === 'en' ? 'en-US' : 'zh-CN', { hour12: false })
}

export function formatBytes(n?: number): string {
  if (n === undefined || n === null || n === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 2)} ${units[i]}`
}

export function formatDuration(ms?: number): string {
  if (ms === undefined || ms === null) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(2)}s`
}

export function translateError(msg: string): string {
  if (!msg) return translate(getCurrentLang(), 'common.unknown')
  return msg
}
