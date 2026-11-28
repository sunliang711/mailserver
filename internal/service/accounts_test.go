package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
)

func newAccountTest(t *testing.T) (*AccountService, *db.Store) {
	t.Helper()
	dir := t.TempDir()
	store, err := db.Open(filepath.Join(dir, "accounts.db"), dir, config.DefaultSystemSettings(3000, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return NewAccountService(store), store
}

func accountTestActor(t *testing.T, s *AccountService, username, role string) (AccountActor, string) {
	t.Helper()
	hash, err := crypto.HashPassword("password123")
	if err != nil {
		t.Fatal(err)
	}
	admin := &db.Admin{ID: crypto.NewULID(), Username: username, PasswordHash: hash, Role: role, IsActive: true}
	if err := s.store.CreateAdmin(context.Background(), admin); err != nil {
		t.Fatal(err)
	}
	token, _, err := s.CreateSession(context.Background(), admin, "127.0.0.1", "test-browser")
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return AccountActor{Principal: p, IP: "127.0.0.1", UserAgent: "test-browser"}, token
}

func TestAccountSessionPasswordChange(t *testing.T) {
	s, store := newAccountTest(t)
	ctx := context.Background()
	actor, first := accountTestActor(t, s, "admin", db.RoleAdmin)
	second, _, err := s.CreateSession(ctx, actor.Principal.Admin, "192.0.2.2", "second-browser")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("independent logins must issue independent credentials")
	}
	var hash string
	if err := store.DB().QueryRowContext(ctx, `SELECT token_hash FROM admin_sessions WHERE id = ?`, actor.Principal.Session.ID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == first || hash != crypto.HashAPIKey(first) {
		t.Fatal("session credential must only be stored as a hash")
	}
	result, err := s.ChangePassword(ctx, actor, "password123", "new-password123")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{first, second} {
		if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
			t.Fatalf("old credential still accepted: %v", err)
		}
	}
	p, err := s.Authenticate(ctx, result.Token)
	if err != nil || p.Session.ID != actor.Principal.Session.ID || !p.Session.ExpiresAt.Equal(actor.Principal.Session.ExpiresAt) {
		t.Fatalf("current device must keep its session and original expiry: %v", err)
	}
	sessions, err := s.ListSessions(ctx, p)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("expected only the current session, got %d: %v", len(sessions), err)
	}
	if _, err := s.ChangePassword(ctx, AccountActor{Principal: p}, "wrong-password", "another-password"); !errors.Is(err, ErrActorPassword) {
		t.Fatalf("password change must verify the current password: %v", err)
	}
}

func TestAccountPermissionsAndRevocation(t *testing.T) {
	s, _ := newAccountTest(t)
	ctx := context.Background()
	admin, _ := accountTestActor(t, s, "admin", db.RoleAdmin)
	viewer, viewerToken := accountTestActor(t, s, "viewer", db.RoleViewer)
	if _, err := s.CreateAccount(ctx, admin, "wrong-password", "new-viewer", "password123", db.RoleViewer); !errors.Is(err, ErrActorPassword) {
		t.Fatalf("account creation must verify the actor password: %v", err)
	}
	if _, err := s.CreateAccount(ctx, viewer, "password123", "new-viewer", "password123", db.RoleViewer); !errors.Is(err, ErrForbidden) {
		t.Fatalf("viewer created an account: %v", err)
	}
	created, err := s.CreateAccount(ctx, admin, "password123", "new-viewer", "password123", db.RoleViewer)
	if err != nil || created.Role != db.RoleViewer {
		t.Fatalf("create viewer: %v", err)
	}
	if _, err := s.CreateAccount(ctx, admin, "password123", "new-viewer", "password123", db.RoleAdmin); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected duplicate username error: %v", err)
	}
	if err := s.RevokeSession(ctx, viewer, admin.Principal.Session.ID); !errors.Is(err, db.ErrNotFound) {
		t.Fatalf("viewer revoked another account's session: %v", err)
	}
	if err := s.RevokeSession(ctx, viewer, viewer.Principal.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(ctx, viewerToken); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked credential still accepted: %v", err)
	}
	if err := s.RevokeSession(ctx, viewer, admin.Principal.Session.ID); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked actor must not perform further operations: %v", err)
	}
}

func TestAccountChangesInvalidateSessions(t *testing.T) {
	for _, operation := range []string{"disable", "role", "password"} {
		t.Run(operation, func(t *testing.T) {
			s, _ := newAccountTest(t)
			ctx := context.Background()
			actor, _ := accountTestActor(t, s, "actor", db.RoleAdmin)
			target, token := accountTestActor(t, s, "target", db.RoleAdmin)
			role, active, password := db.RoleViewer, false, "reset-password123"
			update := AccountUpdate{}
			switch operation {
			case "disable":
				update.IsActive = &active
			case "role":
				update.Role = &role
			case "password":
				update.Password = &password
			}
			if _, err := s.UpdateAccount(ctx, actor, "password123", target.Principal.Admin.ID, update); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("old session survived %s: %v", operation, err)
			}
			if operation == "disable" {
				active = true
				if _, err := s.UpdateAccount(ctx, actor, "password123", target.Principal.Admin.ID, AccountUpdate{IsActive: &active}); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
					t.Fatal("re-enabling an account must not revive its old session")
				}
			}
		})
	}
}

