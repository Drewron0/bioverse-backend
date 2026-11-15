package donation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/conservation"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/payment"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	// ErrCoralFirstGatingFailed indicates Bio-Shield purchase attempted without owning an active coral.
	ErrCoralFirstGatingFailed = errors.New("coral-first gating: user must adopt at least one coral fragment before unlocking bio-shield")
	// ErrDonationNotFound indicates donation ID not found.
	ErrDonationNotFound = errors.New("donation record not found")
	// ErrInsufficientPoints indicates user does not have enough points.
	ErrInsufficientPoints = errors.New("insufficient points balance")
	// ErrIdempotencyKeyReused indicates idempotency key was previously used with different parameters.
	ErrIdempotencyKeyReused = errors.New("idempotency key reused with different parameters")
	// ErrAmountMismatch indicates payment amount reported by gateway doesn't match donation.
	ErrAmountMismatch = errors.New("settlement amount mismatch between webhook and donation record")
)

// CheckoutInput represents input payload for creating a donation.
type CheckoutInput struct {
	IdempotencyKey   string
	UserID           uuid.UUID
	DonationType     store.DonationType
	ProjectID        *uuid.UUID
	UnitQuantity     int
	IncludeBioShield bool
	TargetPetID      *uuid.UUID
	CoralNickname    string
	SponsoredQuotaID *uuid.UUID
	PointsToUse      int64
	PaymentMethod    store.PaymentMethodType
	BankCode         string
	IsAnonymous      bool
}

// DonationResult represents checkout output returned to the client.
type DonationResult struct {
	DonationID         uuid.UUID
	IdempotencyKey     string
	DonationType       store.DonationType
	GrossProjectValue  decimal.Decimal
	MoneyAmountPaid    decimal.Decimal
	CurrencyCode       string
	SettlementAmount   decimal.Decimal
	SettlementCurrency string
	FxRateSnapshot     decimal.Decimal
	PointsUsed         int64
	PointsDiscount     decimal.Decimal
	PaymentMethod      store.PaymentMethodType
	QRISPayload        string
	VANumber           string
	BankCode           string
	PaymentStatus      store.PaymentStatus
	ExpiredAt          time.Time
}

// Service owns donation business rules, point locking, and allocation orchestration.
type Service struct {
	pool                *pgxpool.Pool
	store               store.Querier
	conservationService *conservation.Service
	gateway             payment.Gateway
}

// NewService constructs a new donation service.
func NewService(pool *pgxpool.Pool, q store.Querier, consSvc *conservation.Service, gw payment.Gateway) *Service {
	if gw == nil {
		gw = payment.NewXenditClient("", nil)
	}
	return &Service{
		pool:                pool,
		store:               q,
		conservationService: consSvc,
		gateway:             gw,
	}
}

