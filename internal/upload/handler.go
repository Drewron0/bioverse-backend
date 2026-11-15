package upload

import (
	"encoding/json"
	"net/http"

	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/chi/v5"
)

// Handler serves HTTP endpoints for client upload presigning.
type Handler struct {
	service *Service
}

// NewHandler constructs an upload HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes mounts upload endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Consumer uploads
	r.Route("/v1/app/uploads", func(ur chi.Router) {
		ur.Use(authMiddleware)
		ur.Post("/presign", h.handleUserPresign)
	})

	// Administrative uploads
	r.Route("/v1/admin/uploads", func(ar chi.Router) {
		ar.Use(authMiddleware)
		ar.Post("/presign", h.handleAdminPresign)
	})
}

type presignRequest struct {
	FileName    string  `json:"file_name"`
	ContentType string  `json:"content_type"`
	SizeBytes   int64   `json:"size_bytes"`
	Purpose     Purpose `json:"purpose"`
}

func (h *Handler) handleUserPresign(w http.ResponseWriter, r *http.Request) {
	h.processPresign(w, r, false)
}

func (h *Handler) handleAdminPresign(w http.ResponseWriter, r *http.Request) {
	h.processPresign(w, r, true)
}

func (h *Handler) processPresign(w http.ResponseWriter, r *http.Request, isAdmin bool) {
	var req presignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.RequestPresignedUpload(r.Context(), req.FileName, req.ContentType, req.SizeBytes, req.Purpose, isAdmin)
	if err != nil {
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}
