package catalog_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/catalog"
	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
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
	catalogSvc := catalog.NewService(pool, queries, false)
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
	idempotencyKey := uuid.New().String()
	_, err = catalogSvc.BuyProductWithPoints(ctx, userID, prod.ID, 1, idempotencyKey)
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

	// 6. Buy Streak Freeze product with points (with idempotency key)
	buyRes, err := catalogSvc.BuyProductWithPoints(ctx, userID, prod.ID, 1, idempotencyKey)
	if err != nil {
		t.Fatalf("failed to purchase product: %v", err)
	}
	if buyRes.SkuCode != "STREAK_FREEZE_1X" {
		t.Errorf("expected sku STREAK_FREEZE_1X, got %s", buyRes.SkuCode)
	}
	if buyRes.RemainingPointsBalance != 350 {
		t.Errorf("expected remaining points 350, got %d", buyRes.RemainingPointsBalance)
	}

	// 6b. Test Idempotency: Re-submitting with the same Idempotency-Key must not deduct points again!
	idempotentRes, err := catalogSvc.BuyProductWithPoints(ctx, userID, prod.ID, 1, idempotencyKey)
	if err != nil {
		t.Fatalf("idempotent purchase failed: %v", err)
	}
	if idempotentRes.RemainingPointsBalance != 350 {
		t.Errorf("expected remaining points still 350 on retry, got %d", idempotentRes.RemainingPointsBalance)
	}

	// 7. Verify points balance deducted exactly once (500 - 150 = 350)
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

	// 10. Subscribe to BioVerse Plus Monthly plan with multi-currency verification
	plans, err := catalogSvc.ListSubscriptionPlans(ctx, "USD")
	if err != nil || len(plans) == 0 {
		t.Fatalf("expected plans in catalog, got %v", err)
	}
	if len(plans[0].Prices) == 0 {
		t.Errorf("expected multi-currency prices array in subscription plan")
	}

	// Fetch seeded project for adoption subscription
	seededProjectID := uuid.MustParse("0193297a-96e0-7988-9271-e0e649ce4201")
	sub, err := catalogSvc.SubscribePlan(ctx, catalog.CreateSubscriptionInput{
		UserID:            userID,
		PlanID:            plans[0].ID,
		ProjectID:         seededProjectID,
		UnitQuantity:      1,
		PaymentMethodType: store.PaymentMethodTypeEwallet,
	})
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	if sub.Status != store.SubscriptionStatusActive {
		t.Errorf("expected active subscription, got %v", sub.Status)
	}
	if sub.PlanName == "" {
		t.Errorf("expected plan name to be populated")
	}
	if sub.AuthorizationURL != nil {
		t.Errorf("expected authorization URL to be nil without payment gateway integration, got %v", *sub.AuthorizationURL)
	}

	// 10b. Verify production gating prevents unauthenticated subscriptions
	prodSvc := catalog.NewService(pool, queries, true)
	_, err = prodSvc.SubscribePlan(ctx, catalog.CreateSubscriptionInput{
		UserID:            userID,
		PlanID:            plans[0].ID,
		ProjectID:         seededProjectID,
		UnitQuantity:      1,
		PaymentMethodType: store.PaymentMethodTypeEwallet,
	})
	if !errors.Is(err, catalog.ErrSubscriptionNotAvailableInProduction) {
		t.Errorf("expected ErrSubscriptionNotAvailableInProduction in production, got %v", err)
	}

	// 11. Test Subscription State Machine: Pause -> Resume -> Cancel
	pausedSub, err := catalogSvc.PauseSubscription(ctx, userID, sub.ID, "Traveling this month")
	if err != nil {
		t.Fatalf("failed to pause subscription: %v", err)
	}
	if pausedSub.Status != store.SubscriptionStatusPaused {
		t.Errorf("expected paused status, got %v", pausedSub.Status)
	}

	// Pausing again when already paused must fail
	_, err = catalogSvc.PauseSubscription(ctx, userID, sub.ID, "Already paused")
	if !errors.Is(err, catalog.ErrInvalidSubscriptionState) {
		t.Errorf("expected ErrInvalidSubscriptionState when pausing already paused sub, got %v", err)
	}

	resumedSub, err := catalogSvc.ResumeSubscription(ctx, userID, sub.ID)
	if err != nil {
		t.Fatalf("failed to resume subscription: %v", err)
	}
	if resumedSub.Status != store.SubscriptionStatusActive {
		t.Errorf("expected active status after resume, got %v", resumedSub.Status)
	}

	cancelledSub, err := catalogSvc.CancelSubscription(ctx, userID, sub.ID, "No longer needed")
	if err != nil {
		t.Fatalf("failed to cancel subscription: %v", err)
	}
	if cancelledSub.Status != store.SubscriptionStatusCancelled {
		t.Errorf("expected cancelled status, got %v", cancelledSub.Status)
	}

	subs, err := catalogSvc.ListUserSubscriptions(ctx, userID)
	if err != nil || len(subs) != 1 {
		t.Fatalf("expected 1 user subscription, got %d", len(subs))
	}

	t.Logf("Catalog and inventory invariants verified for user %s", userID)
}
