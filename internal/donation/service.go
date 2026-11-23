package donation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/conservation"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/email"
	"github.com/Drewron0/bioverse-backend/internal/platform/notification"
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
	// ErrVirtualPetNotFound indicates specified target pet was not found or does not belong to user.
	ErrVirtualPetNotFound = errors.New("target pet not found or does not belong to user")
	// ErrNoActiveVirtualPet indicates bio-shield addon requires at least one active virtual pet.
	ErrNoActiveVirtualPet = errors.New("no active virtual pet found to apply bio-shield boost")
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
	ProjectID          *uuid.UUID
	ProjectTitle       string
	UnitQuantity       int
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
	PaymentURL         string
	ExpiredAt          time.Time
}

// Service owns donation business rules, point locking, and allocation orchestration.
type Service struct {
	pool                *pgxpool.Pool
	store               store.Querier
	conservationService *conservation.Service
	gateway             payment.Gateway
	emailSender         email.Sender
	pushNotifier        notification.Dispatcher
}

// NewService constructs a new donation service.
func NewService(pool *pgxpool.Pool, q store.Querier, consSvc *conservation.Service, gw payment.Gateway, emailSender email.Sender, pushNotifier notification.Dispatcher) *Service {
	if gw == nil {
		gw = payment.NewXenditClient("", nil)
	}
	return &Service{
		pool:                pool,
		store:               q,
		conservationService: consSvc,
		gateway:             gw,
		emailSender:         emailSender,
		pushNotifier:        pushNotifier,
	}
}

