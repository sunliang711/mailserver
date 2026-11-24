package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
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
	"github.com/sunliang711/mailserver/internal/scheduler"
	"github.com/sunliang711/mailserver/internal/service"
	"github.com/sunliang711/mailserver/web"
)

func TestAdminAndSafeKeepBackupE2E(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 3000, DataDir: dir},
		Defaults: config.DefaultSystemSettings(3000, false),
	}
	key, err := config.EnsureMasterKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(filepath.Join(dir, "test.db"), dir, cfg.Defaults)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	auditLog := audit.NewLogger(store)
	factory := email.NewFactory(store, cipher)
	mailSvc := service.NewMailService(zerolog.Nop(), factory, store, auditLog)
	migSvc := migration.NewService(store, cipher)
	sched := scheduler.NewEngine(store, factory, auditLog, zerolog.Nop(), cipher, migSvc)
	if err := sched.Start(); err != nil {
		t.Fatal(err)
	}
	defer sched.Stop()

	webFS, err := web.DistFS()
	if err != nil {
		t.Fatal(err)
	}
	adminSrv := NewAdminServer(cfg, store, cipher, factory, mailSvc, sched, migSvc, auditLog, webFS)
	mailHandler := handler.NewMailHandler(zerolog.Nop(), handler.NewStoreClientAuth(store), mailSvc, store, auditLog)
	engine := NewGinEngine(middleware.NewRequestLogger(zerolog.Nop()), adminSrv)
	RegisterRoutes(engine, mailHandler, adminSrv)

	ts := httptest.NewServer(engine)
	defer ts.Close()

	// 1. Setup status & initialization
	resp, err := http.Get(ts.URL + "/admin/api/v1/setup/status")
	if err != nil {
		t.Fatal(err)
	}
	var setupSt struct {
		Initialized bool `json:"initialized"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&setupSt)
	_ = resp.Body.Close()
	if setupSt.Initialized {
		t.Fatal("expected initialized=false initially")
	}

	httpClient := &http.Client{}
	initRes, err := http.Post(ts.URL+"/admin/api/v1/setup/init", "application/json",
		strings.NewReader(`{"username":"admin","password":"password123"}`))
	if err != nil {
		t.Fatal(err)
	}
	if initRes.StatusCode != http.StatusCreated {
		t.Fatalf("setup init status = %d", initRes.StatusCode)
	}
	cookies := initRes.Cookies()
	_ = initRes.Body.Close()

	doAdmin := func(method, path string, body any) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, ts.URL+path, r)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		res, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// 2. Create client and verify POST /send records mail_log even when SMTP is not configured
	cRes := doAdmin(http.MethodPost, "/admin/api/v1/clients", map[string]any{
		"name":           "orders-app",
		"description":    "Order notifications",
		"enabled":        true,
		"rate_limit_rpm": 100,
	})
	if cRes.StatusCode != http.StatusCreated {
		t.Fatalf("create client status = %d", cRes.StatusCode)
	}
	var createdClient struct {
		AuthKey string `json:"auth_key"`
	}
	_ = json.NewDecoder(cRes.Body).Decode(&createdClient)
	_ = cRes.Body.Close()
	if !strings.HasPrefix(createdClient.AuthKey, "ms_") {
		t.Fatalf("unexpected auth_key %q", createdClient.AuthKey)
	}

	sendRes, err := http.Post(ts.URL+"/send", "application/json", strings.NewReader(`{
		"to": "user@example.com",
		"subject": "Order #1",
		"body": "Confirmed",
		"auth_key": "`+createdClient.AuthKey+`"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	// Expect 500 because SMTP is unconfigured, and mail_logs should record the failed attempt
	if sendRes.StatusCode != http.StatusInternalServerError {
		t.Fatalf("send without SMTP status = %d, want 500", sendRes.StatusCode)
	}
	_ = sendRes.Body.Close()

	logsRes := doAdmin(http.MethodGet, "/admin/api/v1/mail-logs", nil)
	var logsBody struct {
		Items []db.MailLog `json:"items"`
		Total int64        `json:"total"`
	}
	_ = json.NewDecoder(logsRes.Body).Decode(&logsBody)
	_ = logsRes.Body.Close()
	if logsBody.Total != 1 || logsBody.Items[0].ClientName != "orders-app" || logsBody.Items[0].Status != "failed" {
		t.Fatalf("unexpected mail logs: %+v", logsBody)
	}

	// 3. Mock SafeKeep server for test-safekeep & migration_backup cron run
	var mu sync.Mutex
	var uploadedFile []byte
	var uploadedAPIKey, uploadedMode, uploadedPath, uploadedTargets string

	skMock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/upstreams" && r.Method == http.MethodGet {
			if r.Header.Get("X-API-Key") != "sk_test_12345" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":"up_1","name":"prod-s3","type":"s3","is_default":true}]}`))
			return
		}
		if r.URL.Path == "/api/v1/backups" && r.Method == http.MethodPost {
			if err := r.ParseMultipartForm(16 << 20); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f, _, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			mu.Lock()
			uploadedFile = b
			uploadedAPIKey = r.Header.Get("X-API-Key")
			uploadedMode = r.FormValue("mode")
			uploadedPath = r.FormValue("path")
			uploadedTargets = r.FormValue("targets")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"task":{"id":"tsk_01","status":"succeeded","remote_filename":"backup.msbundle"}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer skMock.Close()

	// Find migration_backup job
	jobsRes := doAdmin(http.MethodGet, "/admin/api/v1/cron-jobs", nil)
	var jobsBody struct {
		Items []db.CronJob `json:"items"`
	}
	_ = json.NewDecoder(jobsRes.Body).Decode(&jobsBody)
	_ = jobsRes.Body.Close()

	var mbJobID string
	for _, j := range jobsBody.Items {
		if j.JobKey == "migration_backup" {
			mbJobID = j.ID
			break
		}
	}
	if mbJobID == "" {
		t.Fatal("migration_backup job not found")
	}

	// Test SafeKeep connection endpoint
	testSKRes := doAdmin(http.MethodPost, "/admin/api/v1/cron-jobs/test-safekeep", map[string]any{
		"api_base_url": skMock.URL,
		"api_key":      "sk_test_12345",
	})
	if testSKRes.StatusCode != http.StatusOK {
		t.Fatalf("test-safekeep status = %d", testSKRes.StatusCode)
	}
	_ = testSKRes.Body.Close()

	// Update migration_backup job with optional encryption passphrase
	updRes := doAdmin(http.MethodPut, "/admin/api/v1/cron-jobs/"+mbJobID, map[string]any{
		"cron_expr":   "0 4 * * *",
		"enabled":     true,
		"timeout_sec": 120,
		"config": map[string]any{
			"api_base_url":       skMock.URL,
			"api_key":            "sk_test_12345",
			"targets":            "prod-s3",
			"path":               "mailserver-backups",
			"mode":               "sync",
			"passphrase":         "backup-pass-999",
			"include_admins":     true,
			"include_mail_logs":  true,
			"include_audit_logs": false,
		},
	})
	if updRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(updRes.Body)
		t.Fatalf("update cron job status = %d: %s", updRes.StatusCode, string(b))
	}
	_ = updRes.Body.Close()

	// Trigger run now
	runRes := doAdmin(http.MethodPost, "/admin/api/v1/cron-jobs/"+mbJobID+"/run", nil)
	if runRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(runRes.Body)
		t.Fatalf("run cron job status = %d: %s", runRes.StatusCode, string(b))
	}
	var runOut db.CronJobRun
	_ = json.NewDecoder(runRes.Body).Decode(&runOut)
	_ = runRes.Body.Close()
	if runOut.Status != "succeeded" {
		t.Fatalf("cron run status = %q, output=%q", runOut.Status, runOut.OutputLog)
	}

	mu.Lock()
	defer mu.Unlock()
	if uploadedAPIKey != "sk_test_12345" || uploadedMode != "sync" || uploadedPath != "mailserver-backups" || uploadedTargets != "prod-s3" {
		t.Fatalf("unexpected SafeKeep upload metadata: key=%q mode=%q path=%q targets=%q", uploadedAPIKey, uploadedMode, uploadedPath, uploadedTargets)
	}
	// Verify uploaded bundle can be inspected with configured passphrase
	insp, err := migSvc.Inspect(context.Background(), uploadedFile, "backup-pass-999")
	if err != nil {
		t.Fatalf("uploaded migration bundle failed inspect: %v", err)
	}
	if insp.Counts.Clients != 1 || insp.Counts.MailLogs != 1 {
		t.Fatalf("unexpected bundle counts: %+v", insp.Counts)
	}

	// 4. Verify embedded Web UI HTML is served on GET / and respects X-Forwarded-Prefix & BasePath
	spaRes, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	spaBody, _ := io.ReadAll(spaRes.Body)
	_ = spaRes.Body.Close()
	if spaRes.StatusCode != http.StatusOK || !strings.Contains(string(spaBody), "<div id=\"root\"></div>") {
		t.Fatalf("expected embedded SPA HTML on /, status=%d", spaRes.StatusCode)
	}

	xfpReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	xfpReq.Header.Set("X-Forwarded-Prefix", "/api/mail")
	xfpRes, err := httpClient.Do(xfpReq)
	if err != nil {
		t.Fatal(err)
	}
	xfpBody, _ := io.ReadAll(xfpRes.Body)
	_ = xfpRes.Body.Close()
	if !strings.Contains(string(xfpBody), `<base href="/api/mail/" />`) || !strings.Contains(string(xfpBody), `window.__MAILSERVER_BASE__ = "/api/mail";`) {
		t.Fatalf("expected injected /api/mail base tag and script, got:\n%s", string(xfpBody))
	}

	// Verify WrapHandler strips configured BasePath when upstream proxy forwards unstripped paths
	cfg.Server.BasePath = "/api/mail"
	wrappedTS := httptest.NewServer(adminSrv.WrapHandler(engine))
	defer wrappedTS.Close()
	subHealth, err := http.Get(wrappedTS.URL + "/api/mail/healthz")
	if err != nil {
		t.Fatal(err)
	}
	_ = subHealth.Body.Close()
	if subHealth.StatusCode != http.StatusOK {
		t.Fatalf("wrapped /api/mail/healthz status = %d, want 200", subHealth.StatusCode)
	}
}

