package fieldops

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/crypto"
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
	// ErrOperatorNotFound indicates field operator not found.
	ErrOperatorNotFound = errors.New("field operator not found")
	// ErrDuplicateNIK indicates operator with same NIK already registered.
	ErrDuplicateNIK = errors.New("operator with this NIK is already registered")
	// ErrInvalidMilestoneMonth indicates milestone month is not 1, 3, or 6.
	ErrInvalidMilestoneMonth = errors.New("milestone month must be 1, 3, or 6")
	// ErrPayoutNotFound indicates payout record not found.
	ErrPayoutNotFound = errors.New("payout record not found")
)

// Service provides domain logic for field operations, operator KYC, work logs, and payouts.
type Service struct {
	pool          *pgxpool.Pool
	store         store.Querier
	encryptionKey string
	hmacSecret    string
}

// NewService constructs a field operations service.
func NewService(pool *pgxpool.Pool, q store.Querier, encKey, hmacSec string) *Service {
	return &Service{
		pool:          pool,
		store:         q,
		encryptionKey: encKey,
		hmacSecret:    hmacSec,
	}
}

// RegisterOperatorInput defines input for onboarding a field operator.
type RegisterOperatorInput struct {
	ZoneID                uuid.UUID
	GroupID               *uuid.UUID
	FullName              string
	PhoneNumber           string
	Email                 string
	OperatorRole          store.OperatorRole
	NIK                   string
	BankCode              string
	BankAccountNumber     string
	BankAccountHolder     string
	RatePerPlantingIDR    decimal.Decimal
	RatePerMaintenanceIDR decimal.Decimal
}

// RegisterOperator encrypts KYC sensitive data and creates operator profile.
func (s *Service) RegisterOperator(ctx context.Context, in RegisterOperatorInput) (*store.FieldOperators, error) {
	nikHash := crypto.GenerateSearchableHash(in.NIK, s.hmacSecret)

	// Check duplicate NIK via blinded HMAC
	_, err := s.store.GetFieldOperatorByNIKHash(ctx, pgtype.Text{String: nikHash, Valid: true})
	if err == nil {
		return nil, ErrDuplicateNIK
	}

	encryptedNIK, err := crypto.Encrypt([]byte(in.NIK), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt NIK: %w", err)
	}

	encryptedBank, err := crypto.Encrypt([]byte(in.BankAccountNumber), s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt bank account: %w", err)
	}

	opID, _ := uuid.NewV7()
	var groupID pgtype.UUID
	if in.GroupID != nil {
		groupID = pgtype.UUID{Bytes: *in.GroupID, Valid: true}
	}

	op, err := s.store.CreateFieldOperator(ctx, store.CreateFieldOperatorParams{
		ID:                     opID,
		ZoneID:                 in.ZoneID,
		GroupID:                groupID,
		VerifiedByAdminID:      pgtype.UUID{Valid: false},
		FullName:               in.FullName,
		CanLogin:               false,
		Email:                  pgtype.Text{String: in.Email, Valid: in.Email != ""},
		PhoneNumber:            in.PhoneNumber,
		PasswordHash:           pgtype.Text{Valid: false},
		OperatorRole:           in.OperatorRole,
		NikEncrypted:           encryptedNIK,
		NikHmac:                pgtype.Text{String: nikHash, Valid: true},
		KycStatus:              store.KycVerificationStatusVerified,
		BankCode:               pgtype.Text{String: in.BankCode, Valid: in.BankCode != ""},
		BankAccountEncrypted:   encryptedBank,
		BankAccountHolder:      pgtype.Text{String: in.BankAccountHolder, Valid: in.BankAccountHolder != ""},
		RatePerPlantingIdr:     decimalutil.ToNumeric(in.RatePerPlantingIDR),
		RatePerMaintenanceIdr: decimalutil.ToNumeric(in.RatePerMaintenanceIDR),
		IsActive:               true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create operator: %w", err)
	}

	return &op, nil
}

// GetOperator retrieves an operator by ID.
func (s *Service) GetOperator(ctx context.Context, id uuid.UUID) (*store.FieldOperators, error) {
	op, err := s.store.GetFieldOperatorByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOperatorNotFound
		}
		return nil, err
	}
	return &op, nil
}

