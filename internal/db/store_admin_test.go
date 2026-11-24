package db

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
)

func TestResetAdminOnStartup_EmptyAndExisting(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "db.sqlite"), dir, config.DefaultSystemSettings(3000, false))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// 1. Empty DB creates default "admin" with must_change_password=true
	var out bytes.Buffer
	user, pass, err := store.ResetAdminOnStartup(ctx, &out)
	if err != nil {
		t.Fatalf("ResetAdminOnStartup on empty db: %v", err)
	}
	if user != "admin" || pass == "" {
		t.Fatalf("unexpected user=%q pass=%q", user, pass)
	}
	a, err := store.GetAdminByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("GetAdminByUsername: %v", err)
	}
	if !a.MustChangePassword || !a.IsActive || !crypto.VerifyPassword(a.PasswordHash, pass) {
		t.Fatalf("unexpected admin state after empty reset: %+v", a)
	}

	// 2. If another admin exists and "admin" is disabled, ResetAdminOnStartup still prefers "admin" and reactivates it
	otherHash, _ := crypto.HashPassword("other-pass-123")
	if err := store.CreateAdmin(ctx, &Admin{
		ID:           crypto.NewULID(),
		Username:     "operator",
		PasswordHash: otherHash,
		IsActive:     true,
		CreatedAt:    time.Now().UTC().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateAdminActive(ctx, a.ID, false); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	user2, pass2, err := store.ResetAdminOnStartup(ctx, &out)
	if err != nil {
		t.Fatalf("ResetAdminOnStartup on existing db: %v", err)
	}
	if user2 != "admin" || pass2 == pass {
		t.Fatalf("expected new temp pass for admin, got user=%q pass2=%q", user2, pass2)
	}
	a2, err := store.GetAdminByUsername(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !a2.IsActive || !a2.MustChangePassword || !crypto.VerifyPassword(a2.PasswordHash, pass2) {
		t.Fatalf("expected reactivated admin with temp pass, got %+v", a2)
	}
}

func TestMigrateLegacyAdminsWithoutMustChangeColumn(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rawDB.Exec(`CREATE TABLE admins (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 1,
		last_login_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);`)
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := crypto.HashPassword("legacy-pass-123")
	now := time.Now().UTC()
	_, err = rawDB.Exec(
		`INSERT INTO admins(id, username, password_hash, is_active, created_at, updated_at) VALUES ('adm_1', 'legacy', ?, 1, ?, ?)`,
		hash, now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = rawDB.Close()

	store, err := Open(dbPath, dir, config.DefaultSystemSettings(3000, false))
	if err != nil {
		t.Fatalf("Open legacy db failed: %v", err)
	}
	defer store.Close()

	a, err := store.GetAdminByUsername(context.Background(), "legacy")
	if err != nil {
		t.Fatalf("GetAdminByUsername failed after migration: %v", err)
	}
	if a.MustChangePassword || !a.IsActive {
		t.Fatalf("unexpected legacy admin state: %+v", a)
	}
}
