package migration

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sunliang711/mailserver/internal/config"
	"github.com/sunliang711/mailserver/internal/crypto"
	"github.com/sunliang711/mailserver/internal/db"
)

func TestExportInspectImportAcrossInstances(t *testing.T) {
	ctx := context.Background()

	// Instance A
	dirA := t.TempDir()
	keyA, _ := config.EnsureMasterKey(dirA, "")
	cipherA, _ := crypto.NewCipher(keyA)
	storeA, err := db.Open(filepath.Join(dirA, "a.db"), dirA, config.DefaultSystemSettings(3000, false))
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()

	if err := storeA.SaveSMTPConfig(ctx, cipherA, db.SMTPConfig{
		Host:     "smtp.example.com",
		Port:     465,
		User:     "noreply@example.com",
		Password: "smtp-secret-password",
	}, "admin"); err != nil {
		t.Fatal(err)
	}

	plainKey, prefix, hash, _ := crypto.GenerateAPIKey()
	encKey, _ := cipherA.EncryptString(plainKey)
	if err := storeA.CreateClient(ctx, &db.Client{
		ID:           crypto.NewULID(),
		Name:         "billing-service",
		KeyPrefix:    prefix,
		KeyHash:      hash,
		KeyEncrypted: encKey,
		Enabled:      true,
		RateLimitRPM: 60,
	}); err != nil {
		t.Fatal(err)
	}

	if err := storeA.InsertMailLog(ctx, &db.MailLog{
		ID:         crypto.NewULID(),
		ClientName: "billing-service",
		Source:     "api",
		Recipient:  "user@example.com",
		Subject:    "Invoice",
		Status:     "succeeded",
		DurationMs: 42,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	svcA := NewService(storeA, cipherA)
	bundle, err := svcA.Export(ctx, ExportOptions{
		IncludeMailLogs:  true,
		IncludeAdmins:    true,
		IncludeAuditLogs: true,
		Passphrase:       "migrate-pass-2026",
	})
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	// Instance B with distinct master key
	dirB := t.TempDir()
	keyB, _ := config.EnsureMasterKey(dirB, "")
	cipherB, _ := crypto.NewCipher(keyB)
	storeB, err := db.Open(filepath.Join(dirB, "b.db"), dirB, config.DefaultSystemSettings(3000, false))
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()

	svcB := NewService(storeB, cipherB)
	summary, err := svcB.Inspect(ctx, bundle, "migrate-pass-2026")
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if summary.Counts.Clients != 1 || summary.Counts.MailLogs != 1 {
		t.Fatalf("unexpected summary counts: %+v", summary.Counts)
	}

	counts, err := svcB.Import(ctx, bundle, ImportOptions{
		Passphrase:       "migrate-pass-2026",
		Mode:             "replace",
		ConflictStrategy: "overwrite",
		IncludeMailLogs:  true,
		IncludeAdmins:    true,
		IncludeAuditLogs: true,
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if counts.Clients != 1 || counts.MailLogs != 1 {
		t.Fatalf("unexpected import counts: %+v", counts)
	}

	smtpB, err := storeB.GetSMTPConfig(ctx, cipherB)
	if err != nil || smtpB.Password != "smtp-secret-password" {
		t.Fatalf("re-encrypted SMTP password mismatch: %q, err=%v", smtpB.Password, err)
	}

	clB, err := storeB.GetClientByKeyHash(ctx, crypto.HashAPIKey(plainKey))
	if err != nil || clB.Name != "billing-service" {
		t.Fatalf("imported client lookup failed: %+v, err=%v", clB, err)
	}
	decKeyB, err := cipherB.DecryptString(clB.KeyEncrypted)
	if err != nil || decKeyB != plainKey {
		t.Fatalf("re-encrypted client key mismatch: %q, err=%v", decKeyB, err)
	}
}
