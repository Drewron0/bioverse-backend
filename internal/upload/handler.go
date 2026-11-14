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
	r.Route("/v1/app/uploads", func(ur chi.Router) {
		ur.Use(authMiddleware)
		ur.Post("/presign", h.handlePresign)
	})
}

type presignRequest struct {
	FileName    string  `json:"file_name"`
	ContentType string  `json:"content_type"`
	Purpose     Purpose `json:"purpose"`
}

type presignResponse struct {
	UploadURL        string `json:"upload_url"`
	ObjectKey        string `json:"object_key"`
	ExpiresInSeconds int    `json:"expires_in_seconds"`
}

func (h *Handler) handlePresign(w http.ResponseWriter, r *http.Request) {
	var req presignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid JSON payload provided.", nil)
		return
	}

	result, err := h.service.RequestPresignedUpload(r.Context(), req.FileName, req.ContentType, req.Purpose)
	if err != nil {
		httperr.BadRequest(w, r, err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(presignResponse{
		UploadURL:        result.UploadURL,
		ObjectKey:        result.ObjectKey,
		ExpiresInSeconds: result.ExpiresInSeconds,
	})
}
