package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/migration"
	"github.com/sunliang711/mailserver/internal/scheduler"
	"github.com/sunliang711/mailserver/internal/service"
)

// --- Setup & Auth ---

func (s *AdminServer) handleSetupStatus(c *gin.Context) {
	n, err := s.store.CountAdmins(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"initialized": n > 0, "version": s.version})
}

func (s *AdminServer) handleSetupInit(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Password) < 8 {
		writeError(c, http.StatusBadRequest, "validation_error", "username >= 3 chars and password >= 8 chars required")
		return
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}
	admin := &db.Admin{
		ID:           crypto.NewULID(),
		Username:     req.Username,
		PasswordHash: hash,
		IsActive:     true,
	}
	if err := s.store.CreateInitialAdminIfEmpty(c.Request.Context(), admin); err != nil {
		if errors.Is(err, db.ErrAlreadyInitialized) {
			writeError(c, http.StatusConflict, "already_initialized", "system is already initialized")
			return
		}
		writeError(c, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}

	tok, err := s.signAdminToken(admin)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	s.setAdminCookie(c, tok)
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "system.setup_init",
		ResourceType: "admin",
		ResourceID:   admin.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})
	c.JSON(http.StatusCreated, gin.H{"admin": admin, "token": tok})
}

func (s *AdminServer) handleAdminLogin(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	admin, err := s.store.GetAdminByUsername(c.Request.Context(), strings.TrimSpace(req.Username))
	if err != nil || !admin.IsActive || !crypto.VerifyPassword(admin.PasswordHash, req.Password) {
		s.auditLog.Record(c.Request.Context(), audit.Event{
			ActorType:    "admin",
			ActorName:    strings.TrimSpace(req.Username),
			Action:       "admin.login",
			ResourceType: "admin",
			Status:       "failure",
			IPAddress:    c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
		})
		writeError(c, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}

	_ = s.store.UpdateAdminLogin(c.Request.Context(), admin.ID)
	tok, err := s.signAdminToken(admin)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	s.setAdminCookie(c, tok)
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "admin.login",
		ResourceType: "admin",
		ResourceID:   admin.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"admin": admin, "token": tok})
}

func (s *AdminServer) handleAdminLogout(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(c.Request),
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *AdminServer) handleAdminMe(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"admin": currentAdmin(c), "version": s.version})
}

func (s *AdminServer) handleAdminChangePassword(c *gin.Context) {
	admin := currentAdmin(c)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if !crypto.VerifyPassword(admin.PasswordHash, req.OldPassword) {
		writeError(c, http.StatusBadRequest, "invalid_password", "current password is incorrect")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(c, http.StatusBadRequest, "weak_password", "new password must be at least 8 characters")
		return
	}
	hash, err := crypto.HashPassword(req.NewPassword)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}
	if err := s.store.UpdateAdminPassword(c.Request.Context(), admin.ID, hash); err != nil {
		writeError(c, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}
	admin.PasswordHash = hash
	if tok, err := s.signAdminToken(admin); err == nil {
		s.setAdminCookie(c, tok)
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "admin.change_password",
		ResourceType: "admin",
		ResourceID:   admin.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *AdminServer) setAdminCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(c.Request),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

// --- Dashboard ---

func (s *AdminServer) handleDashboardSummary(c *gin.Context) {
	sum, err := s.store.GetDashboardSummary(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "summary_error", err.Error())
		return
	}
	for i := range sum.CronJobs {
		sum.CronJobs[i] = sanitizeCronJobDTO(sum.CronJobs[i])
	}
	c.JSON(http.StatusOK, sum)
}

// --- Mail Logs & Web Test Send ---

func parseMailLogFilter(c *gin.Context) db.MailLogFilter {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	f := db.MailLogFilter{
		ClientID: c.Query("client_id"),
		Status:   c.Query("status"),
		Source:   c.Query("source"),
		Query:    c.Query("q"),
		Limit:    limit,
		Offset:   offset,
	}
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			f.From = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			f.To = &t
		}
	}
	return f
}

