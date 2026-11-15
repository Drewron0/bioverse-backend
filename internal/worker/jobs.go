package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
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

// ProcessDueSubscriptions scans for recurring subscriptions that are due for billing and advances their cycle.
func (j *JobRunner) ProcessDueSubscriptions(ctx context.Context, batchSize int32) (int, error) {
	if batchSize <= 0 {
		batchSize = 50
	}

	today := pgtype.Date{
		Time:  time.Now().UTC(),
		Valid: true,
	}

	dueSubs, err := j.store.ListDueSubscriptions(ctx, store.ListDueSubscriptionsParams{
		NextBillingDate: today,
		Limit:           batchSize,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list due subscriptions: %w", err)
	}

	processedCount := 0
	for _, sub := range dueSubs {
		err := database.WithTx(ctx, j.pool, func(tx pgx.Tx) error {
			txStore := store.New(tx)

			nextBilling := time.Now().UTC().AddDate(0, 1, 0)
			if sub.Frequency == store.SubscriptionFrequencyWeekly {
				nextBilling = time.Now().UTC().AddDate(0, 0, 7)
			} else if sub.Frequency == store.SubscriptionFrequencyYearly {
				nextBilling = time.Now().UTC().AddDate(1, 0, 0)
			}

			nextDate := pgtype.Date{
				Time:  time.Date(nextBilling.Year(), nextBilling.Month(), nextBilling.Day(), 0, 0, 0, 0, time.UTC),
				Valid: true,
			}

			_, err := txStore.AdvanceSubscriptionBillingCycle(ctx, store.AdvanceSubscriptionBillingCycleParams{
				ID:              sub.ID,
				NextBillingDate: nextDate,
			})
			if err != nil {
				return fmt.Errorf("failed to advance subscription billing cycle: %w", err)
			}

			return nil
		})
		if err != nil {
			j.logger.Error("failed to process due subscription", slog.String("subscription_id", sub.ID.String()), slog.String("error", err.Error()))
			continue
		}
		processedCount++
	}

	if processedCount > 0 {
		j.logger.Info("subscription billing check cycle completed", slog.Int("processed_count", processedCount))
	}
	return processedCount, nil
}
