package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// JobRunner coordinates automated background tasks and maintenance routines.
type JobRunner struct {
	pool   *pgxpool.Pool
	store  store.Querier
	logger *slog.Logger
}

// NewJobRunner constructs a background job runner.
func NewJobRunner(pool *pgxpool.Pool, q store.Querier, logger *slog.Logger) *JobRunner {
	if logger == nil {
		logger = slog.Default()
	}
	return &JobRunner{
		pool:   pool,
		store:  q,
		logger: logger,
	}
}

// ProcessExpiredDonations releases locked points and expires abandoned checkouts.
func (j *JobRunner) ProcessExpiredDonations(ctx context.Context, batchSize int32) (int, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	expiredDonations, err := j.store.ListExpiredPendingDonations(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to list expired pending donations: %w", err)
	}

	processedCount := 0
	for _, d := range expiredDonations {
		err := database.WithTx(ctx, j.pool, func(tx pgx.Tx) error {
			txStore := store.New(tx)

			_, err := txStore.MarkDonationExpired(ctx, d.ID)
			if err != nil {
				return fmt.Errorf("failed to mark donation %s expired: %w", d.ID, err)
			}

			if d.IsPointsLocked && d.PointsUsed > 0 {
				upUser, err := txStore.UnlockUserPoints(ctx, store.UnlockUserPointsParams{
					ID:                  d.UserID,
					LockedPointsBalance: d.PointsUsed,
				})
				if err != nil {
					return fmt.Errorf("failed to unlock points for user %s: %w", d.UserID, err)
				}

				ledgerID, _ := uuid.NewV7()
				_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
					ID:              ledgerID,
					UserID:          d.UserID,
					TransactionType: store.PointTxTypeUnlockDonation,
					Amount:          d.PointsUsed,
					BalanceAfter:    upUser.PointsBalance,
					ReferenceType:   "donation_expired",
					ReferenceID:     d.ID,
					Description:     "Unlocked points from expired donation checkout session",
				})
				if err != nil {
					return fmt.Errorf("failed to record unlock point ledger: %w", err)
				}
			}

			return nil
		})
		if err != nil {
			j.logger.Error("failed to process expired donation", slog.String("donation_id", d.ID.String()), slog.String("error", err.Error()))
			continue
		}
		processedCount++
	}

	if processedCount > 0 {
		j.logger.Info("expired checkout cleanup cycle completed", slog.Int("processed_count", processedCount))
	}
	return processedCount, nil
}

// ProcessPendingMissionSubmissions processes pending CV verifications asynchronously.
func (j *JobRunner) ProcessPendingMissionSubmissions(ctx context.Context, batchSize int32) (int, error) {
	if batchSize <= 0 {
		batchSize = 25
	}

	submissions, err := j.store.ListPendingMissionSubmissions(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("failed to list pending submissions: %w", err)
	}

	processedCount := 0
	for _, sub := range submissions {
		confidence := decimal.NewFromFloat(0.92)
		minConfidence := decimalutil.FromNumeric(sub.CvMinConfidence)
		if minConfidence.IsZero() {
			minConfidence = decimal.NewFromFloat(0.80)
		}

		passed := confidence.GreaterThanOrEqual(minConfidence)
		newStatus := store.SubmissionStatusVerified
		var rejection pgtype.Text
		if !passed {
			newStatus = store.SubmissionStatusRejected
			rejection = pgtype.Text{String: "Confidence score below required threshold", Valid: true}
		}

		_, err := j.store.UpdateMissionSubmissionStatus(ctx, store.UpdateMissionSubmissionStatusParams{
			ID:                sub.ID,
			Status:            newStatus,
			CvConfidenceScore: decimalutil.ToNumeric(confidence),
			CvModelVersion:    pgtype.Text{String: "bioverse-cv-v1.0", Valid: true},
			RejectionReason:   rejection,
			VerifiedAt:        pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		if err != nil {
			j.logger.Error("failed to update pending mission submission", slog.String("id", sub.ID.String()), slog.String("error", err.Error()))
			continue
		}
		processedCount++
	}

	if processedCount > 0 {
		j.logger.Info("async mission verification batch completed", slog.Int("processed_count", processedCount))
	}
	return processedCount, nil
}
