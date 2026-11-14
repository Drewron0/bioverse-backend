package gamification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/database"
	"github.com/Drewron01/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	// ErrPetNotFound indicates the virtual pet was not found.
	ErrPetNotFound = errors.New("virtual pet not found")
	// ErrMissionNotFound indicates the eco mission was not found.
	ErrMissionNotFound = errors.New("eco mission not found")
	// ErrMissionAlreadyCompletedToday indicates daily mission was already submitted today.
	ErrMissionAlreadyCompletedToday = errors.New("mission already submitted or completed for today")
	// ErrInvalidProofImage indicates proof image URL or data is empty.
	ErrInvalidProofImage = errors.New("proof image is required")
)

// ComputerVisionVerifier defines the contract for verifying eco-mission camera proofs.
// This allows seamless transition from mock/stub to production AI models without code changes.
type ComputerVisionVerifier interface {
	VerifyProof(ctx context.Context, targetLabel string, minConfidence decimal.Decimal, imageURL string) (CVResult, error)
}

// CVResult contains the evaluation of the camera proof image.
type CVResult struct {
	Passed          bool
	ConfidenceScore decimal.Decimal
	DetectedLabel   string
	ModelVersion    string
	JobID           string
	RawResponse     []byte
	RejectionReason string
}

// StubCVVerifier provides an accurate, predictable implementation for local & staging environments.
type StubCVVerifier struct{}

// VerifyProof evaluates the proof image using the stub verifier.
func (v *StubCVVerifier) VerifyProof(_ context.Context, targetLabel string, minConfidence decimal.Decimal, imageURL string) (CVResult, error) {
	// In the stub, if imageURL is provided and not empty, we consider it a verified match.
	confidence := decimal.NewFromFloat(0.92)
	passed := confidence.GreaterThanOrEqual(minConfidence)
	rejection := ""
	if !passed {
		rejection = fmt.Sprintf("Confidence score %.2f is below threshold %.2f", confidence.InexactFloat64(), minConfidence.InexactFloat64())
	}

	return CVResult{
		Passed:          passed,
		ConfidenceScore: confidence,
		DetectedLabel:   targetLabel,
		ModelVersion:    "bioverse-cv-stub-v1.0",
		JobID:           fmt.Sprintf("cv-job-%d", time.Now().UnixNano()),
		RawResponse:     []byte(fmt.Sprintf(`{"detected": "%s", "confidence": %.2f, "status": "approved"}`, targetLabel, confidence.InexactFloat64())),
		RejectionReason: rejection,
	}, nil
}

// Service coordinates virtual pet actions, streaks, eco-missions, and points.
type Service struct {
	pool       *pgxpool.Pool
	store      store.Querier
	cvVerifier ComputerVisionVerifier
}

// NewService constructs a gamification domain service.
func NewService(pool *pgxpool.Pool, q store.Querier, verifier ComputerVisionVerifier) *Service {
	if verifier == nil {
		verifier = &StubCVVerifier{}
	}
	return &Service{
		pool:       pool,
		store:      q,
		cvVerifier: verifier,
	}
}

// ListUserPets returns all virtual pets owned by the user.
func (s *Service) ListUserPets(ctx context.Context, userID uuid.UUID) ([]store.ListVirtualPetsByUserIDRow, error) {
	return s.store.ListVirtualPetsByUserID(ctx, userID)
}

// GetUserPet retrieves a specific pet by ID for the user.
func (s *Service) GetUserPet(ctx context.Context, userID, petID uuid.UUID) (*store.GetVirtualPetByIDRow, error) {
	pet, err := s.store.GetVirtualPetByID(ctx, store.GetVirtualPetByIDParams{
		ID:     petID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPetNotFound
		}
		return nil, fmt.Errorf("failed to query virtual pet: %w", err)
	}
	return &pet, nil
}

// AdoptPetInput holds payload for creating a new pet.
type AdoptPetInput struct {
	UserID      uuid.UUID
	PetName     string
	PetSpecies  string
	CoralType   string
	FragmentID  *uuid.UUID
	MakePrimary bool
}

