package conservation_test

import (
	"context"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/conservation"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
)

func TestConservationService(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	queries := store.New(pool)
	svc := conservation.NewService(pool, queries)

	// 1. List Projects
	projects, err := svc.ListProjects(ctx)
	if err != nil {
		t.Fatalf("failed to list projects: %v", err)
	}
	if len(projects) == 0 {
		t.Fatalf("expected at least 1 seeded conservation project")
	}

	// 2. Get Project Detail
	proj, err := svc.GetProject(ctx, projects[0].ID)
	if err != nil {
		t.Fatalf("failed to get project by id: %v", err)
	}
	if proj.ID != projects[0].ID {
		t.Errorf("expected project ID %s, got %s", projects[0].ID, proj.ID)
	}

	// 3. Impact Summary
	summary, err := svc.GetImpactSummary(ctx)
	if err != nil {
		t.Fatalf("failed to get impact summary: %v", err)
	}
	if summary == nil {
		t.Fatalf("expected impact summary not nil")
	}

	// 4. List News Articles
	news, err := svc.ListNewsArticles(ctx, 5)
	if err != nil {
		t.Fatalf("failed to list news: %v", err)
	}
	t.Logf("Found %d news articles, project: %s", len(news), proj.Title)
}
