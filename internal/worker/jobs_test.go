package worker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/Drewron0/bioverse-backend/internal/worker"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

func TestExpiredDonationsCleanupWorker(t *testing.T) {
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
	identitySvc := identity.NewService(pool, queries, cfg, nil, nil)
	runner := worker.NewJobRunner(pool, queries, nil)

	// Fetch active seeded project by immutable UUID
	seededProjectID := uuid.MustParse("0193297a-96e0-7988-9271-e0e649ce4201")
	project, err := queries.GetConservationProjectByID(ctx, seededProjectID)
	if err != nil {
		t.Fatalf("failed to get seeded project: %v", err)
	}
	projectID := project.ID

	// 1. Create user and credit 200 points
	uniqueSuffix := time.Now().UnixNano()
	authRes, err := identitySvc.Register(ctx, identity.RegisterInput{
		FullName: "Worker Tester",
		Email:    fmt.Sprintf("worker_%d@bioverse.id", uniqueSuffix),
		Password: "Password123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	userID := authRes.User.ID

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM donations WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM user_tokens WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
	})

	_, err = queries.CreditUserPoints(ctx, store.CreditUserPointsParams{
		ID:            userID,
		PointsBalance: 200,
	})
	if err != nil {
		t.Fatalf("failed to credit points: %v", err)
	}

	// 2. Lock 100 points
	_, err = queries.LockUserPoints(ctx, store.LockUserPointsParams{
		ID:                  userID,
		LockedPointsBalance: 100,
	})
	if err != nil {
		t.Fatalf("failed to lock points: %v", err)
	}

	// 3. Insert an expired pending donation (expired 2 hours ago)
	donationID, _ := uuid.NewV7()
	pastExpiry := time.Now().UTC().Add(-2 * time.Hour)

	_, err = queries.CreateDonation(ctx, store.CreateDonationParams{
		ID:                       donationID,
		IdempotencyKey:           fmt.Sprintf("expired-test-%d", uniqueSuffix),
		UserID:                   userID,
		ProjectID:                projectID,
		DonationType:             store.DonationTypeCoralAdoption,
		UnitQuantity:             1,
		CurrencyCode:             "USD",
		GrossProjectValue:        decimalutil.ToNumeric(decimal.NewFromFloat(3.20)),
		MoneyAmountPaid:          decimalutil.ToNumeric(decimal.NewFromFloat(2.20)),
		SettlementCurrencyCode:   "IDR",
		SettlementAmount:         decimalutil.ToNumeric(decimal.NewFromInt(35200)),
		FxRateSnapshot:           decimalutil.ToNumeric(decimal.NewFromInt(16000)),
		GatewayFeeSettlement:     decimalutil.ToNumeric(decimal.Zero),
		MoneyPaymentMethod:       store.PaymentMethodTypeQris,
		PaymentProvider:          "xendit",
		IsMixedPayment:           true,
		PointsUsed:               100,
		PointsRateSnapshot:       decimalutil.ToNumeric(decimal.NewFromInt(100)),
		PointsMonetaryEquivalent: decimalutil.ToNumeric(decimal.NewFromInt(1)),
		IsPointsLocked:           true,
		PaymentStatus:            store.PaymentStatusPending,
		ExpiredAt:                pgtype.Timestamptz{Time: pastExpiry, Valid: true},
	})
	if err != nil {
		t.Fatalf("failed to insert expired donation: %v", err)
	}

	// 4. Run worker cleanup routine
	processed, err := runner.ProcessExpiredDonations(ctx, 50)
	if err != nil {
		t.Fatalf("failed to process expired donations: %v", err)
	}
	if processed < 1 {
		t.Errorf("expected at least 1 expired donation processed, got %d", processed)
	}

	// 5. Invariant Test: Verify donation is marked expired
	d, err := queries.GetDonationByID(ctx, donationID)
	if err != nil {
		t.Fatalf("failed to get donation: %v", err)
	}
	if d.PaymentStatus != store.PaymentStatusExpired {
		t.Errorf("expected status expired, got %v", d.PaymentStatus)
	}
	if d.IsPointsLocked {
		t.Errorf("expected is_points_locked to be false")
	}

	// 6. Invariant Test: Verify user locked points were completely unlocked (0)
	user, err := queries.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.LockedPointsBalance != 0 {
		t.Errorf("expected locked points 0, got %d", user.LockedPointsBalance)
	}
	if user.PointsBalance != 200 {
		t.Errorf("expected total balance 200, got %d", user.PointsBalance)
	}

	t.Logf("Worker expired checkout cleanup and point unlocking verified for donation %s", donationID)
}

func TestDueSubscriptionsBillingWorker(t *testing.T) {
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
	identitySvc := identity.NewService(pool, queries, cfg, nil, nil)
	runner := worker.NewJobRunner(pool, queries, nil)

	uniqueSuffix := time.Now().UnixNano()
	userRes, err := identitySvc.Register(ctx, identity.RegisterInput{
		FullName:              fmt.Sprintf("Worker Sub User %d", uniqueSuffix),
		Email:                 fmt.Sprintf("workersub_%d@bioverse.test", uniqueSuffix),
		Password:              "StrongP@ssw0rd!2026",
		Timezone:              "Asia/Jakarta",
		PreferredCurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	userID := userRes.User.ID

	subID, _ := uuid.NewV7()
	pastDate := pgtype.Date{
		Time:  time.Now().UTC().AddDate(0, 0, -2),
		Valid: true,
	}

	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCleanup()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM donation_subscriptions WHERE id = $1", subID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM user_tokens WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
	})

	_, err = queries.CreateDonationSubscription(ctx, store.CreateDonationSubscriptionParams{
		ID:              subID,
		UserID:          userID,
		PlanID:          pgtype.UUID{Valid: false},
		ProjectID:       pgtype.UUID{Valid: false},
		Frequency:       store.SubscriptionFrequencyMonthly,
		DurationMonths:  pgtype.Int4{Int32: 12, Valid: true},
		UnitQuantity:    1,
		CurrencyCode:    "USD",
		AmountPerCycle:  decimalutil.ToNumeric(decimal.NewFromInt(10)),
		PaymentMethod:   store.PaymentMethodTypeEwallet,
		PaymentProvider: "xendit",
		Status:          store.SubscriptionStatusActive,
		NextBillingDate: pastDate,
	})
	if err != nil {
		t.Fatalf("failed to create test subscription: %v", err)
	}

	processed, err := runner.ProcessDueSubscriptions(ctx, 10)
	if err != nil {
		t.Fatalf("failed to process due subscriptions: %v", err)
	}
	if processed < 1 {
		t.Errorf("expected at least 1 due subscription processed, got %d", processed)
	}

	// Verify cycle incremented
	var cycles int32
	var nextDate pgtype.Date
	err = pool.QueryRow(ctx, "SELECT completed_cycles, next_billing_date FROM donation_subscriptions WHERE id = $1", subID).Scan(&cycles, &nextDate)
	if err != nil {
		t.Fatalf("failed to query updated subscription: %v", err)
	}
	if cycles != 1 {
		t.Errorf("expected completed_cycles 1, got %d", cycles)
	}
	if !nextDate.Time.After(time.Now().UTC()) {
		t.Errorf("expected next billing date to be in future, got %v", nextDate.Time)
	}

	t.Logf("Worker due subscription billing verified for subscription %s (cycles: %d)", subID, cycles)
}
