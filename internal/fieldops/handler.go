package fieldops

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Handler serves HTTP endpoints for field operations and payouts.
type Handler struct {
	service *Service
}

// NewHandler constructs a fieldops HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes mounts fieldops endpoints restricted to administrators.
func (h *Handler) RegisterRoutes(r chi.Router, adminMiddleware func(http.Handler) http.Handler) {
	r.Group(func(protected chi.Router) {
		protected.Use(adminMiddleware)

		// Admin Field Operations routes
		protected.Post("/v1/admin/fieldops/operators", h.handleRegisterOperator)
		protected.Get("/v1/admin/fieldops/operators/{id}", h.handleGetOperator)
		protected.Post("/v1/admin/fieldops/work-logs", h.handleRecordWorkLog)
		protected.Post("/v1/admin/fieldops/media", h.handleRecordMedia)
		protected.Post("/v1/admin/fieldops/payouts", h.handleCreatePayout)
		protected.Post("/v1/admin/fieldops/payouts/{id}/approve", h.handleApprovePayout)
		protected.Post("/v1/admin/fieldops/payouts/{id}/disburse", h.handleDisbursePayout)
	})
}

type registerOperatorRequest struct {
	ZoneID                uuid.UUID          `json:"zone_id"`
	GroupID               *uuid.UUID         `json:"group_id,omitempty"`
	FullName              string             `json:"full_name"`
	PhoneNumber           string             `json:"phone_number"`
	Email                 string             `json:"email"`
	OperatorRole          store.OperatorRole `json:"operator_role"`
	NIK                   string             `json:"nik"`
	BankCode              string             `json:"bank_code"`
	BankAccountNumber     string             `json:"bank_account_number"`
	BankAccountHolder     string             `json:"bank_account_holder"`
	RatePerPlantingIDR    decimal.Decimal    `json:"rate_per_planting_idr"`
	RatePerMaintenanceIDR decimal.Decimal    `json:"rate_per_maintenance_idr"`
}

