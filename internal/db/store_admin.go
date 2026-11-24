package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
)

var (
	ErrNotFound           = errors.New("record not found")
	ErrAlreadyInitialized = errors.New("system is already initialized")
)

// DB returns the underlying *sql.DB for transactional operations such as migration.
func (s *Store) DB() *sql.DB {
	return s.db
}

// DataDir returns the configured root data directory.
func (s *Store) DataDir() string {
	return s.dataDir
}

// --- System Settings ---

func (s *Store) GetSystemSettings(ctx context.Context) (config.SystemSettings, error) {
	settings := s.cfg.Defaults
	rows, err := s.db.QueryContext(ctx, `SELECT key, value_json FROM system_settings`)
	if err != nil {
		return settings, err
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return settings, err
		}
		switch k {
		case "public_base_url":
			_ = json.Unmarshal([]byte(v), &settings.PublicBaseURL)
		case "send_timeout_sec":
			_ = json.Unmarshal([]byte(v), &settings.SendTimeoutSec)
		case "store_mail_body":
			_ = json.Unmarshal([]byte(v), &settings.StoreMailBody)
		case "default_rate_limit_rpm":
			_ = json.Unmarshal([]byte(v), &settings.DefaultRateLimitRPM)
		case "max_body_bytes":
			_ = json.Unmarshal([]byte(v), &settings.MaxBodyBytes)
		}
	}
	return settings, rows.Err()
}

