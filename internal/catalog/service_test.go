package catalog_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/catalog"
	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
)

func TestCatalogAndInventoryEndToEnd(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	queries := store.New(pool)
	catalogSvc := catalog.NewService(pool, queries)
	identitySvc := identity.NewService(pool, queries, cfg, nil, nil)

	// 1. Register test user
	uniqueSuffix := time.Now().UnixNano()
	authRes, err := identitySvc.Register(ctx, identity.RegisterInput{
		FullName: "Inventory Tester",
		Email:    fmt.Sprintf("shopper_%d@bioverse.id", uniqueSuffix),
		Password: "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	userID := authRes.User.ID

	// 2. Fetch seeded Streak Freeze product
	prod, err := queries.GetDigitalProductBySKU(ctx, store.GetDigitalProductBySKUParams{
		SkuCode:      "STREAK_FREEZE_1X",
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("failed to query streak freeze: %v", err)
	}

	// 3. Invariant Test: User with 0 points cannot purchase product (ErrInsufficientPoints)
	_, err = catalogSvc.BuyProductWithPoints(ctx, userID, prod.ID, 1)
	if err == nil {
		t.Fatalf("expected ErrInsufficientPoints, got nil")
	}

	// 4. Invariant Test: User cannot consume streak freeze when inventory is 0 (ErrItemNotInInventory)
	err = catalogSvc.ConsumeStreakFreeze(ctx, userID)
	if err == nil {
		t.Fatalf("expected ErrItemNotInInventory, got nil")
	}

	// 5. Credit points to user to allow purchase (e.g. 500 points)
	_, err = queries.CreditUserPoints(ctx, store.CreditUserPointsParams{
		ID:            userID,
		PointsBalance: 500,
	})
	if err != nil {
		t.Fatalf("failed to credit points: %v", err)
	}

	// 6. Buy Streak Freeze product with points
	inv, err := catalogSvc.BuyProductWithPoints(ctx, userID, prod.ID, 1)
	if err != nil {
		t.Fatalf("failed to purchase product: %v", err)
	}
	if inv.Quantity != 1 {
		t.Errorf("expected quantity 1, got %d", inv.Quantity)
	}

	// 7. Verify points balance deducted (500 - 150 = 350)
	user, err := queries.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.PointsBalance != 350 {
		t.Errorf("expected balance 350, got %d", user.PointsBalance)
	}

	// 8. Consume Streak Freeze
	err = catalogSvc.ConsumeStreakFreeze(ctx, userID)
	if err != nil {
		t.Fatalf("failed to consume streak freeze: %v", err)
	}

	// 9. Verify inventory is now 0
	items, err := catalogSvc.ListUserInventory(ctx, userID)
	if err != nil {
		t.Fatalf("failed to list inventory: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 active items after consumption, got %d", len(items))
	}

	// 10. Subscribe to BioVerse Plus Monthly plan
	plans, err := catalogSvc.ListSubscriptionPlans(ctx, "USD")
	if err != nil || len(plans) == 0 {
		t.Fatalf("expected plans in catalog, got %v", err)
	}

	sub, err := catalogSvc.SubscribePlan(ctx, userID, plans[0].ID, store.PaymentMethodTypeEwallet)
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	if sub.Status != store.SubscriptionStatusActive {
		t.Errorf("expected active subscription, got %v", sub.Status)
	}

	subs, err := catalogSvc.ListUserSubscriptions(ctx, userID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("expected 1 user subscription, got %d", len(subs))
	}

	t.Logf("Catalog and inventory invariants verified for user %s", userID)
}
