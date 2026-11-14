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
