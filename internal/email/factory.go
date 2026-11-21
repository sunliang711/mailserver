package email

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/sunliang711/emailagent"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
)

type Client interface {
	SendEmail(to, subject, body string) error
	Close() error
}

type Factory struct {
	store  *db.Store
	cipher *crypto.Cipher
}

func NewFactory(store *db.Store, cipher *crypto.Cipher) *Factory {
	return &Factory{store: store, cipher: cipher}
}

func (f *Factory) NewClient(ctx context.Context) (Client, error) {
	if f.store == nil {
		return nil, errors.New("mail store not initialized")
	}
	cfg, err := f.store.GetSMTPConfig(ctx, f.cipher)
	if err != nil {
		return nil, err
	}
	return f.NewClientWithConfig(cfg)
}

func (f *Factory) NewClientWithConfig(cfg db.SMTPConfig) (Client, error) {
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 || strings.TrimSpace(cfg.User) == "" || cfg.Password == "" {
		return nil, errors.New("SMTP settings are not configured")
	}
	return emailagent.NewEmailAgent(
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
	)
}

// TestSMTP verifies SMTP configuration by establishing a connection and authenticating via emailagent.
func (f *Factory) TestSMTP(ctx context.Context, cfg db.SMTPConfig, timeout time.Duration) error {
	if strings.TrimSpace(cfg.Host) == "" || cfg.Port <= 0 || strings.TrimSpace(cfg.User) == "" || cfg.Password == "" {
		return errors.New("host, port, user, and password are required")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(strings.TrimSpace(cfg.Host), fmt.Sprintf("%d", cfg.Port)))
	if err != nil {
		return fmt.Errorf("tcp connect failed: %w", err)
	}
	_ = conn.Close()

	client, err := f.NewClientWithConfig(cfg)
	if err != nil {
		return err
	}
	return client.Close()
}
