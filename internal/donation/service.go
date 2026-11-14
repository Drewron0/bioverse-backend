package donation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/conservation"
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
	// ErrCoralFirstGatingFailed indicates Bio-Shield purchase attempted without owning an active coral.
	ErrCoralFirstGatingFailed = errors.New("coral-first gating: user must adopt at least one coral fragment before unlocking bio-shield")
	// ErrDonationNotFound indicates donation ID not found.
	ErrDonationNotFound = errors.New("donation record not found")
	// ErrInsufficientPoints indicates user does not have enough points.
	ErrInsufficientPoints = errors.New("insufficient points balance")
)

// CheckoutInput represents input payload for creating a donation.
type CheckoutInput struct {
	IdempotencyKey     string
	UserID             uuid.UUID
	ProjectID          uuid.UUID
	UnitQuantity       int
	IncludeBioShield   bool
	TargetPetID        *uuid.UUID
	CoralNickname      string
	PointsToUse        int64
	PaymentMethod      store.PaymentMethodType
}

// DonationResult represents checkout output returned to the client.
type DonationResult struct {
	DonationID         uuid.UUID
	IdempotencyKey     string
	GrossProjectValue  decimal.Decimal
	MoneyAmountPaid    decimal.Decimal
	CurrencyCode       string
	SettlementAmount   decimal.Decimal
	SettlementCurrency string
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
}

// NewService constructs a new donation service.
func NewService(pool *pgxpool.Pool, q store.Querier, consSvc *conservation.Service) *Service {
	return &Service{
		pool:                pool,
		store:               q,
		conservationService: consSvc,
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

	project, err := s.store.GetConservationProjectByID(ctx, in.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid conservation project: %w", err)
	}

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

	grossValue := coralTotal.Add(bioShieldPrice)

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

	switch in.PaymentMethod {
	case store.PaymentMethodTypeQris:
		qrisPayload = fmt.Sprintf("00020101021226580016ID.CO.XENDIT.WWW01189360091800000000005204581253033605802ID5914BIOVERSE REEF6007DENPASAR62070703A016304%s", donationID.String()[:4])
	case store.PaymentMethodTypeVirtualAccount:
		bankCode = "MANDIRI"
		vaNumber = fmt.Sprintf("88908%d", time.Now().Unix()%100000000)
	default:
		bankCode = "MANDIRI"
		vaNumber = fmt.Sprintf("88908%d", time.Now().Unix()%100000000)
	}

	donationType := store.DonationTypeCoralAdoption
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

		d, err := txStore.CreateDonation(ctx, store.CreateDonationParams{
			ID:                       donationID,
			IdempotencyKey:           in.IdempotencyKey,
			UserID:                   in.UserID,
			ProjectID:                in.ProjectID,
			DonationType:             donationType,
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
			ExpiredAt:                pgtype.Timestamptz{Time: expiredAt, Valid: true},
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
		GrossProjectValue:  decimalutil.FromNumeric(createdDonation.GrossProjectValue),
		MoneyAmountPaid:    decimalutil.FromNumeric(createdDonation.MoneyAmountPaid),
		CurrencyCode:       createdDonation.CurrencyCode,
		SettlementAmount:   decimalutil.FromNumeric(createdDonation.SettlementAmount),
		SettlementCurrency: createdDonation.SettlementCurrencyCode,
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
			ExternalGatewayRef: pgtype.Text{String: fmt.Sprintf("XND-SIM-%s", donationID.String()[:8]), Valid: true},
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