func (s *AdminServer) handleListMailLogs(c *gin.Context) {
	f := parseMailLogFilter(c)
	items, total, err := s.store.ListMailLogs(c.Request.Context(), f)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func (s *AdminServer) handleGetMailLog(c *gin.Context) {
	m, err := s.store.GetMailLog(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "mail log not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, m)
}

func (s *AdminServer) handleExportMailLogs(c *gin.Context) {
	f := parseMailLogFilter(c)
	logs, err := s.store.ExportAllMailLogs(c.Request.Context(), f)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "export_failed", err.Error())
		return
	}
	format := strings.ToLower(c.Query("format"))
	if format == "json" {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="mail-logs.json"`)
		_ = json.NewEncoder(c.Writer).Encode(logs)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="mail-logs.csv"`)
	cw := csv.NewWriter(c.Writer)
	_ = cw.Write([]string{"id", "created_at", "source", "client_name", "recipient", "subject", "status", "duration_ms", "client_ip", "error_message"})
	for _, l := range logs {
		_ = cw.Write([]string{
			l.ID,
			l.CreatedAt.Format(time.RFC3339),
			l.Source,
			l.ClientName,
			l.Recipient,
			l.Subject,
			l.Status,
			strconv.FormatInt(l.DurationMs, 10),
			l.ClientIP,
			l.ErrorMessage,
		})
	}
	cw.Flush()
}

func (s *AdminServer) handleWebTestSend(c *gin.Context) {
	admin := currentAdmin(c)
	var req struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	req.To = strings.TrimSpace(req.To)
	req.Subject = strings.TrimSpace(req.Subject)
	if req.To == "" || req.Subject == "" || req.Body == "" {
		writeError(c, http.StatusBadRequest, "validation_error", "to, subject and body are required")
		return
	}

	sendCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	err := s.mailService.SendEmail(sendCtx, service.SendEmailInput{
		To:         req.To,
		Subject:    req.Subject,
		Body:       req.Body,
		Source:     "web_test",
		ClientID:   admin.ID,
		ClientName: admin.Username,
		ClientIP:   c.ClientIP(),
	})
	if err != nil {
		_ = s.store.UpdateSMTPStatus(c.Request.Context(), "failed", err.Error())
		writeError(c, http.StatusBadGateway, "send_failed", err.Error())
		return
	}
	_ = s.store.UpdateSMTPStatus(c.Request.Context(), "ok", "")
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// --- Clients (Multi-Client auth_key) ---

type clientDTO struct {
	db.Client
	HasAPIKey bool `json:"has_api_key"`
}

func toClientDTO(c db.Client) clientDTO {
	return clientDTO{
		Client:    c,
		HasAPIKey: c.KeyEncrypted != "",
	}
}

func (s *AdminServer) handleListClients(c *gin.Context) {
	list, err := s.store.ListClients(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	dtos := make([]clientDTO, 0, len(list))
	for _, cl := range list {
		dtos = append(dtos, toClientDTO(cl))
	}
	c.JSON(http.StatusOK, gin.H{"items": dtos})
}

func (s *AdminServer) handleCreateClient(c *gin.Context) {
	admin := currentAdmin(c)
	var req struct {
		Name         string     `json:"name"`
		Description  string     `json:"description"`
		Enabled      bool       `json:"enabled"`
		RateLimitRPM int        `json:"rate_limit_rpm"`
		ExpiresAt    *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(c, http.StatusBadRequest, "validation_error", "client name is required")
		return
	}
	if req.RateLimitRPM < 0 {
		writeError(c, http.StatusBadRequest, "validation_error", "rate_limit_rpm must be >= 0")
		return
	}

	plainKey, prefix, hash, err := crypto.GenerateAPIKey()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "keygen_error", err.Error())
		return
	}
	keyEnc, err := s.cipher.EncryptString(plainKey)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "encrypt_error", err.Error())
		return
	}

	cl := &db.Client{
		ID:           crypto.NewULID(),
		Name:         req.Name,
		Description:  strings.TrimSpace(req.Description),
		KeyPrefix:    prefix,
		KeyHash:      hash,
		KeyEncrypted: keyEnc,
		Enabled:      req.Enabled,
		RateLimitRPM: req.RateLimitRPM,
		ExpiresAt:    req.ExpiresAt,
	}
	if err := s.store.CreateClient(c.Request.Context(), cl); err != nil {
		writeError(c, http.StatusBadRequest, "create_failed", err.Error())
		return
	}

	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "client.create",
		ResourceType: "client",
		ResourceID:   cl.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"name": cl.Name, "key_prefix": prefix},
	})
	c.JSON(http.StatusCreated, gin.H{
		"client":   toClientDTO(*cl),
		"auth_key": plainKey,
	})
}

