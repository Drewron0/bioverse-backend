package donation

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/conservation"
	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
)

func TestDonationAndCoralAdoptionEndToEnd(t *testing.T) {
	cfg, err := config.Load()
	if err != nil {
		t.Skip("skipping test: config unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, cfg.Database)
	if err != nil {
		t.Skipf("skipping test: database unavailable: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
	})

	queries := store.New(pool)
	identitySvc := identity.NewService(pool, queries, cfg, nil, nil)
	consSvc := conservation.NewService(pool, queries)
	donSvc := NewService(pool, queries, consSvc, nil, nil, nil)

	// 1. Create a fresh test user
	uniqueSuffix := time.Now().UnixNano()
	regResult, err := identitySvc.Register(ctx, identity.RegisterInput{
		FullName: "Donation Tester",
		Email:    fmt.Sprintf("donor_%d@bioverse.id", uniqueSuffix),
		Password: "Password1234!",
		Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("failed to register test user: %v", err)
	}
	userID := regResult.User.ID

	// 2. Setup isolated test conservation zone, project, and spider frame with fragments_planted = 0.
	// This ensures the test is completely decoupled from seeded frame capacity and leaves no persistent artifacts.
	testZoneID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO conservation_zones (
			id, name, region, province, ecosystem_type, allocation_type, latitude, longitude
		) VALUES (
			$1, $2, 'Bali Barat', 'Bali', 'marine_coral', 'b2c_pool', -8.1436100, 114.6534200
		)
	`, testZoneID, fmt.Sprintf("Test Sanctuary Zone %d", uniqueSuffix))
	if err != nil {
		t.Fatalf("failed to create test conservation zone: %v", err)
	}

	testProjectID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO conservation_projects (
			id, zone_id, title, description, objectives, category, species_or_type_name,
			image_url, status, unit_name, currency_code, price_per_unit_money, price_per_unit_points,
			target_amount, target_units, donation_start_date, donation_end_date, estimated_planting_date
		) VALUES (
			$1, $2, $3, 'Isolated test coral project', 'Restore test reef structure', 'coral',
			'Acropora millepora', 'https://assets.bioverse.id/projects/test.jpg', 'open_for_donation',
			'fragment', 'USD', 3.20, 320, 3200.00, 1000, CURRENT_DATE, CURRENT_DATE + INTERVAL '90 days', CURRENT_DATE + INTERVAL '120 days'
		)
	`, testProjectID, testZoneID, fmt.Sprintf("Test Coral Project %d", uniqueSuffix))
	if err != nil {
		t.Fatalf("failed to create test conservation project: %v", err)
	}

	testFrameID, _ := uuid.NewV7()
	testFrameCode := fmt.Sprintf("TEST-SF-%d", uniqueSuffix)
	_, err = pool.Exec(ctx, `
		INSERT INTO spider_frames (
			id, frame_code, zone_id, depth_meters, latitude, longitude,
			max_capacity_fragments, fragments_planted, physical_health_status, is_active
		) VALUES (
			$1, $2, $3, 5.50, -8.1436100, 114.6534200, 15, 0, 'healthy', true
		)
	`, testFrameID, testFrameCode, testZoneID)
	if err != nil {
		t.Fatalf("failed to create test spider frame: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelCleanup()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM coral_fragments WHERE spider_frame_id = $1 OR adopted_by_user_id = $2", testFrameID, userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM donations WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM spider_frames WHERE id = $1", testFrameID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM conservation_projects WHERE id = $1", testProjectID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM conservation_zones WHERE id = $1", testZoneID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM user_tokens WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
	})

	// 3. Test Coral-First Gating: Bio-Shield must be rejected because user owns 0 active corals
	_, err = donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   uuid.New().String(),
		UserID:           userID,
		ProjectID:        &testProjectID,
		UnitQuantity:     1,
		IncludeBioShield: true,
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != ErrCoralFirstGatingFailed {
		t.Fatalf("expected ErrCoralFirstGatingFailed, got %v", err)
	}

	// 4. Checkout coral adoption (1 fragment at $3.20)
	idempotencyKey := uuid.New().String()
	checkoutRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   idempotencyKey,
		UserID:           userID,
		ProjectID:        &testProjectID,
		UnitQuantity:     1,
		IncludeBioShield: false,
		CoralNickname:    "My First Baby Coral",
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	if checkoutRes.GrossProjectValue.String() != "3.2" && checkoutRes.GrossProjectValue.String() != "3.20" {
		t.Errorf("expected gross project value 3.20, got %s", checkoutRes.GrossProjectValue.String())
	}
	if checkoutRes.QRISPayload == "" {
		t.Error("expected non-empty QRIS payload for QRIS checkout")
	}

	// 5. Process payment simulation webhook
	paidDonation, err := donSvc.ProcessPaidDonation(ctx, checkoutRes.DonationID, nil, "My First Baby Coral")
	if err != nil {
		t.Fatalf("failed to process paid donation: %v", err)
	}
	if paidDonation.PaymentStatus != store.PaymentStatusPaid {
		t.Errorf("expected payment status paid, got %s", paidDonation.PaymentStatus)
	}

	// 6. Verify user now has an active adopted coral fragment
	corals, err := consSvc.ListUserAdoptedCorals(ctx, userID)
	if err != nil || len(corals) == 0 {
		t.Fatalf("expected adopted coral fragment, got %v", err)
	}
	if corals[0].Nickname.String != "My First Baby Coral" {
		t.Errorf("expected nickname 'My First Baby Coral', got %s", corals[0].Nickname.String)
	}

	// 7. Verify Coral-First Gating now passes for Bio-Shield add-on!
	bioShieldRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   uuid.New().String(),
		UserID:           userID,
		ProjectID:        &testProjectID,
		UnitQuantity:     1,
		IncludeBioShield: true,
		CoralNickname:    "Protected Coral",
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("expected Bio-Shield checkout to succeed after adopting coral, got %v", err)
	}

	// Total should be $3.20 + $1.80 = $5.00
	if bioShieldRes.GrossProjectValue.String() != "5" && bioShieldRes.GrossProjectValue.String() != "5.00" {
		t.Errorf("expected gross value 5.00, got %s", bioShieldRes.GrossProjectValue.String())
	}
}
