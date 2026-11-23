package gamification_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/gamification"
	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
)

func TestGamificationEndToEnd(t *testing.T) {
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

	q := store.New(pool)
	identitySvc := identity.NewService(pool, q, cfg, nil, nil)
	gamifySvc := gamification.NewService(pool, q, nil)

	// 1. Register test user
	testEmail := fmt.Sprintf("pet_trainer_%d@bioverse.test", time.Now().UnixNano())
	authRes, err := identitySvc.Register(ctx, identity.RegisterInput{
		FullName: "Asep Sunandar",
		Email:    testEmail,
		Password: "SecurePassword123!",
	})
	if err != nil {
		t.Fatalf("failed to register user: %v", err)
	}
	userID := authRes.User.ID

	// 2. Adopt first virtual pet (Acep)
	petAcep, err := gamifySvc.AdoptPet(ctx, gamification.AdoptPetInput{
		UserID:      userID,
		PetName:     "Acep",
		PetSpecies:  "turtle",
		CoralType:   "Acropora",
		MakePrimary: true,
	})
	if err != nil {
		t.Fatalf("failed to adopt pet: %v", err)
	}
	if !petAcep.IsPrimary {
		t.Errorf("expected first pet to be primary, got %v", petAcep.IsPrimary)
	}
	if petAcep.Level != 1 {
		t.Errorf("expected level 1, got %d", petAcep.Level)
	}

	// 3. Interact with pet (Acep)
	updatedAcep, err := gamifySvc.InteractWithPet(ctx, userID, petAcep.ID)
	if err != nil {
		t.Fatalf("failed to interact with pet: %v", err)
	}
	if updatedAcep.ExperiencePoints <= 0 {
		t.Errorf("expected XP increase after interaction, got %d", updatedAcep.ExperiencePoints)
	}

	// 4. Create an eco-mission in database for testing
	missionID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO eco_missions (
			id, title, description, instructions, category, difficulty, points_reward, xp_reward, cv_target_label, cv_min_confidence, is_daily, is_active
		) VALUES (
			$1, 'Reusable Tumbler Snap', 'Snap a photo of your reusable water bottle to save plastic', 'Take a photo of your bottle', 'lifestyle', 'easy', 50, 25, 'reusable_tumbler', 0.80, true, true
		) ON CONFLICT (id) DO NOTHING
	`, missionID)
	if err != nil {
		t.Fatalf("failed to insert test eco mission: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM user_mission_submissions WHERE mission_id = $1", missionID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM eco_missions WHERE id = $1", missionID)
	}()

	// 5. Submit eco-mission proof
	subResult, err := gamifySvc.SubmitMissionProof(ctx, userID, missionID, &petAcep.ID, "https://storage.bioverse.id/proofs/tumbler123.jpg")
	if err != nil {
		t.Fatalf("failed to submit mission proof: %v", err)
	}
	if subResult.Status != store.SubmissionStatusVerified {
		t.Errorf("expected submission to be approved, got %v", subResult.Status)
	}
	if subResult.AwardedPoints != 50 {
		t.Errorf("expected 50 points awarded, got %d", subResult.AwardedPoints)
	}
	if subResult.CurrentStreak != 1 {
		t.Errorf("expected streak to advance to 1, got %d", subResult.CurrentStreak)
	}

	// 6. Verify user points balance updated
	user, err := q.GetUserByID(ctx, userID)
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if user.PointsBalance != 50 {
		t.Errorf("expected points balance 50, got %d", user.PointsBalance)
	}

	// 7. Verify leaderboard ranking and category filtering
	var coralProjectID uuid.UUID
	err = pool.QueryRow(ctx, "SELECT id FROM conservation_projects WHERE category = 'coral' LIMIT 1").Scan(&coralProjectID)
	if err != nil {
		coralProjectID, _ = uuid.NewV7()
		var zoneID uuid.UUID
		_ = pool.QueryRow(ctx, "SELECT id FROM conservation_zones LIMIT 1").Scan(&zoneID)
		_, _ = pool.Exec(ctx, `
			INSERT INTO conservation_projects (
				id, zone_id, title, description, objectives, category, species_or_type_name,
				image_url, price_per_unit_money, target_amount, target_units, donation_start_date
			) VALUES ($1, $2, 'Coral Leaderboard Test', 'Desc', 'Obj', 'coral', 'Acropora', 'https://img.test', 25.00, 2500.00, 100, NOW())
		`, coralProjectID, zoneID)
	}

	donationID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO donations (
			id, idempotency_key, user_id, project_id, cycle_number, donation_type,
			unit_quantity, currency_code, gross_project_value, money_amount_paid,
			settlement_currency_code, settlement_amount, fx_rate_snapshot, gateway_fee_settlement,
			money_payment_method, payment_provider, is_mixed_payment, points_used,
			is_points_locked, payment_status, expired_at, paid_at
		) VALUES (
			$1, $2, $3, $4, 1, 'coral_adoption',
			1, 'USD', 25.00, 25.00,
			'IDR', 400000.00, 16000.00, 0,
			'qris', 'xendit', false, 0,
			false, 'paid', NOW() + INTERVAL '1 day', NOW()
		)
	`, donationID, fmt.Sprintf("idem-leaderboard-%d", time.Now().UnixNano()), userID, coralProjectID)
	if err != nil {
		t.Fatalf("failed to insert paid test donation: %v", err)
	}

	boardCoral, err := gamifySvc.GetLeaderboard(ctx, 10, 0, "all_time", "coral")
	if err != nil {
		t.Fatalf("failed to get coral leaderboard: %v", err)
	}
	if len(boardCoral) == 0 {
		t.Errorf("expected at least 1 user in coral leaderboard")
	}

	boardAll, err := gamifySvc.GetLeaderboard(ctx, 10, 0, "all_time", "all")
	if err != nil {
		t.Fatalf("failed to get all leaderboard: %v", err)
	}
	if len(boardAll) == 0 {
		t.Errorf("expected at least 1 user in all leaderboard")
	}

	boardTree, err := gamifySvc.GetLeaderboard(ctx, 10, 0, "all_time", "tree")
	if err != nil {
		t.Fatalf("failed to get tree leaderboard: %v", err)
	}
	for _, b := range boardTree {
		if b.ID == userID {
			t.Errorf("did not expect user to appear in tree leaderboard")
		}
	}

	// 8. Prevent duplicate submission on the same day
	_, err = gamifySvc.SubmitMissionProof(ctx, userID, missionID, &petAcep.ID, "https://storage.bioverse.id/proofs/tumbler123.jpg")
	if err == nil {
		t.Fatalf("expected error submitting same mission on same day, got nil")
	}

	// 9. Verify pet ownership invariant
	foreignPetID := uuid.New()
	_, err = gamifySvc.SubmitMissionProof(ctx, userID, missionID, &foreignPetID, "https://storage.bioverse.id/proofs/tumbler123.jpg")
	if err == nil {
		t.Fatalf("expected error submitting proof for unowned pet, got nil")
	}

	// 10. Verify inventory ownership invariant for equipping items
	unownedProductID := uuid.New()
	_, err = gamifySvc.EquipPetItem(ctx, userID, petAcep.ID, unownedProductID)
	if err == nil || !strings.Contains(err.Error(), "inventory") {
		t.Fatalf("expected unowned item equip to fail with inventory error, got %v", err)
	}

	t.Logf("Gamification end-to-end verified successfully for user %s and pet %s", userID, petAcep.PetName)
}
