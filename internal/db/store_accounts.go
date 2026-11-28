package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type AdminSession struct {
	ID              string
	AdminID         string
	TokenHash       string
	PasswordVersion string
	ClientIP        string
	UserAgent       string
	CreatedAt       time.Time
	LastSeenAt      time.Time
	ExpiresAt       time.Time
}

type accountQuerier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// AccountStore 使账号、会话与审计操作共用同一事务。
type AccountStore struct {
	q accountQuerier
}

func (s *Store) Accounts() *AccountStore {
	return &AccountStore{q: s.db}
}

func (s *Store) WithAccountsTx(ctx context.Context, fn func(*AccountStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(&AccountStore{q: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *AccountStore) GetAdmin(ctx context.Context, id string) (*Admin, error) {
	return scanAdmin(s.q.QueryRowContext(ctx, `SELECT id, username, password_hash, is_active, must_change_password, role, last_login_at, created_at, updated_at FROM admins WHERE id = ?`, id))
}

func (s *AccountStore) CountActiveAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins WHERE is_active = 1 AND role = 'admin'`).Scan(&count)
	return count, err
}

func (s *AccountStore) UsernameExists(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := s.q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM admins WHERE username = ?)`, username).Scan(&exists)
	return exists, err
}

func (s *AccountStore) CreateAdmin(ctx context.Context, a *Admin) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO admins(id, username, password_hash, role, is_active, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Username, a.PasswordHash, a.Role, boolToInt(a.IsActive), a.CreatedAt, a.UpdatedAt)
	return err
}

func (s *AccountStore) UpdateAdmin(ctx context.Context, a *Admin) error {
	_, err := s.q.ExecContext(ctx, `UPDATE admins SET password_hash = ?, role = ?, is_active = ?, must_change_password = ?, updated_at = ? WHERE id = ?`,
		a.PasswordHash, a.Role, boolToInt(a.IsActive), boolToInt(a.MustChangePassword), a.UpdatedAt, a.ID)
	return err
}

const sessionColumns = `id, admin_id, token_hash, password_version, client_ip, user_agent, created_at, last_seen_at, expires_at`

func scanAdminSession(row rowScanner) (*AdminSession, error) {
	var sess AdminSession
	err := row.Scan(&sess.ID, &sess.AdminID, &sess.TokenHash, &sess.PasswordVersion, &sess.ClientIP, &sess.UserAgent, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *AccountStore) GetSession(ctx context.Context, id string) (*AdminSession, error) {
	return scanAdminSession(s.q.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM admin_sessions WHERE id = ?`, id))
}

func (s *AccountStore) GetSessionByHash(ctx context.Context, hash string) (*AdminSession, error) {
	return scanAdminSession(s.q.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM admin_sessions WHERE token_hash = ?`, hash))
}

func (s *AccountStore) CreateSession(ctx context.Context, sess *AdminSession) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO admin_sessions(`+sessionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.AdminID, sess.TokenHash, sess.PasswordVersion, sess.ClientIP, sess.UserAgent, sess.CreatedAt, sess.LastSeenAt, sess.ExpiresAt)
	return err
}

func (s *AccountStore) UpdateSession(ctx context.Context, sess *AdminSession) error {
	_, err := s.q.ExecContext(ctx, `UPDATE admin_sessions SET token_hash = ?, password_version = ?, last_seen_at = ? WHERE id = ?`,
		sess.TokenHash, sess.PasswordVersion, sess.LastSeenAt, sess.ID)
	return err
}

func (s *AccountStore) DeleteSession(ctx context.Context, id string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}

func (s *AccountStore) DeleteAdminSessions(ctx context.Context, adminID, keepID string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM admin_sessions WHERE admin_id = ? AND id != ?`, adminID, keepID)
	return err
}

func (s *AccountStore) DeleteExpiredSessions(ctx context.Context, now, idleBefore time.Time) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ? OR last_seen_at <= ?`, now, idleBefore)
	return err
}

func (s *AccountStore) ListSessions(ctx context.Context, adminID string) ([]AdminSession, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+sessionColumns+` FROM admin_sessions WHERE admin_id = ? ORDER BY last_seen_at DESC, id DESC`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminSession, 0)
	for rows.Next() {
		sess, err := scanAdminSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sess)
	}
	return out, rows.Err()
}

func (s *AccountStore) RecordAudit(ctx context.Context, a *AuditLog) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO audit_logs(actor_type, actor_id, actor_name, action, resource_type, resource_id, status, ip_address, user_agent, detail_json, created_at) VALUES ('admin', ?, ?, ?, ?, ?, 'success', ?, ?, ?, ?)`,
		a.ActorID, a.ActorName, a.Action, a.ResourceType, a.ResourceID, a.IPAddress, a.UserAgent, a.DetailJSON, a.CreatedAt)
	return err
}
