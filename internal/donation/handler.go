package donation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Drewron01/bioverse-backend/internal/identity"
	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves HTTP endpoints for donation checkouts and webhook processing.
type Handler struct {
	service *Service
}

// NewHandler constructs a new donation HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes registers donation routes on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/v1/app/donations", func(dr chi.Router) {
		dr.Use(authMiddleware)
		dr.Post("/checkout", h.handleCheckout)
		dr.Get("/{id}", h.handleGetDonation)
		dr.Get("/history", h.handleListHistory)
	})

	r.Route("/v1/webhooks/xendit", func(wr chi.Router) {
		wr.Post("/simulate-paid", h.handleSimulatePaid)
	})
}

type checkoutRequest struct {
	ProjectID        string `json:"project_id"`
	UnitQuantity     int    `json:"unit_quantity"`
	IncludeBioShield bool   `json:"include_bio_shield"`
	TargetPetID      string `json:"target_pet_id,omitempty"`
	CoralNickname    string `json:"coral_nickname,omitempty"`
	PointsToUse      int64  `json:"points_to_use"`
	PaymentMethod    string `json:"payment_method"`
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

	projectID, err := uuid.Parse(req.ProjectID)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid project_id UUID format.", nil)
		return
	}

	var targetPetID *uuid.UUID
	if req.TargetPetID != "" {
		petID, err := uuid.Parse(req.TargetPetID)
		if err == nil {
			targetPetID = &petID
		}
	}

	paymentMethod := store.PaymentMethodType(req.PaymentMethod)
	if paymentMethod == "" {
		paymentMethod = store.PaymentMethodTypeQris
	}

	result, err := h.service.Checkout(r.Context(), CheckoutInput{
		IdempotencyKey:   idempotencyKey,
		UserID:           userID,
		ProjectID:        projectID,
		UnitQuantity:     req.UnitQuantity,
		IncludeBioShield: req.IncludeBioShield,
		TargetPetID:      targetPetID,
		CoralNickname:    req.CoralNickname,
		PointsToUse:      req.PointsToUse,
		PaymentMethod:    paymentMethod,
	})
	if err != nil {
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": result,
	})
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": donation,
	})
}

func (h *Handler) handleListHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	donations, err := h.service.ListUserDonations(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": donations,
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