func (h *Handler) handleRegisterOperator(w http.ResponseWriter, r *http.Request) {
	var req registerOperatorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.FullName == "" || req.NIK == "" || req.PhoneNumber == "" {
		httperr.BadRequest(w, r, "full_name, nik, and phone_number are required", nil)
		return
	}

	op, err := h.service.RegisterOperator(r.Context(), RegisterOperatorInput{
		ZoneID:                req.ZoneID,
		GroupID:               req.GroupID,
		FullName:              req.FullName,
		PhoneNumber:           req.PhoneNumber,
		Email:                 req.Email,
		OperatorRole:          req.OperatorRole,
		NIK:                   req.NIK,
		BankCode:              req.BankCode,
		BankAccountNumber:     req.BankAccountNumber,
		BankAccountHolder:     req.BankAccountHolder,
		RatePerPlantingIDR:    req.RatePerPlantingIDR,
		RatePerMaintenanceIDR: req.RatePerMaintenanceIDR,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateNIK) {
			httperr.Conflict(w, r, "Operator with this NIK already registered")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, op)
}

func (h *Handler) handleGetOperator(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	opID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid operator UUID", nil)
		return
	}

	op, err := h.service.GetOperator(r.Context(), opID)
	if err != nil {
		if errors.Is(err, ErrOperatorNotFound) {
			httperr.NotFound(w, r, "Field operator not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, op)
}

type recordWorkLogRequest struct {
	OperatorID      uuid.UUID              `json:"operator_id"`
	ProjectID       uuid.UUID              `json:"project_id"`
	SpiderFrameID   *uuid.UUID             `json:"spider_frame_id,omitempty"`
	CoralFragmentID *uuid.UUID             `json:"coral_fragment_id,omitempty"`
	TaskType        store.OperatorTaskType `json:"task_type"`
	UnitsCompleted  int32                  `json:"units_completed"`
	FeeEarnedIDR    decimal.Decimal        `json:"fee_earned_idr"`
	ProofImageURL   string                 `json:"proof_image_url"`
	WorkDate        *time.Time             `json:"work_date,omitempty"`
}

func (h *Handler) handleRecordWorkLog(w http.ResponseWriter, r *http.Request) {
	var req recordWorkLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.OperatorID == uuid.Nil || req.ProjectID == uuid.Nil || req.ProofImageURL == "" {
		httperr.BadRequest(w, r, "operator_id, project_id, and proof_image_url are required", nil)
		return
	}

	workDate := time.Now().UTC()
	if req.WorkDate != nil && !req.WorkDate.IsZero() {
		workDate = *req.WorkDate
	}

	log, err := h.service.RecordWorkLog(r.Context(), RecordWorkLogInput{
		OperatorID:      req.OperatorID,
		ProjectID:       req.ProjectID,
		SpiderFrameID:   req.SpiderFrameID,
		CoralFragmentID: req.CoralFragmentID,
		TaskType:        req.TaskType,
		UnitsCompleted:  req.UnitsCompleted,
		FeeEarnedIDR:    req.FeeEarnedIDR,
		ProofImageURL:   req.ProofImageURL,
		WorkDate:        workDate,
	})
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, log)
}

type recordMediaRequest struct {
	ProjectID       uuid.UUID       `json:"project_id"`
	SpiderFrameID   *uuid.UUID      `json:"spider_frame_id,omitempty"`
	CoralFragmentID *uuid.UUID      `json:"coral_fragment_id,omitempty"`
	MilestoneMonth  int32           `json:"milestone_month"`
	MilestoneTitle  string          `json:"milestone_title"`
	MediaURL        string          `json:"media_url"`
	Latitude        decimal.Decimal `json:"latitude"`
	Longitude       decimal.Decimal `json:"longitude"`
	DepthMeters     decimal.Decimal `json:"depth_meters"`
	CapturedAt      *time.Time      `json:"captured_at,omitempty"`
	Notes           *string         `json:"notes,omitempty"`
}

func (h *Handler) handleRecordMedia(w http.ResponseWriter, r *http.Request) {
	var req recordMediaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	capturedAt := time.Now().UTC()
	if req.CapturedAt != nil && !req.CapturedAt.IsZero() {
		capturedAt = *req.CapturedAt
	}

	notes := ""
	if req.Notes != nil {
		notes = *req.Notes
	}

	ev, err := h.service.RecordMediaEvidence(r.Context(), RecordMediaEvidenceInput{
		ProjectID:       req.ProjectID,
		SpiderFrameID:   req.SpiderFrameID,
		CoralFragmentID: req.CoralFragmentID,
		MilestoneMonth:  req.MilestoneMonth,
		MilestoneTitle:  req.MilestoneTitle,
		MediaURL:        req.MediaURL,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		DepthMeters:     req.DepthMeters,
		CapturedAt:      capturedAt,
		Notes:           notes,
	})
	if err != nil {
		if errors.Is(err, ErrInvalidMilestoneMonth) {
			httperr.BadRequest(w, r, "milestone_month must be 1, 3, or 6", nil)
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, ev)
}

type createPayoutRequest struct {
	OperatorID       uuid.UUID        `json:"operator_id"`
	PeriodStart      string           `json:"period_start"`
	PeriodEnd        string           `json:"period_end"`
	GrossAmountIDR   *decimal.Decimal `json:"gross_amount_idr,omitempty"`
	UnitsPlanted     *int32           `json:"units_planted,omitempty"`
	MaintenanceTasks *int32           `json:"maintenance_tasks,omitempty"`
}

type payoutBatchResponse struct {
	ID              string `json:"id"`
	OperatorID      string `json:"operator_id"`
	GrossAmountIDR  string `json:"gross_amount_idr"`
	TransferFeeIDR  string `json:"transfer_fee_idr"`
	NetDisbursedIDR string `json:"net_disbursed_idr"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
}

func toPayoutBatchResponse(p *store.OperatorPayouts) payoutBatchResponse {
	return payoutBatchResponse{
		ID:              p.ID.String(),
		OperatorID:      p.OperatorID.String(),
		GrossAmountIDR:  decimalutil.FromNumeric(p.GrossAmountIdr).StringFixed(2),
		TransferFeeIDR:  decimalutil.FromNumeric(p.TransferFeeIdr).StringFixed(2),
		NetDisbursedIDR: decimalutil.FromNumeric(p.NetDisbursedIdr).StringFixed(2),
		Status:          string(p.Status),
		CreatedAt:       p.CreatedAt.Time.Format(time.RFC3339),
	}
}

func (h *Handler) handleCreatePayout(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")

	var req createPayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.OperatorID == uuid.Nil {
		httperr.BadRequest(w, r, "operator_id is required", nil)
		return
	}

	start, err := time.Parse("2006-01-02", req.PeriodStart)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid period_start format (YYYY-MM-DD)", nil)
		return
	}
	end, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid period_end format (YYYY-MM-DD)", nil)
		return
	}

	payout, err := h.service.CreatePayoutWithInput(r.Context(), CreatePayoutInput{
		IdempotencyKey:   idempotencyKey,
		OperatorID:       req.OperatorID,
		PeriodStart:      start,
		PeriodEnd:        end,
		GrossAmountIDR:   req.GrossAmountIDR,
		UnitsPlanted:     req.UnitsPlanted,
		MaintenanceTasks: req.MaintenanceTasks,
	})
	if err != nil {
		if errors.Is(err, ErrOperatorNotFound) {
			httperr.NotFound(w, r, "Field operator not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, toPayoutBatchResponse(payout))
}

func (h *Handler) handleApprovePayout(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	payoutID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid payout UUID", nil)
		return
	}

	var adminIDPtr *uuid.UUID
	if adminID, ok := identity.GetAdminIDFromContext(r.Context()); ok {
		adminIDPtr = &adminID
	}

	payout, err := h.service.ApprovePayout(r.Context(), payoutID, adminIDPtr)
	if err != nil {
		if errors.Is(err, ErrPayoutNotFound) {
			httperr.NotFound(w, r, "Payout not found")
			return
		}
		if errors.Is(err, ErrInvalidPayoutStatusTransition) {
			httperr.Conflict(w, r, err.Error())
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, toPayoutBatchResponse(payout))
}

type disbursePayoutRequest struct {
	ExternalDisbursementID string `json:"external_disbursement_id,omitempty"`
}

func (h *Handler) handleDisbursePayout(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	payoutID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid payout UUID", nil)
		return
	}

	var req disbursePayoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	payout, err := h.service.DisbursePayout(r.Context(), payoutID, req.ExternalDisbursementID)
	if err != nil {
		if errors.Is(err, ErrPayoutNotFound) {
			httperr.NotFound(w, r, "Payout not found")
			return
		}
		if errors.Is(err, ErrPayoutNotApproved) {
			httperr.Conflict(w, r, err.Error())
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, toPayoutBatchResponse(payout))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
