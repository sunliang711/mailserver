package handler

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/service"
)

const badRequestMessage = `{"to":"receiver","subject":"your subject","body":"your content","auth_key":"someKey"} as request body`

type MailSender interface {
	SendEmail(ctx context.Context, input service.SendEmailInput) error
}

type MailHandler struct {
	logger  zerolog.Logger
	authKey string
	sender  MailSender
}

type emailContent struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	AuthKey string `json:"auth_key"`
}

func NewMailHandler(logger zerolog.Logger, cfg *config.Config, sender MailSender) *MailHandler {
	return &MailHandler{
		logger:  logger.With().Str("component", "handler.mail").Logger(),
		authKey: cfg.Auth.Key,
		sender:  sender,
	}
}

func (h *MailHandler) SendEmail(ctx *gin.Context) {
	var req emailContent
	if err := ctx.ShouldBindJSON(&req); err != nil || req.To == "" || req.Subject == "" || req.Body == "" || req.AuthKey == "" {
		if err != nil {
			h.logger.Error().Err(err).Msg("bad request")
		} else {
			h.logger.Error().Msg("bad request")
		}

		ctx.JSON(400, gin.H{
			"code": 1,
			"msg":  badRequestMessage,
		})
		return
	}

	if req.AuthKey != h.authKey {
		msg := "Invalid auth key"
		h.logger.Error().Msg(msg)
		ctx.JSON(400, gin.H{
			"code": 1,
			"msg":  msg,
		})
		return
	}

	err := h.sender.SendEmail(ctx.Request.Context(), service.SendEmailInput{
		To:      req.To,
		Subject: req.Subject,
		Body:    req.Body,
	})
	if err != nil {
		var createClientErr *service.CreateClientError
		if errors.As(err, &createClientErr) {
			msg := createClientErr.Error()
			h.logger.Error().Err(err).Msg("new email agent error")
			ctx.JSON(500, gin.H{
				"code": 1,
				"msg":  msg,
			})
			return
		}

		h.logger.Error().Err(err).Msg("send email error")
		ctx.JSON(500, gin.H{
			"code": 1,
			"msg":  "send email error",
		})
		return
	}

	ctx.JSON(200, gin.H{
		"code": 0,
		"msg":  "email sent",
	})
	h.logger.Info().Msg("email sent")
}
