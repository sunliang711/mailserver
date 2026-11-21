package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
)

const FormatVersion = 1

type ExportOptions struct {
	IncludeMailLogs  bool   `json:"include_mail_logs"`
	IncludeAdmins    bool   `json:"include_admins"`
	IncludeAuditLogs bool   `json:"include_audit_logs"`
	Passphrase       string `json:"passphrase"`
}

type ImportOptions struct {
	Passphrase       string `json:"passphrase"`
	Mode             string `json:"mode"`              // replace | merge
	ConflictStrategy string `json:"conflict_strategy"` // skip | overwrite
	IncludeMailLogs  bool   `json:"include_mail_logs"`
	IncludeAdmins    bool   `json:"include_admins"`
	IncludeAuditLogs bool   `json:"include_audit_logs"`
}

type Bundle struct {
	FormatVersion int           `json:"format_version"`
	ExportedAt    time.Time     `json:"exported_at"`
	Includes      BundleInclude `json:"includes"`
	Checksum      string        `json:"checksum_sha256"`
	Data          BundleData    `json:"data"`
}

type BundleInclude struct {
	MailLogs  bool `json:"mail_logs"`
	Admins    bool `json:"admins"`
	AuditLogs bool `json:"audit_logs"`
}

type BundleData struct {
	SystemSettings []SettingRecord `json:"system_settings"`
	Clients        []ClientRecord  `json:"clients"`
	CronJobs       []db.CronJob    `json:"cron_jobs"`
	Admins         []AdminRecord   `json:"admins,omitempty"`
	MailLogs       []db.MailLog    `json:"mail_logs,omitempty"`
	AuditLogs      []db.AuditLog   `json:"audit_logs,omitempty"`
}

type SettingRecord struct {
	Key       string    `json:"key"`
	ValueJSON string    `json:"value_json"`
	UpdatedAt time.Time `json:"updated_at"`
	UpdatedBy string    `json:"updated_by"`
}

type ClientRecord struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	KeyPrefix    string     `json:"key_prefix"`
	KeyHash      string     `json:"key_hash"`
	APIKeyPlain  string     `json:"api_key_plain,omitempty"`
	Enabled      bool       `json:"enabled"`
	RateLimitRPM int        `json:"rate_limit_rpm"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	TotalSent    int64      `json:"total_sent"`
	TotalFailed  int64      `json:"total_failed"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type AdminRecord struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"password_hash"`
	IsActive     bool       `json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type InspectSummary struct {
	FormatVersion   int           `json:"format_version"`
	ExportedAt      time.Time     `json:"exported_at"`
	Includes        BundleInclude `json:"includes"`
	Counts          BundleCounts  `json:"counts"`
	ClientConflicts []string      `json:"client_conflicts"`
	AdminConflicts  []string      `json:"admin_conflicts"`
}

type BundleCounts struct {
	SystemSettings int `json:"system_settings"`
	Clients        int `json:"clients"`
	CronJobs       int `json:"cron_jobs"`
	Admins         int `json:"admins"`
	MailLogs       int `json:"mail_logs"`
	AuditLogs      int `json:"audit_logs"`
}

type Service struct {
	store  *db.Store
	cipher *crypto.Cipher
}

func NewService(store *db.Store, cipher *crypto.Cipher) *Service {
	return &Service{store: store, cipher: cipher}
}

