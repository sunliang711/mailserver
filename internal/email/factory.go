package email

import (
	"github.com/sunliang711/emailagent"
	"github.com/sunliang711/mailserver/internal/config"
)

type Client interface {
	SendEmail(to, subject, body string) error
	Close() error
}

type Factory struct {
	cfg *config.Config
}

func NewFactory(cfg *config.Config) *Factory {
	return &Factory{cfg: cfg}
}

func (f *Factory) NewClient() (Client, error) {
	return emailagent.NewEmailAgent(
		f.cfg.Email.Host,
		f.cfg.Email.Port,
		f.cfg.Email.User,
		f.cfg.Email.Password,
	)
}
