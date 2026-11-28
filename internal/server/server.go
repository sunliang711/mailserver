package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/email"
	"github.com/sunliang711/mailserver/internal/handler"
	"github.com/sunliang711/mailserver/internal/middleware"
	"github.com/sunliang711/mailserver/internal/migration"
	"github.com/sunliang711/mailserver/internal/scheduler"
	"github.com/sunliang711/mailserver/internal/service"
	"go.uber.org/fx"
)

const (
	adminCookieName = "mailserver_admin_token"
	ctxAdminKey     = "mailserver_admin"
	ctxPrincipalKey = "mailserver_principal"
)

type AdminServer struct {
	cfg         *config.Config
	store       *db.Store
	cipher      *crypto.Cipher
	accounts    *service.AccountService
	factory     *email.Factory
	mailService *service.MailService
	sched       *scheduler.Engine
	migSvc      *migration.Service
	auditLog    *audit.Logger
	webFS       fs.FS
	version     string
}

func NewAdminServer(
	cfg *config.Config,
	store *db.Store,
	cipher *crypto.Cipher,
	factory *email.Factory,
	mailService *service.MailService,
	sched *scheduler.Engine,
	migSvc *migration.Service,
	auditLog *audit.Logger,
	webFS fs.FS,
) *AdminServer {
	return &AdminServer{
		cfg:         cfg,
		store:       store,
		cipher:      cipher,
		accounts:    service.NewAccountService(store),
		factory:     factory,
		mailService: mailService,
		sched:       sched,
		migSvc:      migSvc,
		auditLog:    auditLog,
		webFS:       webFS,
		version:     "dev",
	}
}

func (s *AdminServer) SetVersion(v string) {
	if strings.TrimSpace(v) != "" {
		s.version = strings.TrimSpace(v)
	}
}

func NewGinEngine(requestLogger *middleware.RequestLogger, adminSrv *AdminServer) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(adminSrv.corsMiddleware())
	engine.Use(requestLogger.Handler())

	return engine
}

func RegisterRoutes(engine *gin.Engine, mailHandler *handler.MailHandler, s *AdminServer) {
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "version": s.version})
	})
	engine.GET("/readyz", s.handleReadyz)

	// Legacy & primary mail sending endpoint (contract preserved)
	engine.POST("/send", mailHandler.SendEmail)

	// Web Admin API
	admin := engine.Group("/admin/api/v1")
	{
		admin.GET("/setup/status", s.handleSetupStatus)
		admin.POST("/setup/init", s.handleSetupInit)
		admin.POST("/auth/login", s.handleAdminLogin)
		admin.POST("/auth/logout", s.handleAdminLogout)

		sessionAuthed := admin.Group("")
		sessionAuthed.Use(s.adminAuthMiddleware(true))
		{
			sessionAuthed.GET("/auth/me", s.handleAdminMe)
			sessionAuthed.PUT("/auth/password", s.handleAdminChangePassword)
			sessionAuthed.GET("/auth/sessions", s.handleListSessions)
			sessionAuthed.DELETE("/auth/sessions/:id", s.handleRevokeSession)
		}

		authed := admin.Group("")
		authed.Use(s.adminAuthMiddleware(false), s.consolePermissions())
		{
			authed.GET("/dashboard/summary", s.handleDashboardSummary)

			authed.GET("/mail-logs", s.handleListMailLogs)
			authed.GET("/mail-logs/export", s.handleExportMailLogs)
			authed.GET("/mail-logs/:id", s.handleGetMailLog)
			authed.POST("/mail/test-send", s.handleWebTestSend)

			authed.GET("/clients", s.handleListClients)
			authed.POST("/clients", s.handleCreateClient)
			authed.GET("/clients/:id/secrets", s.handleRevealClientSecrets)
			authed.PUT("/clients/:id", s.handleUpdateClient)
			authed.DELETE("/clients/:id", s.handleDeleteClient)
			authed.POST("/clients/:id/rotate-key", s.handleRotateClientKey)

			authed.GET("/cron-jobs", s.handleListCronJobs)
			authed.POST("/cron-jobs/preview", s.handlePreviewCronSchedule)
			authed.POST("/cron-jobs/test-safekeep", s.handleTestSafeKeep)
			authed.GET("/cron-jobs/:id/secrets", s.handleRevealCronJobSecrets)
			authed.PUT("/cron-jobs/:id", s.handleUpdateCronJob)
			authed.POST("/cron-jobs/:id/run", s.handleRunCronJob)
			authed.GET("/cron-jobs/:id/runs", s.handleListCronJobRuns)

			authed.GET("/audit-logs", s.handleListAuditLogs)
			authed.GET("/audit-logs/export", s.handleExportAuditLogs)

			authed.GET("/settings", s.handleGetSettings)
			authed.PUT("/settings", s.handleUpdateSettings)
			authed.GET("/settings/smtp", s.handleGetSMTPSettings)
			authed.PUT("/settings/smtp", s.handleUpdateSMTPSettings)
			authed.POST("/settings/smtp/test", s.handleTestSMTPSettings)

			authed.GET("/admins", s.handleListAdmins)
			authed.POST("/admins", s.handleCreateAdmin)
			authed.PUT("/admins/:id", s.handleUpdateAdmin)

			authed.POST("/migration/export", s.handleMigrationExport)
			authed.POST("/migration/inspect", s.handleMigrationInspect)
			authed.POST("/migration/import", s.handleMigrationImport)
		}
	}

	// Embedded React SPA fallback
	engine.NoRoute(s.handleSPA)
}

