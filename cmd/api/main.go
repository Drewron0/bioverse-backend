package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/identity"
	"github.com/Drewron01/bioverse-backend/internal/platform/config"
	"github.com/Drewron01/bioverse-backend/internal/platform/database"
	"github.com/Drewron01/bioverse-backend/internal/platform/middleware"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

var startTime = time.Now()

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	appLogger := loggerInit(cfg)
	appLogger.Info("starting BioVerse API server",
		slog.String("app", cfg.App.Name),
		slog.String("env", cfg.App.Environment),
		slog.Int("port", cfg.App.Port),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var dbPool *pgxpool.Pool
	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		appLogger.Warn("database connection could not be established at startup, continuing in degraded state",
			slog.String("error", err.Error()),
		)
	} else {
		dbPool = pool
		defer dbPool.Close()
		appLogger.Info("database connection pool successfully established")
	}

	router := setupRouter(cfg, appLogger, dbPool)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		appLogger.Info("HTTP server listening", slog.String("addr", srv.Addr))
		serverErrors <- srv.ListenAndServe()
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			appLogger.Error("server error encountered", slog.String("error", err.Error()))
			os.Exit(1)
		}
	case sig := <-shutdown:
		appLogger.Info("shutdown signal received", slog.String("signal", sig.String()))

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("graceful shutdown failed", slog.String("error", err.Error()))
			_ = srv.Close()
		}
		appLogger.Info("server shutdown completed cleanly")
	}
}

func loggerInit(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.App.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.App.Environment == "production" || cfg.App.Environment == "staging" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	l := slog.New(handler)
	slog.SetDefault(l)
	return l
}

func setupRouter(cfg *config.Config, log *slog.Logger, db *pgxpool.Pool) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger(log))
	r.Use(middleware.Recoverer(log))
	r.Use(middleware.CORSConfig(cfg.App.AllowedOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/v1", func(v1 chi.Router) {
		v1.Get("/health", healthCheckHandler(db))
	})

	if db != nil {
		queries := store.New(db)
		identityService := identity.NewService(db, queries, cfg)
		identityHandler := identity.NewHandler(identityService)
		authMiddleware := identity.AuthMiddleware(cfg.Auth.JWTSecret)
		identityHandler.RegisterRoutes(r, authMiddleware)
	}

	return r
}

type healthResponse struct {
	Status    string            `json:"status"`
	App       string            `json:"app"`
	Uptime    string            `json:"uptime"`
	Timestamp string            `json:"timestamp"`
	Services  map[string]string `json:"services"`
}

func healthCheckHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		services := make(map[string]string)
		status := "ok"

		if db == nil {
			services["database"] = "disconnected"
			status = "degraded"
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()

			if err := db.Ping(ctx); err != nil {
				services["database"] = "unreachable"
				status = "degraded"
			} else {
				services["database"] = "connected"
			}
		}

		resp := healthResponse{
			Status:    status,
			App:       "BioVerse Backend",
			Uptime:    time.Since(startTime).Truncate(time.Second).String(),
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Services:  services,
		}

		w.Header().Set("Content-Type", "application/json")
		if status == "degraded" {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}