// Checkout creates a pending donation, locks points, and generates payment simulation details.
func (s *Service) Checkout(ctx context.Context, in CheckoutInput) (*DonationResult, error) {
	if in.UnitQuantity <= 0 {
		return nil, errors.New("unit quantity must be at least 1")
	}
	if in.IdempotencyKey == "" {
		return nil, errors.New("idempotency key is required")
	}

	existingDonation, err := s.store.GetDonationByIdempotencyKey(ctx, in.IdempotencyKey)
	if err == nil {
		if existingDonation.UserID != in.UserID || int(existingDonation.UnitQuantity) != in.UnitQuantity {
			return nil, ErrIdempotencyKeyReused
		}
		return &DonationResult{
			DonationID:         existingDonation.ID,
			IdempotencyKey:     existingDonation.IdempotencyKey,
			GrossProjectValue:  decimalutil.FromNumeric(existingDonation.GrossProjectValue),
			MoneyAmountPaid:    decimalutil.FromNumeric(existingDonation.MoneyAmountPaid),
			CurrencyCode:       existingDonation.CurrencyCode,
			SettlementAmount:   decimalutil.FromNumeric(existingDonation.SettlementAmount),
			SettlementCurrency: existingDonation.SettlementCurrencyCode,
			PointsUsed:         existingDonation.PointsUsed,
			PointsDiscount:     decimalutil.FromNumeric(existingDonation.PointsMonetaryEquivalent),
			PaymentMethod:      existingDonation.MoneyPaymentMethod,
			QRISPayload:        existingDonation.QrisPayload.String,
			VANumber:           existingDonation.VaNumber.String,
			BankCode:           existingDonation.BankCode.String,
			PaymentStatus:      existingDonation.PaymentStatus,
			ExpiredAt:          existingDonation.ExpiredAt.Time,
		}, nil
	}

	donationType := in.DonationType
	if string(donationType) == "" {
		donationType = store.DonationTypeCoralAdoption
	}

	var grossValue decimal.Decimal
	var projectID uuid.UUID
	var projectTitle string

	if donationType == store.DonationTypeBioShieldAddon {
		activeCoralsCount, err := s.store.CountActiveCoralFragmentsByUserID(ctx, pgtype.UUID{Bytes: in.UserID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("failed to verify active coral count: %w", err)
		}
		if activeCoralsCount <= 0 {
			return nil, ErrCoralFirstGatingFailed
		}
		grossValue = decimal.NewFromFloat(1.80).Mul(decimal.NewFromInt(int64(in.UnitQuantity)))

		if in.ProjectID != nil {
			projectID = *in.ProjectID
		} else {
			myCorals, _ := s.store.ListCoralFragmentsByUserID(ctx, pgtype.UUID{Bytes: in.UserID, Valid: true})
			if len(myCorals) > 0 {
				projectID = myCorals[0].ProjectID
			}
		}
		projectTitle = "Bio-Shield XP Protection Add-on"
	} else {
		if in.ProjectID == nil {
			return nil, errors.New("project_id is required for adoption")
		}
		projectID = *in.ProjectID

		project, err := s.store.GetConservationProjectByID(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("invalid conservation project: %w", err)
		}
		projectTitle = project.Title

		if in.IncludeBioShield {
			activeCoralsCount, err := s.store.CountActiveCoralFragmentsByUserID(ctx, pgtype.UUID{Bytes: in.UserID, Valid: true})
			if err != nil {
				return nil, fmt.Errorf("failed to verify active coral count: %w", err)
			}
			if activeCoralsCount <= 0 {
				return nil, ErrCoralFirstGatingFailed
			}
		}

		coralPricePerUnit := decimalutil.FromNumeric(project.PricePerUnitMoney)
		coralTotal := coralPricePerUnit.Mul(decimal.NewFromInt(int64(in.UnitQuantity)))

		bioShieldPrice := decimal.Zero
		if in.IncludeBioShield {
			bioShieldPrice = decimal.NewFromFloat(1.80)
		}
		grossValue = coralTotal.Add(bioShieldPrice)
	}

	pointsRate := decimal.NewFromFloat(0.01) // 100 points = $1.00 USD
	pointsDiscount := decimal.Zero
	if in.PointsToUse > 0 {
		pointsDiscount = decimal.NewFromInt(in.PointsToUse).Mul(pointsRate)
		if pointsDiscount.GreaterThan(grossValue) {
			pointsDiscount = grossValue
		}
	}

	moneyAmountPaid := grossValue.Sub(pointsDiscount)
	if moneyAmountPaid.IsNegative() {
		moneyAmountPaid = decimal.Zero
	}

	exchangeRate := decimal.NewFromInt(16000)
	settlementAmount := moneyAmountPaid.Mul(exchangeRate).Round(0)

	donationID, _ := uuid.NewV7()
	expiredAt := time.Now().UTC().Add(24 * time.Hour)

	var qrisPayload string
	var vaNumber string
	var bankCode string
	var externalRef string

	if moneyAmountPaid.IsPositive() {
		user, _ := s.store.GetUserByID(ctx, in.UserID)
		inv, err := s.gateway.CreateInvoice(ctx, donationID.String(), settlementAmount, fmt.Sprintf("BioVerse: %s", projectTitle), user.Email)
		if err == nil && inv != nil {
			externalRef = inv.ID
			if inv.QRISPayload != "" {
				qrisPayload = inv.QRISPayload
			}
			if inv.VANumber != "" {
				vaNumber = inv.VANumber
				bankCode = "MANDIRI"
			}
		}
	}

	if qrisPayload == "" && in.PaymentMethod == store.PaymentMethodTypeQris {
		qrisPayload = fmt.Sprintf("00020101021226580016ID.CO.XENDIT.WWW01189360091800000000005204581253033605802ID5914BIOVERSE REEF6007DENPASAR62070703A016304%s", donationID.String()[:4])
	}
	if vaNumber == "" && in.PaymentMethod != store.PaymentMethodTypeQris {
		bankCode = "MANDIRI"
		vaNumber = fmt.Sprintf("88908%d", time.Now().Unix()%100000000)
	}

	if in.IncludeBioShield {
		donationType = store.DonationTypeBioShieldAddon
	}

	var createdDonation store.Donations

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		if in.PointsToUse > 0 {
			_, err := txStore.LockUserPoints(ctx, store.LockUserPointsParams{
				ID:                  in.UserID,
				LockedPointsBalance: in.PointsToUse,
			})
			if err != nil {
				return ErrInsufficientPoints
			}

			ledgerID, _ := uuid.NewV7()
			user, _ := txStore.GetUserByID(ctx, in.UserID)
			_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
				ID:              ledgerID,
				UserID:          in.UserID,
				TransactionType: store.PointTxTypeLockDonation,
				Amount:          in.PointsToUse,
				BalanceAfter:    user.PointsBalance,
				ReferenceType:   "donations",
				ReferenceID:     donationID,
				Description:     "Lock points for pending donation checkout",
			})
			if err != nil {
				return fmt.Errorf("failed to record point lock ledger: %w", err)
			}
		}

		var sponsoredQuotaUUID pgtype.UUID
		if in.SponsoredQuotaID != nil {
			sponsoredQuotaUUID = pgtype.UUID{Bytes: *in.SponsoredQuotaID, Valid: true}
		}

		d, err := txStore.CreateDonation(ctx, store.CreateDonationParams{
			ID:                       donationID,
			IdempotencyKey:           in.IdempotencyKey,
			UserID:                   in.UserID,
			ProjectID:                projectID,
			DonationType:             donationType,
			IsAnonymous:              in.IsAnonymous,
			UnitQuantity:             int32(in.UnitQuantity),
			CurrencyCode:             "USD",
			GrossProjectValue:        decimalutil.ToNumeric(grossValue),
			MoneyAmountPaid:          decimalutil.ToNumeric(moneyAmountPaid),
			SettlementCurrencyCode:   "IDR",
			SettlementAmount:         decimalutil.ToNumeric(settlementAmount),
			FxRateSnapshot:           decimalutil.ToNumeric(exchangeRate),
			GatewayFeeSettlement:     decimalutil.ToNumeric(decimal.Zero),
			MoneyPaymentMethod:       in.PaymentMethod,
			PaymentProvider:          "xendit",
			IsMixedPayment:           in.PointsToUse > 0 && moneyAmountPaid.IsPositive(),
			BankCode:                 pgtype.Text{String: bankCode, Valid: bankCode != ""},
			VaNumber:                 pgtype.Text{String: vaNumber, Valid: vaNumber != ""},
			QrisPayload:              pgtype.Text{String: qrisPayload, Valid: qrisPayload != ""},
			PointsUsed:               in.PointsToUse,
			PointsRateSnapshot:       decimalutil.ToNumeric(pointsRate),
			PointsMonetaryEquivalent: decimalutil.ToNumeric(pointsDiscount),
			IsPointsLocked:           in.PointsToUse > 0,
			PaymentStatus:            store.PaymentStatusPending,
			ExternalGatewayRef:       pgtype.Text{String: externalRef, Valid: externalRef != ""},
			ExpiredAt:                pgtype.Timestamptz{Time: expiredAt, Valid: true},
			SponsoredQuotaID:         sponsoredQuotaUUID,
		})
		if err != nil {
			return fmt.Errorf("failed to insert donation record: %w", err)
		}
		createdDonation = d
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("checkout transaction failed: %w", err)
	}

	return &DonationResult{
		DonationID:         createdDonation.ID,
		IdempotencyKey:     createdDonation.IdempotencyKey,
		DonationType:       createdDonation.DonationType,
		GrossProjectValue:  decimalutil.FromNumeric(createdDonation.GrossProjectValue),
		MoneyAmountPaid:    decimalutil.FromNumeric(createdDonation.MoneyAmountPaid),
		CurrencyCode:       createdDonation.CurrencyCode,
		SettlementAmount:   decimalutil.FromNumeric(createdDonation.SettlementAmount),
		SettlementCurrency: createdDonation.SettlementCurrencyCode,
		FxRateSnapshot:     exchangeRate,
		PointsUsed:         createdDonation.PointsUsed,
		PointsDiscount:     decimalutil.FromNumeric(createdDonation.PointsMonetaryEquivalent),
		PaymentMethod:      createdDonation.MoneyPaymentMethod,
		QRISPayload:        qrisPayload,
		VANumber:           vaNumber,
		BankCode:           bankCode,
		PaymentStatus:      createdDonation.PaymentStatus,
		ExpiredAt:          expiredAt,
	}, nil
}