func (s *AdminServer) handleRevealClientSecrets(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	cl, err := s.store.GetClientByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "client not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	authKey := ""
	if cl.KeyEncrypted != "" {
		authKey, err = s.cipher.DecryptString(cl.KeyEncrypted)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "decrypt_failed", "failed to decrypt auth_key")
			return
		}
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "client.reveal_secrets",
		ResourceType: "client",
		ResourceID:   cl.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"name": cl.Name},
	})
	c.JSON(http.StatusOK, gin.H{"auth_key": authKey})
}

func (s *AdminServer) handleUpdateClient(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	cl, err := s.store.GetClientByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "client not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	var req struct {
		Name         string     `json:"name"`
		Description  string     `json:"description"`
		Enabled      bool       `json:"enabled"`
		RateLimitRPM int        `json:"rate_limit_rpm"`
		ExpiresAt    *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		cl.Name = strings.TrimSpace(req.Name)
	}
	if req.RateLimitRPM < 0 {
		writeError(c, http.StatusBadRequest, "validation_error", "rate_limit_rpm must be >= 0")
		return
	}
	cl.Description = strings.TrimSpace(req.Description)
	cl.Enabled = req.Enabled
	cl.RateLimitRPM = req.RateLimitRPM
	cl.ExpiresAt = req.ExpiresAt

	if err := s.store.UpdateClient(c.Request.Context(), cl); err != nil {
		writeError(c, http.StatusBadRequest, "update_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "client.update",
		ResourceType: "client",
		ResourceID:   cl.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"name": cl.Name, "enabled": cl.Enabled, "rate_limit_rpm": cl.RateLimitRPM},
	})
	c.JSON(http.StatusOK, toClientDTO(*cl))
}

func (s *AdminServer) handleDeleteClient(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	if err := s.store.DeleteClient(c.Request.Context(), id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "client not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "client.delete",
		ResourceType: "client",
		ResourceID:   id,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (s *AdminServer) handleRotateClientKey(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	plainKey, prefix, hash, err := crypto.GenerateAPIKey()
	if err != nil {
		writeError(c, http.StatusInternalServerError, "keygen_error", err.Error())
		return
	}
	keyEnc, err := s.cipher.EncryptString(plainKey)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "encrypt_error", err.Error())
		return
	}
	if err := s.store.RotateClientKey(c.Request.Context(), id, prefix, hash, keyEnc); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "client not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "rotate_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "client.rotate_key",
		ResourceType: "client",
		ResourceID:   id,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"new_prefix": prefix},
	})
	c.JSON(http.StatusOK, gin.H{
		"auth_key":   plainKey,
		"key_prefix": prefix,
	})
}

// --- Cron Jobs & SafeKeep API Test ---

func sanitizeCronJobDTO(j db.CronJob) db.CronJob {
	j.DefaultSchedule = scheduler.DefaultScheduleFor(j.JobKey)
	if j.TimeoutSec <= 0 {
		j.TimeoutSec = scheduler.DefaultTimeoutFor(j.JobKey)
	}
	if next, err := scheduler.PreviewCron(j.CronExpr, 5); err == nil {
		j.NextRuns = next
	} else {
		j.NextRuns = []time.Time{}
	}
	if j.JobKey == "migration_backup" {
		j.ConfigJSON = scheduler.SanitizeMigrationBackupConfigJSON(j.ConfigJSON)
	}
	return j
}

