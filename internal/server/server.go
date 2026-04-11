package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/handler"
	"github.com/sunliang711/mailserver/internal/middleware"
	"go.uber.org/fx"
)

func NewGinEngine(requestLogger *middleware.RequestLogger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	engine := gin.New()
	engine.Use(requestLogger.Handler())

	return engine
}

func RegisterRoutes(engine *gin.Engine, mailHandler *handler.MailHandler) {
	engine.POST("/send", mailHandler.SendEmail)
}

func NewHTTPServer(cfg *config.Config, engine *gin.Engine) *http.Server {
	return &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: engine,
	}
}

func RegisterLifecycle(lifecycle fx.Lifecycle, cfg *config.Config, srv *http.Server, logger zerolog.Logger) {
	serverLogger := logger.With().Str("component", "server.http").Logger()

	lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			listener, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}

			if cfg.TLS.Enable {
				if _, err := tls.LoadX509KeyPair(cfg.TLS.Cert, cfg.TLS.Key); err != nil {
					_ = listener.Close()
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
			return srv.Shutdown(ctx)
		},
	})
}
