package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite database connection and provides repository methods.
type Store struct {
	db      *sql.DB
	dataDir string
	cfg     *config.Config
}

// Open opens (or creates) the SQLite database in WAL mode, runs migrations, and seeds defaults.
func Open(dbPath, dataDir string, defaults config.SystemSettings) (*Store, error) {
	sqldb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// Serialize writers through a single connection to avoid SQLITE_BUSY under WAL mode.
	sqldb.SetMaxOpenConns(1)
	sqldb.SetMaxIdleConns(1)
	sqldb.SetConnMaxLifetime(0)

	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, p := range pragmas {
		if _, err := sqldb.Exec(p); err != nil {
			_ = sqldb.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}

	s := &Store{db: sqldb, dataDir: dataDir, cfg: &config.Config{Defaults: defaults}}
	if err := s.migrate(context.Background()); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	if err := s.seedDefaults(context.Background(), defaults); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("seed defaults: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Ping checks database connectivity.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS admins (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_active INTEGER NOT NULL DEFAULT 1,
			must_change_password INTEGER NOT NULL DEFAULT 0,
			last_login_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS system_settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL,
			updated_at DATETIME NOT NULL,
			updated_by TEXT NOT NULL DEFAULT ''
		);`,
		`CREATE TABLE IF NOT EXISTS clients (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			key_prefix TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			key_encrypted TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			rate_limit_rpm INTEGER NOT NULL DEFAULT 0,
			expires_at DATETIME,
			total_sent INTEGER NOT NULL DEFAULT 0,
			total_failed INTEGER NOT NULL DEFAULT 0,
			last_used_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_clients_key_hash ON clients(key_hash);`,
		`CREATE TABLE IF NOT EXISTS mail_logs (
			id TEXT PRIMARY KEY,
			client_id TEXT NOT NULL DEFAULT '',
			client_name TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'api',
			recipient TEXT NOT NULL,
			subject TEXT NOT NULL,
			body TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			error_message TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			client_ip TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_mail_logs_created ON mail_logs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_mail_logs_status ON mail_logs(status, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_mail_logs_client ON mail_logs(client_id, created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor_type TEXT NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			actor_name TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'success',
			ip_address TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			request_id TEXT NOT NULL DEFAULT '',
			detail_json TEXT NOT NULL DEFAULT '{}',
			created_at DATETIME NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action, created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS cron_jobs (
			id TEXT PRIMARY KEY,
			job_key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			cron_expr TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			timeout_sec INTEGER NOT NULL DEFAULT 300,
			config_json TEXT NOT NULL DEFAULT '{}',
			last_run_at DATETIME,
			last_status TEXT NOT NULL DEFAULT 'never',
			last_duration_ms INTEGER NOT NULL DEFAULT 0,
			last_summary TEXT NOT NULL DEFAULT '',
			next_run_at DATETIME,
			updated_at DATETIME NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS cron_job_runs (
			id TEXT PRIMARY KEY,
			job_id TEXT NOT NULL REFERENCES cron_jobs(id) ON DELETE CASCADE,
			trigger_type TEXT NOT NULL,
			triggered_by TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			affected_rows INTEGER NOT NULL DEFAULT 0,
			output_log TEXT NOT NULL DEFAULT '',
			started_at DATETIME NOT NULL,
			finished_at DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_cron_runs_job ON cron_job_runs(job_id, started_at DESC);`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	if err := s.ensureColumn(ctx, "admins", "must_change_password", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, columnDef string) error {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan %s column info: %w", table, err)
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnDef)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

func (s *Store) seedDefaults(ctx context.Context, defaults config.SystemSettings) error {
	now := time.Now().UTC()
	pairs := map[string]any{
		"public_base_url":        defaults.PublicBaseURL,
		"send_timeout_sec":       defaults.SendTimeoutSec,
		"store_mail_body":        defaults.StoreMailBody,
		"default_rate_limit_rpm": defaults.DefaultRateLimitRPM,
		"max_body_bytes":         defaults.MaxBodyBytes,
	}
	for k, v := range pairs {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO system_settings(key, value_json, updated_at, updated_by) VALUES (?, ?, ?, 'system')`,
			k, string(b), now,
		); err != nil {
			return err
		}
	}

	type builtinJob struct {
		key        string
		name       string
		desc       string
		expr       string
		enabled    bool
		timeoutSec int
		configJSON string
	}
	jobs := []builtinJob{
		{
			key:        "cleanup_audit_logs",
			name:       "清理过期审计日志",
			desc:       "定期清理超过保留天数的审计日志记录，防止数据库体积无限增长。",
			expr:       "0 2 * * *",
			enabled:    true,
			timeoutSec: 300,
			configJSON: `{"retention_days":90}`,
		},
		{
			key:        "cleanup_mail_records",
			name:       "清理历史发信与执行记录",
			desc:       "定期清理超过保留天数的发信历史记录与定时任务执行日志。",
			expr:       "0 3 * * *",
			enabled:    true,
			timeoutSec: 300,
			configJSON: `{"retention_days":90}`,
		},
		{
			key:        "smtp_health_check",
			name:       "SMTP 连通性巡检",
			desc:       "定时探测当前配置的 SMTP 服务器网络连通性并更新仪表盘状态。",
			expr:       "*/30 * * * *",
			enabled:    true,
			timeoutSec: 60,
			configJSON: `{"probe_timeout_sec":15}`,
		},
		{
			key:        "migration_backup",
			name:       "定时备份环境迁移包至 SafeKeep",
			desc:       "定期导出当前实例配置与凭据迁移包（可选口令加密），调用 SafeKeep API 上传备份。",
			expr:       "0 4 * * *",
			enabled:    false,
			timeoutSec: 300,
			configJSON: `{"path":"mailserver-backups","mode":"sync","include_admins":true,"include_mail_logs":false,"include_audit_logs":false}`,
		},
	}

	for _, j := range jobs {
		if _, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO cron_jobs(id, job_key, name, description, cron_expr, enabled, timeout_sec, config_json, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			crypto.NewULID(), j.key, j.name, j.desc, j.expr, boolToInt(j.enabled), j.timeoutSec, j.configJSON, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
