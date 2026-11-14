package catalog

import (
	"encoding/json"
	"net/http"

	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/chi/v5"
)

// Handler serves HTTP endpoints for the catalog.
type Handler struct {
	service *Service
}

// NewHandler constructs a new catalog HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes registers catalog endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/v1/app/catalog", func(cr chi.Router) {
		cr.Get("/products", h.handleListProducts)
		cr.Get("/plans", h.handleListPlans)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": products,
	})
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": plans,
	})
}
