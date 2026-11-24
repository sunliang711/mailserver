package db

import "time"

type Admin struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	PasswordHash       string     `json:"-"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type SMTPConfig struct {
	Host              string `json:"host"`
	Port              int    `json:"port"`
	User              string `json:"user"`
	Password          string `json:"password,omitempty"`
	PasswordEncrypted string `json:"password_encrypted,omitempty"`
}

type SMTPStatus struct {
	Status   string     `json:"status"` // untested | ok | failed
	Error    string     `json:"error,omitempty"`
	TestedAt *time.Time `json:"tested_at,omitempty"`
}

type Client struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	KeyPrefix    string     `json:"key_prefix"`
	KeyHash      string     `json:"-"`
	KeyEncrypted string     `json:"-"`
	Enabled      bool       `json:"enabled"`
	RateLimitRPM int        `json:"rate_limit_rpm"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	TotalSent    int64      `json:"total_sent"`
	TotalFailed  int64      `json:"total_failed"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type MailLog struct {
	ID           string    `json:"id"`
	ClientID     string    `json:"client_id,omitempty"`
	ClientName   string    `json:"client_name"`
	Source       string    `json:"source"` // api | web_test
	Recipient    string    `json:"recipient"`
	Subject      string    `json:"subject"`
	Body         string    `json:"body,omitempty"`
	Status       string    `json:"status"` // succeeded | failed
	ErrorMessage string    `json:"error_message,omitempty"`
	DurationMs   int64     `json:"duration_ms"`
	ClientIP     string    `json:"client_ip,omitempty"`
	RequestID    string    `json:"request_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type MailLogFilter struct {
	ClientID string
	Status   string
	Source   string
	Query    string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

type AuditLog struct {
	ID           int64     `json:"id"`
	ActorType    string    `json:"actor_type"` // admin | client | system
	ActorID      string    `json:"actor_id"`
	ActorName    string    `json:"actor_name"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Status       string    `json:"status"` // success | failure
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
	RequestID    string    `json:"request_id"`
	DetailJSON   string    `json:"detail_json"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditFilter struct {
	ActorType string
	Action    string
	Status    string
	Query     string
	From      *time.Time
	To        *time.Time
	Limit     int
	Offset    int
}

type CronJob struct {
	ID              string      `json:"id"`
	JobKey          string      `json:"job_key"`
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	CronExpr        string      `json:"cron_expr"`
	DefaultSchedule string      `json:"default_schedule,omitempty"`
	Enabled         bool        `json:"enabled"`
	TimeoutSec      int         `json:"timeout_sec"`
	ConfigJSON      string      `json:"config_json"`
	LastRunAt       *time.Time  `json:"last_run_at,omitempty"`
	LastStatus      string      `json:"last_status"`
	LastDurationMs  int64       `json:"last_duration_ms"`
	LastSummary     string      `json:"last_summary,omitempty"`
	NextRunAt       *time.Time  `json:"next_run_at,omitempty"`
	NextRuns        []time.Time `json:"next_runs,omitempty"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

type CronJobRun struct {
	ID           string     `json:"id"`
	JobID        string     `json:"job_id"`
	TriggerType  string     `json:"trigger_type"` // schedule | manual
	TriggeredBy  string     `json:"triggered_by"`
	Status       string     `json:"status"` // running | succeeded | failed | skipped | timeout
	AffectedRows int64      `json:"affected_rows"`
	OutputLog    string     `json:"output_log"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type DailyStat struct {
	Date      string `json:"date"`
	Succeeded int64  `json:"succeeded"`
	Failed    int64  `json:"failed"`
}

type DashboardSummary struct {
	TotalSent      int64       `json:"total_sent"`
	SucceededCount int64       `json:"succeeded_count"`
	FailedCount    int64       `json:"failed_count"`
	TodaySent      int64       `json:"today_sent"`
	TodaySucceeded int64       `json:"today_succeeded"`
	TodayFailed    int64       `json:"today_failed"`
	EnabledClients int64       `json:"enabled_clients"`
	TotalClients   int64       `json:"total_clients"`
	SMTPConfigured bool        `json:"smtp_configured"`
	SMTPHost       string      `json:"smtp_host,omitempty"`
	SMTPPort       int         `json:"smtp_port,omitempty"`
	SMTPTestStatus string      `json:"smtp_test_status"`
	SMTPTestError  string      `json:"smtp_test_error,omitempty"`
	SMTPTestedAt   *time.Time  `json:"smtp_tested_at,omitempty"`
	DailySeries    []DailyStat `json:"daily_series"`
	RecentMails    []MailLog   `json:"recent_mails"`
	CronJobs       []CronJob   `json:"cron_jobs"`
}
