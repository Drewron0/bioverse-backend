package gamification

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Drewron01/bioverse-backend/internal/identity"
	"github.com/Drewron01/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler serves HTTP endpoints for the gamification domain.
type Handler struct {
	service *Service
}

// NewHandler constructs a gamification handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

// RegisterRoutes mounts all gamification endpoints on the router.
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Group(func(protected chi.Router) {
		protected.Use(authMiddleware)

		// Virtual Pets
		protected.Get("/v1/app/pets", h.listPets)
		protected.Post("/v1/app/pets", h.adoptPet)
		protected.Get("/v1/app/pets/{id}", h.getPet)
		protected.Post("/v1/app/pets/{id}/interact", h.interactPet)

		// Eco-Missions
		protected.Get("/v1/app/missions", h.listMissions)
		protected.Post("/v1/app/missions/{id}/submit", h.submitMission)

		// Streaks & Leaderboard
		protected.Get("/v1/app/streaks", h.getStreak)
		protected.Get("/v1/app/leaderboard", h.getLeaderboard)
	})
}

func (h *Handler) listPets(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	pets, err := h.service.ListUserPets(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"pets": pets})
}

type adoptPetRequest struct {
	PetName     string     `json:"pet_name"`
	PetSpecies  string     `json:"pet_species"`
	CoralType   string     `json:"coral_type"`
	FragmentID  *uuid.UUID `json:"fragment_id,omitempty"`
	MakePrimary bool       `json:"make_primary"`
}

func (h *Handler) adoptPet(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	var req adoptPetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.PetName == "" || req.PetSpecies == "" || req.CoralType == "" {
		httperr.BadRequest(w, r, "pet_name, pet_species, and coral_type are required", nil)
		return
	}

	pet, err := h.service.AdoptPet(r.Context(), AdoptPetInput{
		UserID:      userID,
		PetName:     req.PetName,
		PetSpecies:  req.PetSpecies,
		CoralType:   req.CoralType,
		FragmentID:  req.FragmentID,
		MakePrimary: req.MakePrimary,
	})
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, pet)
}

func (h *Handler) getPet(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	petID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid pet UUID", nil)
		return
	}

	pet, err := h.service.GetUserPet(r.Context(), userID, petID)
	if err != nil {
		if errors.Is(err, ErrPetNotFound) {
			httperr.NotFound(w, r, "Virtual pet not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, pet)
}

func (h *Handler) interactPet(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	petID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid pet UUID", nil)
		return
	}

	pet, err := h.service.InteractWithPet(r.Context(), userID, petID)
	if err != nil {
		if errors.Is(err, ErrPetNotFound) {
			httperr.NotFound(w, r, "Virtual pet not found")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, pet)
}

func (h *Handler) listMissions(w http.ResponseWriter, r *http.Request) {
	missions, err := h.service.ListActiveMissions(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"missions": missions})
}

type submitMissionRequest struct {
	ProofImageURL string     `json:"proof_image_url"`
	TargetPetID   *uuid.UUID `json:"target_pet_id,omitempty"`
}

func (h *Handler) submitMission(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	missionID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid mission UUID", nil)
		return
	}

	var req submitMissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.ProofImageURL == "" {
		httperr.BadRequest(w, r, "proof_image_url is required", nil)
		return
	}

	res, err := h.service.SubmitMissionProof(r.Context(), userID, missionID, req.TargetPetID, req.ProofImageURL)
	if err != nil {
		if errors.Is(err, ErrMissionNotFound) {
			httperr.NotFound(w, r, "Mission not found")
			return
		}
		if errors.Is(err, ErrMissionAlreadyCompletedToday) {
			httperr.Conflict(w, r, "Mission already submitted today")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) getStreak(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	streak, err := h.service.GetUserStreak(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, streak)
}

func (h *Handler) getLeaderboard(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := int32(50)
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
		limit = int32(l)
	}
	offset := int32(0)
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = int32(o)
	}

	board, err := h.service.GetLeaderboard(r.Context(), limit, offset)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"leaderboard": board})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
