package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Drewron01/bioverse-backend/internal/platform/config"
)

func TestHealthCheckEndpoints(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AllowedOrigins: []string{"*"},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := setupRouter(cfg, logger, nil)

	t.Run("GET /healthz returns 200 OK", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("GET /v1/health with nil db returns degraded status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/health", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503 for degraded database, got %d", rec.Code)
		}

		var resp healthResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode health response: %v", err)
		}

		if resp.Status != "degraded" {
			t.Errorf("expected status degraded, got %s", resp.Status)
		}
		if resp.Services["database"] != "disconnected" {
			t.Errorf("expected database disconnected, got %s", resp.Services["database"])
		}
	})
}
