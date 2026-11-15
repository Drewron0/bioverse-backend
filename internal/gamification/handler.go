package gamification

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
		protected.Post("/v1/app/pets/{id}/equip", h.equipPetItem)

		// Eco-Missions
		protected.Get("/v1/app/missions", h.listMissions)
		protected.Post("/v1/app/missions/{id}/submit", h.submitMission)
		protected.Get("/v1/app/mission-submissions/{id}", h.getMissionSubmission)

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

type equipPetItemRequest struct {
	ProductID uuid.UUID `json:"product_id"`
}

func (h *Handler) equipPetItem(w http.ResponseWriter, r *http.Request) {
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

	var req equipPetItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.ProductID == uuid.Nil {
		httperr.BadRequest(w, r, "product_id is required", nil)
		return
	}

	pet, err := h.service.EquipPetItem(r.Context(), userID, petID, req.ProductID)
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
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 50 {
			limit = 50
		} else {
			limit = l
		}
	}
	_ = limit

	missions, err := h.service.ListActiveMissions(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       missions,
		"next_cursor": nil,
	})
}

type submitMissionRequest struct {
	ProofObjectKey string     `json:"proof_object_key"`
	ProofImageURL  string     `json:"proof_image_url,omitempty"`
	PetID          *uuid.UUID `json:"pet_id,omitempty"`
	TargetPetID    *uuid.UUID `json:"target_pet_id,omitempty"`
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

	proofTarget := req.ProofObjectKey
	if proofTarget == "" {
		proofTarget = req.ProofImageURL
	}
	if proofTarget == "" {
		httperr.BadRequest(w, r, "proof_object_key is required", nil)
		return
	}

	targetPetID := req.PetID
	if targetPetID == nil {
		targetPetID = req.TargetPetID
	}

	res, err := h.service.SubmitMissionProof(r.Context(), userID, missionID, targetPetID, proofTarget)
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

	// Returns 202 Accepted with MissionSubmissionReceipt as defined in OpenAPI spec
	receipt := map[string]any{
		"submission_id": res.SubmissionID,
		"status":        res.Status,
		"message":       "Proof photo received. Computer vision verification in progress.",
		"created_at":    time.Now().UTC(),
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

func (h *Handler) getMissionSubmission(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	idStr := chi.URLParam(r, "id")
	subID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid submission UUID", nil)
		return
	}

	sub, err := h.service.GetMissionSubmission(r.Context(), userID, subID)
	if err != nil {
		httperr.NotFound(w, r, "Mission submission not found")
		return
	}

	var score *float64
	if sub.CvConfidenceScore.Valid {
		f, _ := sub.CvConfidenceScore.Float64Value()
		score = &f.Float64
	}

	var pts *int64
	if sub.AwardedPoints > 0 {
		pts = &sub.AwardedPoints
	}
	var xp *int64
	if sub.AwardedXp > 0 {
		xp = &sub.AwardedXp
	}

	var verifiedAt *time.Time
	if sub.VerifiedAt.Valid {
		verifiedAt = &sub.VerifiedAt.Time
	}

	detail := map[string]any{
		"id":                  sub.ID,
		"mission_id":          sub.MissionID,
		"status":              sub.Status,
		"cv_confidence_score": score,
		"awarded_points":      pts,
		"awarded_xp":          xp,
		"verified_at":         verifiedAt,
		"created_at":          sub.SubmittedAt,
	}

	writeJSON(w, http.StatusOK, detail)
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

	var lastDate *string
	if streak.LastStreakDate.Valid {
		d := streak.LastStreakDate.Time.Format("2006-01-02")
		lastDate = &d
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"current_streak":     streak.CurrentStreak,
		"longest_streak":     streak.LongestStreak,
		"last_activity_date": lastDate,
		"is_freeze_active":   false,
		"freeze_count":       0,
	})
}

type leaderboardEntryResponse struct {
	Rank              int64   `json:"rank"`
	UserID            uuid.UUID `json:"user_id"`
	DisplayName       string  `json:"display_name"`
	AvatarURL         *string `json:"avatar_url"`
	TotalDonationsUSD string  `json:"total_donations_usd"`
	TotalUnitsAdopted int64   `json:"total_units_adopted"`
	IsAnonymous       bool    `json:"is_anonymous"`
}

func (h *Handler) getLeaderboard(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := int32(10)
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 50 {
			limit = 50
		} else {
			limit = int32(l)
		}
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

	rate := h.service.GetPointConversionRate(r.Context())
	if rate.IsZero() {
		rate = decimal.NewFromInt(100)
	}

	rankings := make([]leaderboardEntryResponse, 0, len(board))
	for _, b := range board {
		var avatar *string
		if b.AvatarUrl.Valid && b.AvatarUrl.String != "" {
			avatar = &b.AvatarUrl.String
		}
		pointsDec := decimal.NewFromInt(b.PointsBalance)
		usdDec := pointsDec.DivRound(rate, 2)
		usd := usdDec.StringFixed(2)
		units := b.PointsBalance / 100

		rankings = append(rankings, leaderboardEntryResponse{
			Rank:              b.RankPosition,
			UserID:            b.ID,
			DisplayName:       b.FullName,
			AvatarURL:         avatar,
			TotalDonationsUSD: usd,
			TotalUnitsAdopted: units,
			IsAnonymous:       false,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rankings":    rankings,
		"next_cursor": nil,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
