package handler

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/service"
)

const badRequestMessage = `{"to":"receiver","subject":"your subject","body":"your content","auth_key":"someKey"} as request body`

type MailSender interface {
	SendEmail(ctx context.Context, input service.SendEmailInput) error
}

type ClientAuthenticator interface {
	AuthenticateClient(ctx context.Context, authKey string) (*db.Client, error)
}

type MailHandler struct {
	logger   zerolog.Logger
	auth     ClientAuthenticator
	sender   MailSender
	store    *db.Store
	auditLog *audit.Logger

	rateMu      sync.Mutex
	rateBuckets map[string]*rateWindow
}

type rateWindow struct {
	windowStart time.Time
	count       int
}

type emailContent struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	AuthKey string `json:"auth_key"`
}

type storeClientAuth struct {
	store *db.Store
}

func NewStoreClientAuth(store *db.Store) ClientAuthenticator {
	return &storeClientAuth{store: store}
}

func (a *storeClientAuth) AuthenticateClient(ctx context.Context, authKey string) (*db.Client, error) {
	return a.store.GetClientByKeyHash(ctx, crypto.HashAPIKey(authKey))
}

func NewMailHandler(logger zerolog.Logger, auth ClientAuthenticator, sender MailSender, store *db.Store, auditLog *audit.Logger) *MailHandler {
	return &MailHandler{
		logger:      logger.With().Str("component", "handler.mail").Logger(),
		auth:        auth,
		sender:      sender,
		store:       store,
		auditLog:    auditLog,
		rateBuckets: make(map[string]*rateWindow),
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

	client, err := h.auth.AuthenticateClient(ctx.Request.Context(), req.AuthKey)
	if err != nil || client == nil || !client.Enabled || (client.ExpiresAt != nil && time.Now().UTC().After(*client.ExpiresAt)) {
		msg := "Invalid auth key"
		h.logger.Error().Msg(msg)
		if h.auditLog != nil {
			reason := "invalid_key"
			clientID := ""
			clientName := ""
			if client != nil {
				clientID = client.ID
				clientName = client.Name
				if !client.Enabled {
					reason = "client_disabled"
				} else if client.ExpiresAt != nil && time.Now().UTC().After(*client.ExpiresAt) {
					reason = "key_expired"
				}
			}
			h.auditLog.Record(ctx.Request.Context(), audit.Event{
				ActorType:    "client",
				ActorID:      clientID,
				ActorName:    clientName,
				Action:       "mail.auth_failed",
				ResourceType: "client",
				ResourceID:   clientID,
				Status:       "failure",
				IPAddress:    ctx.ClientIP(),
				UserAgent:    ctx.Request.UserAgent(),
				Detail:       map[string]any{"to": req.To, "reason": reason},
			})
		}
		ctx.JSON(400, gin.H{
			"code": 1,
			"msg":  msg,
		})
		return
	}

	if h.store != nil {
		if settings, sErr := h.store.GetSystemSettings(ctx.Request.Context()); sErr == nil {
			if settings.MaxBodyBytes > 0 && int64(len(req.Body)) > settings.MaxBodyBytes {
				ctx.JSON(413, gin.H{
					"code": 1,
					"msg":  "body exceeds maximum allowed size",
				})
				return
			}
			rpm := client.RateLimitRPM
			if rpm <= 0 && settings.DefaultRateLimitRPM > 0 {
				rpm = settings.DefaultRateLimitRPM
			}
			if !h.allowRequest(client.ID, rpm) {
				ctx.JSON(429, gin.H{
					"code": 1,
					"msg":  "rate limit exceeded",
				})
				return
			}
		}
	} else if !h.allowRequest(client.ID, client.RateLimitRPM) {
		ctx.JSON(429, gin.H{
			"code": 1,
			"msg":  "rate limit exceeded",
		})
		return
	}

	sendCtx := ctx.Request.Context()
	if h.store != nil {
		if settings, sErr := h.store.GetSystemSettings(ctx.Request.Context()); sErr == nil && settings.SendTimeoutSec > 0 {
			var cancel context.CancelFunc
			sendCtx, cancel = context.WithTimeout(sendCtx, time.Duration(settings.SendTimeoutSec)*time.Second)
			defer cancel()
		}
	}

	err = h.sender.SendEmail(sendCtx, service.SendEmailInput{
		To:         req.To,
		Subject:    req.Subject,
		Body:       req.Body,
		Source:     "api",
		ClientID:   client.ID,
		ClientName: client.Name,
		ClientIP:   ctx.ClientIP(),
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
	h.logger.Info().Str("client", client.Name).Msg("email sent")
}

func (h *MailHandler) allowRequest(clientID string, rpm int) bool {
	if rpm <= 0 {
		return true
	}
	now := time.Now().UTC()
	h.rateMu.Lock()
	defer h.rateMu.Unlock()
	w, ok := h.rateBuckets[clientID]
	if !ok || now.Sub(w.windowStart) >= time.Minute {
		h.rateBuckets[clientID] = &rateWindow{windowStart: now, count: 1}
		return true
	}
	if w.count >= rpm {
		return false
	}
	w.count++
	return true
}