// RecordWorkLogInput defines payload for field operator work logs.
type RecordWorkLogInput struct {
	OperatorID      uuid.UUID
	ProjectID       uuid.UUID
	SpiderFrameID   *uuid.UUID
	CoralFragmentID *uuid.UUID
	TaskType        store.OperatorTaskType
	UnitsCompleted  int32
	FeeEarnedIDR    decimal.Decimal
	ProofImageURL   string
	WorkDate        time.Time
}

// RecordWorkLog logs an operational restoration task completed on the ground.
func (s *Service) RecordWorkLog(ctx context.Context, in RecordWorkLogInput) (*store.OperatorWorkLogs, error) {
	logID, _ := uuid.NewV7()
	var frameID pgtype.UUID
	if in.SpiderFrameID != nil {
		frameID = pgtype.UUID{Bytes: *in.SpiderFrameID, Valid: true}
	}
	var fragID pgtype.UUID
	if in.CoralFragmentID != nil {
		fragID = pgtype.UUID{Bytes: *in.CoralFragmentID, Valid: true}
	}

	log, err := s.store.CreateOperatorWorkLog(ctx, store.CreateOperatorWorkLogParams{
		ID:                 logID,
		OperatorID:         in.OperatorID,
		EnteredByAdminID:   pgtype.UUID{Valid: false},
		ProjectID:          pgtype.UUID{Bytes: in.ProjectID, Valid: true},
		SpiderFrameID:      frameID,
		CoralFragmentID:    fragID,
		TaskType:           in.TaskType,
		UnitsCompleted:     in.UnitsCompleted,
		FeeEarnedIdr:       decimalutil.ToNumeric(in.FeeEarnedIDR),
		ProofImageUrl:      in.ProofImageURL,
		VerificationStatus: store.SubmissionStatusVerified,
		WorkDate:           pgtype.Timestamptz{Time: in.WorkDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create work log: %w", err)
	}

	return &log, nil
}

// RecordMediaEvidenceInput holds milestone photo updates.
type RecordMediaEvidenceInput struct {
	ProjectID       uuid.UUID
	SpiderFrameID   *uuid.UUID
	CoralFragmentID *uuid.UUID
	MilestoneMonth  int32
	MilestoneTitle  string
	MediaURL        string
	Latitude        decimal.Decimal
	Longitude       decimal.Decimal
	DepthMeters     decimal.Decimal
	CapturedAt      time.Time
	Notes           string
}

// RecordMediaEvidence stores milestone photo updates (months 1, 3, 6).
func (s *Service) RecordMediaEvidence(ctx context.Context, in RecordMediaEvidenceInput) (*store.MediaEvidences, error) {
	if in.MilestoneMonth != 1 && in.MilestoneMonth != 3 && in.MilestoneMonth != 6 {
		return nil, ErrInvalidMilestoneMonth
	}

	evID, _ := uuid.NewV7()
	var frameID pgtype.UUID
	if in.SpiderFrameID != nil {
		frameID = pgtype.UUID{Bytes: *in.SpiderFrameID, Valid: true}
	}
	var fragID pgtype.UUID
	if in.CoralFragmentID != nil {
		fragID = pgtype.UUID{Bytes: *in.CoralFragmentID, Valid: true}
	}

	subtitle := fmt.Sprintf("Milestone Month %d Growth Update", in.MilestoneMonth)
	if in.Notes != "" {
		subtitle = in.Notes
	}

	ev, err := s.store.CreateMediaEvidence(ctx, store.CreateMediaEvidenceParams{
		ID:                    evID,
		ProjectID:             pgtype.UUID{Bytes: in.ProjectID, Valid: true},
		SpiderFrameID:         frameID,
		CoralFragmentID:       fragID,
		CompanyID:             pgtype.UUID{Valid: false},
		CapturedByOperatorID:  pgtype.UUID{Valid: false},
		UploadedByAdminID:     pgtype.UUID{Valid: false},
		MediaType:             store.EvidenceMediaTypePhoto,
		MilestoneMonth:        pgtype.Int4{Int32: in.MilestoneMonth, Valid: true},
		MilestoneTitle:        pgtype.Text{String: in.MilestoneTitle, Valid: true},
		SubtitleMetrics:       pgtype.Text{String: subtitle, Valid: true},
		MediaUrl:              in.MediaURL,
		GpsLatitude:           decimalutil.ToNumeric(in.Latitude),
		GpsLongitude:          decimalutil.ToNumeric(in.Longitude),
		DepthMeters:           decimalutil.ToNumeric(in.DepthMeters),
		IsVerifiedHealthy:     true,
		CapturedAt:            pgtype.Timestamptz{Time: in.CapturedAt, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save media evidence: %w", err)
	}

	return &ev, nil
}

// CreatePayout creates a settlement payout batch for an operator.
func (s *Service) CreatePayout(ctx context.Context, operatorID uuid.UUID, periodStart, periodEnd time.Time, grossAmountIDR decimal.Decimal, unitsPlanted, maintenanceTasks int32) (*store.OperatorPayouts, error) {
	payoutID, _ := uuid.NewV7()
	idempotencyKey := fmt.Sprintf("payout-%s-%s-%s", operatorID.String(), periodStart.Format("20060102"), periodEnd.Format("20060102"))

	var createdPayout store.OperatorPayouts
	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		p, err := txStore.CreateOperatorPayout(ctx, store.CreateOperatorPayoutParams{
			ID:                     payoutID,
			IdempotencyKey:         idempotencyKey,
			OperatorID:             operatorID,
			ApprovedByAdminID:      pgtype.UUID{Valid: false},
			PeriodStart:            pgtype.Date{Time: periodStart, Valid: true},
			PeriodEnd:              pgtype.Date{Time: periodEnd, Valid: true},
			TotalUnitsPlanted:      unitsPlanted,
			TotalMaintenanceTasks:  maintenanceTasks,
			GrossAmountIdr:         decimalutil.ToNumeric(grossAmountIDR),
			TransferFeeIdr:         decimalutil.ToNumeric(decimal.Zero),
			NetDisbursedIdr:        decimalutil.ToNumeric(grossAmountIDR),
			PayoutGatewayProvider:  "xendit",
			Status:                 store.PayoutStatusApproved,
		})
		if err != nil {
			return fmt.Errorf("failed to create operator payout: %w", err)
		}

		createdPayout = p
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &createdPayout, nil
}

// ApprovePayout approves a pending operator payout batch.
func (s *Service) ApprovePayout(ctx context.Context, payoutID uuid.UUID) (*store.OperatorPayouts, error) {
	payout, err := s.store.UpdateOperatorPayoutStatus(ctx, store.UpdateOperatorPayoutStatusParams{
		ID:         payoutID,
		Status:     store.PayoutStatusApproved,
		ApprovedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPayoutNotFound
		}
		return nil, fmt.Errorf("failed to approve operator payout: %w", err)
	}
	return &payout, nil
}

// DisbursePayout records disbursement execution for an approved payout batch.
func (s *Service) DisbursePayout(ctx context.Context, payoutID uuid.UUID, externalDisbursementID string) (*store.OperatorPayouts, error) {
	if externalDisbursementID == "" {
		externalDisbursementID = fmt.Sprintf("disb-xen-%d", time.Now().UnixNano())
	}
	payout, err := s.store.UpdateOperatorPayoutStatus(ctx, store.UpdateOperatorPayoutStatusParams{
		ID:                     payoutID,
		Status:                 store.PayoutStatusDisbursed,
		DisbursedAt:            pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		ExternalDisbursementID: pgtype.Text{String: externalDisbursementID, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPayoutNotFound
		}
		return nil, fmt.Errorf("failed to disburse operator payout: %w", err)
	}
	return &payout, nil
}
