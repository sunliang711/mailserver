package service

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/email"
)

type SendEmailInput struct {
	To         string
	Subject    string
	Body       string
	Source     string // api | web_test
	ClientID   string
	ClientName string
	ClientIP   string
	RequestID  string
}

type CreateClientError struct {
	Err error
}

func (e *CreateClientError) Error() string {
	return fmt.Sprintf("New email agent error: %v", e.Err)
}

func (e *CreateClientError) Unwrap() error {
	return e.Err
}

type SendError struct {
	Err error
}

func (e *SendError) Error() string {
	return "send email error"
}

func (e *SendError) Unwrap() error {
	return e.Err
}

type MailService struct {
	logger   zerolog.Logger
	factory  *email.Factory
	store    *db.Store
	auditLog *audit.Logger
}

func NewMailService(logger zerolog.Logger, factory *email.Factory, store *db.Store, auditLog *audit.Logger) *MailService {
	return &MailService{
		logger:   logger.With().Str("component", "service.mail").Logger(),
		factory:  factory,
		store:    store,
		auditLog: auditLog,
	}
}

func (s *MailService) SendEmail(ctx context.Context, input SendEmailInput) error {
	return s.sendWithClient(ctx, input, func() (email.Client, error) {
		return s.factory.NewClient(ctx)
	})
}

func (s *MailService) SendEmailWithConfig(ctx context.Context, cfg db.SMTPConfig, input SendEmailInput) error {
	return s.sendWithClient(ctx, input, func() (email.Client, error) {
		return s.factory.NewClientWithConfig(cfg)
	})
}

func (s *MailService) sendWithClient(ctx context.Context, input SendEmailInput, dialFn func() (email.Client, error)) error {
	start := time.Now()
	source := input.Source
	if source == "" {
		source = "api"
	}

	client, err := dialFn()
	if err != nil {
		s.recordOutcome(ctx, input, source, "failed", err.Error(), time.Since(start).Milliseconds())
		return &CreateClientError{Err: err}
	}

	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("close email agent failed")
		}
	}()

	if err := client.SendEmail(input.To, input.Subject, input.Body); err != nil {
		s.recordOutcome(ctx, input, source, "failed", err.Error(), time.Since(start).Milliseconds())
		return &SendError{Err: err}
	}

	s.recordOutcome(ctx, input, source, "succeeded", "", time.Since(start).Milliseconds())
	s.logger.Info().Str("to", input.To).Str("source", source).Msg("email sent")
	return nil
}

func (s *MailService) recordOutcome(ctx context.Context, input SendEmailInput, source, status, errMsg string, durationMs int64) {
	if s.store == nil {
		return
	}
	storeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	body := ""
	if settings, err := s.store.GetSystemSettings(storeCtx); err == nil && settings.StoreMailBody {
		body = input.Body
	}

	logID := crypto.NewULID()
	_ = s.store.InsertMailLog(storeCtx, &db.MailLog{
		ID:           logID,
		ClientID:     input.ClientID,
		ClientName:   input.ClientName,
		Source:       source,
		Recipient:    input.To,
		Subject:      input.Subject,
		Body:         body,
		Status:       status,
		ErrorMessage: errMsg,
		DurationMs:   durationMs,
		ClientIP:     input.ClientIP,
		RequestID:    input.RequestID,
		CreatedAt:    time.Now().UTC(),
	})

	if input.ClientID != "" {
		_ = s.store.RecordClientUsage(storeCtx, input.ClientID, status == "succeeded")
	}

	if s.auditLog != nil {
		actorType := "client"
		actorID := input.ClientID
		actorName := input.ClientName
		action := "mail.send"
		if source == "web_test" {
			actorType = "admin"
			action = "mail.web_test"
		}
		auditStatus := "success"
		if status != "succeeded" {
			auditStatus = "failure"
		}
		detail := map[string]any{
			"to":          input.To,
			"subject":     input.Subject,
			"duration_ms": durationMs,
			"source":      source,
		}
		if errMsg != "" {
			detail["error"] = errMsg
		}
		s.auditLog.Record(storeCtx, audit.Event{
			ActorType:    actorType,
			ActorID:      actorID,
			ActorName:    actorName,
			Action:       action,
			ResourceType: "mail_log",
			ResourceID:   logID,
			Status:       auditStatus,
			IPAddress:    input.ClientIP,
			RequestID:    input.RequestID,
			Detail:       detail,
		})
	}
}
