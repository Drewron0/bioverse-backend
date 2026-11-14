package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/config"
	"github.com/Drewron01/bioverse-backend/internal/platform/database"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/Drewron01/bioverse-backend/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	var handler slog.Handler
	if cfg.App.Environment == "production" || cfg.App.Environment == "staging" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	appLogger := slog.New(handler)
	slog.SetDefault(appLogger)

	appLogger.Info("starting BioVerse background worker",
		slog.String("app", cfg.App.Name),
		slog.String("env", cfg.App.Environment),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		appLogger.Error("worker database pool connection failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	queries := store.New(pool)
	jobRunner := worker.NewJobRunner(pool, queries, appLogger)

	// Background ticker for expired checkouts cleanup (every 1 minute)
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				jobCtx, jobCancel := context.WithTimeout(context.Background(), 30*time.Second)
				_, _ = jobRunner.ProcessExpiredDonations(jobCtx, 50)
				jobCancel()
			}
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	appLogger.Info("worker engine running, active schedules: expired checkouts (1m)")

	<-shutdown
	appLogger.Info("shutting down worker gracefully...")
	cancel()

	appLogger.Info("worker stopped successfully")
}