// Checkout creates a pending donation or immediately finalizes points-only checkouts.
func (s *Service) Checkout(ctx context.Context, in CheckoutInput) (*DonationResult, error) {
	if in.UnitQuantity <= 0 {
		return nil, errors.New("unit quantity must be at least 1")
	}
	if in.IdempotencyKey == "" {
		return nil, errors.New("idempotency key is required")
	}

	donationType := store.DonationTypeCoralAdoption
	if in.ProjectID == nil {
		donationType = store.DonationTypeBioShieldAddon
	}

	existingDonation, err := s.store.GetDonationByIdempotencyKey(ctx, in.IdempotencyKey)
	if err == nil {
		if existingDonation.UserID != in.UserID ||
			int(existingDonation.UnitQuantity) != in.UnitQuantity ||
			existingDonation.DonationType != donationType ||
			existingDonation.MoneyPaymentMethod != in.PaymentMethod ||
			existingDonation.PointsUsed != in.PointsToUse ||
			(in.ProjectID != nil && existingDonation.ProjectID != *in.ProjectID) {
			return nil, ErrIdempotencyKeyReused
		}
		return &DonationResult{
			DonationID:         existingDonation.ID,
			IdempotencyKey:     existingDonation.IdempotencyKey,
			DonationType:       existingDonation.DonationType,
			GrossProjectValue:  decimalutil.FromNumeric(existingDonation.GrossProjectValue),
			MoneyAmountPaid:    decimalutil.FromNumeric(existingDonation.MoneyAmountPaid),
			CurrencyCode:       existingDonation.CurrencyCode,
			SettlementAmount:   decimalutil.FromNumeric(existingDonation.SettlementAmount),
			SettlementCurrency: existingDonation.SettlementCurrencyCode,
			FxRateSnapshot:     decimalutil.FromNumeric(existingDonation.FxRateSnapshot),
			PointsUsed:         existingDonation.PointsUsed,
			PointsDiscount:     decimalutil.FromNumeric(existingDonation.PointsMonetaryEquivalent),
			PaymentMethod:      existingDonation.MoneyPaymentMethod,
			QRISPayload:        existingDonation.QrisPayload.String,
			VANumber:           existingDonation.VaNumber.String,
			BankCode:           existingDonation.BankCode.String,
			PaymentStatus:      existingDonation.PaymentStatus,
			PaymentURL:         existingDonation.PaymentUrl.String,
			ExpiredAt:          existingDonation.ExpiredAt.Time,
		}, nil
	}

	if in.DonationType != "" {
		donationType = in.DonationType
	}

	var grossValue decimal.Decimal
	var projectID uuid.UUID
	var projectTitle string
	var projectZoneID uuid.UUID
	var projectSpecies string
	var coralUnitPrice decimal.Decimal

	var bioShieldPrice decimal.Decimal
	if donationType == store.DonationTypeBioShieldAddon || in.IncludeBioShield {
		price, err := s.getBioShieldPrice(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to determine bio-shield price: %w", err)
		}
		bioShieldPrice = price
	}

	if donationType == store.DonationTypeBioShieldAddon {
		activeCoralsCount, err := s.store.CountActiveCoralFragmentsByUserID(ctx, pgtype.UUID{Bytes: in.UserID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("failed to verify active coral count: %w", err)
		}
		if activeCoralsCount <= 0 {
			return nil, ErrCoralFirstGatingFailed
		}
		grossValue = bioShieldPrice.Mul(decimal.NewFromInt(int64(in.UnitQuantity)))

		if in.ProjectID != nil {
			projectID = *in.ProjectID
			p, err := s.store.GetConservationProjectByID(ctx, projectID)
			if err == nil {
				projectZoneID = p.ZoneID
				projectSpecies = p.SpeciesOrTypeName
			}
		} else {
			myCorals, _ := s.store.ListCoralFragmentsByUserID(ctx, pgtype.UUID{Bytes: in.UserID, Valid: true})
			if len(myCorals) > 0 {
				projectID = myCorals[0].ProjectID
				p, err := s.store.GetConservationProjectByID(ctx, projectID)
				if err == nil {
					projectZoneID = p.ZoneID
					projectSpecies = p.SpeciesOrTypeName
				}
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
		projectZoneID = project.ZoneID
		projectSpecies = project.SpeciesOrTypeName

		if in.DonationType == "" {
			switch project.Category {
			case "mangrove":
				donationType = store.DonationTypeMangrovePlanting
			case "tree":
				donationType = store.DonationTypeTreePlanting
			default:
				donationType = store.DonationTypeCoralAdoption
			}
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

		coralUnitPrice = decimalutil.FromNumeric(project.PricePerUnitMoney)
		coralTotal := coralUnitPrice.Mul(decimal.NewFromInt(int64(in.UnitQuantity)))

		addedBioShield := decimal.Zero
		if in.IncludeBioShield {
			addedBioShield = bioShieldPrice
		}
		grossValue = coralTotal.Add(addedBioShield)
	}

	var sponsoredQuotaUUID pgtype.UUID
	if in.SponsoredQuotaID != nil {
		quota, err := s.store.GetSponsoredAdoptionQuota(ctx, *in.SponsoredQuotaID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, errors.New("sponsored quota not found")
			}
			return nil, fmt.Errorf("failed to fetch sponsored quota: %w", err)
		}
		if quota.ProjectID != projectID {
			return nil, errors.New("sponsored quota does not match project")
		}
		if int(quota.TotalQuota-quota.UsedQuota) < in.UnitQuantity {
			return nil, errors.New("insufficient sponsored adoption quota")
		}
		sponsoredQuotaUUID = pgtype.UUID{Bytes: *in.SponsoredQuotaID, Valid: true}

		sponsoredDeduction := coralUnitPrice.Mul(decimal.NewFromInt(int64(in.UnitQuantity)))
		grossValue = grossValue.Sub(sponsoredDeduction)
		if grossValue.IsNegative() {
			grossValue = decimal.Zero
		}
	}

	// Validate target pet if bio-shield boost is involved
	if in.TargetPetID != nil {
		_, err := s.store.GetVirtualPetByID(ctx, store.GetVirtualPetByIDParams{
			ID:     *in.TargetPetID,
			UserID: in.UserID,
		})
		if err != nil {
			return nil, ErrVirtualPetNotFound
		}
	} else if donationType == store.DonationTypeBioShieldAddon {
		userPets, err := s.store.ListVirtualPetsByUserID(ctx, in.UserID)
		if err != nil || len(userPets) == 0 {
			return nil, ErrNoActiveVirtualPet
		}
		in.TargetPetID = &userPets[0].ID
	} else if in.IncludeBioShield {
		if userPets, err := s.store.ListVirtualPetsByUserID(ctx, in.UserID); err == nil && len(userPets) > 0 {
			in.TargetPetID = &userPets[0].ID
		}
	}

	var pointsRate decimal.Decimal
	pointsDiscount := decimal.Zero
	if in.PointsToUse > 0 {
		rate, err := s.getPointsRate(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to determine points rate: %w", err)
		}
		pointsRate = rate
		if pointsRate.IsPositive() && grossValue.IsPositive() {
			maxPointsNeeded := grossValue.Div(pointsRate).Ceil().IntPart()
			if in.PointsToUse > maxPointsNeeded {
				in.PointsToUse = maxPointsNeeded
			}
		} else if grossValue.IsZero() {
			in.PointsToUse = 0
		}
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
	if moneyAmountPaid.IsPositive() {
		rate, err := s.getUSDToIDRExchangeRate(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to determine USD to IDR exchange rate: %w", err)
		}
		exchangeRate = rate
	}
	settlementAmount := moneyAmountPaid.Mul(exchangeRate).Round(0)

	donationID, _ := uuid.NewV7()
	now := time.Now().UTC()
	expiredAt := now.Add(24 * time.Hour)

	var qrisPayload string
	var vaNumber string
	var bankCode string
	var externalRef string
	var paymentURL string

	isPointsOnly := moneyAmountPaid.IsZero()

	if !isPointsOnly {
		user, err := s.store.GetUserByID(ctx, in.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}
		externalID := fmt.Sprintf("donation_%s", donationID)
		inv, err := s.gateway.CreateInvoice(ctx, externalID, settlementAmount, fmt.Sprintf("BioVerse: %s", projectTitle), user.Email)
		if err != nil {
			return nil, fmt.Errorf("failed to create payment invoice: %w", err)
		}
		if inv != nil {
			externalRef = inv.ID
			paymentURL = inv.InvoiceURL
			if !inv.ExpiredAt.IsZero() {
				expiredAt = inv.ExpiredAt
			}
			if inv.QRISPayload != "" {
				qrisPayload = inv.QRISPayload
			}
			if inv.VANumber != "" {
				vaNumber = inv.VANumber
				bankCode = "MANDIRI"
			}
		}
	} else {
		externalRef = "POINTS-PAID"
	}

	var targetPetUUID pgtype.UUID
	if in.TargetPetID != nil {
		targetPetUUID = pgtype.UUID{Bytes: *in.TargetPetID, Valid: true}
	}
	var coralNicknameText pgtype.Text
	if in.CoralNickname != "" {
		coralNicknameText = pgtype.Text{String: in.CoralNickname, Valid: true}
	}
	var paymentURLText pgtype.Text
	if paymentURL != "" {
		paymentURLText = pgtype.Text{String: paymentURL, Valid: true}
	}

	var createdDonation store.Donations

	err = database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		if isPointsOnly {
			// Points-only donation: spend points directly and immediately fulfill
			if in.PointsToUse > 0 {
				_, err := txStore.LockUserPoints(ctx, store.LockUserPointsParams{
					ID:                  in.UserID,
					LockedPointsBalance: in.PointsToUse,
				})
				if err != nil {
					return ErrInsufficientPoints
				}

				user, err := txStore.SpendUserPoints(ctx, store.SpendUserPointsParams{
					ID:            in.UserID,
					PointsBalance: in.PointsToUse,
				})
				if err != nil {
					return ErrInsufficientPoints
				}

				ledgerID, _ := uuid.NewV7()
				_, err = txStore.CreatePointLedgerEntry(ctx, store.CreatePointLedgerEntryParams{
					ID:              ledgerID,
					UserID:          in.UserID,
					TransactionType: store.PointTxTypeSpendDonation,
					Amount:          -in.PointsToUse,
					BalanceAfter:    user.PointsBalance,
					ReferenceType:   "donations",
					ReferenceID:     donationID,
					Description:     "Spent points on direct donation checkout",
				})
				if err != nil {
					return fmt.Errorf("failed to record point spend ledger: %w", err)
				}
			}
		} else {
			// Gateway payment required: lock points pending payment
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
		}

		if in.SponsoredQuotaID != nil {
			for i := 0; i < in.UnitQuantity; i++ {
				_, err = txStore.ClaimSponsoredQuota(ctx, *in.SponsoredQuotaID)
				if err != nil {
					return fmt.Errorf("failed to claim sponsored quota: %w", err)
				}
			}
		}

		paymentStatus := store.PaymentStatusPending
		var paidAt pgtype.Timestamptz
		if isPointsOnly {
			paymentStatus = store.PaymentStatusPaid
			paidAt = pgtype.Timestamptz{Time: now, Valid: true}
		}

		d, err := txStore.CreateDonation(ctx, store.CreateDonationParams{
			ID:                       donationID,
			IdempotencyKey:           in.IdempotencyKey,
			UserID:                   in.UserID,
			ProjectID:                projectID,
			SubscriptionID:           pgtype.UUID{Valid: false},
			SponsoredQuotaID:         sponsoredQuotaUUID,
			CycleNumber:              pgtype.Int4{Valid: false},
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
			IsPointsLocked:           !isPointsOnly && in.PointsToUse > 0,
			PaymentStatus:            paymentStatus,
			ExternalGatewayRef:       pgtype.Text{String: externalRef, Valid: externalRef != ""},
			ExpiredAt:                pgtype.Timestamptz{Time: expiredAt, Valid: true},
			TargetPetID:              targetPetUUID,
			CoralNickname:            coralNicknameText,
			IncludeBioShield:         in.IncludeBioShield,
			PaymentUrl:               paymentURLText,
			PaidAt:                   paidAt,
		})
		if err != nil {
			return fmt.Errorf("failed to insert donation record: %w", err)
		}

		if isPointsOnly {
			// Fulfill adoption: allocate coral fragments if coral_adoption
			if donationType == store.DonationTypeCoralAdoption {
				_, err = s.conservationService.AllocateFragments(
					ctx,
					tx,
					projectZoneID,
					projectID,
					in.UserID,
					donationID,
					in.UnitQuantity,
					projectSpecies,
					in.CoralNickname,
				)
				if err != nil {
					return fmt.Errorf("failed to allocate coral fragments to spider frame: %w", err)
				}
			} else if donationType == store.DonationTypeMangrovePlanting || donationType == store.DonationTypeTreePlanting {
				_, err = txStore.IncrementProjectFundedUnits(ctx, store.IncrementProjectFundedUnitsParams{
					ID:                projectID,
					FundedUnitsCount: int32(in.UnitQuantity),
				})
				if err != nil {
					return fmt.Errorf("failed to increment project funded units: %w", err)
				}
			}

			// Fulfill Bio-Shield boost if requested
			if (in.IncludeBioShield || donationType == store.DonationTypeBioShieldAddon) && in.TargetPetID != nil {
				boostID, _ := uuid.NewV7()
				_, err = txStore.CreatePetBoost(ctx, store.CreatePetBoostParams{
					ID:           boostID,
					UserID:       in.UserID,
					PetID:        *in.TargetPetID,
					DonationID:   pgtype.UUID{Bytes: donationID, Valid: true},
					BoostType:    store.PetBoostTypeBioShieldXp,
					XpMultiplier: decimalutil.ToNumeric(decimal.NewFromFloat(1.25)),
				})
				if err != nil {
					return fmt.Errorf("failed to apply bio-shield boost to pet: %w", err)
				}
			}
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
		ProjectID:          &projectID,
		ProjectTitle:       projectTitle,
		UnitQuantity:       in.UnitQuantity,
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
		PaymentURL:         paymentURL,
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

	effectivePetID := targetPetID
	if effectivePetID == nil && donation.TargetPetID.Valid {
		petID := uuid.UUID(donation.TargetPetID.Bytes)
		effectivePetID = &petID
	}
	if effectivePetID == nil && (donation.IncludeBioShield || donation.DonationType == store.DonationTypeBioShieldAddon) {
		if pets, err := s.store.ListVirtualPetsByUserID(ctx, donation.UserID); err == nil && len(pets) > 0 {
			effectivePetID = &pets[0].ID
		}
	}
	effectiveNickname := coralNickname
	if effectiveNickname == "" && donation.CoralNickname.Valid {
		effectiveNickname = donation.CoralNickname.String
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

		if donation.DonationType == store.DonationTypeCoralAdoption {
			_, err = s.conservationService.AllocateFragments(
				ctx,
				tx,
				project.ZoneID,
				project.ID,
				donation.UserID,
				donationID,
				int(donation.UnitQuantity),
				project.SpeciesOrTypeName,
				effectiveNickname,
			)
			if err != nil {
				return fmt.Errorf("failed to allocate coral fragments to spider frame: %w", err)
			}
		} else if donation.DonationType == store.DonationTypeMangrovePlanting || donation.DonationType == store.DonationTypeTreePlanting {
			_, err = txStore.IncrementProjectFundedUnits(ctx, store.IncrementProjectFundedUnitsParams{
				ID:                project.ID,
				FundedUnitsCount: donation.UnitQuantity,
			})
			if err != nil {
				return fmt.Errorf("failed to increment project funded units: %w", err)
			}
		}

		if (donation.IncludeBioShield || donation.DonationType == store.DonationTypeBioShieldAddon) && effectivePetID != nil {
			boostID, _ := uuid.NewV7()
			_, err = txStore.CreatePetBoost(ctx, store.CreatePetBoostParams{
				ID:           boostID,
				UserID:       donation.UserID,
				PetID:        *effectivePetID,
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

// GetDonationDetail retrieves an enriched donation record with project metadata by UUID.
func (s *Service) GetDonationDetail(ctx context.Context, id uuid.UUID) (*store.GetDonationDetailByIDRow, error) {
	d, err := s.store.GetDonationDetailByID(ctx, id)
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

// ProcessXenditInvoiceWebhook handles idempotency deduplication via webhook_events, strict IDR settlement nominal verification, and atomic state transition in a single transaction.
func (s *Service) ProcessXenditInvoiceWebhook(ctx context.Context, externalEventID, externalID, status string, amount decimal.Decimal, rawPayload []byte) error {
	var donationIDStr string
	if strings.HasPrefix(externalID, "donation_") {
		donationIDStr = strings.TrimPrefix(externalID, "donation_")
	} else {
		donationIDStr = externalID
	}

	donationID, err := uuid.Parse(donationIDStr)
	if err != nil {
		// Not a valid donation UUID; ignore without failing Xendit
		return nil
	}

	if externalEventID == "" {
		externalEventID = fmt.Sprintf("%s_%s", externalID, strings.ToLower(status))
	}

	// 1. Record incoming webhook event outside of business tx so audit trail is NEVER rolled back
	eventID, _ := uuid.NewV7()
	createdEvent, err := s.store.RecordWebhookEvent(ctx, store.RecordWebhookEventParams{
		ID:              eventID,
		Provider:        "xendit",
		ExternalEventID: externalEventID,
		EventType:       "invoice." + strings.ToLower(status),
		Payload:         rawPayload,
		Status:          store.WebhookProcessStatusPending,
	})

	isDuplicate := false
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			isDuplicate = true
		} else {
			return fmt.Errorf("failed to record webhook event: %w", err)
		}
	} else if createdEvent.ID == uuid.Nil {
		isDuplicate = true
	}

	if isDuplicate {
		existing, err := s.store.GetWebhookEventByProviderAndExternalID(ctx, store.GetWebhookEventByProviderAndExternalIDParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
		})
		if err != nil {
			return fmt.Errorf("failed to lookup existing webhook event: %w", err)
		}
		if existing.Status == store.WebhookProcessStatusProcessed || existing.Status == store.WebhookProcessStatusIgnored {
			// Already processed or ignored successfully: idempotent return
			return nil
		}
	}

	if !strings.EqualFold(status, "PAID") {
		_, err = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusIgnored,
			ProcessedAt:     pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		})
		return err
	}

	// 2. Execute business fulfillment transaction atomically
	txErr := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		// Fetch donation
		donation, err := txStore.GetDonationByID(ctx, donationID)
		if err != nil {
			return ErrDonationNotFound
		}

		if donation.PaymentStatus != store.PaymentStatusPending {
			if donation.PaymentStatus == store.PaymentStatusPaid {
				return nil
			}
			return fmt.Errorf("donation cannot be fulfilled from terminal status %s", donation.PaymentStatus)
		}

		// Strict settlement amount verification (gateway invoices & settles strictly in IDR)
		expectedSettlement := decimalutil.FromNumeric(donation.SettlementAmount)
		if !amount.Equal(expectedSettlement) {
			errMsg := fmt.Sprintf("Settlement amount mismatch: gateway reported %s, donation expects %s IDR",
				amount.String(), expectedSettlement.String())
			return fmt.Errorf("%w: %s", ErrAmountMismatch, errMsg)
		}

		// Mark donation as paid
		now := time.Now().UTC()
		_, err = txStore.UpdateDonationPaymentStatus(ctx, store.UpdateDonationPaymentStatusParams{
			ID:                 donationID,
			PaymentStatus:      store.PaymentStatusPaid,
			PaidAt:             pgtype.Timestamptz{Time: now, Valid: true},
			ExternalGatewayRef: pgtype.Text{String: fmt.Sprintf("XND-INV-%s", externalEventID), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update donation payment status: %w", err)
		}

		if donation.IsPointsLocked && donation.PointsUsed > 0 {
			user, err := txStore.SpendUserPoints(ctx, store.SpendUserPointsParams{
				ID:            donation.UserID,
				PointsBalance: donation.PointsUsed,
			})
			if err != nil {
				return fmt.Errorf("failed to spend user points: %w", err)
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
				Description:     "Spent points on confirmed donation",
			})
			if err != nil {
				return fmt.Errorf("failed to create point spend ledger entry: %w", err)
			}
		}

		project, err := txStore.GetConservationProjectByID(ctx, donation.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get conservation project: %w", err)
		}

		if donation.DonationType == store.DonationTypeCoralAdoption {
			var coralNickname string
			if donation.CoralNickname.Valid {
				coralNickname = donation.CoralNickname.String
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
				return fmt.Errorf("failed to allocate fragments: %w", err)
			}
		}

		if donation.IncludeBioShield || donation.DonationType == store.DonationTypeBioShieldAddon {
			var petID *uuid.UUID
			if donation.TargetPetID.Valid {
				p := uuid.UUID(donation.TargetPetID.Bytes)
				petID = &p
			} else {
				if userPets, err := txStore.ListVirtualPetsByUserID(ctx, donation.UserID); err == nil && len(userPets) > 0 {
					petID = &userPets[0].ID
				}
			}

			if petID != nil {
				boostID, _ := uuid.NewV7()
				_, err = txStore.CreatePetBoost(ctx, store.CreatePetBoostParams{
					ID:           boostID,
					UserID:       donation.UserID,
					PetID:        *petID,
					DonationID:   pgtype.UUID{Bytes: donationID, Valid: true},
					BoostType:    store.PetBoostTypeBioShieldXp,
					XpMultiplier: decimalutil.ToNumeric(decimal.NewFromFloat(1.25)),
				})
				if err != nil {
					return fmt.Errorf("failed to apply bio-shield boost to pet: %w", err)
				}
			}
		}

		// Optional notification dispatch after commit
		if s.emailSender != nil {
			donor, err := txStore.GetUserByID(ctx, donation.UserID)
			if err == nil {
				_ = s.emailSender.Send(ctx, donor.Email, "Donation Confirmed - BioVerse",
					fmt.Sprintf("Thank you %s! Your donation for %d units in %s has been received.",
						donor.FullName, donation.UnitQuantity, project.Title))
			}
		}

		return nil
	})

	now := time.Now().UTC()
	if txErr != nil {
		// Preserve audit trail in webhook_events even when business tx failed/rolled back
		_, _ = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
			Provider:        "xendit",
			ExternalEventID: externalEventID,
			Status:          store.WebhookProcessStatusFailed,
			ErrorMessage:    pgtype.Text{String: txErr.Error(), Valid: true},
			ProcessedAt:     pgtype.Timestamptz{Time: now, Valid: true},
		})
		return txErr
	}

	// 3. Mark webhook event as processed
	_, err = s.store.UpdateWebhookEventStatus(ctx, store.UpdateWebhookEventStatusParams{
		Provider:        "xendit",
		ExternalEventID: externalEventID,
		Status:          store.WebhookProcessStatusProcessed,
		ProcessedAt:     pgtype.Timestamptz{Time: now, Valid: true},
	})
	return err
}

func (s *Service) getBioShieldPrice(ctx context.Context) (decimal.Decimal, error) {
	setting, err := s.store.GetAppSetting(ctx, "bio_shield_addon_price_usd")
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to retrieve bio_shield_addon_price_usd setting: %w", err)
	}
	var rawStr string
	if err := json.Unmarshal(setting.SettingValue, &rawStr); err == nil {
		if parsed, err := decimal.NewFromString(rawStr); err == nil && parsed.IsPositive() {
			return parsed, nil
		}
	} else {
		var num float64
		if err := json.Unmarshal(setting.SettingValue, &num); err == nil && num > 0 {
			return decimal.NewFromFloat(num), nil
		}
	}
	return decimal.Zero, errors.New("invalid or non-positive bio_shield_addon_price_usd setting value")
}

func (s *Service) getPointsRate(ctx context.Context) (decimal.Decimal, error) {
	setting, err := s.store.GetAppSetting(ctx, "points_per_usd")
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to retrieve points_per_usd setting: %w", err)
	}
	var rawStr string
	var pointsPerUSD decimal.Decimal
	if err := json.Unmarshal(setting.SettingValue, &rawStr); err == nil {
		pointsPerUSD, _ = decimal.NewFromString(rawStr)
	} else {
		var num float64
		if err := json.Unmarshal(setting.SettingValue, &num); err == nil && num > 0 {
			pointsPerUSD = decimal.NewFromFloat(num)
		}
	}
	if pointsPerUSD.IsPositive() {
		return decimal.NewFromInt(1).Div(pointsPerUSD), nil
	}
	return decimal.Zero, errors.New("invalid or non-positive points_per_usd setting value")
}

func (s *Service) getUSDToIDRExchangeRate(ctx context.Context) (decimal.Decimal, error) {
	record, err := s.store.GetLatestExchangeRate(ctx, store.GetLatestExchangeRateParams{
		BaseCode:  "USD",
		QuoteCode: "IDR",
	})
	if err != nil {
		return decimal.Zero, fmt.Errorf("failed to retrieve latest USD-IDR exchange rate: %w", err)
	}
	rateDec := decimalutil.FromNumeric(record.Rate)
	if !rateDec.IsPositive() {
		return decimal.Zero, errors.New("non-positive USD-IDR exchange rate retrieved from database")
	}
	return rateDec, nil
}