func (s *AdminServer) handlePreviewCronSchedule(c *gin.Context) {
	var req struct {
		Schedule string `json:"schedule"`
		CronExpr string `json:"cron_expr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	expr := strings.TrimSpace(req.Schedule)
	if expr == "" {
		expr = strings.TrimSpace(req.CronExpr)
	}
	times, err := scheduler.PreviewCron(expr, 5)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_cron", fmt.Sprintf("cron 表达式无效：%v", err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"next_runs": times})
}

func (s *AdminServer) handleTestSafeKeep(c *gin.Context) {
	var req struct {
		JobID      string `json:"job_id"`
		APIBaseURL string `json:"api_base_url"`
		APIKey     string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	apiKey := strings.TrimSpace(req.APIKey)
	if (apiKey == "" || apiKey == "*****" || apiKey == "••••••") && req.JobID != "" {
		if job, err := s.store.GetCronJobByID(c.Request.Context(), req.JobID); err == nil {
			revealed, _, decErr := scheduler.RevealMigrationBackupSecrets(s.cipher, job.ConfigJSON)
			if decErr == nil && revealed != "" {
				apiKey = revealed
			}
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	upstreams, err := scheduler.TestSafeKeepConnection(ctx, nil, req.APIBaseURL, apiKey)
	if err != nil {
		writeError(c, http.StatusBadGateway, "safekeep_test_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "upstreams": upstreams})
}

func (s *AdminServer) handleListCronJobs(c *gin.Context) {
	jobs, err := s.store.ListCronJobs(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	for i := range jobs {
		jobs[i] = sanitizeCronJobDTO(jobs[i])
	}
	c.JSON(http.StatusOK, gin.H{"items": jobs})
}

func (s *AdminServer) handleRevealCronJobSecrets(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	job, err := s.store.GetCronJobByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "cron job not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	var apiKey, passphrase string
	if job.JobKey == "migration_backup" {
		apiKey, passphrase, err = scheduler.RevealMigrationBackupSecrets(s.cipher, job.ConfigJSON)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "decrypt_failed", err.Error())
			return
		}
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "cron.reveal_secrets",
		ResourceType: "cron_job",
		ResourceID:   job.ID,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"job_key": job.JobKey},
	})
	c.JSON(http.StatusOK, gin.H{
		"api_key":    apiKey,
		"passphrase": passphrase,
	})
}

func (s *AdminServer) handleUpdateCronJob(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	existing, err := s.store.GetCronJobByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "cron job not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	var req struct {
		CronExpr   string          `json:"cron_expr"`
		Enabled    bool            `json:"enabled"`
		TimeoutSec *int            `json:"timeout_sec"`
		Config     json.RawMessage `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := scheduler.ValidateCron(req.CronExpr); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_cron", fmt.Sprintf("invalid cron expression: %v", err))
		return
	}
	timeoutSec := existing.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = scheduler.DefaultTimeoutFor(existing.JobKey)
	}
	if req.TimeoutSec != nil {
		if *req.TimeoutSec < 1 || *req.TimeoutSec > 86400 {
			writeError(c, http.StatusBadRequest, "invalid_timeout", "timeout_sec must be between 1 and 86400 seconds")
			return
		}
		timeoutSec = *req.TimeoutSec
	}

	rawCfg := strings.TrimSpace(string(req.Config))
	cfgStr := existing.ConfigJSON
	if rawCfg != "" && rawCfg != "null" {
		if existing.JobKey == "migration_backup" {
			cfgStr, err = scheduler.MergeMigrationBackupConfig(s.cipher, existing.ConfigJSON, req.Config)
			if err != nil {
				writeError(c, http.StatusBadRequest, "invalid_config", err.Error())
				return
			}
		} else {
			cfgStr, err = scheduler.NormalizeJobConfig(existing.JobKey, req.Config)
			if err != nil {
				writeError(c, http.StatusBadRequest, "invalid_config", err.Error())
				return
			}
		}
	}
	if err := s.store.UpdateCronJob(c.Request.Context(), id, strings.TrimSpace(req.CronExpr), req.Enabled, timeoutSec, cfgStr); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "cron job not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}
	if s.sched != nil {
		_ = s.sched.ReloadAll(c.Request.Context())
	}

	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "cron.update",
		ResourceType: "cron_job",
		ResourceID:   id,
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"job_key": existing.JobKey, "cron_expr": strings.TrimSpace(req.CronExpr), "enabled": req.Enabled, "timeout_sec": timeoutSec},
	})
	job, _ := s.store.GetCronJobByID(c.Request.Context(), id)
	if job == nil {
		c.JSON(http.StatusOK, gin.H{"status": "updated"})
		return
	}
	c.JSON(http.StatusOK, sanitizeCronJobDTO(*job))
}

