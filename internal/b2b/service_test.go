package b2b_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/b2b"
	"github.com/Drewron0/bioverse-backend/internal/platform/config"
	"github.com/Drewron0/bioverse-backend/internal/platform/database"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestB2BAndESGEndToEnd(t *testing.T) {
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
	b2bSvc := b2b.NewService(pool, queries)

	// Fetch immutable seeded project
	seededProjectID := uuid.MustParse("0193297a-96e0-7988-9271-e0e649ce4201")
	project, err := queries.GetConservationProjectByID(ctx, seededProjectID)
	if err != nil {
		t.Fatalf("failed to get seeded project: %v", err)
	}
	projectID := project.ID

	uniqueSuffix := time.Now().UnixNano()

	// 1. Register corporate partner
	company, err := b2bSvc.RegisterCompany(ctx, b2b.RegisterCompanyInput{
		LegalName:             fmt.Sprintf("PT Samudera Lestari TBK %d", uniqueSuffix),
		OJKTicker:             "SMDR",
		VectorLogoURL:         "https://assets.bioverse.id/logos/smdr.svg",
		PICName:               "Hendra Setiawan",
		WorkEmail:             fmt.Sprintf("esg_%d@samudera.test", uniqueSuffix),
		ContactPhone:          "+62215556677",
		AnnualFragmentTarget:  1000,
		PlaqueInscriptionText: "Dedicated to Marine Biodiversity Restoration",
	})
	if err != nil {
		t.Fatalf("failed to register company: %v", err)
	}
	if company.LegalName == "" {
		t.Errorf("expected valid legal name")
	}

	// 2. Add corporate member
	member, err := b2bSvc.AddCompanyMember(ctx, company.ID, "Rina Melati", "rina@samudera.test", store.B2bRoleCorporateAdmin, store.B2bPermissionAdmin)
	if err != nil {
		t.Fatalf("failed to add member: %v", err)
	}

	// 3. Create seeded package if none exists
	pkgID, _ := uuid.NewV7()
	_, err = pool.Exec(ctx, `
		INSERT INTO b2b_packages (id, sku_code, name, package_type, currency_code, price_amount, spider_frames_count, coral_fragments_count)
		VALUES ($1, $2, 'Corporate Coral Oasis', 'enterprise_suite', 'USD', 5000.00, 10, 150)
		ON CONFLICT (sku_code) DO NOTHING
	`, pkgID, fmt.Sprintf("CORP_TIER_%d", uniqueSuffix))
	if err != nil {
		t.Fatalf("failed to insert b2b package: %v", err)
	}

	pkgs, err := b2bSvc.ListActivePackages(ctx)
	if err != nil || len(pkgs) == 0 {
		t.Fatalf("failed to list packages: %v", err)
	}

	// 4. Create B2B contract with 1 employee adoption quota
	contract, quota, err := b2bSvc.CreateContract(ctx, b2b.CreateContractInput{
		CompanyID:       company.ID,
		PackageID:       pkgs[0].ID,
		CreatedByMember: member.ID,
		ContractValue:   decimal.NewFromInt(5000),
		CurrencyCode:    "USD",
		StartDate:       time.Now().UTC(),
		EndDate:         time.Now().UTC().AddDate(1, 0, 0),
	}, projectID, 1)
	if err != nil {
		t.Fatalf("failed to create contract: %v", err)
	}
	if contract.Status != store.ContractStatusActive {
		t.Errorf("expected active contract, got %v", contract.Status)
	}

	// 5. Claim the 1 employee quota
	claimedQuota, err := b2bSvc.ClaimSponsoredQuota(ctx, quota.ID)
	if err != nil {
		t.Fatalf("failed to claim quota: %v", err)
	}
	if claimedQuota.UsedQuota != 1 {
		t.Errorf("expected used quota 1, got %d", claimedQuota.UsedQuota)
	}

	// 6. Invariant Test: Claiming again when total_quota (1) == claimed_quota (1) MUST fail with ErrQuotaExhausted
	_, err = b2bSvc.ClaimSponsoredQuota(ctx, quota.ID)
	if err == nil {
		t.Fatalf("expected ErrQuotaExhausted, got nil")
	}

	// 7. Record POJK 51/2017 & GRI 101 Biodiversity metric
	metric, err := b2bSvc.RecordESGBiodiversityMetric(ctx, company.ID, 2026, 3, "Coral Survival Rate", "%", decimal.NewFromFloat(66.67), decimal.NewFromFloat(85.50), decimal.NewFromFloat(80.00))
	if err != nil {
		t.Fatalf("failed to record metric: %v", err)
	}
	if metric.Status != store.ComplianceStatusCompliant {
		t.Errorf("expected compliant status, got %v", metric.Status)
	}

	// 8. Generate ESG audit report
	rep, err := b2bSvc.GenerateESGAuditReport(ctx, company.ID, &member.ID, "Q3 2026 ESG Biodiversity Compliance Audit", 2026, 3, "https://storage.bioverse.id/esg/report_q3_2026.pdf", "https://storage.bioverse.id/esg/dataset_q3_2026.csv")
	if err != nil {
		t.Fatalf("failed to generate ESG report: %v", err)
	}
	if rep.AssuranceGrade != "Grade_A" {
		t.Errorf("expected Grade_A assurance, got %s", rep.AssuranceGrade)
	}

	reports, err := b2bSvc.ListESGAuditReports(ctx, company.ID)
	if err != nil || len(reports) != 1 {
		t.Fatalf("expected 1 audit report, got %d", len(reports))
	}

	t.Logf("B2B contract, quota exhaust invariant, and ESG reporting verified for %s", company.LegalName)
}