// Export decrypts local secrets, builds a portable Bundle, and encrypts it with opts.Passphrase.
func (s *Service) Export(ctx context.Context, opts ExportOptions) ([]byte, error) {
	if opts.Passphrase != "" && len(opts.Passphrase) < 6 {
		return nil, fmt.Errorf("migration passphrase must be at least 6 characters (or empty)")
	}

	var data BundleData

	// 1. System settings (decrypt smtp_config password for cross-instance portability)
	rows, err := s.store.DB().QueryContext(ctx, `SELECT key, value_json, updated_at, updated_by FROM system_settings`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r SettingRecord
		if err := rows.Scan(&r.Key, &r.ValueJSON, &r.UpdatedAt, &r.UpdatedBy); err != nil {
			rows.Close()
			return nil, err
		}
		if r.Key == "smtp_config" {
			var sc db.SMTPConfig
			if json.Unmarshal([]byte(r.ValueJSON), &sc) == nil && sc.PasswordEncrypted != "" {
				plain, decErr := s.cipher.DecryptString(sc.PasswordEncrypted)
				if decErr != nil {
					rows.Close()
					return nil, fmt.Errorf("decrypt smtp password: %w", decErr)
				}
				sc.Password = plain
				sc.PasswordEncrypted = ""
				if b, mErr := json.Marshal(sc); mErr == nil {
					r.ValueJSON = string(b)
				}
			}
		}
		data.SystemSettings = append(data.SystemSettings, r)
	}
	rows.Close()

	// 2. Clients
	clients, err := s.store.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range clients {
		apiKeyPlain, err := s.cipher.DecryptString(c.KeyEncrypted)
		if err != nil {
			return nil, fmt.Errorf("decrypt client %s api key: %w", c.Name, err)
		}
		data.Clients = append(data.Clients, ClientRecord{
			ID:           c.ID,
			Name:         c.Name,
			Description:  c.Description,
			KeyPrefix:    c.KeyPrefix,
			KeyHash:      c.KeyHash,
			APIKeyPlain:  apiKeyPlain,
			Enabled:      c.Enabled,
			RateLimitRPM: c.RateLimitRPM,
			ExpiresAt:    c.ExpiresAt,
			TotalSent:    c.TotalSent,
			TotalFailed:  c.TotalFailed,
			LastUsedAt:   c.LastUsedAt,
			CreatedAt:    c.CreatedAt,
			UpdatedAt:    c.UpdatedAt,
		})
	}

	// 3. Cron jobs
	cronJobs, err := s.store.ListCronJobs(ctx)
	if err != nil {
		return nil, err
	}
	for i := range cronJobs {
		if cronJobs[i].JobKey == "migration_backup" {
			cfgJSON, cErr := reencryptCronConfigForExport(s.cipher, cronJobs[i].ConfigJSON)
			if cErr != nil {
				return nil, fmt.Errorf("decrypt migration_backup cron config: %w", cErr)
			}
			cronJobs[i].ConfigJSON = cfgJSON
		}
	}
	data.CronJobs = cronJobs

	// 4. Optional Admins
	if opts.IncludeAdmins {
		admins, err := s.store.ListAdmins(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range admins {
			data.Admins = append(data.Admins, AdminRecord{
				ID:           a.ID,
				Username:     a.Username,
				PasswordHash: a.PasswordHash,
				IsActive:     a.IsActive,
				LastLoginAt:  a.LastLoginAt,
				CreatedAt:    a.CreatedAt,
				UpdatedAt:    a.UpdatedAt,
			})
		}
	}

	// 5. Optional Mail Logs
	if opts.IncludeMailLogs {
		logs, err := s.store.ExportAllMailLogs(ctx, db.MailLogFilter{})
		if err != nil {
			return nil, err
		}
		data.MailLogs = logs
	}

	// 6. Optional Audit Logs
	if opts.IncludeAuditLogs {
		logs, err := s.store.ExportAllAuditLogs(ctx, db.AuditFilter{})
		if err != nil {
			return nil, err
		}
		data.AuditLogs = logs
	}

	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(dataBytes)

	bundle := Bundle{
		FormatVersion: FormatVersion,
		ExportedAt:    time.Now().UTC(),
		Includes: BundleInclude{
			MailLogs:  opts.IncludeMailLogs,
			Admins:    opts.IncludeAdmins,
			AuditLogs: opts.IncludeAuditLogs,
		},
		Checksum: hex.EncodeToString(sum[:]),
		Data:     data,
	}

	rawBundle, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	return crypto.EncryptWithPassphrase(rawBundle, opts.Passphrase)
}

func decodeBundle(encrypted []byte, passphrase string) (*Bundle, error) {
	raw, err := crypto.DecryptWithPassphrase(encrypted, passphrase)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		FormatVersion int             `json:"format_version"`
		ExportedAt    time.Time       `json:"exported_at"`
		Includes      BundleInclude   `json:"includes"`
		Checksum      string          `json:"checksum_sha256"`
		Data          json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse bundle json: %w", err)
	}
	if envelope.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported migration format_version %d", envelope.FormatVersion)
	}
	if envelope.Checksum != "" {
		sum := sha256.Sum256(envelope.Data)
		if hex.EncodeToString(sum[:]) != envelope.Checksum {
			return nil, fmt.Errorf("bundle checksum verification failed")
		}
	}
	var data BundleData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return nil, fmt.Errorf("parse bundle data: %w", err)
	}
	return &Bundle{
		FormatVersion: envelope.FormatVersion,
		ExportedAt:    envelope.ExportedAt,
		Includes:      envelope.Includes,
		Checksum:      envelope.Checksum,
		Data:          data,
	}, nil
}

