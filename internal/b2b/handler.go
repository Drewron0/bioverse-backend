package b2b

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Handler serves HTTP endpoints for the B2B portal.
type Handler struct {
	service *Service
}

// NewHandler constructs a B2B HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes registers B2B admin and client portal endpoints.
func (h *Handler) RegisterRoutes(r chi.Router, adminMiddleware, userMiddleware func(http.Handler) http.Handler) {
	r.Group(func(protected chi.Router) {
		protected.Use(adminMiddleware)
		protected.Use(identity.RequireAdminRole("super_admin", "operations", "finance"))

		// Admin B2B & ESG endpoints
		protected.Post("/v1/admin/b2b/companies", h.handleRegisterCompany)
		protected.Get("/v1/admin/b2b/companies/{id}", h.handleGetCompany)
		protected.Post("/v1/admin/b2b/companies/{id}/members", h.handleAddMember)
		protected.Get("/v1/admin/b2b/companies/{id}/members", h.handleListMembers)
		protected.Post("/v1/admin/b2b/companies/{id}/contracts", h.handleCreateContract)
		protected.Post("/v1/admin/b2b/companies/{id}/esg/metrics", h.handleRecordESGMetric)
		protected.Post("/v1/admin/b2b/companies/{id}/esg/reports", h.handleGenerateESGReport)
		protected.Get("/v1/admin/b2b/companies/{id}/esg/reports", h.handleListESGReports)
	})

	r.Group(func(portal chi.Router) {
		portal.Use(userMiddleware)

		// B2B Client Web Portal (accessible to authenticated corporate members)
		portal.Get("/v1/b2b/dashboard", h.handleGetClientDashboard)
		portal.Get("/v1/b2b/reports", h.handleListClientReports)
	})
}

type registerCompanyRequest struct {
	LegalName             string `json:"legal_name"`
	OJKTicker             string `json:"ojk_ticker,omitempty"`
	VectorLogoURL         string `json:"vector_logo_url,omitempty"`
	PICName               string `json:"pic_name"`
	WorkEmail             string `json:"work_email"`
	ContactPhone          string `json:"contact_phone"`
	AnnualFragmentTarget  int32  `json:"annual_fragment_target"`
	PlaqueInscriptionText string `json:"plaque_inscription_text,omitempty"`
}

func (h *Handler) handleRegisterCompany(w http.ResponseWriter, r *http.Request) {
	var req registerCompanyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.LegalName == "" || req.PICName == "" || req.WorkEmail == "" {
		httperr.BadRequest(w, r, "legal_name, pic_name, and work_email are required", nil)
		return
	}

	comp, err := h.service.RegisterCompany(r.Context(), RegisterCompanyInput{
		LegalName:             req.LegalName,
		OJKTicker:             req.OJKTicker,
		VectorLogoURL:         req.VectorLogoURL,
		PICName:               req.PICName,
		WorkEmail:             req.WorkEmail,
		ContactPhone:          req.ContactPhone,
		AnnualFragmentTarget:  req.AnnualFragmentTarget,
		PlaqueInscriptionText: req.PlaqueInscriptionText,
	})
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, comp)
}

func (h *Handler) handleGetCompany(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	comp, err := h.service.GetCompany(r.Context(), compID)
	if err != nil {
		if errors.Is(err, ErrCompanyNotFound) {
			httperr.NotFound(w, r, "Company not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, comp)
}

type addMemberRequest struct {
	FullName string             `json:"full_name"`
	Email    string             `json:"email"`
	Role     store.B2bRole      `json:"role"`
	Perm     store.B2bPermission `json:"permission"`
}

func (h *Handler) handleAddMember(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	member, err := h.service.AddCompanyMember(r.Context(), compID, req.FullName, req.Email, req.Role, req.Perm)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, member)
}

func (h *Handler) handleListMembers(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	members, err := h.service.ListCompanyMembers(r.Context(), compID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	if members == nil {
		members = []store.CompanyMembers{}
	}
	writeJSON(w, http.StatusOK, members)
}

type createContractRequest struct {
	PackageID       uuid.UUID       `json:"package_id"`
	CreatedByMember uuid.UUID       `json:"created_by_member_id"`
	ContractValue   decimal.Decimal `json:"contract_value"`
	CurrencyCode    string          `json:"currency_code,omitempty"`
	StartDate       string          `json:"start_date,omitempty"`
	EndDate         string          `json:"end_date,omitempty"`
	ProjectID       uuid.UUID       `json:"project_id"`
	EmployeeQuota   int32           `json:"employee_quota"`
}

func (h *Handler) handleCreateContract(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	var req createContractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	start := time.Now().UTC()
	if s, err := time.Parse("2006-01-02", strings.TrimSpace(req.StartDate)); err == nil {
		start = s
	}
	end := start.AddDate(1, 0, 0)
	if e, err := time.Parse("2006-01-02", strings.TrimSpace(req.EndDate)); err == nil {
		end = e
	}
	currency := "USD"
	if strings.TrimSpace(req.CurrencyCode) != "" {
		currency = strings.TrimSpace(req.CurrencyCode)
	}

	contract, quota, err := h.service.CreateContract(r.Context(), CreateContractInput{
		CompanyID:       compID,
		PackageID:       req.PackageID,
		CreatedByMember: req.CreatedByMember,
		ContractValue:   req.ContractValue,
		CurrencyCode:    currency,
		StartDate:       start,
		EndDate:         end,
	}, req.ProjectID, req.EmployeeQuota)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"contract_id":    contract.ID,
		"status":         contract.Status,
		"contract_value": decimalutil.FromNumeric(contract.ContractValue).StringFixed(2),
		"created_at":     contract.CreatedAt.Time.Format(time.RFC3339),
		"quota":          quota,
	})
}

func (h *Handler) handleClaimQuota(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	quotaID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid quota UUID", nil)
		return
	}

	quota, err := h.service.ClaimSponsoredQuota(r.Context(), quotaID)
	if err != nil {
		if errors.Is(err, ErrQuotaExhausted) {
			httperr.Forbidden(w, r, "Sponsored adoption quota exhausted")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, quota)
}

type recordESGMetricRequest struct {
	FiscalYear int32           `json:"fiscal_year"`
	Quarter    int32           `json:"fiscal_quarter"`
	Indicator  string          `json:"indicator_name"`
	Unit       string          `json:"unit"`
	Baseline   decimal.Decimal `json:"baseline_value"`
	Current    decimal.Decimal `json:"current_value"`
	Target     decimal.Decimal `json:"target_value"`
}

func (h *Handler) handleRecordESGMetric(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	var req recordESGMetricRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	metric, err := h.service.RecordESGBiodiversityMetric(r.Context(), compID, req.FiscalYear, req.Quarter, req.Indicator, req.Unit, req.Baseline, req.Current, req.Target)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, metric)
}