func (s *AdminServer) handleRunCronJob(c *gin.Context) {
	admin := currentAdmin(c)
	id := c.Param("id")
	if s.sched == nil {
		writeError(c, http.StatusServiceUnavailable, "no_scheduler", "scheduler not running")
		return
	}
	run, err := s.sched.RunNow(c.Request.Context(), id, admin.Username)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "cron job not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "run_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, run)
}

func (s *AdminServer) handleListCronJobRuns(c *gin.Context) {
	id := c.Param("id")
	limit, _ := strconv.Atoi(c.Query("limit"))
	runs, err := s.store.ListCronJobRuns(c.Request.Context(), id, limit)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": runs})
}

// --- Audit Logs ---

func parseAuditFilter(c *gin.Context) db.AuditFilter {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	f := db.AuditFilter{
		ActorType: c.Query("actor_type"),
		Action:    c.Query("action"),
		Status:    c.Query("status"),
		Query:     c.Query("q"),
		Limit:     limit,
		Offset:    offset,
	}
	if fromStr := c.Query("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			f.From = &t
		}
	}
	if toStr := c.Query("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			f.To = &t
		}
	}
	return f
}

func (s *AdminServer) handleListAuditLogs(c *gin.Context) {
	f := parseAuditFilter(c)
	logs, total, err := s.store.ListAuditLogs(c.Request.Context(), f)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": logs, "total": total})
}

func (s *AdminServer) handleExportAuditLogs(c *gin.Context) {
	f := parseAuditFilter(c)
	logs, err := s.store.ExportAllAuditLogs(c.Request.Context(), f)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "export_failed", err.Error())
		return
	}
	format := strings.ToLower(c.Query("format"))
	if format == "json" {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Header("Content-Disposition", `attachment; filename="audit-logs.json"`)
		_ = json.NewEncoder(c.Writer).Encode(logs)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	cw := csv.NewWriter(c.Writer)
	_ = cw.Write([]string{"id", "created_at", "actor_type", "actor_name", "action", "resource_type", "resource_id", "status", "ip_address", "detail_json"})
	for _, l := range logs {
		_ = cw.Write([]string{
			strconv.FormatInt(l.ID, 10),
			l.CreatedAt.Format(time.RFC3339),
			l.ActorType,
			l.ActorName,
			l.Action,
			l.ResourceType,
			l.ResourceID,
			l.Status,
			l.IPAddress,
			l.DetailJSON,
		})
	}
	cw.Flush()
}

// --- Settings, SMTP & Admins ---

