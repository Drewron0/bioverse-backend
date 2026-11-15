package config

import (
	"os"
	"testing"
	"time"
)

func TestConfigLoadDefaults(t *testing.T) {
	_ = os.Setenv("PORT", "9090")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test?sslmode=disable")
	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("DATABASE_URL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error loading config, got %v", err)
	}

	if cfg.App.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.App.Port)
	}

	if cfg.App.Name != "BioVerse" {
		t.Errorf("expected app name BioVerse, got %s", cfg.App.Name)
	}

	if cfg.Auth.AccessExpiry != 15*time.Minute {
		t.Errorf("expected access expiry 15m, got %v", cfg.Auth.AccessExpiry)
	}
}

func TestConfigProductionRejectsDefaultSecrets(t *testing.T) {
	_ = os.Setenv("APP_ENV", "production")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test?sslmode=disable")
	_ = os.Setenv("JWT_SECRET", "super_secret_key_bioverse_2026_change_in_production")
	defer func() {
		_ = os.Unsetenv("APP_ENV")
		_ = os.Unsetenv("DATABASE_URL")
		_ = os.Unsetenv("JWT_SECRET")
	}()

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when loading default secret in production, got nil")
	}
}

func TestConfigProductionRejectsMissingCVServiceURL(t *testing.T) {
	_ = os.Setenv("APP_ENV", "production")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test?sslmode=disable")
	_ = os.Setenv("JWT_SECRET", "super_secret_key_bioverse_2026_valid_prod_key_123456789")
	_ = os.Setenv("APP_ENCRYPTION_KEY", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = os.Setenv("HMAC_SECRET", "non_default_hmac_secret_for_testing_123456")
	_ = os.Setenv("XENDIT_SECRET_KEY", "xnd_production_secret_valid_key")
	_ = os.Setenv("XENDIT_WEBHOOK_TOKEN", "valid_webhook_token_for_prod")
	_ = os.Setenv("CV_SERVICE_URL", "")
	defer func() {
		_ = os.Unsetenv("APP_ENV")
		_ = os.Unsetenv("DATABASE_URL")
		_ = os.Unsetenv("JWT_SECRET")
		_ = os.Unsetenv("APP_ENCRYPTION_KEY")
		_ = os.Unsetenv("HMAC_SECRET")
		_ = os.Unsetenv("XENDIT_SECRET_KEY")
		_ = os.Unsetenv("XENDIT_WEBHOOK_TOKEN")
		_ = os.Unsetenv("CV_SERVICE_URL")
	}()

	_, err := Load()
	if err == nil {
		t.Fatalf("expected error when CV_SERVICE_URL is missing in production, got nil")
	}
}

func TestConfigStagingPermitsMissingCVServiceURL(t *testing.T) {
	_ = os.Setenv("APP_ENV", "staging")
	_ = os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test?sslmode=disable")
	_ = os.Setenv("JWT_SECRET", "super_secret_key_bioverse_2026_valid_prod_key_123456789")
	_ = os.Setenv("APP_ENCRYPTION_KEY", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = os.Setenv("HMAC_SECRET", "non_default_hmac_secret_for_testing_123456")
	_ = os.Setenv("XENDIT_SECRET_KEY", "xnd_production_secret_valid_key")
	_ = os.Setenv("XENDIT_WEBHOOK_TOKEN", "valid_webhook_token_for_prod")
	_ = os.Setenv("CV_SERVICE_URL", "")
	_ = os.Setenv("ALLOW_SIMULATE_PAID", "true")
	defer func() {
		_ = os.Unsetenv("APP_ENV")
		_ = os.Unsetenv("DATABASE_URL")
		_ = os.Unsetenv("JWT_SECRET")
		_ = os.Unsetenv("APP_ENCRYPTION_KEY")
		_ = os.Unsetenv("HMAC_SECRET")
		_ = os.Unsetenv("XENDIT_SECRET_KEY")
		_ = os.Unsetenv("XENDIT_WEBHOOK_TOKEN")
		_ = os.Unsetenv("CV_SERVICE_URL")
		_ = os.Unsetenv("ALLOW_SIMULATE_PAID")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected staging to boot cleanly without CV_SERVICE_URL, got %v", err)
	}

	if cfg.Payment.AllowSimulatePaid {
		t.Errorf("expected AllowSimulatePaid to be forced false in staging, got true")
	}
}
