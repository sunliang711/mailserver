package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sunliang711/mailserver/internal/audit"
	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
	"github.com/sunliang711/mailserver/internal/email"
	"github.com/sunliang711/mailserver/internal/handler"
	"github.com/sunliang711/mailserver/internal/middleware"
	"github.com/sunliang711/mailserver/internal/migration"
	"github.com/sunliang711/mailserver/internal/service"
)

func TestAccountsAndSessionsAPI(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Server: config.ServerConfig{DataDir: dir}, Defaults: config.DefaultSystemSettings(3000, false)}
	store, err := db.Open(filepath.Join(dir, "api.db"), dir, cfg.Defaults)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cipher, err := crypto.NewCipher(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	auditLog := audit.NewLogger(store)
	factory := email.NewFactory(store, cipher)
	mailSvc := service.NewMailService(zerolog.Nop(), factory, store, auditLog)
	server := NewAdminServer(cfg, store, cipher, factory, mailSvc, nil, migration.NewService(store, cipher), auditLog, nil)
	engine := NewGinEngine(middleware.NewRequestLogger(zerolog.Nop()), server)
	RegisterRoutes(engine, handler.NewMailHandler(zerolog.Nop(), handler.NewStoreClientAuth(store), mailSvc, store, auditLog), server)

	request := func(token, method, path string, payload any, status int) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/admin/api/v1"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "account-api-test")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s status=%d, want %d", method, path, res.Code, status)
		}
		return res
	}
	var setup struct {
		Token string     `json:"token"`
		Admin accountDTO `json:"admin"`
	}
	res := request("", http.MethodPost, "/setup/init", map[string]string{"username": "admin", "password": "password123"}, http.StatusCreated)
	if err := json.Unmarshal(res.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	if setup.Admin.Role != db.RoleAdmin {
		t.Fatal("initial account must be an administrator")
	}
	cookies := res.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("login cookie must be HttpOnly and SameSite=Lax")
	}
	request("old.jwt.credential", http.MethodGet, "/auth/me", nil, http.StatusUnauthorized)
	request(setup.Token, http.MethodPost, "/admins", map[string]string{"username": "viewer", "password": "password123", "role": "viewer"}, http.StatusBadRequest)
	request(setup.Token, http.MethodPost, "/admins", map[string]string{"username": "viewer", "password": "password123", "role": "viewer", "current_password": "password123"}, http.StatusCreated)
	var viewerLogin struct {
		Token string     `json:"token"`
		Admin accountDTO `json:"admin"`
	}
	res = request("", http.MethodPost, "/auth/login", map[string]string{"username": "viewer", "password": "password123"}, http.StatusOK)
	if err := json.Unmarshal(res.Body.Bytes(), &viewerLogin); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/auth/me", "/auth/sessions", "/dashboard/summary", "/mail-logs", "/mail-logs/export", "/clients", "/cron-jobs", "/audit-logs", "/audit-logs/export"} {
		request(viewerLogin.Token, http.MethodGet, path, nil, http.StatusOK)
	}
	for _, path := range []string{"/admins", "/settings", "/settings/smtp", "/clients/anything/secrets", "/cron-jobs/anything/secrets"} {
		request(viewerLogin.Token, http.MethodGet, path, nil, http.StatusForbidden)
	}
	for _, path := range []string{"/clients", "/clients/anything/rotate-key", "/mail/test-send", "/settings/smtp/test", "/cron-jobs/preview", "/cron-jobs/test-safekeep", "/cron-jobs/anything/run", "/migration/export", "/migration/import", "/migration/inspect", "/admins"} {
		request(viewerLogin.Token, http.MethodPost, path, map[string]string{}, http.StatusForbidden)
	}
	for _, path := range []string{"/clients/anything", "/settings", "/cron-jobs/anything", "/admins/anything"} {
		request(viewerLogin.Token, http.MethodPut, path, map[string]string{}, http.StatusForbidden)
	}
	request(viewerLogin.Token, http.MethodDelete, "/clients/anything", nil, http.StatusForbidden)

	res = request(viewerLogin.Token, http.MethodGet, "/auth/sessions", nil, http.StatusOK)
	var sessions struct {
		Items []sessionDTO `json:"items"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions.Items) != 1 || !sessions.Items[0].Current || sessions.Items[0].UserAgent != "account-api-test" {
		t.Fatal("session list must describe the current device")
	}
	for _, secret := range []string{"token_hash", "password_version", "password_hash"} {
		if strings.Contains(res.Body.String(), secret) {
			t.Fatal("session DTO leaked credential metadata")
		}
	}
	request(setup.Token, http.MethodPut, "/admins/"+setup.Admin.ID, map[string]string{"role": "viewer", "current_password": "password123"}, http.StatusConflict)
	request(setup.Token, http.MethodPut, "/admins/"+viewerLogin.Admin.ID, map[string]string{"role": "admin", "current_password": "password123"}, http.StatusOK)
	request(viewerLogin.Token, http.MethodGet, "/auth/me", nil, http.StatusUnauthorized)

	// 注销必须撤销服务端凭证，持有原始 Cookie 或 Bearer 也无法继续访问。
	request(setup.Token, http.MethodPost, "/auth/logout", nil, http.StatusOK)
	request(setup.Token, http.MethodGet, "/auth/me", nil, http.StatusUnauthorized)
	req := httptest.NewRequest(http.MethodGet, "/admin/api/v1/auth/me", nil)
	req.AddCookie(cookies[0])
	res = httptest.NewRecorder()
	engine.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatal("logout did not revoke the original cookie")
	}
}
