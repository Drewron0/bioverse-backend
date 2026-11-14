package b2b

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
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

// RegisterRoutes registers B2B endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(protected chi.Router) {
		protected.Use(authMiddleware)

		protected.Post("/v1/b2b/companies", h.handleRegisterCompany)
		protected.Get("/v1/b2b/companies/{id}", h.handleGetCompany)
		protected.Post("/v1/b2b/companies/{id}/members", h.handleAddMember)
		protected.Get("/v1/b2b/companies/{id}/members", h.handleListMembers)
		protected.Post("/v1/b2b/companies/{id}/contracts", h.handleCreateContract)
		protected.Post("/v1/b2b/quotas/{id}/claim", h.handleClaimQuota)
		protected.Post("/v1/b2b/companies/{id}/esg/metrics", h.handleRecordESGMetric)
		protected.Post("/v1/b2b/companies/{id}/esg/reports", h.handleGenerateESGReport)
		protected.Get("/v1/b2b/companies/{id}/esg/reports", h.handleListESGReports)
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

	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

type createContractRequest struct {
	PackageID       uuid.UUID       `json:"package_id"`
	CreatedByMember uuid.UUID       `json:"created_by_member_id"`
	ContractValue   decimal.Decimal `json:"contract_value"`
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
	end := start.AddDate(1, 0, 0)

	contract, quota, err := h.service.CreateContract(r.Context(), CreateContractInput{
		CompanyID:       compID,
		PackageID:       req.PackageID,
		CreatedByMember: req.CreatedByMember,
		ContractValue:   req.ContractValue,
		CurrencyCode:    "USD",
		StartDate:       start,
		EndDate:         end,
	}, req.ProjectID, req.EmployeeQuota)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"contract": contract,
		"quota":    quota,
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

	writeJSON(w, http.StatusCreated, rep)
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

	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
