package migration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
)

func TestAccountRolesMigrationAndLegacyBundles(t *testing.T) {
	ctx := context.Background()
	newInstance := func() (*db.Store, *Service) {
		t.Helper()
		dir := t.TempDir()
		store, err := db.Open(filepath.Join(dir, "migration.db"), dir, config.DefaultSystemSettings(3000, false))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		key, err := config.EnsureMasterKey(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		cipher, err := crypto.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		return store, NewService(store, cipher)
	}
	source, svc := newInstance()
	for _, role := range []string{db.RoleAdmin, db.RoleViewer} {
		if err := source.CreateAdmin(ctx, &db.Admin{ID: crypto.NewULID(), Username: role, Role: role, PasswordHash: "test-hash", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	blob, err := svc.Export(ctx, ExportOptions{IncludeAdmins: true})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeBundle(blob, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{true: "legacy", false: "current"}[legacy], func(t *testing.T) {
			target, targetSvc := newInstance()
			bundle := *decoded
			bundle.Data.Admins = append([]AdminRecord(nil), decoded.Data.Admins...)
			if legacy {
				for i := range bundle.Data.Admins {
					bundle.Data.Admins[i].Role = ""
				}
			}
			data, err := json.Marshal(bundle.Data)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			bundle.Checksum = hex.EncodeToString(sum[:])
			raw, err := json.Marshal(bundle)
			if err != nil {
				t.Fatal(err)
			}
			encrypted, err := crypto.EncryptWithPassphrase(raw, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := targetSvc.Import(ctx, encrypted, ImportOptions{IncludeAdmins: true}); err != nil {
				t.Fatal(err)
			}
			viewer, err := target.GetAdminByUsername(ctx, db.RoleViewer)
			if err != nil {
				t.Fatal(err)
			}
			expected := db.RoleViewer
			if legacy {
				expected = db.RoleAdmin
			}
			if viewer.Role != expected {
				t.Fatalf("imported role = %s, want %s", viewer.Role, expected)
			}
		})
	}

	t.Run("overwrite revokes sessions and preserves last administrator", func(t *testing.T) {
		target, targetSvc := newInstance()
		admin := &db.Admin{ID: crypto.NewULID(), Username: db.RoleAdmin, Role: db.RoleAdmin, PasswordHash: "target-hash", IsActive: true}
		if err := target.CreateAdmin(ctx, admin); err != nil {
			t.Fatal(err)
		}
		sess := &db.AdminSession{ID: "existing-session", AdminID: admin.ID, TokenHash: "test-token-hash", PasswordVersion: crypto.HashAPIKey(admin.PasswordHash), CreatedAt: time.Now().UTC(), LastSeenAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Hour)}
		if err := target.Accounts().CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
		if _, err := targetSvc.Import(ctx, blob, ImportOptions{IncludeAdmins: true, ConflictStrategy: "overwrite"}); err != nil {
			t.Fatal(err)
		}
		if _, err := target.Accounts().GetSession(ctx, sess.ID); err != db.ErrNotFound {
			t.Fatalf("overwriting an account must revoke its existing session: %v", err)
		}
		var count int
		if err := target.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_sessions`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("migration must not copy source sessions: %v", err)
		}
	})
}

func TestAccountOverwriteRevokesSessionsAndProtectsLastAdmin(t *testing.T) {
	for _, role := range []string{db.RoleAdmin, db.RoleViewer} {
		t.Run(role, func(t *testing.T) {
			ctx := context.Background()
			newInstance := func() (*db.Store, *Service) {
				dir := t.TempDir()
				store, err := db.Open(filepath.Join(dir, "accounts.db"), dir, config.DefaultSystemSettings(3000, false))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { store.Close() })
				key, err := config.EnsureMasterKey(dir, "")
				if err != nil {
					t.Fatal(err)
				}
				cipher, err := crypto.NewCipher(key)
				if err != nil {
					t.Fatal(err)
				}
				return store, NewService(store, cipher)
			}
			source, sourceSvc := newInstance()
			if err := source.CreateAdmin(ctx, &db.Admin{ID: "source-admin", Username: "admin", PasswordHash: "source-hash", Role: role, IsActive: true}); err != nil {
				t.Fatal(err)
			}
			target, targetSvc := newInstance()
			if err := target.CreateAdmin(ctx, &db.Admin{ID: "target-admin", Username: "admin", PasswordHash: "target-hash", Role: db.RoleAdmin, IsActive: true}); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if err := target.Accounts().CreateSession(ctx, &db.AdminSession{ID: "target-session", AdminID: "target-admin", TokenHash: "target-token-hash", PasswordVersion: crypto.HashAPIKey("target-hash"), CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			bundle, err := sourceSvc.Export(ctx, ExportOptions{IncludeAdmins: true})
			if err != nil {
				t.Fatal(err)
			}
			_, err = targetSvc.Import(ctx, bundle, ImportOptions{IncludeAdmins: true, Mode: "merge", ConflictStrategy: "overwrite"})
			if role == db.RoleViewer && err == nil {
				t.Fatal("import must not demote the last administrator")
			}
			if role == db.RoleAdmin && err != nil {
				t.Fatal(err)
			}
			account, err := target.GetAdminByID(ctx, "target-admin")
			if err != nil {
				t.Fatal(err)
			}
			sessions, err := target.Accounts().ListSessions(ctx, "target-admin")
			if err != nil {
				t.Fatal(err)
			}
			if role == db.RoleAdmin && (len(sessions) != 0 || account.PasswordHash != "source-hash") {
				t.Fatal("overwriting an account must revoke its sessions")
			}
			if role == db.RoleViewer && (len(sessions) != 1 || account.Role != db.RoleAdmin || account.PasswordHash != "target-hash") {
				t.Fatal("failed import must preserve credentials, role, and session")
			}
		})
	}
}
