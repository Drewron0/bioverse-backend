package gamification

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/middleware"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
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
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware, adminMiddleware func(http.Handler) http.Handler) {
	submissionLimiter := middleware.RateLimit(30, time.Minute)

	// Public Leaderboard route (OpenAPI security: [])
	r.Get("/v1/app/leaderboard", h.getLeaderboard)

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
		protected.With(submissionLimiter).Post("/v1/app/missions/{id}/submit", h.submitMission)
		protected.Get("/v1/app/mission-submissions/{id}", h.getMissionSubmission)

		// Streaks
		protected.Get("/v1/app/streaks", h.getStreak)
	})

	r.Group(func(adminRouter chi.Router) {
		adminRouter.Use(adminMiddleware)
		adminRouter.Use(identity.RequireAdminRole("super_admin", "operations", "content"))
		adminRouter.Post("/v1/admin/gamification/missions", h.handleAdminCreateMission)
		adminRouter.Put("/v1/admin/gamification/missions/{id}", h.handleAdminUpdateMission)
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

	if pets == nil {
		pets = []store.ListVirtualPetsByUserIDRow{}
	}

	// Returns array directly to comply with OpenAPI type: array schema for client generation
	writeJSON(w, http.StatusOK, pets)
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

	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))

	missions, err := h.service.ListActiveMissions(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	// Filter by cursor if provided
	if cursor != "" {
		foundIdx := -1
		for i, m := range missions {
			if m.ID.String() == cursor {
				foundIdx = i
				break
			}
		}
		if foundIdx >= 0 && foundIdx+1 < len(missions) {
			missions = missions[foundIdx+1:]
		} else if foundIdx >= 0 {
			missions = []store.EcoMissions{}
		}
	}

	var nextCursor *string
	if len(missions) > limit {
		cursorVal := missions[limit-1].ID.String()
		nextCursor = &cursorVal
		missions = missions[:limit]
	}

	if missions == nil {
		missions = []store.EcoMissions{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       missions,
		"next_cursor": nextCursor,
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

	if req.ProofObjectKey != "" {
		if !strings.HasPrefix(req.ProofObjectKey, "mission_proofs/") || strings.Contains(req.ProofObjectKey, "..") {
			httperr.BadRequest(w, r, "proof_object_key must begin with 'mission_proofs/' and not contain path traversals", nil)
			return
		}
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
	cursor := r.URL.Query().Get("cursor")
	period := r.URL.Query().Get("period")

	limit := int32(50)
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
	} else if c, err := strconv.Atoi(cursor); err == nil && c >= 0 {
		offset = int32(c)
	}

	board, err := h.service.GetLeaderboard(r.Context(), limit, offset, period)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	rankings := make([]leaderboardEntryResponse, 0, len(board))
	for _, b := range board {
		var avatar *string
		if b.AvatarUrl.Valid && b.AvatarUrl.String != "" {
			avatar = &b.AvatarUrl.String
		}
		usdDec := decimalutil.FromNumeric(b.TotalDonationsUsd)
		usd := usdDec.StringFixed(2)
		displayName := b.FullName
		userID := b.ID

		if b.IsAnonymous {
			displayName = "Anonymous Supporter"
			avatar = nil
			userID = uuid.Nil
		}

		rankings = append(rankings, leaderboardEntryResponse{
			Rank:              b.RankPosition,
			UserID:            userID,
			DisplayName:       displayName,
			AvatarURL:         avatar,
			TotalDonationsUSD: usd,
			TotalUnitsAdopted: b.TotalUnitsAdopted,
			IsAnonymous:       b.IsAnonymous,
		})
	}

	var nextCursor *string
	if int32(len(rankings)) == limit {
		nc := strconv.Itoa(int(offset + limit))
		nextCursor = &nc
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"rankings":    rankings,
		"next_cursor": nextCursor,
	})
}

type createMissionRequest struct {
	SponsorCompanyID *uuid.UUID      `json:"sponsor_company_id,omitempty"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	Instructions     string          `json:"instructions"`
	Category         string          `json:"category"`
	Difficulty       string          `json:"difficulty"`
	ImageURL         string          `json:"image_url"`
	PointsReward     int64           `json:"points_reward"`
	XPReward         int64           `json:"xp_reward"`
	RequiresCamera   bool            `json:"requires_camera_proof"`
	CVTargetLabel    string          `json:"cv_target_label"`
	CVMinConfidence  decimal.Decimal `json:"cv_min_confidence"`
	IsDaily          bool            `json:"is_daily"`
}

func (h *Handler) handleAdminCreateMission(w http.ResponseWriter, r *http.Request) {
	var req createMissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.Title == "" || req.Description == "" {
		httperr.BadRequest(w, r, "title and description are required", nil)
		return
	}

	m, err := h.service.CreateMission(r.Context(), CreateMissionInput{
		SponsorCompanyID: req.SponsorCompanyID,
		Title:            req.Title,
		Description:      req.Description,
		Instructions:     req.Instructions,
		Category:         req.Category,
		Difficulty:       req.Difficulty,
		ImageURL:         req.ImageURL,
		PointsReward:     req.PointsReward,
		XPReward:         req.XPReward,
		RequiresCamera:   req.RequiresCamera,
		CVTargetLabel:    req.CVTargetLabel,
		CVMinConfidence:  req.CVMinConfidence,
		IsDaily:          req.IsDaily,
	})
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, m)
}

type updateMissionRequest struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
	PointsReward int64  `json:"points_reward"`
	XPReward     int64  `json:"xp_reward"`
	IsActive     bool   `json:"is_active"`
}

func (h *Handler) handleAdminUpdateMission(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	mID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid mission UUID", nil)
		return
	}

	var req updateMissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	m, err := h.service.UpdateMission(r.Context(), mID, req.Title, req.Description, req.Instructions, req.PointsReward, req.XPReward, req.IsActive)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, m)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