type generateESGReportRequest struct {
	MemberID *uuid.UUID `json:"member_id,omitempty"`
	Title    string     `json:"title"`
	Year     int32      `json:"fiscal_year"`
	Quarter  int32      `json:"fiscal_quarter"`
	PDFURL   string     `json:"pdf_url"`
	CSVURL   string     `json:"csv_url"`
}

func (h *Handler) handleGenerateESGReport(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	var req generateESGReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	rep, err := h.service.GenerateESGAuditReport(r.Context(), compID, req.MemberID, req.Title, req.Year, req.Quarter, req.PDFURL, req.CSVURL)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	receipt := map[string]any{
		"report_id":      rep.ID,
		"auditor_status": rep.AuditorStatus,
		"message":        "ESG audit report compilation in progress.",
		"created_at":     rep.CreatedAt,
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

type EsgReportDTO struct {
	ID               uuid.UUID `json:"id"`
	CompanyID        uuid.UUID `json:"company_id"`
	FiscalYear       int32     `json:"fiscal_year"`
	FiscalQuarter    int32     `json:"fiscal_quarter"`
	AuditorStatus    string    `json:"auditor_status"`
	ChapterPdfURL    *string   `json:"chapter_pdf_url"`
	RawDatasetCsvURL *string   `json:"raw_dataset_csv_url"`
	CreatedAt        string    `json:"created_at"`
}

func toEsgReportDTO(rep store.EsgAuditReports) EsgReportDTO {
	var pdf *string
	if rep.ChapterPdfUrl.Valid && rep.ChapterPdfUrl.String != "" {
		pdf = &rep.ChapterPdfUrl.String
	}
	var csv *string
	if rep.RawDatasetCsvUrl.Valid && rep.RawDatasetCsvUrl.String != "" {
		csv = &rep.RawDatasetCsvUrl.String
	}

	return EsgReportDTO{
		ID:               rep.ID,
		CompanyID:        rep.CompanyID,
		FiscalYear:       rep.FiscalYear,
		FiscalQuarter:    rep.FiscalQuarter,
		AuditorStatus:    string(rep.AuditorStatus),
		ChapterPdfURL:    pdf,
		RawDatasetCsvURL: csv,
		CreatedAt:        rep.CreatedAt.Time.Format(time.RFC3339),
	}
}

func (h *Handler) handleListESGReports(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	compID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid company UUID", nil)
		return
	}

	reports, err := h.service.ListESGAuditReports(r.Context(), compID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	items := make([]EsgReportDTO, len(reports))
	for i, rep := range reports {
		items[i] = toEsgReportDTO(rep)
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleGetClientDashboard(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	member, err := h.service.GetMemberByUserID(r.Context(), userID)
	if err != nil {
		httperr.Forbidden(w, r, "User is not an active member of any registered corporate company")
		return
	}

	dash, err := h.service.GetCorporateDashboard(r.Context(), member.CompanyID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, dash)
}

func (h *Handler) handleListClientReports(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	member, err := h.service.GetMemberByUserID(r.Context(), userID)
	if err != nil {
		httperr.Forbidden(w, r, "User is not an active member of any registered corporate company")
		return
	}

	reports, err := h.service.ListESGAuditReports(r.Context(), member.CompanyID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	items := make([]EsgReportDTO, len(reports))
	for i, rep := range reports {
		items[i] = toEsgReportDTO(rep)
	}

	writeJSON(w, http.StatusOK, items)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