// Inspect decrypts a bundle without modifying the database and reports counts & conflicts.
func (s *Service) Inspect(ctx context.Context, encrypted []byte, passphrase string) (*InspectSummary, error) {
	b, err := decodeBundle(encrypted, passphrase)
	if err != nil {
		return nil, err
	}

	summary := &InspectSummary{
		FormatVersion: b.FormatVersion,
		ExportedAt:    b.ExportedAt,
		Includes:      b.Includes,
		Counts: BundleCounts{
			SystemSettings: len(b.Data.SystemSettings),
			Clients:        len(b.Data.Clients),
			CronJobs:       len(b.Data.CronJobs),
			Admins:         len(b.Data.Admins),
			MailLogs:       len(b.Data.MailLogs),
			AuditLogs:      len(b.Data.AuditLogs),
		},
	}

	existingClients, _ := s.store.ListClients(ctx)
	clNames := make(map[string]bool)
	for _, c := range existingClients {
		clNames[c.ID] = true
		clNames[c.Name] = true
	}
	for _, c := range b.Data.Clients {
		if clNames[c.ID] || clNames[c.Name] {
			summary.ClientConflicts = append(summary.ClientConflicts, c.Name)
		}
	}

	existingAdmins, _ := s.store.ListAdmins(ctx)
	adNames := make(map[string]bool)
	for _, a := range existingAdmins {
		adNames[a.ID] = true
		adNames[a.Username] = true
	}
	for _, a := range b.Data.Admins {
		if adNames[a.ID] || adNames[a.Username] {
			summary.AdminConflicts = append(summary.AdminConflicts, a.Username)
		}
	}

	return summary, nil
}

