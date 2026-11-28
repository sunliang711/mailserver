package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
)

func TestLegacyAdminSchemaGetsAdministratorRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE admins (id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, is_active INTEGER NOT NULL DEFAULT 1, last_login_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO admins(id, username, password_hash, created_at, updated_at) VALUES ('existing', 'legacy-admin', 'existing-hash', ?, ?)`, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, dir, config.DefaultSystemSettings(3000, false))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	admin, err := store.GetAdminByID(context.Background(), "existing")
	if err != nil || admin.Role != RoleAdmin || admin.PasswordHash != "existing-hash" || !admin.IsActive {
		t.Fatalf("legacy administrator was not preserved: %v", err)
	}
	if err := store.migrate(context.Background()); err != nil {
		t.Fatalf("schema migration must be repeatable: %v", err)
	}
}
