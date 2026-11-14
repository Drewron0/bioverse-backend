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
		appLogger.Warn("database connection could not be established for worker, standing by",
			slog.String("error", err.Error()),
		)
	} else {
		defer pool.Close()
		appLogger.Info("worker database pool successfully connected")
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	appLogger.Info("worker engine initialized, waiting for job events")

	<-shutdown
	appLogger.Info("shutting down worker gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = shutdownCtx

	appLogger.Info("worker stopped successfully")
}
