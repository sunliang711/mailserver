package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `server:
  port: 4000
  data_dir: /tmp/from-yaml
tls:
  enable: false
  cert: yaml.pem
  key: yaml.key
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("MAILSERVER_PORT", "5050")
	t.Setenv("MAILSERVER_TLS_ENABLE", "true")
	t.Setenv("MAILSERVER_TLS_CERT", "env.pem")

	cfg, err := Load(cfgPath, map[string]string{"port": "6060"})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Server.Port != 6060 {
		t.Fatalf("port = %d, want 6060 (CLI override)", cfg.Server.Port)
	}
	if cfg.Server.DataDir != "/tmp/from-yaml" {
		t.Fatalf("data_dir = %q, want /tmp/from-yaml", cfg.Server.DataDir)
	}
	if !cfg.TLS.Enable || cfg.TLS.Cert != "env.pem" || cfg.TLS.Key != "yaml.key" {
		t.Fatalf("unexpected tls config: %+v", cfg.TLS)
	}
}

func TestEnsureMasterKey(t *testing.T) {
	dir := t.TempDir()
	k1, err := EnsureMasterKey(dir, "")
	if err != nil {
		t.Fatalf("EnsureMasterKey 1: %v", err)
	}
	if k1 == nil || len(k1) != 32 {
		t.Fatalf("key len = %d, want 32", len(k1))
	}

	k2, err := EnsureMasterKey(dir, "")
	if err != nil {
		t.Fatalf("EnsureMasterKey 2: %v", err)
	}
	if string(k1) != string(k2) {
		t.Fatal("reloaded master key does not match generated key")
	}
}

func TestNormalizeBasePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"/api/mail", "/api/mail"},
		{"/api/mail/", "/api/mail"},
		{"api/mail/", "/api/mail"},
		{"//api//mail///", "/api/mail"},
		{`"/bad<script>`, ""},
		{"/bad/../path", ""},
	}
	for _, tc := range cases {
		if got := NormalizeBasePath(tc.in); got != tc.want {
			t.Fatalf("NormalizeBasePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResetAdminPrecedence(t *testing.T) {
	t.Setenv("RESET_ADMIN", "true")
	cfg, err := Load("", nil)
	if err != nil {
		t.Fatalf("Load with RESET_ADMIN: %v", err)
	}
	if !cfg.Server.ResetAdmin {
		t.Fatal("expected ResetAdmin=true when RESET_ADMIN=true")
	}

	t.Setenv("MAILSERVER_RESET_ADMIN", "false")
	cfg, err = Load("", nil)
	if err != nil {
		t.Fatalf("Load with MAILSERVER_RESET_ADMIN override: %v", err)
	}
	if cfg.Server.ResetAdmin {
		t.Fatal("expected MAILSERVER_RESET_ADMIN=false to override RESET_ADMIN=true")
	}

	cfg, err = Load("", map[string]string{"reset-admin": "true"})
	if err != nil {
		t.Fatalf("Load with CLI override: %v", err)
	}
	if !cfg.Server.ResetAdmin {
		t.Fatal("expected CLI reset-admin=true override to win")
	}
}
