package b2b

import (
	"context"
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
	// ErrCompanyNotFound indicates company not found.
	ErrCompanyNotFound = errors.New("company record not found")
	// ErrQuotaExhausted indicates sponsored coral quota fully claimed.
	ErrQuotaExhausted = errors.New("sponsored adoption quota is fully claimed")
	// ErrPackageNotFound indicates B2B sponsorship package not found.
	ErrPackageNotFound = errors.New("b2b package not found")
)

// Service coordinates B2B corporate partnerships, contracts, quotas, and ESG compliance reporting.
type Service struct {
	pool  *pgxpool.Pool
	store store.Querier
}

// NewService constructs a new B2B domain service.
func NewService(pool *pgxpool.Pool, q store.Querier) *Service {
	return &Service{
		pool:  pool,
		store: q,
	}
}

// RegisterCompanyInput defines onboarding payload for a corporate partner.
type RegisterCompanyInput struct {
	LegalName             string
	OJKTicker             string
	VectorLogoURL         string
	PICName               string
	WorkEmail             string
	ContactPhone          string
	AnnualFragmentTarget  int32
	PlaqueInscriptionText string
}

// RegisterCompany creates a corporate partner entity.
func (s *Service) RegisterCompany(ctx context.Context, in RegisterCompanyInput) (*store.Companies, error) {
	compID, _ := uuid.NewV7()
	c, err := s.store.CreateCompany(ctx, store.CreateCompanyParams{
		ID:                            compID,
		LegalName:                     in.LegalName,
		IdxOjkTicker:                  pgtype.Text{String: in.OJKTicker, Valid: in.OJKTicker != ""},
		VectorLogoUrl:                 pgtype.Text{String: in.VectorLogoURL, Valid: in.VectorLogoURL != ""},
		PicName:                       in.PICName,
		WorkEmail:                     in.WorkEmail,
		ContactPhone:                  in.ContactPhone,
		MandatoryFramework:            "POJK_51_2017",
		BiodiversityStandard:          "GRI_101",
		ReportingFrequency:            "quarterly",
		AnnualFragmentTarget:          in.AnnualFragmentTarget,
		DefaultPlaqueInscriptionText: pgtype.Text{String: in.PlaqueInscriptionText, Valid: in.PlaqueInscriptionText != ""},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register company: %w", err)
	}
	return &c, nil
}

// GetCompany retrieves company details.
func (s *Service) GetCompany(ctx context.Context, id uuid.UUID) (*store.Companies, error) {
	c, err := s.store.GetCompanyByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCompanyNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ListCompanies returns all corporate partners.
func (s *Service) ListCompanies(ctx context.Context) ([]store.Companies, error) {
	return s.store.ListCompanies(ctx)
}

// AddCompanyMember invites/adds a team member or auditor to the company portal.
func (s *Service) AddCompanyMember(ctx context.Context, companyID uuid.UUID, fullName, email string, role store.B2bRole, perm store.B2bPermission) (*store.CompanyMembers, error) {
	mID, _ := uuid.NewV7()
	member, err := s.store.CreateCompanyMember(ctx, store.CreateCompanyMemberParams{
		ID:           mID,
		CompanyID:    companyID,
		UserID:       pgtype.UUID{Valid: false},
		InvitedEmail: email,
		FullName:     fullName,
		Role:         role,
		Permission:   perm,
		IsAccepted:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add company member: %w", err)
	}
	return &member, nil
}

// ListCompanyMembers retrieves team members for a company.
func (s *Service) ListCompanyMembers(ctx context.Context, companyID uuid.UUID) ([]store.CompanyMembers, error) {
	return s.store.ListCompanyMembers(ctx, companyID)
}

// ListActivePackages returns corporate sponsorship tier packages.
func (s *Service) ListActivePackages(ctx context.Context) ([]store.B2bPackages, error) {
	return s.store.ListActiveB2BPackages(ctx)
}

// CreateContractInput defines contract payload.
type CreateContractInput struct {
	CompanyID       uuid.UUID
	PackageID       uuid.UUID
	CreatedByMember uuid.UUID
	ContractValue   decimal.Decimal
	CurrencyCode    string
	StartDate       time.Time
	EndDate         time.Time
}

// CreateContract creates a corporate sponsorship agreement and initializes employee adoption quota.
func (s *Service) CreateContract(ctx context.Context, in CreateContractInput, projectID uuid.UUID, employeeQuota int32) (*store.B2bContracts, *store.SponsoredAdoptionQuotas, error) {
	contractID, _ := uuid.NewV7()

	var contract store.B2bContracts
	var quota store.SponsoredAdoptionQuotas

	err := database.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		txStore := store.New(tx)

		c, err := txStore.CreateB2BContract(ctx, store.CreateB2BContractParams{
			ID:                contractID,
			CompanyID:         in.CompanyID,
			PackageID:         in.PackageID,
			CreatedByMemberID: in.CreatedByMember,
			Status:            store.ContractStatusActive,
			CurrencyCode:      in.CurrencyCode,
			ContractValue:     decimalutil.ToNumeric(in.ContractValue),
			StartDate:         pgtype.Date{Time: in.StartDate, Valid: true},
			EndDate:           pgtype.Date{Time: in.EndDate, Valid: true},
			AutoRenew:         false,
		})
		if err != nil {
			return fmt.Errorf("failed to create contract: %w", err)
		}
		contract = c

		quotaID, _ := uuid.NewV7()
		q, err := txStore.CreateSponsoredAdoptionQuota(ctx, store.CreateSponsoredAdoptionQuotaParams{
			ID:         quotaID,
			CompanyID:  in.CompanyID,
			ContractID: contractID,
			ProjectID:  projectID,
			TotalQuota: employeeQuota,
		})
		if err != nil {
			return fmt.Errorf("failed to create adoption quota: %w", err)
		}
		quota = q

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return &contract, &quota, nil
}

// ClaimSponsoredQuota claims 1 fragment from company sponsored quota (employee free adoption).
func (s *Service) ClaimSponsoredQuota(ctx context.Context, quotaID uuid.UUID) (*store.SponsoredAdoptionQuotas, error) {
	q, err := s.store.ClaimSponsoredQuota(ctx, quotaID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrQuotaExhausted
		}
		return nil, fmt.Errorf("failed to claim sponsored quota: %w", err)
	}
	return &q, nil
}

// RecordESGBiodiversityMetric records biodiversity indicators for POJK 51/2017 & GRI 101 compliance.
func (s *Service) RecordESGBiodiversityMetric(ctx context.Context, companyID uuid.UUID, year, quarter int32, indicator, unit string, baseline, current, target decimal.Decimal) (*store.EsgBiodiversityMetrics, error) {
	var variance decimal.Decimal
	if !baseline.IsZero() {
		variance = current.Sub(baseline).Div(baseline).Mul(decimal.NewFromInt(100)).Round(2)
	}
	status := store.ComplianceStatusCompliant
	if current.LessThan(target) {
		status = store.ComplianceStatusAtRisk
	}

	mID, _ := uuid.NewV7()
	metric, err := s.store.CreateESGBiodiversityMetric(ctx, store.CreateESGBiodiversityMetricParams{
		ID:                 mID,
		CompanyID:          companyID,
		FiscalYear:         year,
		FiscalQuarter:      quarter,
		IndicatorName:      indicator,
		Unit:               unit,
		BaselineValue:      decimalutil.ToNumeric(baseline),
		CurrentValue:       decimalutil.ToNumeric(current),
		TargetValue:        decimalutil.ToNumeric(target),
		VariancePercentage: decimalutil.ToNumeric(variance),
		Status:             status,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to record ESG metric: %w", err)
	}
	return &metric, nil
}

// GenerateESGAuditReport creates a structured ESG audit report.
func (s *Service) GenerateESGAuditReport(ctx context.Context, companyID uuid.UUID, memberID *uuid.UUID, title string, year, quarter int32, pdfURL, csvURL string) (*store.EsgAuditReports, error) {
	repID, _ := uuid.NewV7()
	var memID pgtype.UUID
	if memberID != nil {
		memID = pgtype.UUID{Bytes: *memberID, Valid: true}
	}

	sum, _ := s.store.GetCompanyDashboardSummary(ctx, companyID)
	readiness := decimal.Zero
	if sum.AnnualFragmentTarget > 0 {
		readiness = decimal.NewFromInt(int64(sum.UsedSponsoredQuota)).
			Div(decimal.NewFromInt(int64(sum.AnnualFragmentTarget))).
			Mul(decimal.NewFromInt(100)).
			Round(2)
		if readiness.GreaterThan(decimal.NewFromInt(100)) {
			readiness = decimal.NewFromInt(100)
		}
	}

	rep, err := s.store.CreateESGAuditReport(ctx, store.CreateESGAuditReportParams{
		ID:                   repID,
		CompanyID:            companyID,
		GeneratedByMemberID: memID,
		Title:                title,
		FiscalYear:           year,
		FiscalQuarter:        quarter,
		ReportingPeriodLabel: fmt.Sprintf("Q%d %d", quarter, year),
		Pojk51ReadinessPct:   decimalutil.ToNumeric(readiness),
		GriCriteriaMet:       12,
		AssuranceGrade:       "Grade_A",
		ChapterPdfUrl:        pgtype.Text{String: pdfURL, Valid: pdfURL != ""},
		RawDatasetCsvUrl:     pgtype.Text{String: csvURL, Valid: csvURL != ""},
		AuditorStatus:        store.ReportAuditStatusCertified,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate ESG audit report: %w", err)
	}
	return &rep, nil
}

// ListESGAuditReports returns all generated reports for a company.
func (s *Service) ListESGAuditReports(ctx context.Context, companyID uuid.UUID) ([]store.EsgAuditReports, error) {
	return s.store.ListESGAuditReportsByCompany(ctx, companyID)
}

// CorporateDashboardDTO provides an overview of adoption targets and impact for corporate partners.
type CorporateDashboardDTO struct {
	CompanyID            string `json:"company_id"`
	LegalName            string `json:"legal_name"`
	AnnualFragmentTarget int32  `json:"annual_fragment_target"`
	TotalSponsoredQuota  int32  `json:"total_sponsored_quota"`
	UsedSponsoredQuota   int32  `json:"used_sponsored_quota"`
	RemainingQuota       int32  `json:"remaining_quota"`
	Pojk51ReadinessPct   string `json:"pojk51_readiness_pct"`
}

// GetMemberByUserID looks up the company membership for an authenticated user.
func (s *Service) GetMemberByUserID(ctx context.Context, userID uuid.UUID) (*store.CompanyMembers, error) {
	mem, err := s.store.GetCompanyMemberByUserID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user is not an active member of any registered corporate company")
		}
		return nil, err
	}
	return &mem, nil
}

// GetCorporateDashboard returns progress metrics for the corporate client.
func (s *Service) GetCorporateDashboard(ctx context.Context, companyID uuid.UUID) (*CorporateDashboardDTO, error) {
	sum, err := s.store.GetCompanyDashboardSummary(ctx, companyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get corporate dashboard summary: %w", err)
	}

	remaining := sum.TotalSponsoredQuota - sum.UsedSponsoredQuota
	if remaining < 0 {
		remaining = 0
	}

	readiness := decimal.Zero
	if sum.AnnualFragmentTarget > 0 {
		readiness = decimal.NewFromInt(int64(sum.UsedSponsoredQuota)).
			Div(decimal.NewFromInt(int64(sum.AnnualFragmentTarget))).
			Mul(decimal.NewFromInt(100)).
			Round(2)
		if readiness.GreaterThan(decimal.NewFromInt(100)) {
			readiness = decimal.NewFromInt(100)
		}
	}

	return &CorporateDashboardDTO{
		CompanyID:            sum.CompanyID.String(),
		LegalName:            sum.LegalName,
		AnnualFragmentTarget: sum.AnnualFragmentTarget,
		TotalSponsoredQuota:  sum.TotalSponsoredQuota,
		UsedSponsoredQuota:   sum.UsedSponsoredQuota,
		RemainingQuota:       remaining,
		Pojk51ReadinessPct:   readiness.StringFixed(2),
	}, nil
}
