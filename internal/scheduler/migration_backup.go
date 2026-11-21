package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/migration"
)

// MigrationBackupConfig defines persisted & DTO configuration for the migration_backup cron job.
type MigrationBackupConfig struct {
	APIBaseURL          string `json:"api_base_url"`
	APIKey              string `json:"api_key,omitempty"`
	APIKeyEncrypted     string `json:"api_key_encrypted,omitempty"`
	HasAPIKey           bool   `json:"has_api_key,omitempty"`
	Targets             string `json:"targets"`
	Path                string `json:"path"`
	Mode                string `json:"mode"`
	Rename              string `json:"rename"`
	Passphrase          string `json:"passphrase,omitempty"`
	PassphraseEncrypted string `json:"passphrase_encrypted,omitempty"`
	HasPassphrase       bool   `json:"has_passphrase,omitempty"`
	IncludeAdmins       bool   `json:"include_admins"`
	IncludeMailLogs     bool   `json:"include_mail_logs"`
	IncludeAuditLogs    bool   `json:"include_audit_logs"`
}

func defaultMigrationBackupConfig() MigrationBackupConfig {
	return MigrationBackupConfig{
		Path:          "mailserver-backups",
		Mode:          "sync",
		IncludeAdmins: true,
	}
}

func ParseMigrationBackupConfig(raw string) MigrationBackupConfig {
	cfg := defaultMigrationBackupConfig()
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}
	if cfg.Mode == "" {
		cfg.Mode = "sync"
	}
	return cfg
}

