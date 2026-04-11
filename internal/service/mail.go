package service

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/email"
)

type SendEmailInput struct {
	To      string
	Subject string
	Body    string
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
	logger  zerolog.Logger
	factory *email.Factory
}

func NewMailService(logger zerolog.Logger, factory *email.Factory) *MailService {
	return &MailService{
		logger:  logger.With().Str("component", "service.mail").Logger(),
		factory: factory,
	}
}

func (s *MailService) SendEmail(_ context.Context, input SendEmailInput) error {
	client, err := s.factory.NewClient()
	if err != nil {
		return &CreateClientError{Err: err}
	}

	defer func() {
		if closeErr := client.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("close email agent failed")
		}
	}()

	if err := client.SendEmail(input.To, input.Subject, input.Body); err != nil {
		return &SendError{Err: err}
	}

	s.logger.Info().Str("to", input.To).Msg("email sent")
	return nil
}
