package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/service"
)

type stubMailSender struct {
	err   error
	input service.SendEmailInput
}

func (s *stubMailSender) SendEmail(_ context.Context, input service.SendEmailInput) error {
	s.input = input
	return s.err
}

type stubClientAuth struct {
	clients map[string]*db.Client
}

func (a *stubClientAuth) AuthenticateClient(_ context.Context, authKey string) (*db.Client, error) {
	if cl, ok := a.clients[authKey]; ok {
		return cl, nil
	}
	return nil, db.ErrNotFound
}

func TestMailHandlerSendEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	expired := time.Now().UTC().Add(-time.Hour)
	auth := &stubClientAuth{
		clients: map[string]*db.Client{
			"secret":   {ID: "cli_1", Name: "app-1", Enabled: true},
			"disabled": {ID: "cli_2", Name: "app-2", Enabled: false},
			"expired":  {ID: "cli_3", Name: "app-3", Enabled: true, ExpiresAt: &expired},
		},
	}

	tests := []struct {
		name           string
		body           string
		sender         *stubMailSender
		expectedStatus int
		expectedCode   float64
		expectedMsg    string
	}{
		{
			name:           "bad request",
			body:           `{"to":"receiver@example.com","subject":"","body":"hello","auth_key":"secret"}`,
			sender:         &stubMailSender{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   1,
			expectedMsg:    badRequestMessage,
		},
		{
			name:           "invalid auth key",
			body:           `{"to":"receiver@example.com","subject":"subject","body":"hello","auth_key":"wrong"}`,
			sender:         &stubMailSender{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   1,
			expectedMsg:    "Invalid auth key",
		},
		{
			name:           "disabled client",
			body:           `{"to":"receiver@example.com","subject":"subject","body":"hello","auth_key":"disabled"}`,
			sender:         &stubMailSender{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   1,
			expectedMsg:    "Invalid auth key",
		},
		{
			name:           "expired client",
			body:           `{"to":"receiver@example.com","subject":"subject","body":"hello","auth_key":"expired"}`,
			sender:         &stubMailSender{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   1,
			expectedMsg:    "Invalid auth key",
		},
		{
			name: "create client error",
			body: `{"to":"receiver@example.com","subject":"subject","body":"hello","auth_key":"secret"}`,
			sender: &stubMailSender{
				err: &service.CreateClientError{Err: errors.New("dial tcp timeout")},
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   1,
			expectedMsg:    "New email agent error: dial tcp timeout",
		},
		{
			name: "send email error",
			body: `{"to":"receiver@example.com","subject":"subject","body":"hello","auth_key":"secret"}`,
			sender: &stubMailSender{
				err: &service.SendError{Err: errors.New("smtp send failed")},
			},
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   1,
			expectedMsg:    "send email error",
		},
		{
			name:           "success",
			body:           `{"to":"receiver@example.com","subject":"subject","body":"hello","auth_key":"secret"}`,
			sender:         &stubMailSender{},
			expectedStatus: http.StatusOK,
			expectedCode:   0,
			expectedMsg:    "email sent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			mailHandler := NewMailHandler(zerolog.Nop(), auth, tt.sender, nil, nil)
			router.POST("/send", mailHandler.SendEmail)

			req := httptest.NewRequest(http.MethodPost, "/send", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			if recorder.Code != tt.expectedStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.expectedStatus)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal response error: %v", err)
			}

			if resp["code"] != tt.expectedCode {
				t.Fatalf("code = %v, want %v", resp["code"], tt.expectedCode)
			}

			if resp["msg"] != tt.expectedMsg {
				t.Fatalf("msg = %v, want %v", resp["msg"], tt.expectedMsg)
			}
		})
	}
}