func NewHTTPServer(cfg *config.Config, adminSrv *AdminServer, engine *gin.Engine) *http.Server {
	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: adminSrv.WrapHandler(engine),
	}
}

// WrapHandler strips configured server.base_path if an upstream proxy forwards requests without stripping the prefix.
func (s *AdminServer) WrapHandler(next http.Handler) http.Handler {
	basePath := config.NormalizeBasePath(s.cfg.Server.BasePath)
	if basePath == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == basePath || strings.HasPrefix(p, basePath+"/") {
			trimmed := strings.TrimPrefix(p, basePath)
			if trimmed == "" {
				trimmed = "/"
			}
			r2 := r.Clone(r.Context())
			r2.URL.Path = trimmed
			if r2.URL.RawPath != "" {
				if rawTrimmed := strings.TrimPrefix(r2.URL.RawPath, basePath); rawTrimmed != "" {
					r2.URL.RawPath = rawTrimmed
				} else {
					r2.URL.RawPath = "/"
				}
			}
			next.ServeHTTP(w, r2)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RegisterLifecycle(
	lifecycle fx.Lifecycle,
	cfg *config.Config,
	srv *http.Server,
	store *db.Store,
	sched *scheduler.Engine,
	logger zerolog.Logger,
) {
	serverLogger := logger.With().Str("component", "server.http").Logger()

	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			if err := sched.Start(); err != nil {
				return fmt.Errorf("start scheduler: %w", err)
			}

			listener, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				sched.Stop()
				return err
			}

			if cfg.TLS.Enable {
				if _, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key); err != nil {
					_ = listener.Close()
					sched.Stop()
					return err
				}
			}

			serverLogger.Info().
				Str("addr", srv.Addr).
				Bool("tls_enabled", cfg.TLS.Enable).
				Msg("http server starting")

			go func() {
				var serveErr error
				if cfg.TLS.Enable {
					serveErr = srv.ServeTLS(listener, cfg.TLS.Cert, cfg.TLS.Key)
				} else {
					serveErr = srv.Serve(listener)
				}

				if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
					serverLogger.Error().Err(serveErr).Msg("http server exited unexpectedly")
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			serverLogger.Info().Msg("http server stopping")
			sched.Stop()
			err := srv.Shutdown(ctx)
			_ = store.Close()
			return err
		},
	})
}