func TestResetAdminFlowE2E(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: 3000, DataDir: dir},
		Defaults: config.DefaultSystemSettings(3000, false),
	}
	key, err := config.EnsureMasterKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	store, err := db.Open(filepath.Join(dir, "reset_test.db"), dir, cfg.Defaults)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	auditLog := audit.NewLogger(store)
	factory := email.NewFactory(store, cipher)
	mailSvc := service.NewMailService(zerolog.Nop(), factory, store, auditLog)
	migSvc := migration.NewService(store, cipher)
	sched := scheduler.NewEngine(store, factory, auditLog, zerolog.Nop(), cipher, migSvc)
	if err := sched.Start(); err != nil {
		t.Fatal(err)
	}
	defer sched.Stop()

	adminSrv := NewAdminServer(cfg, store, cipher, factory, mailSvc, sched, migSvc, auditLog, nil)
	mailHandler := handler.NewMailHandler(zerolog.Nop(), handler.NewStoreClientAuth(store), mailSvc, store, auditLog)
	engine := NewGinEngine(middleware.NewRequestLogger(zerolog.Nop()), adminSrv)
	RegisterRoutes(engine, mailHandler, adminSrv)

	ts := httptest.NewServer(engine)
	defer ts.Close()

	httpClient := &http.Client{}
	doWithCookies := func(method, path string, body any, cks []*http.Cookie) *http.Response {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, err := http.NewRequest(method, ts.URL+path, r)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		for _, ck := range cks {
			req.AddCookie(ck)
		}
		res, err := httpClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	// 1. Setup initial custom admin account and capture old session cookies
	initRes := doWithCookies(http.MethodPost, "/admin/api/v1/setup/init", map[string]string{
		"username": "eagle",
		"password": "password123",
	}, nil)
	if initRes.StatusCode != http.StatusCreated {
		t.Fatalf("setup init status = %d", initRes.StatusCode)
	}
	oldCookies := initRes.Cookies()
	_ = initRes.Body.Close()

	sumRes := doWithCookies(http.MethodGet, "/admin/api/v1/dashboard/summary", nil, oldCookies)
	_ = sumRes.Body.Close()
	if sumRes.StatusCode != http.StatusOK {
		t.Fatalf("initial dashboard summary = %d, want 200", sumRes.StatusCode)
	}

	// 2. Simulate restart with RESET_ADMIN=true
	var banner bytes.Buffer
	resetUser, tempPass, err := store.ResetAdminOnStartup(context.Background(), &banner)
	if err != nil {
		t.Fatalf("ResetAdminOnStartup failed: %v", err)
	}
	if resetUser != "eagle" || !strings.HasPrefix(tempPass, "adm_") {
		t.Fatalf("unexpected reset result user=%q tempPass=%q", resetUser, tempPass)
	}
	if !strings.Contains(banner.String(), tempPass) || !strings.Contains(banner.String(), "eagle") {
		t.Fatalf("expected credentials banner to contain username and temp password, got:\n%s", banner.String())
	}
	cfg.Server.ResetAdmin = true

	// Setup status reports reset_admin_env_active=true
	stRes, err := http.Get(ts.URL + "/admin/api/v1/setup/status")
	if err != nil {
		t.Fatal(err)
	}
	var stBody struct {
		Initialized         bool `json:"initialized"`
		ResetAdminEnvActive bool `json:"reset_admin_env_active"`
	}
	_ = json.NewDecoder(stRes.Body).Decode(&stBody)
	_ = stRes.Body.Close()
	if !stBody.Initialized || !stBody.ResetAdminEnvActive {
		t.Fatalf("unexpected setup status after reset: %+v", stBody)
	}

	// Previous session must be invalidated immediately
	oldSessionRes := doWithCookies(http.MethodGet, "/admin/api/v1/auth/me", nil, oldCookies)
	_ = oldSessionRes.Body.Close()
	if oldSessionRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected old session to return 401 after reset, got %d", oldSessionRes.StatusCode)
	}

	// Old password must fail
	oldPwdRes := doWithCookies(http.MethodPost, "/admin/api/v1/auth/login", map[string]string{
		"username": "eagle",
		"password": "password123",
	}, nil)
	_ = oldPwdRes.Body.Close()
	if oldPwdRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected old password login to return 401, got %d", oldPwdRes.StatusCode)
	}

	// 3. Login with temporary password -> must_change_password=true
	loginRes := doWithCookies(http.MethodPost, "/admin/api/v1/auth/login", map[string]string{
		"username": "eagle",
		"password": tempPass,
	}, nil)
	if loginRes.StatusCode != http.StatusOK {
		t.Fatalf("temp password login status = %d", loginRes.StatusCode)
	}
	var loginBody struct {
		Admin               db.Admin `json:"admin"`
		ResetAdminEnvActive bool     `json:"reset_admin_env_active"`
	}
	_ = json.NewDecoder(loginRes.Body).Decode(&loginBody)
	tempCookies := loginRes.Cookies()
	_ = loginRes.Body.Close()
	if !loginBody.Admin.MustChangePassword || !loginBody.ResetAdminEnvActive {
		t.Fatalf("expected must_change_password=true and reset_admin_env_active=true, got %+v", loginBody)
	}

	// Management APIs must be blocked with 403 while must_change_password=true
	blockedRes := doWithCookies(http.MethodGet, "/admin/api/v1/dashboard/summary", nil, tempCookies)
	blockedBody, _ := io.ReadAll(blockedRes.Body)
	_ = blockedRes.Body.Close()
	if blockedRes.StatusCode != http.StatusForbidden || !strings.Contains(string(blockedBody), "password_change_required") {
		t.Fatalf("expected 403 password_change_required, got %d: %s", blockedRes.StatusCode, string(blockedBody))
	}

	// /auth/me remains accessible so the UI can show the force-reset screen
	meRes := doWithCookies(http.MethodGet, "/admin/api/v1/auth/me", nil, tempCookies)
	_ = meRes.Body.Close()
	if meRes.StatusCode != http.StatusOK {
		t.Fatalf("expected /auth/me to return 200 during force-reset, got %d", meRes.StatusCode)
	}

	// 4. Complete forced password change without supplying old_password
	chgRes := doWithCookies(http.MethodPut, "/admin/api/v1/auth/password", map[string]string{
		"old_password": "",
		"new_password": "new-strong-password-456",
	}, tempCookies)
	if chgRes.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(chgRes.Body)
		t.Fatalf("force password change status = %d: %s", chgRes.StatusCode, string(b))
	}
	var chgBody struct {
		Admin db.Admin `json:"admin"`
	}
	_ = json.NewDecoder(chgRes.Body).Decode(&chgBody)
	newCookies := chgRes.Cookies()
	_ = chgRes.Body.Close()
	if chgBody.Admin.MustChangePassword {
		t.Fatal("expected must_change_password=false after password update")
	}

	// Temp-password cookie is invalidated; new cookie accesses console normally
	staleTempRes := doWithCookies(http.MethodGet, "/admin/api/v1/dashboard/summary", nil, tempCookies)
	_ = staleTempRes.Body.Close()
	if staleTempRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected stale temp cookie to return 401, got %d", staleTempRes.StatusCode)
	}

	okRes := doWithCookies(http.MethodGet, "/admin/api/v1/dashboard/summary", nil, newCookies)
	_ = okRes.Body.Close()
	if okRes.StatusCode != http.StatusOK {
		t.Fatalf("expected dashboard summary 200 after password change, got %d", okRes.StatusCode)
	}

	// Temporary password no longer works; new password works
	tempAgainRes := doWithCookies(http.MethodPost, "/admin/api/v1/auth/login", map[string]string{
		"username": "eagle",
		"password": tempPass,
	}, nil)
	_ = tempAgainRes.Body.Close()
	if tempAgainRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected temp password to be rejected after change, got %d", tempAgainRes.StatusCode)
	}
}
