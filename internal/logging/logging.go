package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/config"
)

func New(cfg *config.Config) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.CallerMarshalFunc = func(_ uintptr, file string, line int) string {
		return formatCaller(file, line)
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Caller().Logger()
	logger.Info().
		Int("server_port", cfg.Server.Port).
		Bool("tls_enabled", cfg.TLS.Enable).
		Msg("config loaded")

	return logger
}

func formatCaller(file string, line int) string {
	cleaned := filepath.ToSlash(file)
	parts := strings.Split(cleaned, "/")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}

	return fmt.Sprintf("%s:%d", strings.Join(parts, "/"), line)
}