// AdoptPet creates a new virtual pet for the user.
func (s *Service) AdoptPet(ctx context.Context, in AdoptPetInput) (*store.VirtualPets, error) {
	var createdPet store.VirtualPets
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		existingPets, err := txStore.ListVirtualPetsByUserID(ctx, in.UserID)
		if err != nil {
			return fmt.Errorf("failed to check existing user pets: %w", err)
		}

		isPrimary := in.MakePrimary || len(existingPets) == 0
		if isPrimary && len(existingPets) > 0 {
			if err := txStore.ClearUserPrimaryPets(ctx, in.UserID); err != nil {
				return fmt.Errorf("failed to clear existing primary pet: %w", err)
			}
		}

		petID, _ := uuid.NewV7()
		var fragID pgtype.UUID
		if in.FragmentID != nil {
			fragID = pgtype.UUID{Bytes: *in.FragmentID, Valid: true}
		}

		pet, err := txStore.CreateVirtualPet(ctx, store.CreateVirtualPetParams{
			ID:         petID,
			UserID:     in.UserID,
			FragmentID: fragID,
			PetName:    in.PetName,
			PetSpecies: in.PetSpecies,
			CoralType:  in.CoralType,
			IsPrimary:  isPrimary,
		})
		if err != nil {
			return fmt.Errorf("failed to insert virtual pet: %w", err)
		}

		createdPet = pet
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &createdPet, nil
}

