package donation

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Handler serves HTTP endpoints for donation checkouts and webhook processing.
type Handler struct {
	service           *Service
	webhookToken      string
	allowSimulatePaid bool
}

// NewHandler constructs a new donation HTTP handler.
func NewHandler(svc *Service, webhookToken string, allowSimulatePaid bool) *Handler {
	return &Handler{
		service:           svc,
		webhookToken:      webhookToken,
		allowSimulatePaid: allowSimulatePaid,
	}
}

// RegisterRoutes registers donation routes on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/v1/app/donations", func(dr chi.Router) {
		dr.Use(authMiddleware)
		dr.Post("/checkout", h.handleCheckout)
		dr.Get("/{id}", h.handleGetDonation)
		dr.Get("/", h.handleListHistory)
		dr.Get("/history", h.handleListHistory)
	})

	r.Route("/v1/webhooks/xendit", func(wr chi.Router) {
		wr.Post("/", h.handleXenditWebhook)
		if h.allowSimulatePaid {
			wr.Post("/simulate-paid", h.handleSimulatePaid)
		}
	})
}

type checkoutRequest struct {
	DonationType       string  `json:"donation_type"`
	ProjectID          *string `json:"project_id"`
	UnitQuantity       int     `json:"unit_quantity"`
	IncludeBioShield   bool    `json:"include_bio_shield"`
	TargetPetID        *string `json:"target_pet_id"`
	CoralNickname      string  `json:"coral_nickname"`
	PaymentMethodType  string  `json:"payment_method_type"`
	BankCode           string  `json:"bank_code"`
	SponsoredQuotaID   *string `json:"sponsored_quota_id"`
	PointsToBurn       int64   `json:"points_to_burn"`
	PointsToUse        int64   `json:"points_to_use"`
	PaymentMethod      string  `json:"payment_method"`
	IsAnonymous        bool    `json:"is_anonymous"`
}

type checkoutResponse struct {
	DonationID             string                 `json:"donation_id"`
	DonationType           string                 `json:"donation_type"`
	Status                 string                 `json:"status"`
	GrossAmount            string                 `json:"gross_amount"`
	DiscountAmount         string                 `json:"discount_amount"`
	NetPayableAmount       string                 `json:"net_payable_amount"`
	CurrencyCode           string                 `json:"currency_code"`
	SettlementCurrencyCode string                 `json:"settlement_currency_code"`
	SettlementAmount       string                 `json:"settlement_amount"`
	FxRateSnapshot         string                 `json:"fx_rate_snapshot"`
	PointsBurned           int64                  `json:"points_burned"`
	PaymentMethodType      string                 `json:"payment_method_type"`
	PaymentDetails         map[string]interface{} `json:"payment_details"`
	ExpiredAt              string                 `json:"expired_at"`
	CreatedAt              string                 `json:"created_at"`
}

