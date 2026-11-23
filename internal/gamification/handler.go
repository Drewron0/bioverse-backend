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
	"github.com/jackc/pgx/v5/pgtype"
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

	// Public Leaderboard and Species routes
	r.Get("/v1/app/leaderboard", h.getLeaderboard)
	r.Get("/v1/app/pets/species", h.getSpecies)

	r.Group(func(protected chi.Router) {
		protected.Use(authMiddleware)

		// Virtual Pets
		protected.Get("/v1/app/pets", h.listPets)
		protected.Post("/v1/app/pets", h.adoptPet)
		protected.Get("/v1/app/pets/species", h.getSpecies)
		protected.Get("/v1/app/pets/{id}", h.getPet)
		protected.Post("/v1/app/pets/{id}/interact", h.interactPet)
		protected.Post("/v1/app/pets/{id}/equip", h.equipPetItem)

		// Eco-Missions
		protected.Get("/v1/app/missions", h.listMissions)
		protected.With(submissionLimiter).Post("/v1/app/missions/{id}/submit", h.submitMission)
		protected.Get("/v1/app/mission-submissions/my", h.listMySubmissions)
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

type virtualPetResponse struct {
	ID                string  `json:"id"`
	UserID            string  `json:"user_id"`
	FragmentID        *string `json:"fragment_id"`
	EquippedProductID *string `json:"equipped_product_id"`
	PetName           string  `json:"pet_name"`
	PetSpecies        string  `json:"pet_species"`
	CoralType         string  `json:"coral_type"`
	GrowthStage       string  `json:"growth_stage"`
	HealthPercent     int32   `json:"health_percent"`
	Level             int32   `json:"level"`
	ExperiencePoints  int64   `json:"experience_points"`
	XPToNextLevel     int64   `json:"xp_to_next_level"`
	IsPrimary         bool    `json:"is_primary"`
	LastInteractedAt  *string `json:"last_interacted_at"`
	AdoptedAt         string  `json:"adopted_at"`
	CreatedAt         string  `json:"created_at"`
	FragmentCode      *string `json:"fragment_code,omitempty"`
	CoralNickname     *string `json:"coral_nickname,omitempty"`
}

func toVirtualPetResponse(
	id, userID uuid.UUID,
	fragmentID, equippedProductID *uuid.UUID,
	petName, petSpecies, coralType string,
	growthStage string,
	healthPercent, level int32,
	xp int64,
	isPrimary bool,
	lastInteractedAt pgtype.Timestamptz,
	adoptedAt, createdAt time.Time,
	fragmentCode, coralNickname *string,
) virtualPetResponse {
	var fragIDStr *string
	if fragmentID != nil && *fragmentID != uuid.Nil {
		s := fragmentID.String()
		fragIDStr = &s
	}
	var equipIDStr *string
	if equippedProductID != nil && *equippedProductID != uuid.Nil {
		s := equippedProductID.String()
		equipIDStr = &s
	}
	var lastInterStr *string
	if lastInteractedAt.Valid {
		s := lastInteractedAt.Time.UTC().Format(time.RFC3339)
		lastInterStr = &s
	}

	return virtualPetResponse{
		ID:                id.String(),
		UserID:            userID.String(),
		FragmentID:        fragIDStr,
		EquippedProductID: equipIDStr,
		PetName:           petName,
		PetSpecies:        petSpecies,
		CoralType:         coralType,
		GrowthStage:       growthStage,
		HealthPercent:     healthPercent,
		Level:             level,
		ExperiencePoints:  xp,
		XPToNextLevel:     CalculateXPToNextLevel(xp, level),
		IsPrimary:         isPrimary,
		LastInteractedAt:  lastInterStr,
		AdoptedAt:         adoptedAt.UTC().Format(time.RFC3339),
		CreatedAt:         createdAt.UTC().Format(time.RFC3339),
		FragmentCode:      fragmentCode,
		CoralNickname:     coralNickname,
	}
}

func mapListRowToPetResponse(p store.ListVirtualPetsByUserIDRow) virtualPetResponse {
	var fragID *uuid.UUID
	if p.FragmentID.Valid {
		u := uuid.UUID(p.FragmentID.Bytes)
		fragID = &u
	}
	var eqID *uuid.UUID
	if p.EquippedProductID.Valid {
		u := uuid.UUID(p.EquippedProductID.Bytes)
		eqID = &u
	}
	var fragCode *string
	if p.FragmentCode.Valid {
		fragCode = &p.FragmentCode.String
	}
	var coralNick *string
	if p.CoralNickname.Valid {
		coralNick = &p.CoralNickname.String
	}
	return toVirtualPetResponse(
		p.ID, p.UserID, fragID, eqID,
		p.PetName, p.PetSpecies, p.CoralType,
		string(p.GrowthStage), p.HealthPercent, p.Level, p.ExperiencePoints,
		p.IsPrimary, p.LastInteractedAt, p.AdoptedAt.Time, p.CreatedAt.Time,
		fragCode, coralNick,
	)
}

func mapGetRowToPetResponse(p store.GetVirtualPetByIDRow) virtualPetResponse {
	var fragID *uuid.UUID
	if p.FragmentID.Valid {
		u := uuid.UUID(p.FragmentID.Bytes)
		fragID = &u
	}
	var eqID *uuid.UUID
	if p.EquippedProductID.Valid {
		u := uuid.UUID(p.EquippedProductID.Bytes)
		eqID = &u
	}
	var fragCode *string
	if p.FragmentCode.Valid {
		fragCode = &p.FragmentCode.String
	}
	var coralNick *string
	if p.CoralNickname.Valid {
		coralNick = &p.CoralNickname.String
	}
	return toVirtualPetResponse(
		p.ID, p.UserID, fragID, eqID,
		p.PetName, p.PetSpecies, p.CoralType,
		string(p.GrowthStage), p.HealthPercent, p.Level, p.ExperiencePoints,
		p.IsPrimary, p.LastInteractedAt, p.AdoptedAt.Time, p.CreatedAt.Time,
		fragCode, coralNick,
	)
}

func mapStorePetToResponse(p *store.VirtualPets) virtualPetResponse {
	var fragID *uuid.UUID
	if p.FragmentID.Valid {
		u := uuid.UUID(p.FragmentID.Bytes)
		fragID = &u
	}
	var eqID *uuid.UUID
	if p.EquippedProductID.Valid {
		u := uuid.UUID(p.EquippedProductID.Bytes)
		eqID = &u
	}
	return toVirtualPetResponse(
		p.ID, p.UserID, fragID, eqID,
		p.PetName, p.PetSpecies, p.CoralType,
		string(p.GrowthStage), p.HealthPercent, p.Level, p.ExperiencePoints,
		p.IsPrimary, p.LastInteractedAt, p.AdoptedAt.Time, p.CreatedAt.Time,
		nil, nil,
	)
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

	resp := make([]virtualPetResponse, 0, len(pets))
	for _, p := range pets {
		resp = append(resp, mapListRowToPetResponse(p))
	}

	writeJSON(w, http.StatusOK, resp)
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

	writeJSON(w, http.StatusCreated, mapStorePetToResponse(pet))
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

	writeJSON(w, http.StatusOK, mapGetRowToPetResponse(*pet))
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

	writeJSON(w, http.StatusOK, mapStorePetToResponse(pet))
}

func (h *Handler) getSpecies(w http.ResponseWriter, r *http.Request) {
	species := []map[string]any{
		{
			"species_id":       "turtle",
			"name":             "Penyu Hijau (Sea Turtle)",
			"description":      "Penjaga terumbu karang yang tenang, tangguh mengarungi samudera luas.",
			"avatar_asset_url": "https://images.unsplash.com/photo-1544551763-46a013bb70d5",
			"element":          "ocean",
			"rarity":           "common",
		},
		{
			"species_id":       "clownfish",
			"name":             "Ikan Badut (Clownfish)",
			"description":      "Sahabat setia anemon laut yang ceria, gesit, dan penuh semangat.",
			"avatar_asset_url": "https://images.unsplash.com/photo-1524704654690-b56c05c78a00",
			"element":          "reef",
			"rarity":           "common",
		},
		{
			"species_id":       "manta_ray",
			"name":             "Pari Manta (Manta Ray)",
			"description":      "Penjelajah pelagis yang anggun dengan sayap lebar nan menawan.",
			"avatar_asset_url": "https://images.unsplash.com/photo-1544551763-77ef2d0cfc6c",
			"element":          "pelagic",
			"rarity":           "rare",
		},
		{
			"species_id":       "crab",
			"name":             "Kepiting Bakau (Mangrove Crab)",
			"description":      "Penjaga garis pantai dan akar bakau yang pantang menyerah.",
			"avatar_asset_url": "https://images.unsplash.com/photo-1559827291-72ee739d0d9a",
			"element":          "mangrove",
			"rarity":           "common",
		},
		{
			"species_id":       "seahorse",
			"name":             "Kuda Laut (Seahorse)",
			"description":      "Penghuni padang lamun yang tenang, setia, dan eksotis.",
			"avatar_asset_url": "https://images.unsplash.com/photo-1546026423-cc4642628d2b",
			"element":          "seagrass",
			"rarity":           "rare",
		},
	}
	writeJSON(w, http.StatusOK, species)
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

	writeJSON(w, http.StatusOK, mapStorePetToResponse(pet))
}

type missionResponseItem struct {
	ID                     string  `json:"id"`
	SponsorCompanyID       *string `json:"sponsor_company_id"`
	Title                  string  `json:"title"`
	Description            string  `json:"description"`
	Instructions           string  `json:"instructions"`
	Category               string  `json:"category"`
	Difficulty             string  `json:"difficulty"`
	ImageURL               *string `json:"image_url"`
	PointsReward           int64   `json:"points_reward"`
	XPReward               int64   `json:"xp_reward"`
	RequiresCameraProof    bool    `json:"requires_camera_proof"`
	CVTargetLabel          *string `json:"cv_target_label"`
	CVMinConfidence        float64 `json:"cv_min_confidence"`
	IsDaily                bool    `json:"is_daily"`
	IsActive               bool    `json:"is_active"`
	EstimatedMinutes       int32   `json:"estimated_minutes"`
	CompletedToday         bool    `json:"completed_today"`
	MyLastSubmissionStatus *string `json:"my_last_submission_status"`
	CreatedAt              string  `json:"created_at"`
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

	var items []missionResponseItem

	userID, isAuth := identity.GetUserIDFromContext(r.Context())
	if isAuth {
		userMissions, err := h.service.store.ListActiveMissionsForUser(r.Context(), userID)
		if err == nil {
			items = make([]missionResponseItem, 0, len(userMissions))
			for _, m := range userMissions {
				var sponsorID *string
				if m.SponsorCompanyID.Valid {
					s := uuid.UUID(m.SponsorCompanyID.Bytes).String()
					sponsorID = &s
				}
				var imgURL *string
				if m.ImageUrl.Valid {
					imgURL = &m.ImageUrl.String
				}
				var targetLabel *string
				if m.CvTargetLabel.Valid {
					targetLabel = &m.CvTargetLabel.String
				}
				conf, _ := decimalutil.FromNumeric(m.CvMinConfidence).Float64()

				var lastStatus *string
				if len(m.MyLastSubmissionStatus) > 0 {
					s := string(m.MyLastSubmissionStatus)
					lastStatus = &s
				}

				items = append(items, missionResponseItem{
					ID:                     m.ID.String(),
					SponsorCompanyID:       sponsorID,
					Title:                  m.Title,
					Description:            m.Description,
					Instructions:           m.Instructions,
					Category:               m.Category.String,
					Difficulty:             m.Difficulty.String,
					ImageURL:               imgURL,
					PointsReward:           m.PointsReward,
					XPReward:               m.XpReward,
					RequiresCameraProof:    m.RequiresCameraProof,
					CVTargetLabel:          targetLabel,
					CVMinConfidence:        conf,
					IsDaily:                m.IsDaily,
					IsActive:               m.IsActive,
					EstimatedMinutes:       m.EstimatedMinutes,
					CompletedToday:         m.CompletedToday,
					MyLastSubmissionStatus: lastStatus,
					CreatedAt:              m.CreatedAt.Time.UTC().Format(time.RFC3339),
				})
			}
		}
	}

	if items == nil {
		rawMissions, err := h.service.ListActiveMissions(r.Context())
		if err != nil {
			httperr.InternalServerError(w, r)
			return
		}
		items = make([]missionResponseItem, 0, len(rawMissions))
		for _, m := range rawMissions {
			var sponsorID *string
			if m.SponsorCompanyID.Valid {
				s := uuid.UUID(m.SponsorCompanyID.Bytes).String()
				sponsorID = &s
			}
			var imgURL *string
			if m.ImageUrl.Valid {
				imgURL = &m.ImageUrl.String
			}
			var targetLabel *string
			if m.CvTargetLabel.Valid {
				targetLabel = &m.CvTargetLabel.String
			}
			conf, _ := decimalutil.FromNumeric(m.CvMinConfidence).Float64()

			items = append(items, missionResponseItem{
				ID:                     m.ID.String(),
				SponsorCompanyID:       sponsorID,
				Title:                  m.Title,
				Description:            m.Description,
				Instructions:           m.Instructions,
				Category:               m.Category.String,
				Difficulty:             m.Difficulty.String,
				ImageURL:               imgURL,
				PointsReward:           m.PointsReward,
				XPReward:               m.XpReward,
				RequiresCameraProof:    m.RequiresCameraProof,
				CVTargetLabel:          targetLabel,
				CVMinConfidence:        conf,
				IsDaily:                m.IsDaily,
				IsActive:               m.IsActive,
				EstimatedMinutes:       m.EstimatedMinutes,
				CompletedToday:         false,
				MyLastSubmissionStatus: nil,
				CreatedAt:              m.CreatedAt.Time.UTC().Format(time.RFC3339),
			})
		}
	}

	// Filter by cursor if provided
	if cursor != "" {
		foundIdx := -1
		for i, m := range items {
			if m.ID == cursor {
				foundIdx = i
				break
			}
		}
		if foundIdx >= 0 && foundIdx+1 < len(items) {
			items = items[foundIdx+1:]
		} else if foundIdx >= 0 {
			items = []missionResponseItem{}
		}
	}

	var nextCursor *string
	if len(items) > limit {
		cursorVal := items[limit-1].ID
		nextCursor = &cursorVal
		items = items[:limit]
	}

	if items == nil {
		items = []missionResponseItem{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
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

	message := "Proof photo received. Computer vision verification in progress."
	if res.Status == store.SubmissionStatusNeedsReview {
		message = "Proof photo received. Automated checker was unavailable, queued for review."
	}

	// Returns 202 Accepted with MissionSubmissionReceipt as defined in OpenAPI spec
	receipt := map[string]any{
		"submission_id": res.SubmissionID,
		"status":        res.Status,
		"message":       message,
		"created_at":    time.Now().UTC(),
	}
	writeJSON(w, http.StatusAccepted, receipt)
}

func (h *Handler) listMySubmissions(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required")
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 100 {
			limit = 100
		} else {
			limit = l
		}
	}

	offsetStr := r.URL.Query().Get("offset")
	offset := 0
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}

	submissions, err := h.service.store.ListSubmissionsByUserID(r.Context(), store.ListSubmissionsByUserIDParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	type submissionItem struct {
		ID                string   `json:"id"`
		MissionID         string   `json:"mission_id"`
		MissionTitle      string   `json:"mission_title"`
		MissionCategory   string   `json:"mission_category"`
		PetID             *string  `json:"pet_id"`
		ProofImageURL     string   `json:"proof_image_url"`
		Status            string   `json:"status"`
		AwardedPoints     int64    `json:"awarded_points"`
		AwardedXP         int64    `json:"awarded_xp"`
		CvConfidenceScore *float64 `json:"cv_confidence_score"`
		RejectionReason   *string  `json:"rejection_reason"`
		SubmittedAt       string   `json:"submitted_at"`
		VerifiedAt        *string  `json:"verified_at"`
	}

	items := make([]submissionItem, 0, len(submissions))
	for _, s := range submissions {
		var petIDStr *string
		if s.PetID.Valid {
			str := uuid.UUID(s.PetID.Bytes).String()
			petIDStr = &str
		}
		var score *float64
		if s.CvConfidenceScore.Valid {
			f, _ := s.CvConfidenceScore.Float64Value()
			score = &f.Float64
		}
		var rejReason *string
		if s.RejectionReason.Valid {
			rejReason = &s.RejectionReason.String
		}
		var verifiedAtStr *string
		if s.VerifiedAt.Valid {
			str := s.VerifiedAt.Time.UTC().Format(time.RFC3339)
			verifiedAtStr = &str
		}

		items = append(items, submissionItem{
			ID:                s.ID.String(),
			MissionID:         s.MissionID.String(),
			MissionTitle:      s.MissionTitle,
			MissionCategory:   s.MissionCategory.String,
			PetID:             petIDStr,
			ProofImageURL:     s.ProofImageUrl.String,
			Status:            string(s.Status),
			AwardedPoints:     s.AwardedPoints,
			AwardedXP:         s.AwardedXp,
			CvConfidenceScore: score,
			RejectionReason:   rejReason,
			SubmittedAt:       s.SubmittedAt.Time.UTC().Format(time.RFC3339),
			VerifiedAt:        verifiedAtStr,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
	})
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
	category := r.URL.Query().Get("category")

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

	board, err := h.service.GetLeaderboard(r.Context(), limit, offset, period, category)
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