func SanitizeMigrationBackupConfigJSON(raw string) string {
	cfg := ParseMigrationBackupConfig(raw)
	cfg.HasAPIKey = cfg.APIKeyEncrypted != "" || cfg.APIKey != ""
	cfg.HasPassphrase = cfg.PassphraseEncrypted != "" || cfg.Passphrase != ""
	cfg.APIKey = ""
	cfg.APIKeyEncrypted = ""
	cfg.Passphrase = ""
	cfg.PassphraseEncrypted = ""
	b, err := json.Marshal(cfg)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func isMaskedPlaceholder(v string) bool {
	return v == "*****" || v == "••••••"
}

func MergeMigrationBackupConfig(c *crypto.Cipher, existingRaw string, incoming json.RawMessage) (string, error) {
	if c == nil {
		return "", errors.New("cipher is required")
	}
	existing := ParseMigrationBackupConfig(existingRaw)

	var req struct {
		APIBaseURL       string  `json:"api_base_url"`
		APIKey           *string `json:"api_key"`
		Targets          string  `json:"targets"`
		Path             string  `json:"path"`
		Mode             string  `json:"mode"`
		Rename           string  `json:"rename"`
		Passphrase       *string `json:"passphrase"`
		IncludeAdmins    *bool   `json:"include_admins"`
		IncludeMailLogs  *bool   `json:"include_mail_logs"`
		IncludeAuditLogs *bool   `json:"include_audit_logs"`
	}
	if len(incoming) > 0 {
		if err := json.Unmarshal(incoming, &req); err != nil {
			return "", fmt.Errorf("invalid migration_backup config: %w", err)
		}
	}

	baseURL := strings.TrimSpace(req.APIBaseURL)
	if baseURL != "" {
		u, err := url.Parse(baseURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "", errors.New("api_base_url must be a valid http or https URL")
		}
		baseURL = strings.TrimRight(baseURL, "/")
	}

	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "sync"
	}
	if mode != "sync" && mode != "async" {
		return "", errors.New("mode must be 'sync' or 'async'")
	}

	rename := strings.TrimSpace(req.Rename)
	if rename == "." || rename == ".." || rename == "/" {
		return "", errors.New("rename filename is invalid")
	}

	apiKeyEnc := existing.APIKeyEncrypted
	if req.APIKey != nil {
		val := strings.TrimSpace(*req.APIKey)
		if val == "" {
			apiKeyEnc = ""
		} else if !isMaskedPlaceholder(val) {
			enc, err := c.EncryptString(val)
			if err != nil {
				return "", err
			}
			apiKeyEnc = enc
		}
	}

	passEnc := existing.PassphraseEncrypted
	if req.Passphrase != nil {
		val := *req.Passphrase
		if val == "" {
			passEnc = ""
		} else if !isMaskedPlaceholder(strings.TrimSpace(val)) {
			if len(val) < 6 {
				return "", errors.New("passphrase must be at least 6 characters (or leave empty)")
			}
			enc, err := c.EncryptString(val)
			if err != nil {
				return "", err
			}
			passEnc = enc
		}
	}

	includeAdmins := existing.IncludeAdmins
	if req.IncludeAdmins != nil {
		includeAdmins = *req.IncludeAdmins
	}
	includeMailLogs := existing.IncludeMailLogs
	if req.IncludeMailLogs != nil {
		includeMailLogs = *req.IncludeMailLogs
	}
	includeAuditLogs := existing.IncludeAuditLogs
	if req.IncludeAuditLogs != nil {
		includeAuditLogs = *req.IncludeAuditLogs
	}

	out := MigrationBackupConfig{
		APIBaseURL:          baseURL,
		APIKeyEncrypted:     apiKeyEnc,
		Targets:             strings.TrimSpace(req.Targets),
		Path:                strings.TrimSpace(req.Path),
		Mode:                mode,
		Rename:              rename,
		PassphraseEncrypted: passEnc,
		IncludeAdmins:       includeAdmins,
		IncludeMailLogs:     includeMailLogs,
		IncludeAuditLogs:    includeAuditLogs,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func RevealMigrationBackupSecrets(c *crypto.Cipher, raw string) (apiKey, passphrase string, err error) {
	if c == nil {
		return "", "", errors.New("cipher is required")
	}
	cfg := ParseMigrationBackupConfig(raw)
	if cfg.APIKeyEncrypted != "" {
		apiKey, err = c.DecryptString(cfg.APIKeyEncrypted)
		if err != nil {
			return "", "", fmt.Errorf("decrypt api_key: %w", err)
		}
	}
	if cfg.PassphraseEncrypted != "" {
		passphrase, err = c.DecryptString(cfg.PassphraseEncrypted)
		if err != nil {
			return "", "", fmt.Errorf("decrypt passphrase: %w", err)
		}
	}
	return apiKey, passphrase, nil
}

type SafeKeepUpstream struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsDefault bool   `json:"is_default"`
}

// TestSafeKeepConnection validates api_base_url & api_key via GET /api/v1/upstreams.
func TestSafeKeepConnection(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]SafeKeepUpstream, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("api_base_url is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("api_base_url must be a valid http or https URL")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("api_key is required")
	}
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/upstreams", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", strings.TrimSpace(apiKey))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect SafeKeep failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var errOut struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &errOut)
		if errOut.Error.Message != "" {
			return nil, fmt.Errorf("SafeKeep API %d (%s): %s", resp.StatusCode, errOut.Error.Code, errOut.Error.Message)
		}
		return nil, fmt.Errorf("SafeKeep API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var okOut struct {
		Items []SafeKeepUpstream `json:"items"`
	}
	if err := json.Unmarshal(body, &okOut); err != nil {
		return nil, fmt.Errorf("parse SafeKeep response: %w", err)
	}
	return okOut.Items, nil
}

