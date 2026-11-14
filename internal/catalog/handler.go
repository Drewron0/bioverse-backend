package catalog

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

	writeJSON(w, http.StatusOK, map[string]any{"products": products})
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

	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
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

	var req buyProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.ProductID == uuid.Nil {
		httperr.BadRequest(w, r, "product_id is required", nil)
		return
	}

	inv, err := h.service.BuyProductWithPoints(r.Context(), userID, req.ProductID, req.Quantity)
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

	writeJSON(w, http.StatusOK, inv)
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
	PlanID        uuid.UUID               `json:"plan_id"`
	PaymentMethod store.PaymentMethodType `json:"payment_method"`
}

func (h *Handler) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
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
	if req.PaymentMethod == "" {
		req.PaymentMethod = store.PaymentMethodTypeEwallet
	}

	sub, err := h.service.SubscribePlan(r.Context(), userID, req.PlanID, req.PaymentMethod)
	if err != nil {
		if errors.Is(err, ErrPlanNotFound) {
			httperr.NotFound(w, r, "Subscription plan not found")
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

	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
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