func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		httperr.BadRequest(w, r, "Idempotency-Key header is required for checkout.", nil)
		return
	}

	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	if req.UnitQuantity <= 0 {
		req.UnitQuantity = 1
	}

	var projectID *uuid.UUID
	if req.ProjectID != nil && *req.ProjectID != "" {
		pID, err := uuid.Parse(*req.ProjectID)
		if err != nil {
			httperr.BadRequest(w, r, "Invalid project_id UUID format.", nil)
			return
		}
		projectID = &pID
	}

	var targetPetID *uuid.UUID
	if req.TargetPetID != nil && *req.TargetPetID != "" {
		petID, err := uuid.Parse(*req.TargetPetID)
		if err == nil {
			targetPetID = &petID
		}
	}

	var sponsoredQuotaID *uuid.UUID
	if req.SponsoredQuotaID != nil && *req.SponsoredQuotaID != "" {
		qID, err := uuid.Parse(*req.SponsoredQuotaID)
		if err == nil {
			sponsoredQuotaID = &qID
		}
	}

	payMethodStr := req.PaymentMethodType
	if payMethodStr == "" {
		payMethodStr = req.PaymentMethod
	}
	if payMethodStr == "" {
		payMethodStr = string(store.PaymentMethodTypeQris)
	}

	pointsToBurn := req.PointsToBurn
	if pointsToBurn <= 0 {
		pointsToBurn = req.PointsToUse
	}

	donationType := store.DonationType(req.DonationType)
	if string(donationType) == "" {
		donationType = store.DonationTypeCoralAdoption
	}

	result, err := h.service.Checkout(r.Context(), CheckoutInput{
		IdempotencyKey:   idempotencyKey,
		UserID:           userID,
		DonationType:     donationType,
		ProjectID:        projectID,
		UnitQuantity:     req.UnitQuantity,
		IncludeBioShield: req.IncludeBioShield,
		TargetPetID:      targetPetID,
		CoralNickname:    req.CoralNickname,
		SponsoredQuotaID: sponsoredQuotaID,
		PointsToUse:      pointsToBurn,
		PaymentMethod:    store.PaymentMethodType(payMethodStr),
		BankCode:         req.BankCode,
		IsAnonymous:      req.IsAnonymous,
	})
	if err != nil {
		if errors.Is(err, ErrIdempotencyKeyReused) {
			httperr.ConflictWithCode(w, r, "IDEMPOTENCY_KEY_REUSED", "Idempotency key has already been used with different request parameters")
			return
		}
		if errors.Is(err, ErrCoralFirstGatingFailed) {
			httperr.Forbidden(w, r, err.Error())
			return
		}
		if errors.Is(err, ErrInsufficientPoints) {
			httperr.BadRequest(w, r, err.Error(), nil)
			return
		}
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	paymentDetails := map[string]interface{}{
		"qr_code_string": result.QRISPayload,
		"va_number":      result.VANumber,
		"bank_code":      result.BankCode,
		"payment_url":    nil,
	}

	resp := checkoutResponse{
		DonationID:             result.DonationID.String(),
		DonationType:           string(result.DonationType),
		Status:                 string(result.PaymentStatus),
		GrossAmount:            result.GrossProjectValue.StringFixed(2),
		DiscountAmount:         result.PointsDiscount.StringFixed(2),
		NetPayableAmount:       result.MoneyAmountPaid.StringFixed(2),
		CurrencyCode:           result.CurrencyCode,
		SettlementCurrencyCode: result.SettlementCurrency,
		SettlementAmount:       result.SettlementAmount.StringFixed(2),
		FxRateSnapshot:         result.FxRateSnapshot.StringFixed(6),
		PointsBurned:           result.PointsUsed,
		PaymentMethodType:      string(result.PaymentMethod),
		PaymentDetails:         paymentDetails,
		ExpiredAt:              result.ExpiredAt.Format(time.RFC3339),
		CreatedAt:              time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleGetDonation(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	donationID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid donation ID format.", nil)
		return
	}

	donation, err := h.service.GetDonation(r.Context(), donationID)
	if err != nil {
		if errors.Is(err, ErrDonationNotFound) {
			httperr.NotFound(w, r, "Donation record not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(donation)
}

func (h *Handler) handleListHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 50 {
			limit = 50
		} else {
			limit = l
		}
	}

	var cursorTime *time.Time
	cursorStr := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if cursorStr != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, cursorStr); err == nil {
			cursorTime = &parsed
		} else if parsed, err := time.Parse(time.RFC3339, cursorStr); err == nil {
			cursorTime = &parsed
		}
	}

	items, nextCursor, err := h.service.ListUserDonationsWithCursor(r.Context(), userID, cursorTime, limit)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       items,
		"next_cursor": nextCursor,
	})
}

type xenditCallbackPayload struct {
	ID         string          `json:"id"`
	ExternalID string          `json:"external_id"`
	Status     string          `json:"status"`
	Amount     decimal.Decimal `json:"amount"`
}

func (h *Handler) handleXenditWebhook(w http.ResponseWriter, r *http.Request) {
	callbackToken := r.Header.Get("x-callback-token")
	if callbackToken == "" {
		httperr.Unauthorized(w, r, "Missing x-callback-token webhook signature.")
		return
	}

	// Fail-closed: webhook verification token MUST be configured and match in constant time
	if strings.TrimSpace(h.webhookToken) == "" {
		httperr.InternalServerError(w, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(callbackToken), []byte(h.webhookToken)) != 1 {
		httperr.Unauthorized(w, r, "Invalid x-callback-token webhook signature.")
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		httperr.BadRequest(w, r, "Failed to read webhook request body.", nil)
		return
	}

	var payload xenditCallbackPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		httperr.BadRequest(w, r, "Invalid webhook payload JSON.", nil)
		return
	}

	err = h.service.ProcessXenditInvoiceWebhook(r.Context(), payload.ID, payload.ExternalID, payload.Status, payload.Amount, bodyBytes)
	if err != nil {
		if errors.Is(err, ErrAmountMismatch) {
			httperr.BadRequest(w, r, err.Error(), nil)
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Webhook processed successfully.",
	})
}

type simulatePaidRequest struct {
	DonationID    string `json:"donation_id"`
	CoralNickname string `json:"coral_nickname,omitempty"`
}

func (h *Handler) handleSimulatePaid(w http.ResponseWriter, r *http.Request) {
	var req simulatePaidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	donationID, err := uuid.Parse(req.DonationID)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid donation_id UUID format.", nil)
		return
	}

	updatedDonation, err := h.service.ProcessPaidDonation(r.Context(), donationID, nil, req.CoralNickname)
	if err != nil {
		if errors.Is(err, ErrDonationNotFound) {
			httperr.NotFound(w, r, "Donation not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "paid_processed",
		"data":   updatedDonation,
	})
}
