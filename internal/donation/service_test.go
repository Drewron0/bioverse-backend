package donation

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/conservation"
	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM pet_boosts WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM virtual_pets WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM point_ledgers WHERE user_id = $1", userID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM webhook_events WHERE provider = 'xendit' AND (external_event_id LIKE 'evt_%' OR payload::text LIKE $1)", "%"+userID.String()+"%")
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

	// 8. Process Xendit webhook for the Bio-Shield adoption with "donation_" prefix
	webhookExtID := fmt.Sprintf("donation_%s", bioShieldRes.DonationID)
	err = donSvc.ProcessXenditInvoiceWebhook(ctx, "evt_"+uuid.New().String(), webhookExtID, "PAID", bioShieldRes.SettlementAmount, []byte(`{"status":"PAID"}`))
	if err != nil {
		t.Fatalf("failed to process webhook for bio-shield adoption: %v", err)
	}
	updatedBioDonation, err := donSvc.GetDonation(ctx, bioShieldRes.DonationID)
	if err != nil || updatedBioDonation.PaymentStatus != store.PaymentStatusPaid {
		t.Fatalf("expected donation to be marked paid, got %v", updatedBioDonation.PaymentStatus)
	}

	// 9. Standalone Bio-Shield purchase ($1.80) does NOT allocate corals but grants pet boost
	petID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO virtual_pets (
			id, user_id, pet_name, pet_species, coral_type, experience_points, health_percent,
			level, growth_stage, is_primary, last_interacted_at
		) VALUES (
			$1, $2, 'Shelly', 'turtle', 'Acropora', 0, 100, 1, 'hatchling', true, NOW()
		)
	`, petID, userID)
	if err != nil {
		t.Fatalf("failed to create virtual pet for test: %v", err)
	}

	coralsBeforeStandalone, _ := consSvc.ListUserAdoptedCorals(ctx, userID)
	standaloneRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey:   uuid.New().String(),
		UserID:           userID,
		DonationType:     store.DonationTypeBioShieldAddon,
		ProjectID:        &testProjectID,
		UnitQuantity:     1,
		TargetPetID:      &petID,
		PaymentMethod:    store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("expected standalone Bio-Shield checkout to succeed, got %v", err)
	}
	if standaloneRes.GrossProjectValue.String() != "1.8" && standaloneRes.GrossProjectValue.String() != "1.80" {
		t.Errorf("expected gross value 1.80, got %s", standaloneRes.GrossProjectValue.String())
	}

	// Process webhook with bare UUID (testing prefix resilience)
	err = donSvc.ProcessXenditInvoiceWebhook(ctx, "evt_"+uuid.New().String(), standaloneRes.DonationID.String(), "PAID", standaloneRes.SettlementAmount, []byte(`{"status":"PAID"}`))
	if err != nil {
		t.Fatalf("failed to process webhook with bare UUID: %v", err)
	}

	// Verify coral count did NOT increase
	coralsAfterStandalone, _ := consSvc.ListUserAdoptedCorals(ctx, userID)
	if len(coralsAfterStandalone) != len(coralsBeforeStandalone) {
		t.Errorf("expected coral count %d to remain unchanged after standalone bio-shield, got %d",
			len(coralsBeforeStandalone), len(coralsAfterStandalone))
	}

	// Verify pet boost was created
	var boostCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM pet_boosts WHERE user_id = $1 AND pet_id = $2", userID, petID).Scan(&boostCount)
	if err != nil || boostCount == 0 {
		t.Errorf("expected pet boost to be created, got count %d, err %v", boostCount, err)
	}

	// 10. Points-only donation: 100% points covers entire payment -> completes synchronously
	_, err = pool.Exec(ctx, "UPDATE users SET points_balance = 500 WHERE id = $1", userID)
	if err != nil {
		t.Fatalf("failed to credit points: %v", err)
	}

	pointsDonationKey := uuid.New().String()
	pointsRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey: pointsDonationKey,
		UserID:         userID,
		ProjectID:      &testProjectID,
		UnitQuantity:   1,
		CoralNickname:  "Points Coral",
		PointsToUse:    320, // 320 points = $3.20 covers 100%
		PaymentMethod:  store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("expected points-only checkout to succeed, got %v", err)
	}
	if pointsRes.PaymentStatus != store.PaymentStatusPaid {
		t.Errorf("expected points-only donation to complete as paid immediately, got %s", pointsRes.PaymentStatus)
	}
	if !pointsRes.MoneyAmountPaid.IsZero() {
		t.Errorf("expected money amount paid to be 0, got %s", pointsRes.MoneyAmountPaid.String())
	}

	// Verify points balance was debited immediately (500 - 320 = 180)
	var remainingPoints int64
	err = pool.QueryRow(ctx, "SELECT points_balance FROM users WHERE id = $1", userID).Scan(&remainingPoints)
	if err != nil || remainingPoints != 180 {
		t.Errorf("expected points balance 180, got %d (err: %v)", remainingPoints, err)
	}

	// 11. Idempotency test: replaying same idempotency key with different parameters returns ErrIdempotencyKeyReused
	_, err = donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey: pointsDonationKey,
		UserID:         userID,
		ProjectID:      &testProjectID,
		UnitQuantity:   2, // Different quantity
		PaymentMethod:  store.PaymentMethodTypeQris,
	})
	if err != ErrIdempotencyKeyReused {
		t.Errorf("expected ErrIdempotencyKeyReused, got %v", err)
	}

	// 12. Webhook replay idempotency test: replaying identical webhook event must succeed idempotently without error
	pendingKey := uuid.New().String()
	pendingRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey: pendingKey,
		UserID:         userID,
		ProjectID:      &testProjectID,
		UnitQuantity:   1,
		PaymentMethod:  store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("checkout pending donation failed: %v", err)
	}

	replayWebhookEventID := "evt_double_" + uuid.New().String()
	replayWebhookExtID := "donation_" + pendingRes.DonationID.String()

	// Initial webhook delivery
	err = donSvc.ProcessXenditInvoiceWebhook(ctx, replayWebhookEventID, replayWebhookExtID, "PAID", pendingRes.SettlementAmount, []byte(`{"status":"PAID"}`))
	if err != nil {
		t.Fatalf("initial webhook delivery failed: %v", err)
	}

	// Replay delivery of the EXACT same event (triggers ON CONFLICT DO NOTHING)
	err = donSvc.ProcessXenditInvoiceWebhook(ctx, replayWebhookEventID, replayWebhookExtID, "PAID", pendingRes.SettlementAmount, []byte(`{"status":"PAID"}`))
	if err != nil {
		t.Fatalf("replaying identical webhook delivery failed (expected idempotent success): %v", err)
	}

	// 13. Audit trail persistence test: failed webhook (e.g. nominal mismatch) must rollback donation tx but preserve webhook_events record with status 'failed'
	failedDonationKey := uuid.New().String()
	failedDonRes, err := donSvc.Checkout(ctx, CheckoutInput{
		IdempotencyKey: failedDonationKey,
		UserID:         userID,
		ProjectID:      &testProjectID,
		UnitQuantity:   1,
		PaymentMethod:  store.PaymentMethodTypeQris,
	})
	if err != nil {
		t.Fatalf("checkout failed for audit test donation: %v", err)
	}

	failedEventID := "evt_mismatch_" + uuid.New().String()
	wrongAmount := failedDonRes.SettlementAmount.Add(decimal.NewFromInt(50000)) // mismatched amount
	err = donSvc.ProcessXenditInvoiceWebhook(ctx, failedEventID, "donation_"+failedDonRes.DonationID.String(), "PAID", wrongAmount, []byte(`{"status":"PAID","amount":999999}`))
	if err == nil {
		t.Fatalf("expected error on nominal mismatch, got nil")
	}

	// Verify donation remains pending (business transaction rolled back)
	var donStatus store.PaymentStatus
	err = pool.QueryRow(ctx, "SELECT payment_status FROM donations WHERE id = $1", failedDonRes.DonationID).Scan(&donStatus)
	if err != nil || donStatus != store.PaymentStatusPending {
		t.Fatalf("expected donation to remain pending, got %v (err: %v)", donStatus, err)
	}

	// Verify webhook event audit record is PRESERVED with status 'failed' (audit not rolled back)
	var eventStatus store.WebhookProcessStatus
	var errMsg *string
	err = pool.QueryRow(ctx, "SELECT status, error_message FROM webhook_events WHERE provider = 'xendit' AND external_event_id = $1", failedEventID).Scan(&eventStatus, &errMsg)
	if err != nil {
		t.Fatalf("failed to query webhook_events audit log: %v", err)
	}
	if eventStatus != store.WebhookProcessStatusFailed {
		t.Errorf("expected webhook event status 'failed', got '%s'", eventStatus)
	}
	if errMsg == nil || !strings.Contains(*errMsg, "Settlement amount mismatch") {
		t.Errorf("expected error message to contain 'Settlement amount mismatch', got %v", errMsg)
	}

	// 14. Terminal status test: late webhook on expired donation must be rejected and must not mark donation paid
	_, err = pool.Exec(ctx, "UPDATE donations SET payment_status = 'expired', is_points_locked = false WHERE id = $1", failedDonRes.DonationID)
	if err != nil {
		t.Fatalf("failed to set donation to expired: %v", err)
	}

	lateEventID := "evt_late_" + uuid.New().String()
	err = donSvc.ProcessXenditInvoiceWebhook(ctx, lateEventID, "donation_"+failedDonRes.DonationID.String(), "PAID", failedDonRes.SettlementAmount, []byte(`{"status":"PAID"}`))
	if err == nil {
		t.Fatalf("expected error processing late webhook on expired donation, got nil")
	}

	var statusAfterLate store.PaymentStatus
	_ = pool.QueryRow(ctx, "SELECT payment_status FROM donations WHERE id = $1", failedDonRes.DonationID).Scan(&statusAfterLate)
	if statusAfterLate != store.PaymentStatusExpired {
		t.Errorf("expected donation to remain expired after late webhook, got %s", statusAfterLate)
	}
}
