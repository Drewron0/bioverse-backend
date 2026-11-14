package conservation

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Drewron01/bioverse-backend/internal/identity"
	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves HTTP endpoints for conservation projects and adopted fragments.
type Handler struct {
	service *Service
}

// NewHandler constructs a new conservation HTTP handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes registers conservation routes on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/v1/app/conservation", func(cr chi.Router) {
		cr.Get("/projects", h.handleListProjects)
		cr.Get("/projects/{id}", h.handleGetProject)

		cr.Group(func(protected chi.Router) {
			protected.Use(authMiddleware)
			protected.Get("/corals/my", h.handleListMyCorals)
		})
	})
}

func (h *Handler) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.service.ListProjects(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": projects,
	})
}

func (h *Handler) handleGetProject(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid project ID UUID format.", nil)
		return
	}

	project, err := h.service.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			httperr.NotFound(w, r, "Conservation project not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": project,
	})
}

func (h *Handler) handleListMyCorals(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

	corals, err := h.service.ListUserAdoptedCorals(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": corals,
	})
}
