package app

import (
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/email"
	"github.com/sunliang711/mailserver/internal/handler"
	"github.com/sunliang711/mailserver/internal/logging"
	"github.com/sunliang711/mailserver/internal/middleware"
	"github.com/sunliang711/mailserver/internal/server"
	"github.com/sunliang711/mailserver/internal/service"
	"go.uber.org/fx"
)

func New() *fx.App {
	return fx.New(
		fx.WithLogger(logging.NewFxLogger),
		fx.Provide(
			config.New,
			logging.New,
			middleware.NewRequestLogger,
			email.NewFactory,
			service.NewMailService,
			newMailSender,
			handler.NewMailHandler,
			server.NewGinEngine,
			server.NewHTTPServer,
		),
		fx.Invoke(
			server.RegisterRoutes,
			server.RegisterLifecycle,
		),
	)
}

func newMailSender(mailService *service.MailService) handler.MailSender {
	return mailService
}