func (s *AdminServer) handleGetSettings(c *gin.Context) {
	settings, err := s.store.GetSystemSettings(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (s *AdminServer) handleUpdateSettings(c *gin.Context) {
	admin := currentAdmin(c)
	var settings config.SystemSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if settings.SendTimeoutSec <= 0 || settings.DefaultRateLimitRPM < 0 || settings.MaxBodyBytes < 0 {
		writeError(c, http.StatusBadRequest, "validation_error", "send_timeout_sec > 0, default_rate_limit_rpm >= 0, and max_body_bytes >= 0 are required")
		return
	}
	if err := s.store.UpdateSystemSettings(c.Request.Context(), settings, admin.Username); err != nil {
		writeError(c, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "settings.update",
		ResourceType: "system_settings",
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       settings,
	})
	c.JSON(http.StatusOK, settings)
}

type smtpAdminDTO struct {
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	User        string     `json:"user"`
	HasPassword bool       `json:"has_password"`
	Password    string     `json:"password,omitempty"`
	TestStatus  string     `json:"test_status"`
	TestError   string     `json:"test_error,omitempty"`
	TestedAt    *time.Time `json:"tested_at,omitempty"`
}

func toSMTPAdminDTO(cfg db.SMTPConfig, st db.SMTPStatus) smtpAdminDTO {
	return smtpAdminDTO{
		Host:        cfg.Host,
		Port:        cfg.Port,
		User:        cfg.User,
		HasPassword: cfg.Password != "" || cfg.PasswordEncrypted != "",
		Password:    cfg.Password,
		TestStatus:  st.Status,
		TestError:   st.Error,
		TestedAt:    st.TestedAt,
	}
}

type smtpUpdateReq struct {
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	User          string  `json:"user"`
	Password      *string `json:"password"`
	ClearPassword bool    `json:"clear_password"`
}

func mergeSMTPRequest(existing db.SMTPConfig, req smtpUpdateReq) db.SMTPConfig {
	cfg := db.SMTPConfig{
		Host: strings.TrimSpace(req.Host),
		Port: req.Port,
		User: strings.TrimSpace(req.User),
	}
	if cfg.Port <= 0 {
		cfg.Port = 465
	}
	if req.ClearPassword {
		cfg.Password = ""
		cfg.PasswordEncrypted = ""
	} else if req.Password != nil && *req.Password != "" && *req.Password != "*****" && *req.Password != "••••••" {
		cfg.Password = *req.Password
	} else {
		cfg.Password = existing.Password
		cfg.PasswordEncrypted = existing.PasswordEncrypted
	}
	return cfg
}

func (s *AdminServer) handleGetSMTPSettings(c *gin.Context) {
	cfg, err := s.store.GetSMTPConfig(c.Request.Context(), s.cipher)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "get_smtp_failed", err.Error())
		return
	}
	st, _ := s.store.GetSMTPStatus(c.Request.Context())
	c.JSON(http.StatusOK, toSMTPAdminDTO(cfg, st))
}

func (s *AdminServer) handleUpdateSMTPSettings(c *gin.Context) {
	admin := currentAdmin(c)
	var req smtpUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	existing, _ := s.store.GetSMTPConfig(c.Request.Context(), s.cipher)
	cfg := mergeSMTPRequest(existing, req)
	if cfg.Host == "" || cfg.Port <= 0 || cfg.Port > 65535 || cfg.User == "" {
		writeError(c, http.StatusBadRequest, "invalid_smtp", "host, port (1-65535) and user are required")
		return
	}
	if cfg.Password == "" && cfg.PasswordEncrypted == "" {
		writeError(c, http.StatusBadRequest, "invalid_smtp", "SMTP password is required")
		return
	}
	if err := s.store.SaveSMTPConfig(c.Request.Context(), s.cipher, cfg, admin.Username); err != nil {
		writeError(c, http.StatusInternalServerError, "save_smtp_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "settings.smtp_update",
		ResourceType: "smtp_config",
		IPAddress:    c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
		Detail:       map[string]any{"host": cfg.Host, "port": cfg.Port, "user": cfg.User},
	})
	saved, _ := s.store.GetSMTPConfig(c.Request.Context(), s.cipher)
	st, _ := s.store.GetSMTPStatus(c.Request.Context())
	c.JSON(http.StatusOK, toSMTPAdminDTO(saved, st))
}