func (s *Store) UpdateSystemSettings(ctx context.Context, settings config.SystemSettings, updatedBy string) error {
	pairs := map[string]any{
		"public_base_url":        strings.TrimRight(strings.TrimSpace(settings.PublicBaseURL), "/"),
		"send_timeout_sec":       settings.SendTimeoutSec,
		"store_mail_body":        settings.StoreMailBody,
		"default_rate_limit_rpm": settings.DefaultRateLimitRPM,
		"max_body_bytes":         settings.MaxBodyBytes,
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for k, v := range pairs {
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO system_settings(key, value_json, updated_at, updated_by)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
			k, string(raw), now, updatedBy,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// --- SMTP Configuration & Health Status ---

func (s *Store) GetSMTPConfig(ctx context.Context, cipher *crypto.Cipher) (SMTPConfig, error) {
	cfg := SMTPConfig{Port: 465}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM system_settings WHERE key = 'smtp_config'`).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, err
	}
	if cfg.PasswordEncrypted != "" && cipher != nil {
		plain, err := cipher.DecryptString(cfg.PasswordEncrypted)
		if err != nil {
			return cfg, fmt.Errorf("decrypt smtp password: %w", err)
		}
		cfg.Password = plain
	}
	return cfg, nil
}

func (s *Store) SaveSMTPConfig(ctx context.Context, cipher *crypto.Cipher, cfg SMTPConfig, updatedBy string) error {
	toStore := SMTPConfig{
		Host: strings.TrimSpace(cfg.Host),
		Port: cfg.Port,
		User: strings.TrimSpace(cfg.User),
	}
	if toStore.Port <= 0 {
		toStore.Port = 465
	}
	if cfg.Password != "" && cipher != nil {
		enc, err := cipher.EncryptString(cfg.Password)
		if err != nil {
			return err
		}
		toStore.PasswordEncrypted = enc
	} else if cfg.PasswordEncrypted != "" {
		toStore.PasswordEncrypted = cfg.PasswordEncrypted
	}

	raw, err := json.Marshal(toStore)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO system_settings(key, value_json, updated_at, updated_by)
		 VALUES ('smtp_config', ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
		string(raw), time.Now().UTC(), updatedBy,
	)
	return err
}

// SeedSMTPFromConfigIfEmpty seeds SMTP settings from legacy config.yaml email section on first boot.
func (s *Store) SeedSMTPFromConfigIfEmpty(ctx context.Context, cipher *crypto.Cipher, emailCfg config.EmailConfig) error {
	if strings.TrimSpace(emailCfg.Host) == "" {
		return nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM system_settings WHERE key = 'smtp_config'`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	port := emailCfg.Port
	if port <= 0 {
		port = 465
	}
	return s.SaveSMTPConfig(ctx, cipher, SMTPConfig{
		Host:     emailCfg.Host,
		Port:     port,
		User:     emailCfg.User,
		Password: emailCfg.Password,
	}, "config_seed")
}

func (s *Store) GetSMTPStatus(ctx context.Context) (SMTPStatus, error) {
	st := SMTPStatus{Status: "untested"}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM system_settings WHERE key = 'smtp_status'`).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return st, nil
		}
		return st, err
	}
	_ = json.Unmarshal([]byte(raw), &st)
	if st.Status == "" {
		st.Status = "untested"
	}
	return st, nil
}

func (s *Store) UpdateSMTPStatus(ctx context.Context, status, errMsg string) error {
	now := time.Now().UTC()
	st := SMTPStatus{
		Status:   status,
		Error:    errMsg,
		TestedAt: &now,
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO system_settings(key, value_json, updated_at, updated_by)
		 VALUES ('smtp_status', ?, ?, 'system')
		 ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
		string(raw), now,
	)
	return err
}

// --- Admins ---

func (s *Store) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdmin(ctx context.Context, a *Admin) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO admins(id, username, password_hash, is_active, must_change_password, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Username, a.PasswordHash, boolToInt(a.IsActive), boolToInt(a.MustChangePassword), a.CreatedAt, a.UpdatedAt,
	)
	return err
}

func (s *Store) CreateInitialAdminIfEmpty(ctx context.Context, a *Admin) error {
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var n int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrAlreadyInitialized
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO admins(id, username, password_hash, is_active, must_change_password, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Username, a.PasswordHash, boolToInt(a.IsActive), boolToInt(a.MustChangePassword), a.CreatedAt, a.UpdatedAt,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetAdminByUsername(ctx context.Context, username string) (*Admin, error) {
	return scanAdmin(s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, is_active, must_change_password, last_login_at, created_at, updated_at
		 FROM admins WHERE username = ?`, username))
}

func (s *Store) GetAdminByID(ctx context.Context, id string) (*Admin, error) {
	return scanAdmin(s.db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, is_active, must_change_password, last_login_at, created_at, updated_at
		 FROM admins WHERE id = ?`, id))
}

func (s *Store) ListAdmins(ctx context.Context) ([]Admin, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, username, password_hash, is_active, must_change_password, last_login_at, created_at, updated_at
		 FROM admins ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Admin
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *a)
	}
	return list, rows.Err()
}

func (s *Store) UpdateAdminLogin(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE admins SET last_login_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}

func (s *Store) UpdateAdminPassword(ctx context.Context, id, passwordHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE admins SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`, passwordHash, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetAdminOnStartup resets an existing admin account (preferring "admin", otherwise the earliest active/created admin)
// or initializes a new "admin" account if none exists, generating a temporary password with must_change_password=true
// and printing it to out.
func (s *Store) ResetAdminOnStartup(ctx context.Context, out io.Writer) (string, string, error) {
	if out == nil {
		out = os.Stdout
	}

	tempPassword, err := crypto.GenerateTempPassword(20)
	if err != nil {
		return "", "", fmt.Errorf("generate temporary admin password: %w", err)
	}
	passHash, err := crypto.HashPassword(tempPassword)
	if err != nil {
		return "", "", fmt.Errorf("hash temporary admin password: %w", err)
	}

	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	var targetID, targetUsername string
	err = tx.QueryRowContext(ctx,
		`SELECT id, username
		 FROM admins
		 ORDER BY CASE WHEN username = 'admin' THEN 0 ELSE 1 END, is_active DESC, created_at ASC
		 LIMIT 1`,
	).Scan(&targetID, &targetUsername)

	created := false
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return "", "", fmt.Errorf("query admin for reset: %w", err)
		}
		created = true
		targetID = crypto.NewULID()
		targetUsername = "admin"
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admins(id, username, password_hash, is_active, must_change_password, created_at, updated_at)
			 VALUES (?, ?, ?, 1, 1, ?, ?)`,
			targetID, targetUsername, passHash, now, now,
		); err != nil {
			return "", "", fmt.Errorf("create temporary admin: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`UPDATE admins
			 SET password_hash = ?, is_active = 1, must_change_password = 1, updated_at = ?
			 WHERE id = ?`,
			passHash, now, targetID,
		); err != nil {
			return "", "", fmt.Errorf("reset admin password: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", "", err
	}

	_ = s.InsertAuditLog(ctx, &AuditLog{
		ActorType:    "system",
		ActorID:      "startup",
		ActorName:    "RESET_ADMIN",
		Action:       "admin.password_reset",
		ResourceType: "admin",
		ResourceID:   targetID,
		Status:       "success",
		DetailJSON:   fmt.Sprintf(`{"username":%q,"created":%t}`, targetUsername, created),
		CreatedAt:    now,
	})

	reason := "Admin Password Reset (RESET_ADMIN active)"
	if created {
		reason = "Initial Setup (RESET_ADMIN active)"
	}
	fmt.Fprintf(out, "\n========================================================================\n")
	fmt.Fprintf(out, "  MailServer - Web UI Admin Credentials (%s)\n", reason)
	fmt.Fprintf(out, "  Username           : %s\n", targetUsername)
	fmt.Fprintf(out, "  Temporary Password : %s\n", tempPassword)
	fmt.Fprintf(out, "  Action Required    : Sign in to the Web UI and set a new password.\n")
	fmt.Fprintf(out, "                       Remove RESET_ADMIN / MAILSERVER_RESET_ADMIN after reset.\n")
	fmt.Fprintf(out, "========================================================================\n\n")

	return targetUsername, tempPassword, nil
}

func (s *Store) UpdateAdminActive(ctx context.Context, id string, active bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE admins SET is_active = ?, updated_at = ? WHERE id = ?`, boolToInt(active), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Clients (Multi-Client auth_key) ---

func (s *Store) CreateClient(ctx context.Context, c *Client) error {
	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	c.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO clients(
			id, name, description, key_prefix, key_hash, key_encrypted,
			enabled, rate_limit_rpm, expires_at, total_sent, total_failed,
			last_used_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Description, c.KeyPrefix, c.KeyHash, c.KeyEncrypted,
		boolToInt(c.Enabled), c.RateLimitRPM, c.ExpiresAt, c.TotalSent, c.TotalFailed,
		c.LastUsedAt, c.CreatedAt, c.UpdatedAt,
	)
	return err
}

func (s *Store) ListClients(ctx context.Context) ([]Client, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, key_prefix, key_hash, key_encrypted,
		        enabled, rate_limit_rpm, expires_at, total_sent, total_failed,
		        last_used_at, created_at, updated_at
		 FROM clients ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Client
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

func (s *Store) GetClientByID(ctx context.Context, id string) (*Client, error) {
	return scanClient(s.db.QueryRowContext(ctx,
		`SELECT id, name, description, key_prefix, key_hash, key_encrypted,
		        enabled, rate_limit_rpm, expires_at, total_sent, total_failed,
		        last_used_at, created_at, updated_at
		 FROM clients WHERE id = ?`, id))
}

func (s *Store) GetClientByKeyHash(ctx context.Context, keyHash string) (*Client, error) {
	return scanClient(s.db.QueryRowContext(ctx,
		`SELECT id, name, description, key_prefix, key_hash, key_encrypted,
		        enabled, rate_limit_rpm, expires_at, total_sent, total_failed,
		        last_used_at, created_at, updated_at
		 FROM clients WHERE key_hash = ?`, keyHash))
}

func (s *Store) UpdateClient(ctx context.Context, c *Client) error {
	c.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE clients SET
			name = ?, description = ?, enabled = ?, rate_limit_rpm = ?,
			expires_at = ?, updated_at = ?
		 WHERE id = ?`,
		c.Name, c.Description, boolToInt(c.Enabled), c.RateLimitRPM,
		c.ExpiresAt, c.UpdatedAt, c.ID,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RotateClientKey(ctx context.Context, id, keyPrefix, keyHash, keyEncrypted string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE clients SET key_prefix = ?, key_hash = ?, key_encrypted = ?, updated_at = ? WHERE id = ?`,
		keyPrefix, keyHash, keyEncrypted, time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteClient(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) RecordClientUsage(ctx context.Context, id string, succeeded bool) error {
	now := time.Now().UTC()
	if succeeded {
		_, err := s.db.ExecContext(ctx,
			`UPDATE clients SET total_sent = total_sent + 1, last_used_at = ?, updated_at = ? WHERE id = ?`,
			now, now, id,
		)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE clients SET total_failed = total_failed + 1, last_used_at = ?, updated_at = ? WHERE id = ?`,
		now, now, id,
	)
	return err
}

// --- Scanners ---

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAdmin(row rowScanner) (*Admin, error) {
	var a Admin
	var isActive, mustChange int
	var lastLogin sql.NullTime
	err := row.Scan(&a.ID, &a.Username, &a.PasswordHash, &isActive, &mustChange, &lastLogin, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.IsActive = isActive == 1
	a.MustChangePassword = mustChange == 1
	if lastLogin.Valid {
		t := lastLogin.Time
		a.LastLoginAt = &t
	}
	return &a, nil
}

func scanClient(row rowScanner) (*Client, error) {
	var c Client
	var enabled int
	var expiresAt, lastUsed sql.NullTime
	err := row.Scan(
		&c.ID, &c.Name, &c.Description, &c.KeyPrefix, &c.KeyHash, &c.KeyEncrypted,
		&enabled, &c.RateLimitRPM, &expiresAt, &c.TotalSent, &c.TotalFailed,
		&lastUsed, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Enabled = enabled == 1
	if expiresAt.Valid {
		t := expiresAt.Time
		c.ExpiresAt = &t
	}
	if lastUsed.Valid {
		t := lastUsed.Time
		c.LastUsedAt = &t
	}
	return &c, nil
}
