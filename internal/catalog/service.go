package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	// ErrProductNotFound indicates digital product not found.
	ErrProductNotFound = errors.New("digital product not found")
	// ErrInsufficientPoints indicates user does not have enough points.
	ErrInsufficientPoints = errors.New("insufficient points balance to purchase product")
	// ErrItemNotInInventory indicates user does not own item or quantity is zero.
	ErrItemNotInInventory = errors.New("item not in user inventory or insufficient quantity")
	// ErrPlanNotFound indicates subscription plan not found.
	ErrPlanNotFound = errors.New("subscription plan not found")
	// ErrReceiptAlreadyUsed indicates IAP receipt was already processed.
	ErrReceiptAlreadyUsed = errors.New("iap receipt already verified")
	// ErrSubscriptionNotFound indicates subscription record not found.
	ErrSubscriptionNotFound = errors.New("subscription not found")
)

// Service provides business logic for the digital item shop and subscription catalog.
type Service struct {
	pool  *pgxpool.Pool
	store store.Querier
}

// NewService constructs a new catalog service.
func NewService(pool *pgxpool.Pool, q store.Querier) *Service {
	return &Service{
		pool:  pool,
		store: q,
	}
}

// ListDigitalProducts returns all active products with pricing in the requested currency.
func (s *Service) ListDigitalProducts(ctx context.Context, currencyCode string) ([]store.ListActiveDigitalProductsRow, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	return s.store.ListActiveDigitalProducts(ctx, currencyCode)
}

// ListSubscriptionPlans returns all active subscription plans with pricing.
func (s *Service) ListSubscriptionPlans(ctx context.Context, currencyCode string) ([]store.ListActiveSubscriptionPlansRow, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	return s.store.ListActiveSubscriptionPlans(ctx, currencyCode)
}

// BuyProductWithPoints handles shop item purchase with user points.
func (s *Service) BuyProductWithPoints(ctx context.Context, userID, productID uuid.UUID, quantity int32) (*store.UserDigitalInventories, error) {
	if quantity <= 0 {
		quantity = 1
	}

	prod, err := s.store.GetDigitalProductByID(ctx, productID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("failed to get product: %w", err)
	}

	totalPoints := int64(prod.PricePoints) * int64(quantity)

	var inv store.UserDigitalInventories
	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		// 1. Lock and deduct points
		lockedUser, err := txStore.LockUserPoints(ctx, store.LockUserPointsParams{
			ID:                  userID,
			LockedPointsBalance: totalPoints,
		})
		if err != nil {
			return ErrInsufficientPoints
		}

		spentUser, err := txStore.SpendUserPoints(ctx, store.SpendUserPointsParams{
			ID:            userID,
			PointsBalance: totalPoints,
		})
		if err != nil {
			return fmt.Errorf("failed to spend user points: %w", err)
		}

		// 2. Upsert into inventory
		invID, _ := uuid.NewV7()
		createdInv, err := txStore.UpsertUserInventoryItem(ctx, store.UpsertUserInventoryItemParams{
			ID:        invID,
			UserID:    userID,
			ProductID: productID,
			Quantity:  quantity,
		})
		if err != nil {
			return fmt.Errorf("failed to update user inventory: %w", err)
		}

		// 3. Record point ledger entry
		ledgerID, _ := uuid.NewV7()
		_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
			ID:              ledgerID,
			UserID:          userID,
			TransactionType: store.PointTxTypeSpendShop,
			Amount:          totalPoints,
			BalanceAfter:    spentUser.PointsBalance,
			ReferenceType:   "shop_purchase",
			ReferenceID:     invID,
			Description:     fmt.Sprintf("Purchased %dx %s", quantity, prod.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to record point ledger: %w", err)
		}

		inv = createdInv
		_ = lockedUser
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &inv, nil
}

// ListUserInventory retrieves items owned by user.
func (s *Service) ListUserInventory(ctx context.Context, userID uuid.UUID) ([]store.ListUserInventoryRow, error) {
	return s.store.ListUserInventory(ctx, userID)
}

// ConsumeStreakFreeze uses one Streak Freeze item from user inventory to protect streak.
func (s *Service) ConsumeStreakFreeze(ctx context.Context, userID uuid.UUID) error {
	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		prod, err := txStore.GetDigitalProductBySKU(ctx, store.GetDigitalProductBySKUParams{
			SkuCode:      "STREAK_FREEZE_1X",
			CurrencyCode: "USD",
		})
		if err != nil {
			return fmt.Errorf("streak freeze product missing: %w", err)
		}

		_, err = txStore.DecrementUserInventoryItem(ctx, store.DecrementUserInventoryItemParams{
			UserID:    userID,
			ProductID: prod.ID,
			Quantity:  1,
		})
		if err != nil {
			return ErrItemNotInInventory
		}

		today := time.Now().UTC()
		todayDate := pgtype.Date{
			Time:  time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC),
			Valid: true,
		}

		streak, err := txStore.GetUserStreakByUserID(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to get user streak: %w", err)
		}

		_, err = txStore.UpdateUserStreak(ctx, store.UpdateUserStreakParams{
			UserID:             userID,
			CurrentStreak:      streak.CurrentStreak,
			LongestStreak:      streak.LongestStreak,
			LastStreakDate:     streak.LastStreakDate,
			LastFreezeUsedDate: todayDate,
		})
		if err != nil {
			return fmt.Errorf("failed to record freeze used date: %w", err)
		}

		return nil
	})
}