// InteractWithPet increases pet health and grants small companion interaction XP.
func (s *Service) InteractWithPet(ctx context.Context, userID, petID uuid.UUID) (*store.VirtualPets, error) {
	var updatedPet store.VirtualPets
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		pet, err := txStore.GetVirtualPetByID(ctx, store.GetVirtualPetByIDParams{
			ID:     petID,
			UserID: userID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrPetNotFound
			}
			return fmt.Errorf("failed to find pet for interaction: %w", err)
		}

		// Calculate XP boost if active
		boosts, err := txStore.ListActivePetBoosts(ctx, petID)
		if err != nil {
			return fmt.Errorf("failed to query active pet boosts: %w", err)
		}

		xpMultiplier := decimal.NewFromInt(1)
		for _, b := range boosts {
			mult := decimalutil.FromNumeric(b.XpMultiplier)
			if mult.GreaterThan(xpMultiplier) {
				xpMultiplier = mult
			}
		}

		baseXp := decimal.NewFromInt(10)
		gainedXp := baseXp.Mul(xpMultiplier).IntPart()

		newHealth := pet.HealthPercent + 10
		if newHealth > 100 {
			newHealth = 100
		}
		newXp := pet.ExperiencePoints + gainedXp
		newLevel := calculatePetLevel(newXp)
		newStage := calculateGrowthStage(newLevel)

		up, err := txStore.UpdateVirtualPetHealthAndXP(ctx, store.UpdateVirtualPetHealthAndXPParams{
			ID:               petID,
			UserID:           userID,
			HealthPercent:    newHealth,
			ExperiencePoints: newXp,
			Level:            newLevel,
			GrowthStage:      newStage,
			LastInteractedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update pet after interaction: %w", err)
		}

		updatedPet = up
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &updatedPet, nil
}

// ListActiveMissions returns all active eco-missions.
func (s *Service) ListActiveMissions(ctx context.Context) ([]store.EcoMissions, error) {
	return s.store.ListActiveMissions(ctx)
}

// MissionSubmissionResult represents the outcome of submitting an eco-mission proof.
type MissionSubmissionResult struct {
	SubmissionID   uuid.UUID
	MissionID      uuid.UUID
	Status         store.SubmissionStatus
	AwardedPoints  int64
	AwardedXP      int64
	CurrentStreak  int32
	VerifiedAt     *time.Time
	RejectionNotes string
}

// SubmitMissionProof handles submission, computer vision verification, reward disbursement, and streak advancement.
func (s *Service) SubmitMissionProof(ctx context.Context, userID, missionID uuid.UUID, targetPetID *uuid.UUID, proofImageURL string) (*MissionSubmissionResult, error) {
	if proofImageURL == "" {
		return nil, ErrInvalidProofImage
	}

	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user profile: %w", err)
	}

	mission, err := s.store.GetMissionByID(ctx, missionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMissionNotFound
		}
		return nil, fmt.Errorf("failed to find mission: %w", err)
	}

	// Calculate user local date according to their timezone
	loc, err := time.LoadLocation(user.Timezone)
	if err != nil {
		loc = time.UTC
	}
	userToday := time.Now().In(loc)
	todayDate := pgtype.Date{
		Time:  time.Date(userToday.Year(), userToday.Month(), userToday.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}

	// Check if already submitted today
	_, err = s.store.GetTodaySubmissionForMission(ctx, store.GetTodaySubmissionForMissionParams{
		UserID:         userID,
		MissionID:      missionID,
		SubmissionDate: todayDate,
	})
	if err == nil {
		return nil, ErrMissionAlreadyCompletedToday
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("failed to verify daily mission limit: %w", err)
	}

	// Run Computer Vision verification
	targetLabel := "general_eco_action"
	if mission.CvTargetLabel.Valid && mission.CvTargetLabel.String != "" {
		targetLabel = mission.CvTargetLabel.String
	}
	minConfidence := decimalutil.FromNumeric(mission.CvMinConfidence)
	if minConfidence.IsZero() {
		minConfidence = decimal.NewFromFloat(0.80)
	}

	cvRes, err := s.cvVerifier.VerifyProof(ctx, targetLabel, minConfidence, proofImageURL)
	if err != nil {
		return nil, fmt.Errorf("computer vision service error: %w", err)
	}

	imageHash := hashString(proofImageURL)
	submissionID, _ := uuid.NewV7()

	var result MissionSubmissionResult
	result.SubmissionID = submissionID
	result.MissionID = missionID

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		if !cvRes.Passed {
			// Record rejected submission
			_, err := txStore.CreateMissionSubmission(ctx, store.CreateMissionSubmissionParams{
				ID:                submissionID,
				UserID:            userID,
				MissionID:         missionID,
				PetID:             uuidToPgUUID(targetPetID),
				SubmissionDate:    todayDate,
				ProofImageUrl:     pgtype.Text{String: proofImageURL, Valid: true},
				ImageHash:         pgtype.Text{String: imageHash, Valid: true},
				Status:            store.SubmissionStatusRejected,
				CvJobID:           pgtype.Text{String: cvRes.JobID, Valid: true},
				CvModelVersion:    pgtype.Text{String: cvRes.ModelVersion, Valid: true},
				CvConfidenceScore: decimalutil.ToNumeric(cvRes.ConfidenceScore),
				CvRawResponse:     cvRes.RawResponse,
				AwardedPoints:     0,
				AwardedXp:         0,
				VerifiedAt:        pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			})
			if err != nil {
				return fmt.Errorf("failed to save rejected submission: %w", err)
			}
			result.Status = store.SubmissionStatusRejected
			result.RejectionNotes = cvRes.RejectionReason
			return nil
		}

		// Calculate XP boost if pet is selected
		awardedXp := mission.XpReward
		if targetPetID != nil {
			boosts, err := txStore.ListActivePetBoosts(ctx, *targetPetID)
			if err == nil {
				multiplier := decimal.NewFromInt(1)
				for _, b := range boosts {
					mult := decimalutil.FromNumeric(b.XpMultiplier)
					if mult.GreaterThan(multiplier) {
						multiplier = mult
					}
				}
				awardedXp = decimal.NewFromInt(mission.XpReward).Mul(multiplier).IntPart()
			}
		}

		nowUtc := time.Now().UTC()
		// Save approved submission
		_, err = txStore.CreateMissionSubmission(ctx, store.CreateMissionSubmissionParams{
			ID:                submissionID,
			UserID:            userID,
			MissionID:         missionID,
			PetID:             uuidToPgUUID(targetPetID),
			SubmissionDate:    todayDate,
			ProofImageUrl:     pgtype.Text{String: proofImageURL, Valid: true},
			ImageHash:         pgtype.Text{String: imageHash, Valid: true},
			Status:            store.SubmissionStatusVerified,
			CvJobID:           pgtype.Text{String: cvRes.JobID, Valid: true},
			CvModelVersion:    pgtype.Text{String: cvRes.ModelVersion, Valid: true},
			CvConfidenceScore: decimalutil.ToNumeric(cvRes.ConfidenceScore),
			CvRawResponse:     cvRes.RawResponse,
			AwardedPoints:     mission.PointsReward,
			AwardedXp:         awardedXp,
			VerifiedAt:        pgtype.Timestamptz{Time: nowUtc, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to save approved submission: %w", err)
		}

		// 1. Credit Points to user
		updatedUser, err := txStore.CreditUserPoints(ctx, store.CreditUserPointsParams{
			ID:            userID,
			PointsBalance: mission.PointsReward,
		})
		if err != nil {
			return fmt.Errorf("failed to credit points: %w", err)
		}

		// 2. Insert Point Ledger entry
		ledgerID, _ := uuid.NewV7()
		_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
			ID:              ledgerID,
			UserID:          userID,
			TransactionType: store.PointTxTypeEarnMission,
			Amount:          mission.PointsReward,
			BalanceAfter:    updatedUser.PointsBalance,
			ReferenceType:   "mission_submission",
			ReferenceID:     submissionID,
			Description:     fmt.Sprintf("Reward for eco-mission: %s", mission.Title),
		})
		if err != nil {
			return fmt.Errorf("failed to create points ledger entry: %w", err)
		}

		// 3. Update target pet XP and Level if assigned
		if targetPetID != nil {
			pet, err := txStore.GetVirtualPetByID(ctx, store.GetVirtualPetByIDParams{
				ID:     *targetPetID,
				UserID: userID,
			})
			if err == nil {
				newXp := pet.ExperiencePoints + awardedXp
				newLevel := calculatePetLevel(newXp)
				newStage := calculateGrowthStage(newLevel)
				_, _ = txStore.UpdateVirtualPetHealthAndXP(ctx, store.UpdateVirtualPetHealthAndXPParams{
					ID:               *targetPetID,
					UserID:           userID,
					HealthPercent:    pet.HealthPercent,
					ExperiencePoints: newXp,
					Level:            newLevel,
					GrowthStage:      newStage,
					LastInteractedAt: pgtype.Timestamptz{Time: nowUtc, Valid: true},
				})
			}
		}

		// 4. Update User Daily Streak
		streak, err := txStore.GetUserStreakByUserID(ctx, userID)
		if err == nil {
			newStreak, longestStreak := calculateNewStreak(streak, todayDate.Time)
			upStreak, err := txStore.UpdateUserStreak(ctx, store.UpdateUserStreakParams{
				UserID:             userID,
				CurrentStreak:      newStreak,
				LongestStreak:      longestStreak,
				LastStreakDate:     todayDate,
				LastFreezeUsedDate: streak.LastFreezeUsedDate,
			})
			if err == nil {
				result.CurrentStreak = upStreak.CurrentStreak
			}
		}

		result.Status = store.SubmissionStatusVerified
		result.AwardedPoints = mission.PointsReward
		result.AwardedXP = awardedXp
		result.VerifiedAt = &nowUtc
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetUserStreak retrieves the current streak state for a user.
func (s *Service) GetUserStreak(ctx context.Context, userID uuid.UUID) (*store.UserStreaks, error) {
	streak, err := s.store.GetUserStreakByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user streak record not found")
		}
		return nil, err
	}
	return &streak, nil
}