func TestAccountLastAdminAndAtomicValidation(t *testing.T) {
	s, store := newAccountTest(t)
	ctx := context.Background()
	actor, token := accountTestActor(t, s, "admin", db.RoleAdmin)
	viewer, inactive, invalidPassword := db.RoleViewer, false, "short"
	for _, tc := range []struct {
		name   string
		update AccountUpdate
		want   error
	}{
		{"last admin", AccountUpdate{Role: &viewer}, ErrLastAdmin},
		{"disable self", AccountUpdate{IsActive: &inactive}, ErrDisableSelf},
		{"invalid password", AccountUpdate{Role: &viewer, Password: &invalidPassword}, ErrAccountInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.UpdateAccount(ctx, actor, "password123", actor.Principal.Admin.ID, tc.update); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
	current, err := store.GetAdminByID(ctx, actor.Principal.Admin.ID)
	if err != nil || current.Role != db.RoleAdmin || !current.IsActive {
		t.Fatalf("failed operations mutated the account: %v", err)
	}
	if _, err := s.Authenticate(ctx, token); err != nil {
		t.Fatalf("failed operations revoked the current session: %v", err)
	}
}

func TestAccountConcurrentLastAdminProtection(t *testing.T) {
	s, store := newAccountTest(t)
	ctx := context.Background()
	first, _ := accountTestActor(t, s, "first", db.RoleAdmin)
	second, _ := accountTestActor(t, s, "second", db.RoleAdmin)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, actor := range []AccountActor{first, second} {
		go func(actor AccountActor) {
			<-start
			role := db.RoleViewer
			_, err := s.UpdateAccount(ctx, actor, "password123", actor.Principal.Admin.ID, AccountUpdate{Role: &role})
			results <- err
		}(actor)
	}
	close(start)
	var succeeded, blocked int
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			succeeded++
		} else if errors.Is(err, ErrLastAdmin) {
			blocked++
		} else {
			t.Fatal(err)
		}
	}
	count, err := store.Accounts().CountActiveAdmins(ctx)
	if err != nil || count != 1 || succeeded != 1 || blocked != 1 {
		t.Fatalf("last admin protection failed: count=%d succeeded=%d blocked=%d err=%v", count, succeeded, blocked, err)
	}
}

func TestAccountAuditFailureRollsBackChangeAndRevocation(t *testing.T) {
	s, store := newAccountTest(t)
	ctx := context.Background()
	actor, _ := accountTestActor(t, s, "actor", db.RoleAdmin)
	target, token := accountTestActor(t, s, "target", db.RoleViewer)
	if _, err := store.DB().ExecContext(ctx, `CREATE TRIGGER reject_account_audit BEFORE INSERT ON audit_logs BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	active := false
	if _, err := s.UpdateAccount(ctx, actor, "password123", target.Principal.Admin.ID, AccountUpdate{IsActive: &active}); err == nil {
		t.Fatal("expected audit failure")
	}
	if _, err := s.Authenticate(ctx, token); err != nil {
		t.Fatalf("account update and revocation must roll back when audit fails: %v", err)
	}
}

func TestAccountSessionExpiry(t *testing.T) {
	for _, tc := range []struct {
		name string
		age  time.Duration
	}{
		{"idle deadline", SessionIdle},
		{"absolute deadline", SessionTTL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newAccountTest(t)
			ctx := context.Background()
			now := time.Now().UTC()
			s.now = func() time.Time { return now }
			actor, token := accountTestActor(t, s, "admin", db.RoleAdmin)
			now = now.Add(tc.age)
			if tc.age == SessionTTL {
				// 单独验证绝对过期，不让空闲超时抢先命中。
				sess := actor.Principal.Session
				sess.LastSeenAt = now.Add(-time.Minute)
				if err := s.store.Accounts().UpdateSession(ctx, sess); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.Authenticate(ctx, token); !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("expired session still accepted: %v", err)
			}
			sessions, err := s.ListSessions(ctx, actor.Principal)
			if err != nil || len(sessions) != 0 {
				t.Fatalf("expired sessions must not appear as active: %v", err)
			}
		})
	}
}

func TestAccountPasswordLimits(t *testing.T) {
	for _, tc := range []struct {
		password string
		valid    bool
	}{
		{"short", false}, {strings.Repeat("a", 8), true}, {strings.Repeat("a", 72), true},
		{strings.Repeat("a", 73), false}, {strings.Repeat("密", 25), false},
	} {
		if ValidAccountPassword(tc.password) != tc.valid {
			t.Fatalf("unexpected validation for password length %d", len(tc.password))
		}
	}
}