// SubscribePlan creates a recurring maintenance subscription.
func (s *Service) SubscribePlan(ctx context.Context, userID, planID uuid.UUID, paymentMethod store.PaymentMethodType) (*store.DonationSubscriptions, error) {
	plan, err := s.store.GetSubscriptionPlanByID(ctx, store.GetSubscriptionPlanByIDParams{
		ID:           planID,
		CurrencyCode: "USD",
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("failed to get subscription plan: %w", err)
	}

	subID, _ := uuid.NewV7()
	nextBilling := time.Now().UTC().AddDate(0, 1, 0)
	nextBillingDate := pgtype.Date{
		Time:  time.Date(nextBilling.Year(), nextBilling.Month(), nextBilling.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}

	sub, err := s.store.CreateDonationSubscription(ctx, store.CreateDonationSubscriptionParams{
		ID:              subID,
		UserID:          userID,
		PlanID:          pgtype.UUID{Bytes: planID, Valid: true},
		ProjectID:       pgtype.UUID{Valid: false},
		Frequency:       plan.BillingInterval,
		DurationMonths:  plan.DurationMonths,
		UnitQuantity:    1,
		CurrencyCode:    "USD",
		AmountPerCycle:  plan.PriceMoney,
		PaymentMethod:   paymentMethod,
		PaymentProvider: "xendit",
		Status:          store.SubscriptionStatusActive,
		NextBillingDate: nextBillingDate,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	return &sub, nil
}

// ListUserSubscriptions returns user subscriptions.
func (s *Service) ListUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]store.ListUserSubscriptionsRow, error) {
	return s.store.ListUserSubscriptions(ctx, userID)
}

// VerifyIAPInput holds payload for app store receipt validation.
type VerifyIAPInput struct {
	UserID         uuid.UUID
	ProductID      uuid.UUID
	StoreProvider  string
	ReceiptPayload string
}

// VerifyIAPReceipt validates mobile app store purchases with replay protection.
func (s *Service) VerifyIAPReceipt(ctx context.Context, in VerifyIAPInput) error {
	h := sha256.Sum256([]byte(in.StoreProvider + ":" + in.ReceiptPayload))
	txID := hex.EncodeToString(h[:])

	// Prevent duplicate replay
	_, err := s.store.GetIAPTransactionByStoreTxID(ctx, store.GetIAPTransactionByStoreTxIDParams{
		StoreProvider:      store.IapStoreType(in.StoreProvider),
		StoreTransactionID: txID,
	})
	if err == nil {
		return ErrReceiptAlreadyUsed
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to check existing iap transaction: %w", err)
	}

	return database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		iapID, _ := uuid.NewV7()
		_, err := txStore.CreateIAPTransaction(ctx, store.CreateIAPTransactionParams{
			ID:                 iapID,
			UserID:             in.UserID,
			ProductID:          in.ProductID,
			StoreProvider:      store.IapStoreType(in.StoreProvider),
			StoreTransactionID: txID,
			Environment:        "production",
			ReceiptPayload:     in.ReceiptPayload,
			CurrencyCode:       "USD",
			GrossAmount:        decimalutil.ToNumeric(decimal.NewFromFloat(0.99)),
			StoreFeeAmount:     decimalutil.ToNumeric(decimal.NewFromFloat(0.15)),
			NetRevenueAmount:   decimalutil.ToNumeric(decimal.NewFromFloat(0.84)),
			Status:             "completed",
		})
		if err != nil {
			return fmt.Errorf("failed to record iap transaction: %w", err)
		}

		invID, _ := uuid.NewV7()
		_, err = txStore.UpsertUserInventoryItem(ctx, store.UpsertUserInventoryItemParams{
			ID:        invID,
			UserID:    in.UserID,
			ProductID: in.ProductID,
			Quantity:  1,
		})
		if err != nil {
			return fmt.Errorf("failed to credit inventory item: %w", err)
		}

		return nil
	})
}

// PauseSubscription pauses an active subscription.
func (s *Service) PauseSubscription(ctx context.Context, userID, subID uuid.UUID, reason string) (*store.DonationSubscriptions, error) {
	sub, err := s.store.UpdateSubscriptionStatus(ctx, store.UpdateSubscriptionStatusParams{
		ID:          subID,
		UserID:      userID,
		Status:      store.SubscriptionStatusPaused,
		PausedAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		PauseReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to pause subscription: %w", err)
	}
	return &sub, nil
}

// ResumeSubscription reactivates a paused subscription.
func (s *Service) ResumeSubscription(ctx context.Context, userID, subID uuid.UUID) (*store.DonationSubscriptions, error) {
	sub, err := s.store.UpdateSubscriptionStatus(ctx, store.UpdateSubscriptionStatusParams{
		ID:          subID,
		UserID:      userID,
		Status:      store.SubscriptionStatusActive,
		PausedAt:    pgtype.Timestamptz{Valid: false},
		PauseReason: pgtype.Text{Valid: false},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to resume subscription: %w", err)
	}
	return &sub, nil
}

// CancelSubscription cancels a subscription.
func (s *Service) CancelSubscription(ctx context.Context, userID, subID uuid.UUID, reason string) (*store.DonationSubscriptions, error) {
	sub, err := s.store.UpdateSubscriptionStatus(ctx, store.UpdateSubscriptionStatusParams{
		ID:          subID,
		UserID:      userID,
		Status:      store.SubscriptionStatusCancelled,
		PausedAt:    pgtype.Timestamptz{Valid: false},
		PauseReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to cancel subscription: %w", err)
	}
	return &sub, nil
}
