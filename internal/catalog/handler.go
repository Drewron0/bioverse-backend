package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves HTTP endpoints for the catalog, digital shop, and user inventory.
type Handler struct {
	service *Service
}

// NewHandler constructs a new catalog HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes registers public and protected catalog endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Public routes
	r.Route("/v1/app/catalog", func(cr chi.Router) {
		cr.Get("/products", h.handleListProducts)
		cr.Get("/plans", h.handleListPlans)
	})

	// Protected routes
	r.Group(func(protected chi.Router) {
		protected.Use(authMiddleware)

		protected.Post("/v1/app/shop/buy", h.handleBuyProduct)
		protected.Post("/v1/app/shop/iap/verify", h.handleVerifyIAPReceipt)
		protected.Get("/v1/app/inventory", h.handleListInventory)
		protected.Post("/v1/app/inventory/streak-freeze/use", h.handleUseStreakFreeze)

		protected.Post("/v1/app/subscriptions", h.handleCreateSubscription)
		protected.Get("/v1/app/subscriptions/my", h.handleListSubscriptions)
		protected.Post("/v1/app/subscriptions/{id}/pause", h.handlePauseSubscription)
		protected.Post("/v1/app/subscriptions/{id}/resume", h.handleResumeSubscription)
		protected.Post("/v1/app/subscriptions/{id}/cancel", h.handleCancelSubscription)
	})
}

func (h *Handler) handleListProducts(w http.ResponseWriter, r *http.Request) {
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = "USD"
	}

	products, err := h.service.ListDigitalProducts(r.Context(), currency)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	// Returns array directly to comply with OpenAPI type: array schema for client generation
	writeJSON(w, http.StatusOK, products)
}

func (h *Handler) handleListPlans(w http.ResponseWriter, r *http.Request) {
	currency := r.URL.Query().Get("currency")
	if currency == "" {
		currency = "USD"
	}

	plans, err := h.service.ListSubscriptionPlans(r.Context(), currency)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	// Returns array directly to comply with OpenAPI type: array schema for client generation
	writeJSON(w, http.StatusOK, plans)
}

type buyProductRequest struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int32     `json:"quantity"`
}

