package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
)

const (
	// 保留本项目的 24 小时最长会话期限，空闲 60 分钟后要求重新登录。
	SessionTTL  = 24 * time.Hour
	SessionIdle = 60 * time.Minute
	minPassword = 8
	maxPassword = 72 // bcrypt 的密码字节数上限。
)

var (
	ErrUnauthenticated = errors.New("session expired or revoked")
	ErrForbidden       = errors.New("administrator permission required")
	ErrActorPassword   = errors.New("current password is incorrect")
	ErrLastAdmin       = errors.New("at least one active administrator is required")
	ErrDisableSelf     = errors.New("cannot disable the signed-in account")
	ErrAccountInput    = errors.New("invalid account parameters")
	ErrUsernameTaken   = errors.New("username is already in use")
)

type AccountService struct {
	store *db.Store
	now   func() time.Time
}

func NewAccountService(store *db.Store) *AccountService {
	return &AccountService{store: store, now: func() time.Time { return time.Now().UTC() }}
}

type Principal struct {
	Admin   *db.Admin
	Session *db.AdminSession
}

type AccountActor struct {
	Principal *Principal
	IP        string
	UserAgent string
}

func validRole(role string) bool {
	return role == db.RoleAdmin || role == db.RoleViewer
}

func ValidAccountPassword(password string) bool {
	return len(password) >= minPassword && len(password) <= maxPassword
}

func (s *AccountService) validSession(sess *db.AdminSession, admin *db.Admin) bool {
	now := s.now()
	return admin.IsActive && validRole(admin.Role) && sess.AdminID == admin.ID &&
		sess.PasswordVersion == crypto.HashAPIKey(admin.PasswordHash) &&
		now.Before(sess.ExpiresAt) && now.Sub(sess.LastSeenAt) < SessionIdle
}

// CreateSession 复核账号状态后签发随机凭证，数据库只保存凭证摘要。
func (s *AccountService) CreateSession(ctx context.Context, admin *db.Admin, ip, userAgent string) (string, *db.AdminSession, error) {
	token, err := crypto.RandomToken(32)
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	sess := &db.AdminSession{ID: crypto.NewULID(), AdminID: admin.ID, TokenHash: crypto.HashAPIKey(token),
		PasswordVersion: crypto.HashAPIKey(admin.PasswordHash), ClientIP: ip, UserAgent: userAgent,
		CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(SessionTTL)}
	err = s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		current, err := repo.GetAdmin(ctx, admin.ID)
		if err != nil {
			return err
		}
		if !current.IsActive || !validRole(current.Role) || current.PasswordHash != admin.PasswordHash || current.Role != admin.Role {
			return ErrUnauthenticated
		}
		if err := repo.DeleteExpiredSessions(ctx, now, now.Add(-SessionIdle)); err != nil {
			return err
		}
		return repo.CreateSession(ctx, sess)
	})
	return token, sess, err
}

func (s *AccountService) Authenticate(ctx context.Context, token string) (*Principal, error) {
	if token == "" {
		return nil, ErrUnauthenticated
	}
	var p *Principal
	err := s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		sess, err := repo.GetSessionByHash(ctx, crypto.HashAPIKey(token))
		if err != nil {
			return err
		}
		admin, err := repo.GetAdmin(ctx, sess.AdminID)
		if err != nil {
			return err
		}
		if !s.validSession(sess, admin) {
			return ErrUnauthenticated
		}
		if s.now().Sub(sess.LastSeenAt) >= time.Minute {
			sess.LastSeenAt = s.now()
			if err := repo.UpdateSession(ctx, sess); err != nil {
				return err
			}
		}
		p = &Principal{Admin: admin, Session: sess}
		return nil
	})
	if errors.Is(err, db.ErrNotFound) {
		return nil, ErrUnauthenticated
	}
	return p, err
}