func (s *AdminServer) handleReadyz(c *gin.Context) {
	if err := s.store.Ping(c.Request.Context()); err != nil {
		writeError(c, http.StatusServiceUnavailable, "db_unready", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}

func (s *AdminServer) isAllowedOrigin(r *http.Request, origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if settings, err := s.store.GetSystemSettings(r.Context()); err == nil && settings.PublicBaseURL != "" {
		if pub, pErr := url.Parse(settings.PublicBaseURL); pErr == nil && strings.EqualFold(pub.Scheme+"://"+pub.Host, u.Scheme+"://"+u.Host) {
			return true
		}
	}
	for _, item := range s.cfg.CORSAllowedOrigins {
		if strings.EqualFold(strings.TrimSpace(item), origin) {
			return true
		}
	}
	if extra := os.Getenv("MAILSERVER_CORS_ORIGINS"); extra != "" {
		for _, item := range strings.Split(extra, ",") {
			if strings.EqualFold(strings.TrimSpace(item), origin) {
				return true
			}
		}
	}
	return false
}

func (s *AdminServer) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Vary", "Origin")
			if s.isAllowedOrigin(c.Request, origin) {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func adminToken(c *gin.Context) string {
	if token, err := c.Cookie(adminCookieName); err == nil && token != "" {
		return token
	}
	if auth := c.GetHeader("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func (s *AdminServer) adminAuthMiddleware(allowMustChange bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, err := s.accounts.Authenticate(c.Request.Context(), adminToken(c))
		if err != nil {
			s.writeAccountError(c, err)
			c.Abort()
			return
		}
		if !allowMustChange && p.Admin.MustChangePassword {
			writeError(c, http.StatusForbidden, "password_change_required", "admin password change required before accessing management console")
			c.Abort()
			return
		}
		c.Set(ctxAdminKey, p.Admin)
		c.Set(ctxPrincipalKey, p)
		c.Next()
	}
}

func (s *AdminServer) consolePermissions() gin.HandlerFunc {
	// 只读账号按明确的路由白名单放行，密钥、SMTP 配置与迁移入口只对管理员开放。
	readRoutes := map[string]bool{
		"/admin/api/v1/dashboard/summary":  true,
		"/admin/api/v1/mail-logs":          true,
		"/admin/api/v1/mail-logs/export":   true,
		"/admin/api/v1/mail-logs/:id":      true,
		"/admin/api/v1/clients":            true,
		"/admin/api/v1/cron-jobs":          true,
		"/admin/api/v1/cron-jobs/:id/runs": true,
		"/admin/api/v1/audit-logs":         true,
		"/admin/api/v1/audit-logs/export":  true,
	}
	return func(c *gin.Context) {
		if currentAdmin(c).Role == db.RoleAdmin || c.Request.Method == http.MethodGet && readRoutes[c.FullPath()] {
			c.Next()
			return
		}
		writeError(c, http.StatusForbidden, "forbidden", "administrator permission required")
		c.Abort()
	}
}

func currentPrincipal(c *gin.Context) *service.Principal {
	p, _ := c.MustGet(ctxPrincipalKey).(*service.Principal)
	return p
}

func accountActor(c *gin.Context) service.AccountActor {
	return service.AccountActor{Principal: currentPrincipal(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func currentAdmin(c *gin.Context) *db.Admin {
	v, ok := c.Get(ctxAdminKey)
	if !ok {
		return nil
	}
	a, _ := v.(*db.Admin)
	return a
}

func writeError(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": msg,
		},
	})
}

func (s *AdminServer) effectiveBasePath(r *http.Request) string {
	if xfp := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix")); xfp != "" {
		if first := strings.TrimSpace(strings.Split(xfp, ",")[0]); first != "" {
			if norm := config.NormalizeBasePath(first); norm != "" {
				return norm
			}
		}
	}
	return config.NormalizeBasePath(s.cfg.Server.BasePath)
}

func (s *AdminServer) handleSPA(c *gin.Context) {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		writeError(c, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if strings.HasPrefix(c.Request.URL.Path, "/admin/api/") || strings.HasPrefix(c.Request.URL.Path, "/api/") {
		writeError(c, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if s.webFS == nil {
		writeError(c, http.StatusNotFound, "no_ui", "web UI not bundled")
		return
	}

	reqPath := strings.TrimPrefix(c.Request.URL.Path, "/")
	if reqPath != "" && reqPath != "index.html" {
		if f, err := s.webFS.Open(reqPath); err == nil {
			stat, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !stat.IsDir() {
				http.FileServer(http.FS(s.webFS)).ServeHTTP(c.Writer, c.Request)
				return
			}
		}
	}

	indexBytes, err := fs.ReadFile(s.webFS, "index.html")
	if err != nil {
		writeError(c, http.StatusNotFound, "not_found", "index.html missing")
		return
	}

	basePath := s.effectiveBasePath(c.Request)
	baseHref := basePath + "/"
	baseJSON, _ := json.Marshal(basePath)
	injection := fmt.Sprintf("<head>\n    <base href=%q />\n    <script>window.__MAILSERVER_BASE__ = %s;</script>", html.EscapeString(baseHref), string(baseJSON))
	rendered := strings.Replace(string(indexBytes), "<head>", injection, 1)

	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(rendered))
}
