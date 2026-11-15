package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

var (
	// ErrProductNotFound indicates digital product not found.
	ErrProductNotFound = errors.New("digital product not found")
	// ErrInsufficientPoints indicates user does not have enough points.
	ErrInsufficientPoints = errors.New("insufficient points balance to purchase product")
	// ErrItemNotInInventory indicates user does not own item or quantity is zero.
	ErrItemNotInInventory = errors.New("item not in user inventory or insufficient quantity")
	// ErrPlanNotFound indicates subscription plan not found.
	ErrPlanNotFound = errors.New("subscription plan not found")
	// ErrProjectNotFound indicates conservation project not found.
	ErrProjectNotFound = errors.New("conservation project not found")
	// ErrReceiptAlreadyUsed indicates IAP receipt was already processed.
	ErrReceiptAlreadyUsed = errors.New("iap receipt already verified")
	// ErrSubscriptionNotFound indicates subscription record not found.
	ErrSubscriptionNotFound = errors.New("subscription not found")
	// ErrInvalidSubscriptionState indicates invalid status transition for subscription.
	ErrInvalidSubscriptionState = errors.New("invalid subscription status transition")
	// ErrIAPNotConfiguredInProduction indicates real-time App Store verification is not enabled in production.
	ErrIAPNotConfiguredInProduction = errors.New("in-app purchase verification is disabled in production without server credentials")
	// ErrSubscriptionNotAvailableInProduction indicates recurring subscription checkout is disabled in production without payment tokenization.
	ErrSubscriptionNotAvailableInProduction = errors.New("recurring subscription payment is disabled until gateway integration is configured")
)

// CurrencyPrice represents currency code and decimal amount as formatted string.
type CurrencyPrice struct {
	CurrencyCode string `json:"currency_code"`
	Amount       string `json:"amount"`
}

// DigitalProductDTO represents a digital item with multi-currency pricing and json attributes.
type DigitalProductDTO struct {
	ID              uuid.UUID             `json:"id"`
	SkuCode         string                `json:"sku_code"`
	Name            string                `json:"name"`
	Description     string                `json:"description"`
	ItemType        store.DigitalItemType `json:"item_type"`
	PricePoints     int64                 `json:"price_points"`
	Prices          []CurrencyPrice       `json:"prices"`
	Attributes      json.RawMessage       `json:"attributes,omitempty"`
	PreviewAssetURL string                `json:"preview_asset_url"`
}

// SubscriptionPlanDTO represents a recurring adoption plan with interval, benefits list, and pricing.
type SubscriptionPlanDTO struct {
	ID              uuid.UUID                   `json:"id"`
	SkuCode         string                      `json:"sku_code"`
	Name            string                      `json:"name"`
	BillingInterval store.SubscriptionFrequency `json:"billing_interval"`
	Prices          []CurrencyPrice             `json:"prices"`
	Benefits        []string                    `json:"benefits"`
}

// SubscriptionDetailDTO represents subscription details complying with OpenAPI SubscriptionDetail.
type SubscriptionDetailDTO struct {
	ID               uuid.UUID                   `json:"id"`
	Status           store.SubscriptionStatus    `json:"status"`
	PlanName         string                      `json:"plan_name"`
	BillingInterval  store.SubscriptionFrequency `json:"billing_interval"`
	NextBillingDate  time.Time                   `json:"next_billing_date"`
	AuthorizationURL *string                     `json:"authorization_url"`
	CreatedAt        time.Time                   `json:"created_at"`
}

// BuyProductResult holds response data for shop item purchase.
type BuyProductResult struct {
	InventoryID            uuid.UUID `json:"inventory_id"`
	SkuCode                string    `json:"sku_code"`
	RemainingPointsBalance int64     `json:"remaining_points_balance"`
}

// CreateSubscriptionInput holds parameters for starting a recurring maintenance subscription.
type CreateSubscriptionInput struct {
	UserID            uuid.UUID
	PlanID            uuid.UUID
	ProjectID         uuid.UUID
	UnitQuantity      int32
	PaymentMethodType store.PaymentMethodType
}

// Service provides business logic for the digital item shop and subscription catalog.
type Service struct {
	pool         *pgxpool.Pool
	store        store.Querier
	isProduction bool
}