// authenticateActor 在事务内重新读取操作者，避免并发停用、降权后仍能执行操作。
func (s *AccountService) authenticateActor(ctx context.Context, repo *db.AccountStore, actor AccountActor) (*db.Admin, error) {
	if actor.Principal == nil {
		return nil, ErrUnauthenticated
	}
	sess, err := repo.GetSession(ctx, actor.Principal.Session.ID)
	if errors.Is(err, db.ErrNotFound) {
		return nil, ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	admin, err := repo.GetAdmin(ctx, actor.Principal.Admin.ID)
	if err != nil {
		return nil, err
	}
	if !s.validSession(sess, admin) || sess.TokenHash != actor.Principal.Session.TokenHash {
		return nil, ErrUnauthenticated
	}
	return admin, nil
}

func (s *AccountService) authorize(ctx context.Context, repo *db.AccountStore, actor AccountActor, password string, requireAdmin bool) (*db.Admin, error) {
	admin, err := s.authenticateActor(ctx, repo, actor)
	if err != nil {
		return nil, err
	}
	if requireAdmin && admin.Role != db.RoleAdmin {
		return nil, ErrForbidden
	}
	if !crypto.VerifyPassword(admin.PasswordHash, password) {
		return nil, ErrActorPassword
	}
	return admin, nil
}

func (s *AccountService) record(ctx context.Context, repo *db.AccountStore, actor AccountActor, action, resource, id string) error {
	return repo.RecordAudit(ctx, &db.AuditLog{
		ActorType: "admin", ActorID: actor.Principal.Admin.ID, ActorName: actor.Principal.Admin.Username,
		Action: action, ResourceType: resource, ResourceID: id, Status: "success",
		IPAddress: actor.IP, UserAgent: actor.UserAgent, DetailJSON: "{}", CreatedAt: s.now(),
	})
}

func (s *AccountService) CreateAccount(ctx context.Context, actor AccountActor, currentPassword, username, password, role string) (*db.Admin, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 64 || !ValidAccountPassword(password) || !validRole(role) {
		return nil, ErrAccountInput
	}
	hash, err := crypto.HashPassword(password)
	if err != nil {
		return nil, err
	}
	admin := &db.Admin{ID: crypto.NewULID(), Username: username, PasswordHash: hash, Role: role, IsActive: true, CreatedAt: s.now(), UpdatedAt: s.now()}
	err = s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		if _, err := s.authorize(ctx, repo, actor, currentPassword, true); err != nil {
			return err
		}
		exists, err := repo.UsernameExists(ctx, username)
		if err != nil {
			return err
		}
		if exists {
			return ErrUsernameTaken
		}
		if err := repo.CreateAdmin(ctx, admin); err != nil {
			return err
		}
		return s.record(ctx, repo, actor, "admin.create", "admin", admin.ID)
	})
	return admin, err
}

type AccountUpdate struct {
	Role     *string
	IsActive *bool
	Password *string
}

type AccountResult struct {
	Admin     *db.Admin
	Token     string
	ExpiresAt time.Time
}

// UpdateAccount 将角色、状态和密码的变更与会话撤销一起提交。
func (s *AccountService) UpdateAccount(ctx context.Context, actor AccountActor, currentPassword, id string, update AccountUpdate) (*AccountResult, error) {
	if update.Role == nil && update.IsActive == nil && update.Password == nil {
		return nil, ErrAccountInput
	}
	if update.Role != nil && !validRole(*update.Role) || update.Password != nil && !ValidAccountPassword(*update.Password) {
		return nil, ErrAccountInput
	}
	var hash string
	var err error
	if update.Password != nil {
		hash, err = crypto.HashPassword(*update.Password)
		if err != nil {
			return nil, err
		}
	}
	result := &AccountResult{}
	err = s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		if _, err := s.authorize(ctx, repo, actor, currentPassword, true); err != nil {
			return err
		}
		target, err := repo.GetAdmin(ctx, id)
		if err != nil {
			return err
		}
		wasActiveAdmin := target.IsActive && target.Role == db.RoleAdmin
		roleChanged, disabled := false, false
		if update.Role != nil {
			roleChanged = target.Role != *update.Role
			target.Role = *update.Role
		}
		if update.IsActive != nil {
			if !*update.IsActive && id == actor.Principal.Admin.ID {
				return ErrDisableSelf
			}
			disabled = target.IsActive && !*update.IsActive
			target.IsActive = *update.IsActive
		}
		if wasActiveAdmin && (!target.IsActive || target.Role != db.RoleAdmin) {
			count, err := repo.CountActiveAdmins(ctx)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdmin
			}
		}
		if hash != "" {
			target.PasswordHash = hash
			target.MustChangePassword = false
		}
		target.UpdatedAt = s.now()
		if err := repo.UpdateAdmin(ctx, target); err != nil {
			return err
		}
		if roleChanged || disabled || hash != "" {
			keep := ""
			if hash != "" && id == actor.Principal.Admin.ID && !roleChanged && target.IsActive {
				keep = actor.Principal.Session.ID
				if err := s.rotateSession(ctx, repo, actor, target, result); err != nil {
					return err
				}
			}
			if err := repo.DeleteAdminSessions(ctx, id, keep); err != nil {
				return err
			}
		}
		result.Admin = target
		return s.record(ctx, repo, actor, "admin.update", "admin", id)
	})
	return result, err
}