func (h *Handler) handleBuyProduct(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		httperr.BadRequest(w, r, "Idempotency-Key header is required", nil)
		return
	}
	if _, err := uuid.Parse(idempotencyKey); err != nil {
		httperr.BadRequest(w, r, "Idempotency-Key must be a valid UUID", nil)
		return
	}

	var req buyProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.ProductID == uuid.Nil {
		httperr.BadRequest(w, r, "product_id is required", nil)
		return
	}

	result, err := h.service.BuyProductWithPoints(r.Context(), userID, req.ProductID, req.Quantity, idempotencyKey)
	if err != nil {
		if errors.Is(err, ErrProductNotFound) {
			httperr.NotFound(w, r, "Digital product not found")
			return
		}
		if errors.Is(err, ErrInsufficientPoints) {
			httperr.Forbidden(w, r, "Insufficient points balance")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleListInventory(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	items, err := h.service.ListUserInventory(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	if items == nil {
		items = []store.ListUserInventoryRow{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) handleUseStreakFreeze(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	err := h.service.ConsumeStreakFreeze(r.Context(), userID)
	if err != nil {
		if errors.Is(err, ErrItemNotInInventory) {
			httperr.Forbidden(w, r, "No Streak Freeze items in inventory")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "streak_freeze_applied"})
}

type createSubscriptionRequest struct {
	PlanID            uuid.UUID               `json:"plan_id"`
	ProjectID         uuid.UUID               `json:"project_id"`
	UnitQuantity      int32                   `json:"unit_quantity"`
	PaymentMethodType store.PaymentMethodType `json:"payment_method_type"`
}

func (h *Handler) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	if h.service.IsProduction() {
		httperr.ServiceUnavailableWithCode(w, r, "NOT_AVAILABLE", "Recurring subscriptions are disabled in production until payment gateway tokenization is configured")
		return
	}

	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.PlanID == uuid.Nil {
		httperr.BadRequest(w, r, "plan_id is required", nil)
		return
	}
	if req.PaymentMethodType == "" {
		req.PaymentMethodType = store.PaymentMethodTypeEwallet
	}

	sub, err := h.service.SubscribePlan(r.Context(), CreateSubscriptionInput{
		UserID:            userID,
		PlanID:            req.PlanID,
		ProjectID:         req.ProjectID,
		UnitQuantity:      req.UnitQuantity,
		PaymentMethodType: req.PaymentMethodType,
	})
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotAvailableInProduction) {
			httperr.ServiceUnavailableWithCode(w, r, "NOT_AVAILABLE", "Recurring subscriptions are disabled in production until payment gateway tokenization is configured")
			return
		}
		if errors.Is(err, ErrPlanNotFound) {
			httperr.NotFound(w, r, "Subscription plan not found")
			return
		}
		if errors.Is(err, ErrProjectNotFound) {
			httperr.NotFound(w, r, "Conservation project not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, sub)
}

func (h *Handler) handleListSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	subs, err := h.service.ListUserSubscriptions(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	if subs == nil {
		subs = []SubscriptionDetailDTO{}
	}
	writeJSON(w, http.StatusOK, subs)
}

type verifyIAPRequest struct {
	ProductID      uuid.UUID `json:"product_id"`
	StoreProvider  string    `json:"store_provider"`
	ReceiptPayload string    `json:"receipt_payload"`
}

func (h *Handler) handleVerifyIAPReceipt(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	var req verifyIAPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.ProductID == uuid.Nil || req.StoreProvider == "" || req.ReceiptPayload == "" {
		httperr.BadRequest(w, r, "product_id, store_provider, and receipt_payload are required", nil)
		return
	}

	err := h.service.VerifyIAPReceipt(r.Context(), VerifyIAPInput{
		UserID:         userID,
		ProductID:      req.ProductID,
		StoreProvider:  req.StoreProvider,
		ReceiptPayload: req.ReceiptPayload,
	})
	if err != nil {
		if errors.Is(err, ErrIAPNotConfiguredInProduction) {
			httperr.ServiceUnavailableWithCode(w, r, "NOT_AVAILABLE", "In-app purchase receipt verification is disabled in production without server credentials")
			return
		}
		if errors.Is(err, ErrProductNotFound) {
			httperr.NotFound(w, r, "Product not found")
			return
		}
		if errors.Is(err, ErrReceiptAlreadyUsed) {
			httperr.Conflict(w, r, "IAP receipt has already been redeemed")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Receipt verified and item granted to inventory",
	})
}

type pauseSubscriptionRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (h *Handler) handlePauseSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	subID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid subscription UUID", nil)
		return
	}

	var req pauseSubscriptionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	sub, err := h.service.PauseSubscription(r.Context(), userID, subID, req.Reason)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			httperr.NotFound(w, r, "Subscription not found")
			return
		}
		if errors.Is(err, ErrInvalidSubscriptionState) {
			httperr.Conflict(w, r, "Subscription is not active and cannot be paused")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

func (h *Handler) handleResumeSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	subID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid subscription UUID", nil)
		return
	}

	sub, err := h.service.ResumeSubscription(r.Context(), userID, subID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			httperr.NotFound(w, r, "Subscription not found")
			return
		}
		if errors.Is(err, ErrInvalidSubscriptionState) {
			httperr.Conflict(w, r, "Subscription is not paused and cannot be resumed")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

type cancelSubscriptionRequest struct {
	Reason string `json:"reason,omitempty"`
}

func (h *Handler) handleCancelSubscription(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	subID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid subscription UUID", nil)
		return
	}

	var req cancelSubscriptionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	sub, err := h.service.CancelSubscription(r.Context(), userID, subID, req.Reason)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			httperr.NotFound(w, r, "Subscription not found")
			return
		}
		if errors.Is(err, ErrInvalidSubscriptionState) {
			httperr.Conflict(w, r, "Subscription cannot be cancelled from its current state")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, sub)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
