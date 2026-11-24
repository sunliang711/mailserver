package app

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/email"
	"github.com/sunliang711/mailserver/internal/handler"
	"github.com/sunliang711/mailserver/internal/logging"
	"github.com/sunliang711/mailserver/internal/middleware"
	"github.com/sunliang711/mailserver/internal/migration"
	"github.com/sunliang711/mailserver/internal/scheduler"
	"github.com/sunliang711/mailserver/internal/server"
	"github.com/sunliang711/mailserver/internal/service"
	"github.com/sunliang711/mailserver/web"
	"go.uber.org/fx"
)

func New(opts ...fx.Option) *fx.App {
	baseOpts := []fx.Option{
		fx.WithLogger(logging.NewFxLogger),
		fx.Provide(
			config.New,
			logging.New,
			newCipher,
			newStore,
			audit.NewLogger,
			middleware.NewRequestLogger,
			email.NewFactory,
			service.NewMailService,
			newMailSender,
			handler.NewStoreClientAuth,
			handler.NewMailHandler,
			migration.NewService,
			scheduler.NewEngine,
			newWebFS,
			server.NewAdminServer,
			server.NewGinEngine,
			server.NewHTTPServer,
		),
		fx.Invoke(
			server.RegisterRoutes,
			server.RegisterLifecycle,
		),
	}
	baseOpts = append(baseOpts, opts...)
	return fx.New(baseOpts...)
}

func NewWithBootstrap(cfgPath string, cliOverrides map[string]string, version string) *fx.App {
	return fx.New(
		fx.WithLogger(logging.NewFxLogger),
		fx.Provide(
			func() (*config.Config, error) {
				return config.Load(cfgPath, cliOverrides)
			},
			logging.New,
			newCipher,
			newStore,
			audit.NewLogger,
			middleware.NewRequestLogger,
			email.NewFactory,
			service.NewMailService,
			newMailSender,
			handler.NewStoreClientAuth,
			handler.NewMailHandler,
			migration.NewService,
			scheduler.NewEngine,
			newWebFS,
			func(
				cfg *config.Config,
				store *db.Store,
				cipher *crypto.Cipher,
				factory *email.Factory,
				mailService *service.MailService,
				sched *scheduler.Engine,
				migSvc *migration.Service,
				auditLog *audit.Logger,
				webFS fs.FS,
			) *server.AdminServer {
				srv := server.NewAdminServer(cfg, store, cipher, factory, mailService, sched, migSvc, auditLog, webFS)
				srv.SetVersion(version)
				return srv
			},
			server.NewGinEngine,
			server.NewHTTPServer,
		),
		fx.Invoke(
			server.RegisterRoutes,
			server.RegisterLifecycle,
		),
	)
}

func newCipher(cfg *config.Config) (*crypto.Cipher, error) {
	key, err := config.EnsureMasterKey(cfg.Server.DataDir, cfg.Server.MasterKey)
	if err != nil {
		return nil, err
	}
	return crypto.NewCipher(key)
}

func newStore(cfg *config.Config, cipher *crypto.Cipher) (*db.Store, error) {
	dbPath := filepath.Join(cfg.Server.DataDir, "mailserver.db")
	store, err := db.Open(dbPath, cfg.Server.DataDir, cfg.Defaults)
	if err != nil {
		return nil, err
	}
	if err := store.SeedSMTPFromConfigIfEmpty(context.Background(), cipher, cfg.Email); err != nil {
		_ = store.Close()
		return nil, err
	}
	if cfg.Server.ResetAdmin {
		if _, _, err := store.ResetAdminOnStartup(context.Background(), os.Stdout); err != nil {
			_ = store.Close()
			return nil, err
		}
	}
	return store, nil
}

func newWebFS() (fs.FS, error) {
	return web.DistFS()
}

func newMailSender(mailService *service.MailService) handler.MailSender {
	return mailService
}