func (s *AccountService) rotateSession(ctx context.Context, repo *db.AccountStore, actor AccountActor, admin *db.Admin, result *AccountResult) error {
	sess, err := repo.GetSession(ctx, actor.Principal.Session.ID)
	if err != nil {
		return err
	}
	token, err := crypto.RandomToken(32)
	if err != nil {
		return err
	}
	sess.TokenHash = crypto.HashAPIKey(token)
	sess.PasswordVersion = crypto.HashAPIKey(admin.PasswordHash)
	sess.LastSeenAt = s.now()
	if err := repo.UpdateSession(ctx, sess); err != nil {
		return err
	}
	result.Token, result.ExpiresAt = token, sess.ExpiresAt
	return nil
}

func (s *AccountService) ChangePassword(ctx context.Context, actor AccountActor, current, next string) (*AccountResult, error) {
	if !ValidAccountPassword(next) {
		return nil, ErrAccountInput
	}
	hash, err := crypto.HashPassword(next)
	if err != nil {
		return nil, err
	}
	result := &AccountResult{}
	err = s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		// 保留启动救援流程：临时密码登录后允许直接设置新密码。
		password := current
		if actor.Principal.Admin.MustChangePassword && current == "" {
			admin, err := repo.GetAdmin(ctx, actor.Principal.Admin.ID)
			if err != nil {
				return err
			}
			sess, err := repo.GetSession(ctx, actor.Principal.Session.ID)
			if err != nil {
				return err
			}
			if !admin.MustChangePassword || !s.validSession(sess, admin) || sess.TokenHash != actor.Principal.Session.TokenHash {
				return ErrUnauthenticated
			}
		} else if _, err := s.authorize(ctx, repo, actor, password, false); err != nil {
			return err
		}
		admin, err := repo.GetAdmin(ctx, actor.Principal.Admin.ID)
		if err != nil {
			return err
		}
		admin.PasswordHash, admin.MustChangePassword, admin.UpdatedAt = hash, false, s.now()
		if err := repo.UpdateAdmin(ctx, admin); err != nil {
			return err
		}
		if err := s.rotateSession(ctx, repo, actor, admin, result); err != nil {
			return err
		}
		if err := repo.DeleteAdminSessions(ctx, admin.ID, actor.Principal.Session.ID); err != nil {
			return err
		}
		result.Admin = admin
		return s.record(ctx, repo, actor, "admin.change_password", "admin", admin.ID)
	})
	return result, err
}

func (s *AccountService) ListSessions(ctx context.Context, p *Principal) ([]db.AdminSession, error) {
	sessions, err := s.store.Accounts().ListSessions(ctx, p.Admin.ID)
	if err != nil {
		return nil, err
	}
	out := make([]db.AdminSession, 0, len(sessions))
	for _, sess := range sessions {
		if s.validSession(&sess, p.Admin) {
			out = append(out, sess)
		}
	}
	return out, nil
}

func (s *AccountService) RevokeSession(ctx context.Context, actor AccountActor, id string) error {
	return s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		admin, err := s.authenticateActor(ctx, repo, actor)
		if err != nil {
			return err
		}
		sess, err := repo.GetSession(ctx, id)
		if err != nil {
			return err
		}
		if sess.AdminID != admin.ID && admin.Role != db.RoleAdmin {
			return db.ErrNotFound
		}
		if err := repo.DeleteSession(ctx, id); err != nil {
			return err
		}
		return s.record(ctx, repo, actor, "auth.session_revoke", "session", id)
	})
}

func (s *AccountService) Logout(ctx context.Context, token, ip, userAgent string) error {
	p, err := s.Authenticate(ctx, token)
	if errors.Is(err, ErrUnauthenticated) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.store.WithAccountsTx(ctx, func(repo *db.AccountStore) error {
		if err := repo.DeleteSession(ctx, p.Session.ID); err != nil {
			return err
		}
		return s.record(ctx, repo, AccountActor{Principal: p, IP: ip, UserAgent: userAgent}, "auth.logout", "session", p.Session.ID)
	})
}