// NewService constructs a new catalog service.
func NewService(pool *pgxpool.Pool, q store.Querier, isProduction bool) *Service {
	return &Service{
		pool:         pool,
		store:        q,
		isProduction: isProduction,
	}
}

// IsProduction reports whether the service is running in production mode.
func (s *Service) IsProduction() bool {
	return s.isProduction
}

// ListDigitalProducts returns all active products with attributes as JSON objects and prices array.
func (s *Service) ListDigitalProducts(ctx context.Context, _ string) ([]DigitalProductDTO, error) {
	query := `
		SELECT dp.id, dp.sku_code, dp.name, dp.description, dp.item_type,
		       dp.price_points, dp.attributes, dp.preview_asset_url,
		       COALESCE(
		           jsonb_agg(
		               jsonb_build_object('currency_code', pp.currency_code, 'amount', pp.amount::text)
		               ORDER BY pp.currency_code DESC
		           ) FILTER (WHERE pp.id IS NOT NULL),
		           '[]'::jsonb
		       ) AS prices
		FROM digital_products dp
		LEFT JOIN product_prices pp ON dp.id = pp.product_id
		WHERE dp.is_active = true
		GROUP BY dp.id
		ORDER BY dp.sku_code ASC;
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query digital products: %w", err)
	}
	defer rows.Close()

	var products []DigitalProductDTO
	for rows.Next() {
		var p DigitalProductDTO
		var rawAttrs []byte
		var rawPrices []byte
		if err := rows.Scan(
			&p.ID,
			&p.SkuCode,
			&p.Name,
			&p.Description,
			&p.ItemType,
			&p.PricePoints,
			&rawAttrs,
			&p.PreviewAssetURL,
			&rawPrices,
		); err != nil {
			return nil, fmt.Errorf("failed to scan digital product: %w", err)
		}
		if len(rawAttrs) > 0 {
			p.Attributes = json.RawMessage(rawAttrs)
		} else {
			p.Attributes = json.RawMessage("{}")
		}
		if len(rawPrices) > 0 {
			_ = json.Unmarshal(rawPrices, &p.Prices)
		}
		if p.Prices == nil {
			p.Prices = []CurrencyPrice{}
		}
		products = append(products, p)
	}
	if products == nil {
		products = []DigitalProductDTO{}
	}
	return products, nil
}

// ListSubscriptionPlans returns all active subscription plans with benefits array and multi-currency prices.
func (s *Service) ListSubscriptionPlans(ctx context.Context, _ string) ([]SubscriptionPlanDTO, error) {
	query := `
		SELECT sp.id, sp.sku_code, sp.name, sp.billing_interval, sp.benefits,
		       COALESCE(
		           jsonb_agg(
		               jsonb_build_object('currency_code', spp.currency_code, 'amount', spp.amount::text)
		               ORDER BY spp.currency_code DESC
		           ) FILTER (WHERE spp.id IS NOT NULL),
		           '[]'::jsonb
		       ) AS prices
		FROM subscription_plans sp
		LEFT JOIN subscription_plan_prices spp ON sp.id = spp.plan_id
		WHERE sp.is_active = true
		GROUP BY sp.id
		ORDER BY sp.sort_order ASC;
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscription plans: %w", err)
	}
	defer rows.Close()

	var plans []SubscriptionPlanDTO
	for rows.Next() {
		var plan SubscriptionPlanDTO
		var rawBenefits []byte
		var rawPrices []byte
		if err := rows.Scan(
			&plan.ID,
			&plan.SkuCode,
			&plan.Name,
			&plan.BillingInterval,
			&rawBenefits,
			&rawPrices,
		); err != nil {
			return nil, fmt.Errorf("failed to scan subscription plan: %w", err)
		}

		if len(rawBenefits) > 0 {
			var strList []string
			if err := json.Unmarshal(rawBenefits, &strList); err == nil {
				plan.Benefits = strList
			} else {
				var obj map[string]any
				if err := json.Unmarshal(rawBenefits, &obj); err == nil {
					for k := range obj {
						plan.Benefits = append(plan.Benefits, k)
					}
				}
			}
		}
		if plan.Benefits == nil {
			plan.Benefits = []string{}
		}

		if len(rawPrices) > 0 {
			_ = json.Unmarshal(rawPrices, &plan.Prices)
		}
		if plan.Prices == nil {
			plan.Prices = []CurrencyPrice{}
		}

		plans = append(plans, plan)
	}
	if plans == nil {
		plans = []SubscriptionPlanDTO{}
	}
	return plans, nil
}