// ProcessPaidDonation executes the atomic state transition when a donation is confirmed paid.
func (s *Service) ProcessPaidDonation(ctx context.Context, donationID uuid.UUID, targetPetID *uuid.UUID, coralNickname string) (*store.Donations, error) {
	donation, err := s.store.GetDonationByID(ctx, donationID)
	if err != nil {
		return nil, ErrDonationNotFound
	}
	if donation.PaymentStatus == store.PaymentStatusPaid {
		return &donation, nil
	}

	project, err := s.store.GetConservationProjectByID(ctx, donation.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	now := time.Now().UTC()
	var updatedDonation store.Donations

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		d, err := txStore.UpdateDonationPaymentStatus(ctx, store.UpdateDonationPaymentStatusParams{
			ID:                 donationID,
			PaymentStatus:      store.PaymentStatusPaid,
			PaidAt:             pgtype.Timestamptz{Time: now, Valid: true},
			ExternalGatewayRef: pgtype.Text{String: fmt.Sprintf("XND-SIM-%s", donationID.String()), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to mark donation as paid: %w", err)
		}
		updatedDonation = d

		if donation.IsPointsLocked && donation.PointsUsed > 0 {
			user, err := txStore.SpendUserPoints(ctx, store.SpendUserPointsParams{
				ID:            donation.UserID,
				PointsBalance: donation.PointsUsed,
			})
			if err != nil {
				return fmt.Errorf("failed to spend locked points: %w", err)
			}

			ledgerID, _ := uuid.NewV7()
			_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
				ID:              ledgerID,
				UserID:          donation.UserID,
				TransactionType: store.PointTxTypeSpendDonation,
				Amount:          -donation.PointsUsed,
				BalanceAfter:    user.PointsBalance,
				ReferenceType:   "donations",
				ReferenceID:     donationID,
				Description:     "Spent points on donation checkout",
			})
			if err != nil {
				return fmt.Errorf("failed to create point spend ledger entry: %w", err)
			}
		}

		_, err = s.conservationService.AllocateFragments(
			ctx,
			tx,
			project.ZoneID,
			project.ID,
			donation.UserID,
			donationID,
			int(donation.UnitQuantity),
			project.SpeciesOrTypeName,
			coralNickname,
		)
		if err != nil {
			return fmt.Errorf("failed to allocate coral fragments to spider frame: %w", err)
		}

		if donation.DonationType == store.DonationTypeBioShieldAddon && targetPetID != nil {
			boostID, _ := uuid.NewV7()
			_, err = txStore.CreatePetBoost(ctx, store.CreatePetBoostParams{
				ID:           boostID,
				UserID:       donation.UserID,
				PetID:        *targetPetID,
				DonationID:   pgtype.UUID{Bytes: donationID, Valid: true},
				BoostType:    store.PetBoostTypeBioShieldXp,
				XpMultiplier: decimalutil.ToNumeric(decimal.NewFromFloat(1.25)),
			})
			if err != nil {
				return fmt.Errorf("failed to apply bio-shield boost to pet: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("process paid donation transaction failed: %w", err)
	}

	return &updatedDonation, nil
}

// GetDonation retrieves a donation record by UUID.
func (s *Service) GetDonation(ctx context.Context, id uuid.UUID) (*store.Donations, error) {
	d, err := s.store.GetDonationByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrDonationNotFound
		}
		return nil, err
	}
	return &d, nil
}

// ListUserDonations lists all donations made by the user.
func (s *Service) ListUserDonations(ctx context.Context, userID uuid.UUID) ([]store.ListDonationsByUserIDRow, error) {
	return s.store.ListDonationsByUserID(ctx, userID)
}

// ListUserDonationsWithCursor returns paginated donation history with next cursor.
func (s *Service) ListUserDonationsWithCursor(ctx context.Context, userID uuid.UUID, cursor *time.Time, limit int) ([]store.ListDonationsByUserIDWithCursorRow, *string, error) {
	if limit <= 0 {
		limit = 10
	} else if limit > 50 {
		limit = 50
	}

	var cursorCreatedAt pgtype.Timestamptz
	if cursor != nil {
		cursorCreatedAt = pgtype.Timestamptz{Time: *cursor, Valid: true}
	}

	rows, err := s.store.ListDonationsByUserIDWithCursor(ctx, store.ListDonationsByUserIDWithCursorParams{
		UserID:          userID,
		Limit:           int32(limit),
		CursorCreatedAt: cursorCreatedAt,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list user donations with cursor: %w", err)
	}

	var nextCursor *string
	if len(rows) == limit {
		lastCreatedAt := rows[len(rows)-1].CreatedAt.Time.Format(time.RFC3339Nano)
		nextCursor = &lastCreatedAt
	}

	return rows, nextCursor, nil
}

// ProcessXenditInvoiceWebhook handles idempotency deduplication via webhook_events, nominal checking, and atomic execution.
func (s *Service) ProcessXenditInvoiceWebhook(ctx context.Context, externalEventID, externalID, status string, amount float64, rawPayload []byte) error {
	if !strings.HasPrefix(externalID, "donation_") {
		return nil
	}

	donationIDStr := strings.TrimPrefix(externalID, "donation_")
	donationID, err := uuid.Parse(donationIDStr)
	if err != nil {
		return fmt.Errorf("invalid donation UUID in external_id: %w", err)
	}

	if externalEventID == "" {
		externalEventID = fmt.Sprintf("%s_%s", externalID, strings.ToLower(status))
	}

	// 1. Check idempotency deduplication in webhook_events
	existingEvent, err := s.store.GetWebhookEventByProviderAndExternalID(ctx, store.GetWebhookEventByProviderAndExternalIDParams{
		Provider:        "xendit",
		ExternalEventID: externalEventID,
	})
	if err == nil {
		if existingEvent.Status == store.WebhookProcessStatusProcessed {
			// Already processed successfully: idempotent return
			return nil
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("failed to query webhook_events: %w", err)
	} else {
		// Insert initial event record
		eventID, _ := uuid.NewV7()
		_, _ = s.store.RecordWebhookEvent(ctx, store.RecordWebhookEventParams{
			ID:              eventID,
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			EventType:       "invoice." + strings.ToLower(status),
			Payload:         rawPayload,
			Status:          store.WebhookProcessStatusPending,
		})
	}

	if !strings.EqualFold(status, "PAID") {
		_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusIgnored,
			ProcessedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		return nil
	}

	// 2. Fetch donation and verify amount
	donation, err := s.store.GetDonationByID(ctx, donationID)
	if err != nil {
		_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusFailed,
			ErrorMessage:    pgtype.Text{String: "Donation record not found", Valid: true},
		})
		return ErrDonationNotFound
	}

	if donation.PaymentStatus == store.PaymentStatusPaid {
		_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusProcessed,
			ProcessedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		return nil
	}

	paidAmountDec := decimal.NewFromFloat(amount)
	expectedSettlement := decimalutil.FromNumeric(donation.SettlementAmount)
	expectedMoney := decimalutil.FromNumeric(donation.MoneyAmountPaid)

	// Check if payment amount matches either settlement amount or gross money paid
	if !paidAmountDec.Equal(expectedSettlement) && !paidAmountDec.Equal(expectedMoney) {
		errMsg := fmt.Sprintf("Amount mismatch: webhook reported %.2f, donation expects %s (settlement) or %s (money)",
			amount, expectedSettlement.String(), expectedMoney.String())
		_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusFailed,
			ErrorMessage:    pgtype.Text{String: errMsg, Valid: true},
		})
		return fmt.Errorf("%w: %s", ErrAmountMismatch, errMsg)
	}

	// 3. Mark donation as paid
	_, err = s.ProcessPaidDonation(ctx, donationID, nil, "")
	if err != nil {
		_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusFailed,
			ErrorMessage:    pgtype.Text{String: err.Error(), Valid: true},
		})
		return fmt.Errorf("failed to process paid donation state: %w", err)
	}

	_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
		Provider:        "xendit",
		ExternalEventID: externalEventID,
		Status:          store.WebhookProcessStatusProcessed,
		ProcessedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})

	return nil
}
