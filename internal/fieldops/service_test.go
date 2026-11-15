package fieldops_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/fieldops"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestFieldOpsEndToEnd(t *testing.T) {
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

	queries := store.New(pool)
	fieldSvc := fieldops.NewService(pool, queries, cfg.Auth.AppEncryptionKey, cfg.Auth.HMACSecret)

	// Fetch immutable seeded project
	seededProjectID := uuid.MustParse("0193297a-96e0-7988-9271-e0e649ce4201")
	project, err := queries.GetConservationProjectByID(ctx, seededProjectID)
	if err != nil {
		t.Fatalf("failed to get seeded project: %v", err)
	}
	projectID := project.ID
	zoneID := project.ZoneID

	uniqueSuffix := time.Now().UnixNano()
	testNIK := fmt.Sprintf("5101%012d", uniqueSuffix%1000000000000)

	// 1. Register new field operator
	op, err := fieldSvc.RegisterOperator(ctx, fieldops.RegisterOperatorInput{
		ZoneID:                zoneID,
		FullName:              "Wayan Suwena",
		PhoneNumber:           fmt.Sprintf("+6281%09d", uniqueSuffix%1000000000),
		Email:                 fmt.Sprintf("wayan_%d@bioverse.test", uniqueSuffix),
		OperatorRole:          store.OperatorRoleSiteManager,
		NIK:                   testNIK,
		BankCode:              "BCA",
		BankAccountNumber:     "1234567890",
		BankAccountHolder:     "Wayan Suwena",
		RatePerPlantingIDR:    decimal.NewFromInt(15000),
		RatePerMaintenanceIDR: decimal.NewFromInt(25000),
	})
	if err != nil {
		t.Fatalf("failed to register field operator: %v", err)
	}
	if op.FullName != "Wayan Suwena" {
		t.Errorf("expected Wayan Suwena, got %s", op.FullName)
	}
	if len(op.NikEncrypted) == 0 {
		t.Errorf("expected NIK to be encrypted in database")
	}

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM operator_payouts WHERE operator_id = $1", op.ID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM operator_work_logs WHERE operator_id = $1", op.ID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM media_evidences WHERE project_id = $1", projectID)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM field_operators WHERE id = $1", op.ID)
	})

	// 2. Invariant Test: Duplicate NIK registration MUST fail with ErrDuplicateNIK
	_, err = fieldSvc.RegisterOperator(ctx, fieldops.RegisterOperatorInput{
		ZoneID:       zoneID,
		FullName:     "Duplicate Wayan",
		PhoneNumber:  fmt.Sprintf("+6282%09d", uniqueSuffix%1000000000),
		NIK:          testNIK,
		OperatorRole: store.OperatorRoleDiver,
	})
	if err == nil {
		t.Fatalf("expected ErrDuplicateNIK, got nil")
	}

	// 3. Record field work log
	workLog, err := fieldSvc.RecordWorkLog(ctx, fieldops.RecordWorkLogInput{
		OperatorID:     op.ID,
		ProjectID:      projectID,
		TaskType:       store.OperatorTaskTypePlanting,
		UnitsCompleted: 10,
		FeeEarnedIDR:   decimal.NewFromInt(150000),
		ProofImageURL:  "https://storage.bioverse.id/fieldops/worklog_01.jpg",
		WorkDate:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to record work log: %v", err)
	}
	if workLog.UnitsCompleted != 10 {
		t.Errorf("expected 10 units, got %d", workLog.UnitsCompleted)
	}

	// 4. Invariant Test: Media evidence with milestone 2 MUST fail with ErrInvalidMilestoneMonth
	_, err = fieldSvc.RecordMediaEvidence(ctx, fieldops.RecordMediaEvidenceInput{
		ProjectID:      projectID,
		MilestoneMonth: 2,
		MilestoneTitle: "Invalid Month 2",
		MediaURL:       "https://storage.bioverse.id/milestones/invalid.jpg",
		Latitude:       decimal.NewFromFloat(-8.1436),
		Longitude:      decimal.NewFromFloat(114.6534),
		DepthMeters:    decimal.NewFromFloat(5.5),
		CapturedAt:     time.Now().UTC(),
	})
	if err == nil {
		t.Fatalf("expected ErrInvalidMilestoneMonth for month 2, got nil")
	}

	// 5. Valid Milestone 1 photo evidence
	media, err := fieldSvc.RecordMediaEvidence(ctx, fieldops.RecordMediaEvidenceInput{
		ProjectID:      projectID,
		MilestoneMonth: 1,
		MilestoneTitle: "Month 1 Coral Attachment Verification",
		MediaURL:       "https://storage.bioverse.id/milestones/month1.jpg",
		Latitude:       decimal.NewFromFloat(-8.1436),
		Longitude:      decimal.NewFromFloat(114.6534),
		DepthMeters:    decimal.NewFromFloat(5.5),
		CapturedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to record valid milestone 1: %v", err)
	}
	if !media.MilestoneMonth.Valid || media.MilestoneMonth.Int32 != 1 {
		t.Errorf("expected milestone month 1, got %v", media.MilestoneMonth)
	}

	// 6. Create payout batch for operator in draft status
	start := time.Now().UTC().AddDate(0, 0, -7)
	end := time.Now().UTC()
	payout, err := fieldSvc.CreatePayout(ctx, op.ID, start, end, decimal.NewFromInt(150000), 10, 0)
	if err != nil {
		t.Fatalf("failed to create payout: %v", err)
	}
	if payout.Status != store.PayoutStatusDraft {
		t.Errorf("expected payout status draft, got %v", payout.Status)
	}

	// Invariant Test: Disbursing a draft payout must fail
	_, err = fieldSvc.DisbursePayout(ctx, payout.ID, "disb_early_test")
	if err != fieldops.ErrPayoutNotApproved {
		t.Fatalf("expected ErrPayoutNotApproved when disbursing draft, got %v", err)
	}

	// 7. Approve payout
	approvedPayout, err := fieldSvc.ApprovePayout(ctx, payout.ID, nil)
	if err != nil {
		t.Fatalf("failed to approve payout: %v", err)
	}
	if approvedPayout.Status != store.PayoutStatusApproved {
		t.Errorf("expected payout status approved, got %v", approvedPayout.Status)
	}

	// 8. Disburse payout
	disbursedPayout, err := fieldSvc.DisbursePayout(ctx, payout.ID, fmt.Sprintf("disb_test_%d", uniqueSuffix))
	if err != nil {
		t.Fatalf("failed to disburse payout: %v", err)
	}
	if disbursedPayout.Status != store.PayoutStatusDisbursed {
		t.Errorf("expected payout status disbursed, got %v", disbursedPayout.Status)
	}

	// Invariant Test: Re-approving disbursed payout must fail
	_, err = fieldSvc.ApprovePayout(ctx, payout.ID, nil)
	if err != fieldops.ErrInvalidPayoutStatusTransition {
		t.Fatalf("expected ErrInvalidPayoutStatusTransition, got %v", err)
	}

	t.Logf("Fieldops KYC, work log, media milestone, and payout invariants verified for operator %s", op.ID)
}