// BuyProductWithPoints handles idempotent shop item purchase using points.
func (s *Service) BuyProductWithPoints(ctx context.Context, userID, productID uuid.UUID, quantity int32, idempotencyKey string) (*BuyProductResult, error) {
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

	idempotencyUUID, parseErr := uuid.Parse(idempotencyKey)

	// Idempotency check: if already processed for this idempotency key, return cached result
	if parseErr == nil && idempotencyUUID != uuid.Nil {
		var existingLedgerID uuid.UUID
		var existingBalance int64
		err := s.pool.QueryRow(ctx, `
			SELECT pl.id, u.points_balance
			FROM point_ledgers pl
			JOIN users u ON pl.user_id = u.id
			WHERE pl.user_id = $1 AND pl.reference_type = 'shop_buy' AND pl.reference_id = $2
			LIMIT 1
		`, userID, idempotencyUUID).Scan(&existingLedgerID, &existingBalance)
		if err == nil {
			inv, err := s.store.GetUserInventoryItem(ctx, store.GetUserInventoryItemParams{
				UserID: userID,
				ID:     productID,
			})
			invID := uuid.Nil
			if err == nil {
				invID = inv.ID
			}
			return &BuyProductResult{
				InventoryID:            invID,
				SkuCode:                prod.SkuCode,
				RemainingPointsBalance: existingBalance,
			}, nil
		}
	}

	totalPoints := int64(prod.PricePoints) * int64(quantity)

	var invID uuid.UUID
	var finalBalance int64

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		// 1. Lock and deduct points
		_, err := txStore.LockUserPoints(ctx, store.LockUserPointsParams{
			ID:                  userID,
			LockedPointsBalance: totalPoints,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInsufficientPoints
			}
			return fmt.Errorf("failed to lock user points: %w", err)
		}

		updatedUser, err := txStore.SpendUserPoints(ctx, store.SpendUserPointsParams{
			ID:            userID,
			PointsBalance: totalPoints,
		})
		if err != nil {
			return fmt.Errorf("failed to deduct points: %w", err)
		}
		finalBalance = updatedUser.PointsBalance

		// 2. Record ledger entry
		ledgerID, _ := uuid.NewV7()
		refID := productID
		if parseErr == nil && idempotencyUUID != uuid.Nil {
			refID = idempotencyUUID
		}

		_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
			ID:              ledgerID,
			UserID:          userID,
			TransactionType: store.PointTxTypeSpendDonation,
			Amount:          -totalPoints,
			BalanceAfter:    updatedUser.PointsBalance,
			ReferenceType:   "shop_buy",
			ReferenceID:     refID,
			Description:     fmt.Sprintf("Purchased %dx %s with points", quantity, prod.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to record point ledger: %w", err)
		}

		// 3. Upsert inventory item
		generatedInvID, _ := uuid.NewV7()
		invRow, err := txStore.UpsertUserInventoryItem(ctx, store.UpsertUserInventoryItemParams{
			ID:        generatedInvID,
			UserID:    userID,
			ProductID: productID,
			Quantity:  quantity,
		})
		if err != nil {
			return fmt.Errorf("failed to credit inventory: %w", err)
		}
		invID = invRow.ID

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &BuyProductResult{
		InventoryID:            invID,
		SkuCode:                prod.SkuCode,
		RemainingPointsBalance: finalBalance,
	}, nil
}

// ListUserInventory returns all items currently owned by the user.
func (s *Service) ListUserInventory(ctx context.Context, userID uuid.UUID) ([]store.ListUserInventoryRow, error) {
	return s.store.ListUserInventory(ctx, userID)
}

// ConsumeStreakFreeze consumes 1 streak freeze from inventory and applies protection for today.
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

