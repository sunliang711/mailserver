export interface Admin {
  id: string
  username: string
  role: 'admin' | 'viewer'
  is_active: boolean
  must_change_password?: boolean
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface AdminSession {
  id: string
  client_ip: string
  user_agent: string
  created_at: string
  last_seen_at: string
  expires_at: string
  current: boolean
}

export interface Client {
  id: string
  name: string
  description: string
  key_prefix: string
  has_api_key?: boolean
  enabled: boolean
  rate_limit_rpm: number
  expires_at?: string
  total_sent: number
  total_failed: number
  last_used_at?: string
  created_at: string
  updated_at: string
}

export interface MailLog {
  id: string
  client_id?: string
  client_name: string
  source: 'api' | 'web_test' | string
  recipient: string
  subject: string
  body?: string
  status: 'succeeded' | 'failed' | string
  error_message?: string
  duration_ms: number
  client_ip?: string
  request_id?: string
  created_at: string
}

export interface AuditLog {
  id: number
  actor_type: 'admin' | 'client' | 'system' | string
  actor_id: string
  actor_name: string
  action: string
  resource_type: string
  resource_id: string
  status: 'success' | 'failure' | string
  ip_address: string
  user_agent: string
  request_id: string
  detail_json: string
  created_at: string
}

export interface CronJob {
  id: string
  job_key: string
  name: string
  description: string
  cron_expr: string
  default_schedule?: string
  enabled: boolean
  timeout_sec: number
  config_json: string
  last_run_at?: string
  last_status: string
  last_duration_ms: number
  last_summary?: string
  next_run_at?: string
  next_runs?: string[]
  updated_at: string
}

export interface CronJobRun {
  id: string
  job_id: string
  trigger_type: 'schedule' | 'manual' | string
  triggered_by: string
  status: string
  affected_rows: number
  output_log: string
  started_at: string
  finished_at?: string
}

export interface SafeKeepUpstream {
  id: string
  name: string
  type: string
  is_default: boolean
}

export interface SystemSettings {
  public_base_url: string
  send_timeout_sec: number
  store_mail_body: boolean
  default_rate_limit_rpm: number
  max_body_bytes: number
}

export interface SMTPConfig {
  host: string
  port: number
  user: string
  has_password: boolean
  password?: string
  test_status: string
  test_error?: string
  tested_at?: string
}

export interface DailyStat {
  date: string
  succeeded: number
  failed: number
}

export interface DashboardSummary {
  total_sent: number
  succeeded_count: number
  failed_count: number
  today_sent: number
  today_succeeded: number
  today_failed: number
  enabled_clients: number
  total_clients: number
  smtp_configured: boolean
  smtp_host?: string
  smtp_port?: number
  smtp_test_status: string
  smtp_test_error?: string
  smtp_tested_at?: string
  daily_series: DailyStat[]
  recent_mails: MailLog[]
  cron_jobs: CronJob[]
}

export interface InspectSummary {
  format_version: number
  exported_at: string
  includes: { mail_logs: boolean; admins: boolean; audit_logs: boolean }
  counts: {
    system_settings: number
    clients: number
    cron_jobs: number
    admins: number
    mail_logs: number
    audit_logs: number
  }
  client_conflicts?: string[]
  admin_conflicts?: string[]
}
