package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
)

func TestMigrationSchemaIntegrity(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Skipf("skipping migration test: local postgres not available: %v", err)
		return
	}
	defer pool.Close()

	// Verify required tables exist in public schema
	requiredTables := []string{
		"users",
		"platform_admins",
		"user_tokens",
		"admin_tokens",
		"currencies",
		"digital_products",
		"companies",
		"company_members",
		"b2b_contracts",
		"conservation_zones",
		"conservation_projects",
		"spider_frames",
		"coral_fragments",
		"field_operators",
		"virtual_pets",
		"eco_missions",
		"user_mission_submissions",
		"point_ledgers",
		"donations",
		"operator_payouts",
		"esg_audit_reports",
		"app_settings",
		"webhook_events",
		"news_articles",
	}

	for _, table := range requiredTables {
		var exists bool
		query := `SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' AND table_name = $1
		);`
		if err := pool.QueryRow(ctx, query, table).Scan(&exists); err != nil {
			t.Fatalf("failed to query table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist in database schema", table)
		}
	}

	// Verify app_settings seeded values
	seededKeys := []string{
		"points_per_usd",
		"coral_fragment_price_usd",
		"bio_shield_addon_price_usd",
		"max_fragments_per_spider_frame",
	}

	for _, key := range seededKeys {
		var count int
		query := `SELECT COUNT(*) FROM app_settings WHERE setting_key = $1;`
		if err := pool.QueryRow(ctx, query, key).Scan(&count); err != nil {
			t.Fatalf("failed to query app_settings key %s: %v", key, err)
		}
		if count == 0 {
			t.Errorf("expected seeded app_settings key %s to exist", key)
		}
	}
}
