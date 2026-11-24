package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultPort    = 3000
	DefaultDataDir = "./data"
)

type Config struct {
	ConfigPath         string       `yaml:"-"`
	Email              EmailConfig  `yaml:"email"`
	Server             ServerConfig `yaml:"server"`
	TLS                TLSConfig    `yaml:"tls"`
	CORSAllowedOrigins []string     `yaml:"-"`
	Defaults           SystemSettings
}

type EmailConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	User     string `yaml:"user" json:"user"`
	Password string `yaml:"password" json:"password,omitempty"`
}

type ServerConfig struct {
	Port               int      `yaml:"port"`
	DataDir            string   `yaml:"data_dir"`
	MasterKey          string   `yaml:"master_key"`
	BasePath           string   `yaml:"base_path"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	ResetAdmin         bool     `yaml:"reset_admin"`
}

type TLSConfig struct {
	Enable bool   `yaml:"enable"`
	Key    string `yaml:"key"`
	Cert   string `yaml:"cert"`
}

// SystemSettings holds dynamic runtime settings managed via the Web UI and stored in SQLite.
type SystemSettings struct {
	PublicBaseURL       string `json:"public_base_url"`
	SendTimeoutSec      int    `json:"send_timeout_sec"`
	StoreMailBody       bool   `json:"store_mail_body"`
	DefaultRateLimitRPM int    `json:"default_rate_limit_rpm"`
	MaxBodyBytes        int64  `json:"max_body_bytes"`
}

// DefaultSystemSettings returns sensible out-of-the-box runtime settings.
func DefaultSystemSettings(port int, tlsEnabled bool) SystemSettings {
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	if port <= 0 {
		port = DefaultPort
	}
	return SystemSettings{
		PublicBaseURL:       fmt.Sprintf("%s://localhost:%d", scheme, port),
		SendTimeoutSec:      30,
		StoreMailBody:       false,
		DefaultRateLimitRPM: 0,
		MaxBodyBytes:        1024 * 1024, // 1 MiB
	}
}

// New resolves bootstrap configuration with precedence:
// Environment variables (MAILSERVER_*) > config.yaml > built-in defaults.
func New() (*Config, error) {
	return Load("", nil)
}

// Load resolves bootstrap configuration with optional explicit config path and CLI overrides.
func Load(explicitConfigPath string, cliOverrides map[string]string) (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:    DefaultPort,
			DataDir: DefaultDataDir,
		},
	}

	cfgPath := strings.TrimSpace(explicitConfigPath)
	explicit := cfgPath != ""
	if !explicit {
		if envPath := strings.TrimSpace(os.Getenv("MAILSERVER_CONFIG")); envPath != "" {
			cfgPath = envPath
			explicit = true
		} else if _, err := os.Stat("config.yaml"); err == nil {
			cfgPath = "config.yaml"
		}
	}

	if cfgPath != "" {
		raw, err := os.ReadFile(cfgPath)
		if err != nil {
			if explicit {
				return nil, fmt.Errorf("read config %q: %w", cfgPath, err)
			}
		} else {
			var fileCfg Config
			if err := yaml.Unmarshal(raw, &fileCfg); err != nil {
				return nil, fmt.Errorf("parse config %q: %w", cfgPath, err)
			}
			cfg.ConfigPath = cfgPath
			if fileCfg.Server.Port > 0 {
				cfg.Server.Port = fileCfg.Server.Port
			}
			if strings.TrimSpace(fileCfg.Server.DataDir) != "" {
				cfg.Server.DataDir = strings.TrimSpace(fileCfg.Server.DataDir)
			}
			if strings.TrimSpace(fileCfg.Server.MasterKey) != "" {
				cfg.Server.MasterKey = strings.TrimSpace(fileCfg.Server.MasterKey)
			}
			if strings.TrimSpace(fileCfg.Server.BasePath) != "" {
				cfg.Server.BasePath = NormalizeBasePath(fileCfg.Server.BasePath)
			}
			if len(fileCfg.Server.CORSAllowedOrigins) > 0 {
				cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, fileCfg.Server.CORSAllowedOrigins...)
			}
			cfg.Server.ResetAdmin = fileCfg.Server.ResetAdmin
			cfg.TLS = fileCfg.TLS
			cfg.Email = fileCfg.Email
		}
	}

	// Environment variable overrides (MAILSERVER_*)
	if v, ok := os.LookupEnv("MAILSERVER_PORT"); ok && strings.TrimSpace(v) != "" {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid MAILSERVER_PORT %q", v)
		}
		cfg.Server.Port = port
	}
	if v, ok := os.LookupEnv("MAILSERVER_DATA_DIR"); ok && strings.TrimSpace(v) != "" {
		cfg.Server.DataDir = strings.TrimSpace(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_MASTER_KEY"); ok && strings.TrimSpace(v) != "" {
		cfg.Server.MasterKey = strings.TrimSpace(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_BASE_PATH"); ok {
		cfg.Server.BasePath = NormalizeBasePath(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_CORS_ORIGINS"); ok {
		cfg.CORSAllowedOrigins = parseCSV(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_TLS_ENABLE"); ok && strings.TrimSpace(v) != "" {
		cfg.TLS.Enable = parseBool(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_TLS_CERT"); ok && strings.TrimSpace(v) != "" {
		cfg.TLS.Cert = strings.TrimSpace(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_TLS_KEY"); ok && strings.TrimSpace(v) != "" {
		cfg.TLS.Key = strings.TrimSpace(v)
	}
	if v, ok := os.LookupEnv("reset_admin"); ok && strings.TrimSpace(v) != "" {
		cfg.Server.ResetAdmin = parseBool(v)
	}
	if v, ok := os.LookupEnv("RESET_ADMIN"); ok && strings.TrimSpace(v) != "" {
		cfg.Server.ResetAdmin = parseBool(v)
	}
	if v, ok := os.LookupEnv("MAILSERVER_RESET_ADMIN"); ok && strings.TrimSpace(v) != "" {
		cfg.Server.ResetAdmin = parseBool(v)
	}

	// CLI flag overrides
	if v, ok := cliOverrides["port"]; ok && strings.TrimSpace(v) != "" {
		port, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid --port %q", v)
		}
		cfg.Server.Port = port
	}
	if v, ok := cliOverrides["data-dir"]; ok && strings.TrimSpace(v) != "" {
		cfg.Server.DataDir = strings.TrimSpace(v)
	}
	if v, ok := cliOverrides["master-key"]; ok && strings.TrimSpace(v) != "" {
		cfg.Server.MasterKey = strings.TrimSpace(v)
	}
	if v, ok := cliOverrides["base-path"]; ok {
		cfg.Server.BasePath = NormalizeBasePath(v)
	}
	if v, ok := cliOverrides["reset-admin"]; ok && strings.TrimSpace(v) != "" {
		cfg.Server.ResetAdmin = parseBool(v)
	}

	cfg.Defaults = DefaultSystemSettings(cfg.Server.Port, cfg.TLS.Enable)
	if v, ok := os.LookupEnv("MAILSERVER_PUBLIC_BASE_URL"); ok && strings.TrimSpace(v) != "" {
		cfg.Defaults.PublicBaseURL = strings.TrimRight(strings.TrimSpace(v), "/")
	}

	return cfg, nil
}

// EnsureMasterKey loads the 32-byte master key from env/config or <dataDir>/master.key,
// generating and persisting one with 0600 permissions on first boot.
func EnsureMasterKey(dataDir, configuredKey string) ([]byte, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	if strings.TrimSpace(configuredKey) != "" {
		return decodeKeyMaterial(configuredKey)
	}

	keyPath := filepath.Join(dataDir, "master.key")
	raw, err := os.ReadFile(keyPath)
	if err == nil {
		key, decErr := decodeKeyMaterial(string(raw))
		if decErr != nil {
			return nil, fmt.Errorf("decode %s: %w", keyPath, decErr)
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", keyPath, err)
	}

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	encoded := hex.EncodeToString(key) + "\n"
	if err := os.WriteFile(keyPath, []byte(encoded), 0o600); err != nil {
		return nil, fmt.Errorf("persist master key: %w", err)
	}
	return key, nil
}

func decodeKeyMaterial(s string) ([]byte, error) {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 64 {
		b, err := hex.DecodeString(trimmed)
		if err == nil && len(b) == 32 {
			return b, nil
		}
	}
	if b, err := base64.StdEncoding.DecodeString(trimmed); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.RawStdEncoding.DecodeString(trimmed); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, errors.New("master key must be 32 bytes (64 hex chars or base64)")
}

func parseCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseBool(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}

// NormalizeBasePath normalizes a reverse-proxy subpath prefix (e.g. "/api/mail/" -> "/api/mail", "/" -> "").
// Any unsafe characters are rejected by returning "".
func NormalizeBasePath(s string) string {
	trimmed := strings.TrimSpace(s)
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" || trimmed == "/" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	for _, r := range trimmed {
		valid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '/' || r == '-' || r == '_' || r == '.' || r == '~'
		if !valid {
			return ""
		}
	}
	if strings.Contains(trimmed, "..") {
		return ""
	}
	for strings.Contains(trimmed, "//") {
		trimmed = strings.ReplaceAll(trimmed, "//", "/")
	}
	return strings.TrimRight(trimmed, "/")
}
