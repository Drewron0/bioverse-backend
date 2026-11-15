package gamification_test

import (
	"context"
	"fmt"
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

	// 7. Verify leaderboard contains the user
	board, err := gamifySvc.GetLeaderboard(ctx, 10, 0)
	if err != nil {
		t.Fatalf("failed to get leaderboard: %v", err)
	}
	if len(board) == 0 {
		t.Errorf("expected at least 1 user in leaderboard")
	}

	// 8. Prevent duplicate submission on the same day
	_, err = gamifySvc.SubmitMissionProof(ctx, userID, missionID, &petAcep.ID, "https://storage.bioverse.id/proofs/tumbler123.jpg")
	if err == nil {
		t.Fatalf("expected error submitting same mission on same day, got nil")
	}

	t.Logf("Gamification end-to-end verified successfully for user %s and pet %s", userID, petAcep.PetName)
}