// Import applies a migration bundle atomically inside a SQLite transaction,
// re-encrypting all secrets using the target environment's master key.
func (s *Service) Import(ctx context.Context, encrypted []byte, opts ImportOptions) (*BundleCounts, error) {
	b, err := decodeBundle(encrypted, opts.Passphrase)
	if err != nil {
		return nil, err
	}

	mode := opts.Mode
	if mode == "" {
		mode = "merge"
	}
	if mode != "replace" && mode != "merge" {
		return nil, fmt.Errorf("invalid import mode %q (must be replace or merge)", mode)
	}
	conflict := opts.ConflictStrategy
	if conflict == "" {
		conflict = "skip"
	}
	if conflict != "skip" && conflict != "overwrite" {
		return nil, fmt.Errorf("invalid conflict_strategy %q (must be skip or overwrite)", conflict)
	}

	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if mode == "replace" {
		if opts.IncludeMailLogs && b.Includes.MailLogs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM mail_logs`); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM clients`); err != nil {
			return nil, err
		}
		if opts.IncludeAuditLogs && b.Includes.AuditLogs {
			if _, err := tx.ExecContext(ctx, `DELETE FROM audit_logs`); err != nil {
				return nil, err
			}
		}
	}

	counts := &BundleCounts{}
	clientIDMap := make(map[string]string, len(b.Data.Clients))

	// 1. System Settings (re-encrypt smtp_config password)
	for _, st := range b.Data.SystemSettings {
		valJSON := st.ValueJSON
		if st.Key == "smtp_config" {
			var sc db.SMTPConfig
			if json.Unmarshal([]byte(valJSON), &sc) == nil && sc.Password != "" {
				enc, encErr := s.cipher.EncryptString(sc.Password)
				if encErr != nil {
					return nil, encErr
				}
				sc.PasswordEncrypted = enc
				sc.Password = ""
				if mBytes, mErr := json.Marshal(sc); mErr == nil {
					valJSON = string(mBytes)
				}
			}
		}
		if mode == "merge" && conflict == "skip" {
			if _, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO system_settings(key, value_json, updated_at, updated_by) VALUES (?, ?, ?, ?)`,
				st.Key, valJSON, st.UpdatedAt, st.UpdatedBy,
			); err != nil {
				return nil, err
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO system_settings(key, value_json, updated_at, updated_by) VALUES (?, ?, ?, ?)
				 ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
				st.Key, valJSON, st.UpdatedAt, st.UpdatedBy,
			); err != nil {
				return nil, err
			}
		}
		counts.SystemSettings++
	}

	// 2. Clients (re-encrypt API keys, preserve key_hash)
	for _, c := range b.Data.Clients {
		keyEnc, err := s.cipher.EncryptString(c.APIKeyPlain)
		if err != nil {
			return nil, err
		}
		targetID := c.ID
		var existingByID, existingByName, existingByHash string
		_ = tx.QueryRowContext(ctx, `SELECT id FROM clients WHERE id = ?`, c.ID).Scan(&existingByID)
		_ = tx.QueryRowContext(ctx, `SELECT id FROM clients WHERE name = ?`, c.Name).Scan(&existingByName)
		_ = tx.QueryRowContext(ctx, `SELECT id FROM clients WHERE key_hash = ?`, c.KeyHash).Scan(&existingByHash)

		if mode == "merge" && conflict == "skip" {
			if existingByID != "" {
				clientIDMap[c.ID] = existingByID
				continue
			}
			if existingByName != "" {
				clientIDMap[c.ID] = existingByName
				continue
			}
			if existingByHash != "" {
				clientIDMap[c.ID] = existingByHash
				continue
			}
		}
		if existingByID == "" {
			if existingByName != "" {
				targetID = existingByName
			} else if existingByHash != "" {
				targetID = existingByHash
			}
		}
		clientIDMap[c.ID] = targetID

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO clients(
				id, name, description, key_prefix, key_hash, key_encrypted,
				enabled, rate_limit_rpm, expires_at, total_sent, total_failed,
				last_used_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name=excluded.name, description=excluded.description, key_prefix=excluded.key_prefix,
				key_hash=excluded.key_hash, key_encrypted=excluded.key_encrypted, enabled=excluded.enabled,
				rate_limit_rpm=excluded.rate_limit_rpm, expires_at=excluded.expires_at,
				total_sent=excluded.total_sent, total_failed=excluded.total_failed,
				last_used_at=excluded.last_used_at, updated_at=excluded.updated_at`,
			targetID, c.Name, c.Description, c.KeyPrefix, c.KeyHash, keyEnc,
			boolInt(c.Enabled), c.RateLimitRPM, c.ExpiresAt, c.TotalSent, c.TotalFailed,
			c.LastUsedAt, c.CreatedAt, c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		counts.Clients++
	}

	// 3. CronJobs
	for _, j := range b.Data.CronJobs {
		if mode == "merge" && conflict == "skip" {
			continue
		}
		cfgJSON := j.ConfigJSON
		if j.JobKey == "migration_backup" {
			var cErr error
			cfgJSON, cErr = reencryptCronConfigForImport(s.cipher, cfgJSON)
			if cErr != nil {
				return nil, fmt.Errorf("re-encrypt migration_backup cron config: %w", cErr)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE cron_jobs SET cron_expr = ?, enabled = ?, timeout_sec = CASE WHEN ? > 0 THEN ? ELSE timeout_sec END, config_json = ?, updated_at = ? WHERE job_key = ?`,
			j.CronExpr, boolInt(j.Enabled), j.TimeoutSec, j.TimeoutSec, cfgJSON, time.Now().UTC(), j.JobKey,
		); err != nil {
			return nil, err
		}
		counts.CronJobs++
	}

	// 4. Optional Admins
	if opts.IncludeAdmins && b.Includes.Admins {
		for _, a := range b.Data.Admins {
			var existingByID, existingByUser string
			_ = tx.QueryRowContext(ctx, `SELECT id FROM admins WHERE id = ?`, a.ID).Scan(&existingByID)
			_ = tx.QueryRowContext(ctx, `SELECT id FROM admins WHERE username = ?`, a.Username).Scan(&existingByUser)

			if mode == "merge" && conflict == "skip" && (existingByID != "" || existingByUser != "") {
				continue
			}
			targetID := a.ID
			if existingByID == "" && existingByUser != "" {
				targetID = existingByUser
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO admins(id, username, password_hash, is_active, last_login_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET username=excluded.username, password_hash=excluded.password_hash, is_active=excluded.is_active, updated_at=excluded.updated_at`,
				targetID, a.Username, a.PasswordHash, boolInt(a.IsActive), a.LastLoginAt, a.CreatedAt, a.UpdatedAt,
			); err != nil {
				return nil, err
			}
			counts.Admins++
		}
	}

	// 5. Optional Mail Logs
	if opts.IncludeMailLogs && b.Includes.MailLogs {
		for _, m := range b.Data.MailLogs {
			cID := m.ClientID
			if mapped, ok := clientIDMap[cID]; ok {
				cID = mapped
			}
			res, err := tx.ExecContext(ctx,
				`INSERT OR IGNORE INTO mail_logs(
					id, client_id, client_name, source, recipient, subject,
					body, status, error_message, duration_ms, client_ip, request_id, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				m.ID, cID, m.ClientName, m.Source, m.Recipient, m.Subject,
				m.Body, m.Status, m.ErrorMessage, m.DurationMs, m.ClientIP, m.RequestID, m.CreatedAt,
			)
			if err != nil {
				return nil, err
			}
			if n, _ := res.RowsAffected(); n > 0 {
				counts.MailLogs++
			}
		}
	}

	// 6. Optional Audit Logs
	if opts.IncludeAuditLogs && b.Includes.AuditLogs {
		for _, l := range b.Data.AuditLogs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO audit_logs(
					actor_type, actor_id, actor_name, action, resource_type, resource_id,
					status, ip_address, user_agent, request_id, detail_json, created_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				l.ActorType, l.ActorID, l.ActorName, l.Action, l.ResourceType, l.ResourceID,
				l.Status, l.IPAddress, l.UserAgent, l.RequestID, l.DetailJSON, l.CreatedAt,
			); err != nil {
				return nil, err
			}
			counts.AuditLogs++
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return counts, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func reencryptCronConfigForExport(c *crypto.Cipher, raw string) (string, error) {
	if raw == "" {
		return "{}", nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return raw, nil
	}
	for _, pair := range [][2]string{
		{"api_key_encrypted", "api_key"},
		{"passphrase_encrypted", "passphrase"},
	} {
		encKey, plainKey := pair[0], pair[1]
		if enc, _ := m[encKey].(string); enc != "" {
			plain, err := c.DecryptString(enc)
			if err != nil {
				return "", err
			}
			m[plainKey] = plain
		}
		delete(m, encKey)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func reencryptCronConfigForImport(c *crypto.Cipher, raw string) (string, error) {
	if raw == "" {
		return "{}", nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return raw, nil
	}
	for _, pair := range [][2]string{
		{"api_key", "api_key_encrypted"},
		{"passphrase", "passphrase_encrypted"},
	} {
		plainKey, encKey := pair[0], pair[1]
		if plain, _ := m[plainKey].(string); plain != "" {
			enc, err := c.EncryptString(plain)
			if err != nil {
				return "", err
			}
			m[encKey] = enc
		}
		delete(m, plainKey)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