// GetLeaderboard returns the global user ranking by points.
func (s *Service) GetLeaderboard(ctx context.Context, limit, offset int32) ([]store.GetLeaderboardRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.GetLeaderboard(ctx, store.GetLeaderboardParams{
		Limit:  limit,
		Offset: offset,
	})
}

// Helpers

func calculatePetLevel(xp int64) int32 {
	if xp <= 0 {
		return 1
	}
	// Level formula: Level = 1 + floor(sqrt(xp / 100))
	lvl := 1 + int32(math.Floor(math.Sqrt(float64(xp)/100.0)))
	if lvl < 1 {
		return 1
	}
	return lvl
}

func calculateGrowthStage(level int32) store.PetGrowthStage {
	switch {
	case level >= 20:
		return store.PetGrowthStageElder
	case level >= 10:
		return store.PetGrowthStageAdult
	case level >= 5:
		return store.PetGrowthStageJuvenile
	default:
		return store.PetGrowthStageHatchling
	}
}

func calculateNewStreak(s store.UserStreaks, today time.Time) (int32, int32) {
	if !s.LastStreakDate.Valid {
		return 1, max(s.LongestStreak, 1)
	}

	lastDate := s.LastStreakDate.Time
	diffDays := int(today.Sub(lastDate).Hours() / 24)

	switch {
	case diffDays == 0:
		// Activity already logged today
		return s.CurrentStreak, s.LongestStreak
	case diffDays == 1:
		// Consecutive day!
		newStreak := s.CurrentStreak + 1
		return newStreak, max(s.LongestStreak, newStreak)
	default:
		// Streak broken (freeze logic can be plugged here if user has digital streak freeze)
		return 1, s.LongestStreak
	}
}

func uuidToPgUUID(u *uuid.UUID) pgtype.UUID {
	if u == nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: *u, Valid: true}
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func max(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