func (s *AdminServer) handleTestSMTPSettings(c *gin.Context) {
	admin := currentAdmin(c)
	existing, err := s.store.GetSMTPConfig(c.Request.Context(), s.cipher)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "get_smtp_failed", err.Error())
		return
	}

	var req struct {
		Host          string  `json:"host"`
		Port          int     `json:"port"`
		User          string  `json:"user"`
		Password      *string `json:"password"`
		ClearPassword bool    `json:"clear_password"`
		TestTo        string  `json:"test_to"`
	}
	cfg := existing
	if c.Request.Body != nil {
		if decErr := json.NewDecoder(c.Request.Body).Decode(&req); decErr == nil && (req.Host != "" || req.User != "") {
			cfg = mergeSMTPRequest(existing, smtpUpdateReq{
				Host:          req.Host,
				Port:          req.Port,
				User:          req.User,
				Password:      req.Password,
				ClearPassword: req.ClearPassword,
			})
		}
	}

	testCtx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	var testErr error
	testTo := strings.TrimSpace(req.TestTo)
	if testTo != "" {
		testErr = s.mailService.SendEmailWithConfig(testCtx, cfg, service.SendEmailInput{
			To:         testTo,
			Subject:    "[MailServer] SMTP 测试邮件",
			Body:       fmt.Sprintf("这是一封来自 MailServer 的 SMTP 测试邮件。\n\n触发管理员: %s\n时间: %s\n", admin.Username, time.Now().Format(time.RFC3339)),
			Source:     "web_test",
			ClientID:   admin.ID,
			ClientName: admin.Username,
			ClientIP:   c.ClientIP(),
		})
	} else {
		testErr = s.factory.TestSMTP(testCtx, cfg, 15*time.Second)
	}

	if testErr != nil {
		_ = s.store.UpdateSMTPStatus(c.Request.Context(), "failed", testErr.Error())
	} else {
		_ = s.store.UpdateSMTPStatus(c.Request.Context(), "ok", "")
	}

	detail := map[string]any{"host": cfg.Host, "port": cfg.Port, "user": cfg.User, "test_to": testTo}
	if testErr != nil {
		detail["error"] = testErr.Error()
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "settings.smtp_test",
		ResourceType: "smtp_config",
		Status:       map[bool]string{true: "success", false: "failure"}[testErr == nil],
		IPAddress:    c.ClientIP(),
		Detail:       detail,
	})
	if testErr != nil {
		writeError(c, http.StatusBadGateway, "smtp_test_failed", testErr.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "host": cfg.Host, "port": cfg.Port})
}

func (s *AdminServer) handleListAdmins(c *gin.Context) {
	admins, err := s.store.ListAdmins(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": admins})
}

func (s *AdminServer) handleCreateAdmin(c *gin.Context) {
	actor := currentAdmin(c)
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if len(req.Username) < 3 || len(req.Password) < 8 {
		writeError(c, http.StatusBadRequest, "validation_error", "username >= 3 chars and password >= 8 chars required")
		return
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "hash_error", err.Error())
		return
	}
	a := &db.Admin{
		ID:           crypto.NewULID(),
		Username:     req.Username,
		PasswordHash: hash,
		IsActive:     true,
	}
	if err := s.store.CreateAdmin(c.Request.Context(), a); err != nil {
		writeError(c, http.StatusBadRequest, "create_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      actor.ID,
		ActorName:    actor.Username,
		Action:       "admin.create",
		ResourceType: "admin",
		ResourceID:   a.ID,
		IPAddress:    c.ClientIP(),
		Detail:       map[string]any{"username": a.Username},
	})
	c.JSON(http.StatusCreated, a)
}

func (s *AdminServer) handleUpdateAdmin(c *gin.Context) {
	actor := currentAdmin(c)
	id := c.Param("id")
	var req struct {
		IsActive *bool   `json:"is_active"`
		Password *string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if req.IsActive != nil {
		if id == actor.ID && !*req.IsActive {
			writeError(c, http.StatusBadRequest, "cannot_disable_self", "cannot disable your own admin account")
			return
		}
		if err := s.store.UpdateAdminActive(c.Request.Context(), id, *req.IsActive); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(c, http.StatusNotFound, "not_found", "admin not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
	}
	if req.Password != nil && *req.Password != "" {
		if len(*req.Password) < 8 {
			writeError(c, http.StatusBadRequest, "weak_password", "password must be at least 8 characters")
			return
		}
		hash, err := crypto.HashPassword(*req.Password)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "hash_error", err.Error())
			return
		}
		if err := s.store.UpdateAdminPassword(c.Request.Context(), id, hash); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeError(c, http.StatusNotFound, "not_found", "admin not found")
				return
			}
			writeError(c, http.StatusInternalServerError, "update_failed", err.Error())
			return
		}
	}
	a, err := s.store.GetAdminByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(c, http.StatusNotFound, "not_found", "admin not found")
			return
		}
		writeError(c, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      actor.ID,
		ActorName:    actor.Username,
		Action:       "admin.update",
		ResourceType: "admin",
		ResourceID:   id,
		IPAddress:    c.ClientIP(),
	})
	c.JSON(http.StatusOK, a)
}