// SubscribePlan creates a recurring maintenance subscription with payment authorization URL.
func (s *Service) SubscribePlan(ctx context.Context, in CreateSubscriptionInput) (*SubscriptionDetailDTO, error) {
	if s.isProduction {
		return nil, ErrSubscriptionNotAvailableInProduction
	}

	if in.UnitQuantity <= 0 {
		in.UnitQuantity = 1
	}

	plan, err := s.store.GetSubscriptionPlanByID(ctx, store.GetSubscriptionPlanByIDParams{
		ID:           in.PlanID,
		CurrencyCode: "USD",
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPlanNotFound
		}
		return nil, fmt.Errorf("failed to get subscription plan: %w", err)
	}

	if in.ProjectID != uuid.Nil {
		_, err := s.store.GetConservationProjectByID(ctx, in.ProjectID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrProjectNotFound
			}
			return nil, fmt.Errorf("failed to get conservation project: %w", err)
		}
	}

	subID, _ := uuid.NewV7()
	nextBilling := time.Now().UTC().AddDate(0, 1, 0)
	if plan.BillingInterval == store.SubscriptionFrequencyWeekly {
		nextBilling = time.Now().UTC().AddDate(0, 0, 7)
	} else if plan.BillingInterval == store.SubscriptionFrequencyYearly {
		nextBilling = time.Now().UTC().AddDate(1, 0, 0)
	}
	nextBillingDate := pgtype.Date{
		Time:  time.Date(nextBilling.Year(), nextBilling.Month(), nextBilling.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}

	projectUUID := pgtype.UUID{Valid: false}
	if in.ProjectID != uuid.Nil {
		projectUUID = pgtype.UUID{Bytes: in.ProjectID, Valid: true}
	}

	sub, err := s.store.CreateDonationSubscription(ctx, store.CreateDonationSubscriptionParams{
		ID:              subID,
		UserID:          in.UserID,
		PlanID:          pgtype.UUID{Bytes: in.PlanID, Valid: true},
		ProjectID:       projectUUID,
		Frequency:       plan.BillingInterval,
		DurationMonths:  plan.DurationMonths,
		UnitQuantity:    in.UnitQuantity,
		CurrencyCode:    "USD",
		AmountPerCycle:  plan.PriceMoney,
		PaymentMethod:   in.PaymentMethodType,
		PaymentProvider: "xendit",
		Status:          store.SubscriptionStatusActive,
		NextBillingDate: nextBillingDate,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	return &SubscriptionDetailDTO{
		ID:               sub.ID,
		Status:           sub.Status,
		PlanName:         plan.Name,
		BillingInterval:  plan.BillingInterval,
		NextBillingDate:  nextBillingDate.Time,
		AuthorizationURL: nil,
		CreatedAt:        sub.CreatedAt.Time,
	}, nil
}

// ListUserSubscriptions returns user subscriptions with joined plan name and frequency.
func (s *Service) ListUserSubscriptions(ctx context.Context, userID uuid.UUID) ([]SubscriptionDetailDTO, error) {
	rows, err := s.store.ListUserSubscriptions(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}

	dtos := make([]SubscriptionDetailDTO, 0, len(rows))
	for _, row := range rows {
		dtos = append(dtos, SubscriptionDetailDTO{
			ID:               row.ID,
			Status:           row.Status,
			PlanName:         row.PlanName.String,
			BillingInterval:  row.Frequency,
			NextBillingDate:  row.NextBillingDate.Time,
			AuthorizationURL: nil,
			CreatedAt:        row.CreatedAt.Time,
		})
	}
	return dtos, nil
}

// VerifyIAPInput holds payload for app store receipt validation.
type VerifyIAPInput struct {
	UserID         uuid.UUID
	ProductID      uuid.UUID
	StoreProvider  string
	ReceiptPayload string
}

// VerifyIAPReceipt validates mobile app store purchases with replay protection and production gating.
func (s *Service) VerifyIAPReceipt(ctx context.Context, in VerifyIAPInput) error {
	if s.isProduction {
		return ErrIAPNotConfiguredInProduction
	}

	prod, err := s.store.GetDigitalProductByID(ctx, in.ProductID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProductNotFound
		}
		return fmt.Errorf("failed to find product: %w", err)
	}

	var priceMoney decimal.Decimal = decimal.NewFromFloat(0.99)
	var grossNumeric pgtype.Numeric
	err = s.pool.QueryRow(ctx, "SELECT amount FROM product_prices WHERE product_id = $1 AND currency_code = 'USD'", in.ProductID).Scan(&grossNumeric)
	if err == nil {
		priceMoney = decimalutil.FromNumeric(grossNumeric)
	}

	storeFee := priceMoney.Mul(decimal.NewFromFloat(0.15))
	netRev := priceMoney.Sub(storeFee)

	h := sha256.Sum256([]byte(in.StoreProvider + ":" + in.ReceiptPayload))
	txID := hex.EncodeToString(h[:])

	// Prevent duplicate replay
	_, err = s.store.GetIAPTransactionByStoreTxID(ctx, store.GetIAPTransactionByStoreTxIDParams{
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
			Environment:        "sandbox",
			ReceiptPayload:     in.ReceiptPayload,
			CurrencyCode:       "USD",
			GrossAmount:        decimalutil.ToNumeric(priceMoney),
			StoreFeeAmount:     decimalutil.ToNumeric(storeFee),
			NetRevenueAmount:   decimalutil.ToNumeric(netRev),
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

		_ = prod
		return nil
	})
}

// PauseSubscription pauses an active subscription with state validation.
func (s *Service) PauseSubscription(ctx context.Context, userID, subID uuid.UUID, reason string) (*SubscriptionDetailDTO, error) {
	var currentStatus store.SubscriptionStatus
	var planName string
	var freq store.SubscriptionFrequency
	var nextBilling pgtype.Date
	var createdAt pgtype.Timestamptz

	err := s.pool.QueryRow(ctx, `
		SELECT ds.status, sp.name, ds.frequency, ds.next_billing_date, ds.created_at
		FROM donation_subscriptions ds
		LEFT JOIN subscription_plans sp ON ds.plan_id = sp.id
		WHERE ds.id = $1 AND ds.user_id = $2
	`, subID, userID).Scan(&currentStatus, &planName, &freq, &nextBilling, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to query subscription: %w", err)
	}

	if currentStatus != store.SubscriptionStatusActive {
		return nil, ErrInvalidSubscriptionState
	}

	sub, err := s.store.UpdateSubscriptionStatus(ctx, store.UpdateSubscriptionStatusParams{
		ID:          subID,
		UserID:      userID,
		Status:      store.SubscriptionStatusPaused,
		PausedAt:    pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		PauseReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to pause subscription: %w", err)
	}

	return &SubscriptionDetailDTO{
		ID:              sub.ID,
		Status:          sub.Status,
		PlanName:        planName,
		BillingInterval: freq,
		NextBillingDate: nextBilling.Time,
		CreatedAt:       createdAt.Time,
	}, nil
}

// ResumeSubscription reactivates a paused subscription with state validation.
func (s *Service) ResumeSubscription(ctx context.Context, userID, subID uuid.UUID) (*SubscriptionDetailDTO, error) {
	var currentStatus store.SubscriptionStatus
	var planName string
	var freq store.SubscriptionFrequency
	var nextBilling pgtype.Date
	var createdAt pgtype.Timestamptz

	err := s.pool.QueryRow(ctx, `
		SELECT ds.status, sp.name, ds.frequency, ds.next_billing_date, ds.created_at
		FROM donation_subscriptions ds
		LEFT JOIN subscription_plans sp ON ds.plan_id = sp.id
		WHERE ds.id = $1 AND ds.user_id = $2
	`, subID, userID).Scan(&currentStatus, &planName, &freq, &nextBilling, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to query subscription: %w", err)
	}

	if currentStatus != store.SubscriptionStatusPaused {
		return nil, ErrInvalidSubscriptionState
	}

	sub, err := s.store.UpdateSubscriptionStatus(ctx, store.UpdateSubscriptionStatusParams{
		ID:          subID,
		UserID:      userID,
		Status:      store.SubscriptionStatusActive,
		PausedAt:    pgtype.Timestamptz{Valid: false},
		PauseReason: pgtype.Text{Valid: false},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to resume subscription: %w", err)
	}

	return &SubscriptionDetailDTO{
		ID:              sub.ID,
		Status:          sub.Status,
		PlanName:        planName,
		BillingInterval: freq,
		NextBillingDate: nextBilling.Time,
		CreatedAt:       createdAt.Time,
	}, nil
}

// CancelSubscription cancels an active or paused subscription with state validation.
func (s *Service) CancelSubscription(ctx context.Context, userID, subID uuid.UUID, reason string) (*SubscriptionDetailDTO, error) {
	var currentStatus store.SubscriptionStatus
	var planName string
	var freq store.SubscriptionFrequency
	var nextBilling pgtype.Date
	var createdAt pgtype.Timestamptz

	err := s.pool.QueryRow(ctx, `
		SELECT ds.status, sp.name, ds.frequency, ds.next_billing_date, ds.created_at
		FROM donation_subscriptions ds
		LEFT JOIN subscription_plans sp ON ds.plan_id = sp.id
		WHERE ds.id = $1 AND ds.user_id = $2
	`, subID, userID).Scan(&currentStatus, &planName, &freq, &nextBilling, &createdAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to query subscription: %w", err)
	}

	if currentStatus == store.SubscriptionStatusCancelled {
		return &SubscriptionDetailDTO{
			ID:              subID,
			Status:          currentStatus,
			PlanName:        planName,
			BillingInterval: freq,
			NextBillingDate: nextBilling.Time,
			CreatedAt:       createdAt.Time,
		}, nil
	}

	if currentStatus != store.SubscriptionStatusActive && currentStatus != store.SubscriptionStatusPaused {
		return nil, ErrInvalidSubscriptionState
	}

	sub, err := s.store.UpdateSubscriptionStatus(ctx, store.UpdateSubscriptionStatusParams{
		ID:          subID,
		UserID:      userID,
		Status:      store.SubscriptionStatusCancelled,
		PausedAt:    pgtype.Timestamptz{Valid: false},
		PauseReason: pgtype.Text{String: reason, Valid: reason != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to cancel subscription: %w", err)
	}

	return &SubscriptionDetailDTO{
		ID:              sub.ID,
		Status:          sub.Status,
		PlanName:        planName,
		BillingInterval: freq,
		NextBillingDate: nextBilling.Time,
		CreatedAt:       createdAt.Time,
	}, nil
}

// CreateProduct creates a digital product.
func (s *Service) CreateProduct(ctx context.Context, sku, name, desc string, itemType store.DigitalItemType, points int64, isIAP bool, assetURL string) (*store.DigitalProducts, error) {
	pID, _ := uuid.NewV7()
	p, err := s.store.CreateDigitalProduct(ctx, store.CreateDigitalProductParams{
		ID:              pID,
		SkuCode:         sku,
		Name:            name,
		Description:     desc,
		ItemType:        itemType,
		PricePoints:     points,
		IsIapOnly:       isIAP,
		Attributes:      []byte("{}"),
		PreviewAssetUrl: assetURL,
		IsActive:        true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create digital product: %w", err)
	}
	return &p, nil
}

// UpdateProduct updates digital product details.
func (s *Service) UpdateProduct(ctx context.Context, id uuid.UUID, name, desc string, points int64, isActive bool) (*store.DigitalProducts, error) {
	p, err := s.store.UpdateDigitalProduct(ctx, store.UpdateDigitalProductParams{
		ID:          id,
		Name:        name,
		Description: desc,
		PricePoints: points,
		IsActive:    isActive,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update digital product: %w", err)
	}
	return &p, nil
}

// CreatePlan creates a subscription plan.
func (s *Service) CreatePlan(ctx context.Context, sku, group, name, desc string, planType store.SubscriptionPlanType, interval store.SubscriptionFrequency, intervalCount int32, durationMonths *int32, benefits []byte) (*store.SubscriptionPlans, error) {
	pID, _ := uuid.NewV7()
	if benefits == nil {
		benefits = []byte("{}")
	}
	var dur pgtype.Int4
	if durationMonths != nil {
		dur = pgtype.Int4{Int32: *durationMonths, Valid: true}
	}

	p, err := s.store.CreateSubscriptionPlan(ctx, store.CreateSubscriptionPlanParams{
		ID:              pID,
		SkuCode:         sku,
		GroupCode:       group,
		Name:            name,
		Description:     desc,
		PlanType:        planType,
		BillingInterval: interval,
		IntervalCount:   intervalCount,
		DurationMonths:  dur,
		Benefits:        benefits,
		IsActive:        true,
		SortOrder:       0,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription plan: %w", err)
	}
	return &p, nil
}

// UpdatePlan updates subscription plan details.
func (s *Service) UpdatePlan(ctx context.Context, id uuid.UUID, name, desc string, isActive bool) (*store.SubscriptionPlans, error) {
	p, err := s.store.UpdateSubscriptionPlan(ctx, store.UpdateSubscriptionPlanParams{
		ID:          id,
		Name:        name,
		Description: desc,
		IsActive:    isActive,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription plan: %w", err)
	}
	return &p, nil
}
