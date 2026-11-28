package server

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/service"
)

type accountDTO struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Role               string     `json:"role"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func toAccountDTO(a *db.Admin) accountDTO {
	return accountDTO{ID: a.ID, Username: a.Username, Role: a.Role, IsActive: a.IsActive,
		MustChangePassword: a.MustChangePassword, LastLoginAt: a.LastLoginAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt}
}

func (s *AdminServer) writeAccountError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUnauthenticated):
		writeError(c, http.StatusUnauthorized, "session_revoked", "session expired or revoked")
	case errors.Is(err, service.ErrForbidden):
		writeError(c, http.StatusForbidden, "forbidden", "administrator permission required")
	case errors.Is(err, service.ErrActorPassword):
		writeError(c, http.StatusBadRequest, "invalid_password", "current password is incorrect")
	case errors.Is(err, service.ErrLastAdmin):
		writeError(c, http.StatusConflict, "last_admin", "at least one active administrator is required")
	case errors.Is(err, service.ErrDisableSelf):
		writeError(c, http.StatusBadRequest, "cannot_disable_self", "cannot disable the signed-in account")
	case errors.Is(err, service.ErrAccountInput):
		writeError(c, http.StatusBadRequest, "validation_error", "username must be 3-64 bytes, password 8-72 bytes, and role admin or viewer")
	case errors.Is(err, service.ErrUsernameTaken):
		writeError(c, http.StatusConflict, "username_taken", "username is already in use")
	case errors.Is(err, db.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "account or session not found")
	default:
		writeError(c, http.StatusInternalServerError, "account_error", "unable to complete account operation")
	}
}

func (s *AdminServer) handleListAdmins(c *gin.Context) {
	admins, err := s.store.ListAdmins(c.Request.Context())
	if err != nil {
		s.writeAccountError(c, err)
		return
	}
	items := make([]accountDTO, 0, len(admins))
	for i := range admins {
		items = append(items, toAccountDTO(&admins[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *AdminServer) handleCreateAdmin(c *gin.Context) {
	var req struct {
		Username        string `json:"username" binding:"required"`
		Password        string `json:"password" binding:"required"`
		Role            string `json:"role" binding:"required"`
		CurrentPassword string `json:"current_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "username, password, role and current_password are required")
		return
	}
	a, err := s.accounts.CreateAccount(c.Request.Context(), accountActor(c), req.CurrentPassword, req.Username, req.Password, req.Role)
	if err != nil {
		s.writeAccountError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toAccountDTO(a))
}

func (s *AdminServer) handleUpdateAdmin(c *gin.Context) {
	var req struct {
		IsActive        *bool   `json:"is_active"`
		Password        *string `json:"password"`
		Role            *string `json:"role"`
		CurrentPassword string  `json:"current_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "current_password and account changes are required")
		return
	}
	result, err := s.accounts.UpdateAccount(c.Request.Context(), accountActor(c), req.CurrentPassword, c.Param("id"),
		service.AccountUpdate{Role: req.Role, IsActive: req.IsActive, Password: req.Password})
	if err != nil {
		s.writeAccountError(c, err)
		return
	}
	if result.Token != "" {
		s.setAdminCookie(c, result.Token, result.ExpiresAt)
	}
	c.JSON(http.StatusOK, toAccountDTO(result.Admin))
}

type sessionDTO struct {
	ID         string    `json:"id"`
	ClientIP   string    `json:"client_ip"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

func (s *AdminServer) handleListSessions(c *gin.Context) {
	p := currentPrincipal(c)
	sessions, err := s.accounts.ListSessions(c.Request.Context(), p)
	if err != nil {
		s.writeAccountError(c, err)
		return
	}
	items := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, sessionDTO{ID: sess.ID, ClientIP: sess.ClientIP, UserAgent: sess.UserAgent,
			CreatedAt: sess.CreatedAt, LastSeenAt: sess.LastSeenAt, ExpiresAt: sess.ExpiresAt, Current: sess.ID == p.Session.ID})
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *AdminServer) handleRevokeSession(c *gin.Context) {
	id := c.Param("id")
	if err := s.accounts.RevokeSession(c.Request.Context(), accountActor(c), id); err != nil {
		s.writeAccountError(c, err)
		return
	}
	if id == currentPrincipal(c).Session.ID {
		s.clearAdminCookie(c)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