func (e *Engine) runMigrationBackup(ctx context.Context, rawCfg string) (int64, string, error) {
	if e.migSvc == nil || e.cipher == nil {
		return 0, "", errors.New("migration backup dependencies are not configured")
	}
	cfg := ParseMigrationBackupConfig(rawCfg)

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.APIBaseURL), "/")
	if baseURL == "" {
		return 0, "", errors.New("SafeKeep api_base_url is not configured")
	}

	apiKey := ""
	if cfg.APIKeyEncrypted != "" {
		var err error
		apiKey, err = e.cipher.DecryptString(cfg.APIKeyEncrypted)
		if err != nil {
			return 0, "", fmt.Errorf("decrypt configured api_key: %w", err)
		}
	}
	if strings.TrimSpace(apiKey) == "" {
		return 0, "", errors.New("SafeKeep api_key is not configured")
	}

	passphrase := ""
	if cfg.PassphraseEncrypted != "" {
		var err error
		passphrase, err = e.cipher.DecryptString(cfg.PassphraseEncrypted)
		if err != nil {
			return 0, "", fmt.Errorf("decrypt passphrase: %w", err)
		}
	}

	bundleBytes, err := e.migSvc.Export(ctx, migration.ExportOptions{
		IncludeAdmins:    cfg.IncludeAdmins,
		IncludeMailLogs:  cfg.IncludeMailLogs,
		IncludeAuditLogs: cfg.IncludeAuditLogs,
		Passphrase:       passphrase,
	})
	if err != nil {
		return 0, "", fmt.Errorf("export migration bundle: %w", err)
	}

	filename := fmt.Sprintf("mailserver-migration-%s.msbundle", time.Now().UTC().Format("20060102-150405"))
	var bodyBuf bytes.Buffer
	mw := multipart.NewWriter(&bodyBuf)
	_ = mw.WriteField("mode", cfg.Mode)
	if cfg.Path != "" {
		_ = mw.WriteField("path", cfg.Path)
	}
	if cfg.Targets != "" {
		_ = mw.WriteField("targets", cfg.Targets)
	}
	if cfg.Rename != "" {
		_ = mw.WriteField("rename", cfg.Rename)
	}
	_ = mw.WriteField("dedup", "false")
	metaJSON, _ := json.Marshal(map[string]any{
		"source":             "mailserver_cron_migration_backup",
		"include_admins":     cfg.IncludeAdmins,
		"include_mail_logs":  cfg.IncludeMailLogs,
		"include_audit_logs": cfg.IncludeAuditLogs,
		"encrypted_with_pwd": passphrase != "",
	})
	_ = mw.WriteField("metadata", string(metaJSON))

	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return 0, "", fmt.Errorf("create multipart form file: %w", err)
	}
	if _, err := fw.Write(bundleBytes); err != nil {
		return 0, "", fmt.Errorf("write bundle to multipart body: %w", err)
	}
	if err := mw.Close(); err != nil {
		return 0, "", fmt.Errorf("close multipart writer: %w", err)
	}

	endpoint := baseURL + "/api/v1/backups"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBuf.Bytes()))
	if err != nil {
		return 0, "", fmt.Errorf("build backup request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-Key", apiKey)

	httpClient := e.httpClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Minute}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("POST %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		var out struct {
			TimedOutToAsync bool `json:"timed_out_to_async"`
			Task            struct {
				ID               string `json:"id"`
				Status           string `json:"status"`
				OriginalFilename string `json:"original_filename"`
				RemoteFilename   string `json:"remote_filename"`
			} `json:"task"`
		}
		_ = json.Unmarshal(respBytes, &out)
		storedName := out.Task.RemoteFilename
		if storedName == "" {
			if cfg.Rename != "" {
				storedName = cfg.Rename
			} else {
				storedName = filename
			}
		}
		modeNote := out.Task.Status
		if resp.StatusCode == http.StatusAccepted && out.TimedOutToAsync {
			modeNote = "timed_out_to_async"
		}
		return 1, fmt.Sprintf("uploaded %s (%d bytes) to %s -> task %s (%s)", storedName, len(bundleBytes), endpoint, out.Task.ID, modeNote), nil
	}

	var errOut struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Task struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			ErrorSummary string `json:"error_summary"`
		} `json:"task"`
	}
	_ = json.Unmarshal(respBytes, &errOut)
	if errOut.Task.ID != "" {
		return 0, "", fmt.Errorf("backup API %d (task %s, %s): %s", resp.StatusCode, errOut.Task.ID, errOut.Task.Status, errOut.Task.ErrorSummary)
	}
	if errOut.Error.Message != "" {
		return 0, "", fmt.Errorf("backup API %d (%s): %s", resp.StatusCode, errOut.Error.Code, errOut.Error.Message)
	}
	return 0, "", fmt.Errorf("backup API returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBytes)))
}