// --- Migration Export & Import ---

func (s *AdminServer) handleMigrationExport(c *gin.Context) {
	admin := currentAdmin(c)
	var opts migration.ExportOptions
	if err := c.ShouldBindJSON(&opts); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	bundleBytes, err := s.migSvc.Export(c.Request.Context(), opts)
	if err != nil {
		writeError(c, http.StatusBadRequest, "export_failed", err.Error())
		return
	}
	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "migration.export",
		ResourceType: "migration",
		IPAddress:    c.ClientIP(),
		Detail: map[string]any{
			"include_mail_logs":  opts.IncludeMailLogs,
			"include_admins":     opts.IncludeAdmins,
			"include_audit_logs": opts.IncludeAuditLogs,
			"size_bytes":         len(bundleBytes),
		},
	})
	filename := fmt.Sprintf("mailserver-migration-%s.msbundle", time.Now().UTC().Format("20060102-150405"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Data(http.StatusOK, "application/octet-stream", bundleBytes)
}

func (s *AdminServer) handleMigrationInspect(c *gin.Context) {
	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_multipart", err.Error())
		return
	}
	passphrase := c.PostForm("passphrase")
	file, _, err := c.Request.FormFile("bundle")
	if err != nil {
		writeError(c, http.StatusBadRequest, "missing_bundle", "bundle file is required")
		return
	}
	defer file.Close()
	blob, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		writeError(c, http.StatusBadRequest, "read_failed", err.Error())
		return
	}
	summary, err := s.migSvc.Inspect(c.Request.Context(), blob, passphrase)
	if err != nil {
		writeError(c, http.StatusBadRequest, "inspect_failed", err.Error())
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (s *AdminServer) handleMigrationImport(c *gin.Context) {
	admin := currentAdmin(c)
	if err := c.Request.ParseMultipartForm(64 << 20); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_multipart", err.Error())
		return
	}
	file, _, err := c.Request.FormFile("bundle")
	if err != nil {
		writeError(c, http.StatusBadRequest, "missing_bundle", "bundle file is required")
		return
	}
	defer file.Close()
	blob, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		writeError(c, http.StatusBadRequest, "read_failed", err.Error())
		return
	}

	opts := migration.ImportOptions{
		Passphrase:       c.PostForm("passphrase"),
		Mode:             c.PostForm("mode"),
		ConflictStrategy: c.PostForm("conflict_strategy"),
		IncludeMailLogs:  c.PostForm("include_mail_logs") == "true",
		IncludeAdmins:    c.PostForm("include_admins") == "true",
		IncludeAuditLogs: c.PostForm("include_audit_logs") == "true",
	}

	counts, err := s.migSvc.Import(c.Request.Context(), blob, opts)
	if err != nil {
		s.auditLog.Record(c.Request.Context(), audit.Event{
			ActorType:    "admin",
			ActorID:      admin.ID,
			ActorName:    admin.Username,
			Action:       "migration.import",
			ResourceType: "migration",
			Status:       "failure",
			IPAddress:    c.ClientIP(),
			Detail:       map[string]any{"error": err.Error()},
		})
		writeError(c, http.StatusBadRequest, "import_failed", err.Error())
		return
	}

	if s.sched != nil {
		_ = s.sched.ReloadAll(c.Request.Context())
	}

	s.auditLog.Record(c.Request.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		ActorName:    admin.Username,
		Action:       "migration.import",
		ResourceType: "migration",
		IPAddress:    c.ClientIP(),
		Detail:       map[string]any{"mode": opts.Mode, "conflict_strategy": opts.ConflictStrategy, "counts": counts},
	})
	c.JSON(http.StatusOK, gin.H{"counts": counts})
}
