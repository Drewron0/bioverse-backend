package conservation

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Drewron01/bioverse-backend/internal/identity"
	"github.com/Drewron01/bioverse-backend/internal/platform/decimalutil"
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
			protected.Get("/corals/{id}", h.handleGetCoral)
			protected.Get("/corals/{id}/timeline", h.handleGetCoralTimeline)
		})
	})

	r.Get("/v1/app/impact/summary", h.handleGetImpactSummary)
	r.Get("/v1/app/news", h.handleListNews)
}

type projectResponse struct {
	ID                    string `json:"id"`
	Title                 string `json:"title"`
	Description           string `json:"description"`
	Objectives            string `json:"objectives"`
	Category              string `json:"category"`
	SpeciesOrTypeName     string `json:"species_or_type_name"`
	Status                string `json:"status"`
	PhysicalHealthStatus  string `json:"physical_health_status"`
	CurrencyCode          string `json:"currency_code"`
	PricePerUnitMoney     string `json:"price_per_unit_money"`
	PricePerUnitPoints    int64  `json:"price_per_unit_points"`
	TargetAmount          string `json:"target_amount"`
	TargetUnits           int32  `json:"target_units"`
	FundedUnitsCount      int32  `json:"funded_units_count"`
	PlantedUnitsCount     int32  `json:"planted_units_count"`
	SurvivingUnitsCount   int32  `json:"surviving_units_count"`
	DonationStartDate     string `json:"donation_start_date"`
	DonationEndDate       string `json:"donation_end_date"`
	EstimatedPlantingDate string `json:"estimated_planting_date"`
	ImageURL              string `json:"image_url"`
	IsUrgent              bool   `json:"is_urgent"`
	CreatedAt             string `json:"created_at"`
}

func (h *Handler) handleListProjects(w http.ResponseWriter, r *http.Request) {
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

	projects, err := h.service.ListProjects(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	categoryFilter := r.URL.Query().Get("category")

	items := make([]projectResponse, 0, len(projects))
	for _, p := range projects {
		if categoryFilter != "" && string(p.Category) != categoryFilter {
			continue
		}

		isUrgent := false
		if p.DonationEndDate.Valid {
			daysUntilEnd := time.Until(p.DonationEndDate.Time).Hours() / 24
			if daysUntilEnd <= 30 && daysUntilEnd >= 0 {
				isUrgent = true
			}
		}
		if p.TargetUnits > 0 && float64(p.FundedUnitsCount)/float64(p.TargetUnits) >= 0.8 {
			isUrgent = true
		}

		items = append(items, projectResponse{
			ID:                    p.ID.String(),
			Title:                 p.Title,
			Description:           p.Description,
			Objectives:            p.Objectives,
			Category:              string(p.Category),
			SpeciesOrTypeName:     p.SpeciesOrTypeName,
			Status:                string(p.Status),
			PhysicalHealthStatus:  string(p.PhysicalHealthStatus),
			CurrencyCode:          p.CurrencyCode,
			PricePerUnitMoney:     decimalutil.FromNumeric(p.PricePerUnitMoney).StringFixed(2),
			PricePerUnitPoints:    p.PricePerUnitPoints,
			TargetAmount:          decimalutil.FromNumeric(p.TargetAmount).StringFixed(2),
			TargetUnits:           p.TargetUnits,
			FundedUnitsCount:      p.FundedUnitsCount,
			PlantedUnitsCount:     p.PlantedUnitsCount,
			SurvivingUnitsCount:   p.SurvivingUnitsCount,
			DonationStartDate:     p.DonationStartDate.Time.Format("2006-01-02"),
			DonationEndDate:       p.DonationEndDate.Time.Format("2006-01-02"),
			EstimatedPlantingDate: p.EstimatedPlantingDate.Time.Format("2006-01-02"),
			ImageURL:              p.ImageUrl,
			IsUrgent:              isUrgent,
			CreatedAt:             time.Now().UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       items,
		"next_cursor": nil,
	})
}

func (h *Handler) handleGetProject(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	projectID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid project ID UUID format.", nil)
		return
	}

	p, err := h.service.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			httperr.NotFound(w, r, "Conservation project not found.")
			return
		}
		httperr.InternalServerError(w, r)
		return
	}

	isUrgent := false
	if p.DonationEndDate.Valid {
		daysUntilEnd := time.Until(p.DonationEndDate.Time).Hours() / 24
		if daysUntilEnd <= 30 && daysUntilEnd >= 0 {
			isUrgent = true
		}
	}
	if p.TargetUnits > 0 && float64(p.FundedUnitsCount)/float64(p.TargetUnits) >= 0.8 {
		isUrgent = true
	}

	resp := projectResponse{
		ID:                    p.ID.String(),
		Title:                 p.Title,
		Description:           p.Description,
		Objectives:            p.Objectives,
		Category:              string(p.Category),
		SpeciesOrTypeName:     p.SpeciesOrTypeName,
		Status:                string(p.Status),
		PhysicalHealthStatus:  string(p.PhysicalHealthStatus),
		CurrencyCode:          p.CurrencyCode,
		PricePerUnitMoney:     decimalutil.FromNumeric(p.PricePerUnitMoney).StringFixed(2),
		PricePerUnitPoints:    p.PricePerUnitPoints,
		TargetAmount:          decimalutil.FromNumeric(p.TargetAmount).StringFixed(2),
		TargetUnits:           p.TargetUnits,
		FundedUnitsCount:      p.FundedUnitsCount,
		PlantedUnitsCount:     p.PlantedUnitsCount,
		SurvivingUnitsCount:   p.SurvivingUnitsCount,
		DonationStartDate:     p.DonationStartDate.Time.Format("2006-01-02"),
		DonationEndDate:       p.DonationEndDate.Time.Format("2006-01-02"),
		EstimatedPlantingDate: p.EstimatedPlantingDate.Time.Format("2006-01-02"),
		ImageURL:              p.ImageUrl,
		IsUrgent:              isUrgent,
		CreatedAt:             time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleListMyCorals(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

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

	corals, err := h.service.ListUserAdoptedCorals(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	type coralItem struct {
		ID              string  `json:"id"`
		FragmentCode    string  `json:"fragment_code"`
		Nickname        *string `json:"nickname"`
		ProjectID       string  `json:"project_id"`
		ProjectName     string  `json:"project_name"`
		SpeciesName     string  `json:"species_name"`
		LifecycleStatus string  `json:"lifecycle_status"`
		HealthStatus    string  `json:"health_status"`
		SpiderFrameTag  *string `json:"spider_frame_tag"`
		LatestPhotoURL  *string `json:"latest_photo_url"`
		PlantedAt       *string `json:"planted_at"`
		CreatedAt       string  `json:"created_at"`
	}

	items := make([]coralItem, 0, len(corals))
	for _, c := range corals {
		var nick *string
		if c.Nickname.Valid {
			nick = &c.Nickname.String
		}
		var photo *string
		if c.LatestPhotoUrl.Valid {
			photo = &c.LatestPhotoUrl.String
		}
		var planted *string
		if c.PlantedAt.Valid {
			p := c.PlantedAt.Time.Format(time.RFC3339)
			planted = &p
		}
		health := "healthy"
		if c.HealthStatus.Valid {
			health = string(c.HealthStatus.EnvironmentalHealthStatus)
		}

		items = append(items, coralItem{
			ID:              c.ID.String(),
			FragmentCode:    c.FragmentCode,
			Nickname:        nick,
			ProjectID:       c.ProjectID.String(),
			ProjectName:     c.ProjectTitle,
			SpeciesName:     c.SpeciesName,
			LifecycleStatus: string(c.LifecycleStatus),
			HealthStatus:    health,
			SpiderFrameTag:  nil,
			LatestPhotoURL:  photo,
			PlantedAt:       planted,
			CreatedAt:       c.CreatedAt.Time.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       items,
		"next_cursor": nil,
	})
}

func (h *Handler) handleGetCoral(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	fragID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid coral fragment UUID format.", nil)
		return
	}

	frag, err := h.service.GetCoralFragment(r.Context(), fragID)
	if err != nil {
		httperr.NotFound(w, r, "Adopted coral fragment not found.")
		return
	}

	var nick *string
	if frag.Nickname.Valid {
		nick = &frag.Nickname.String
	}
	var photo *string
	if frag.LatestPhotoUrl.Valid {
		photo = &frag.LatestPhotoUrl.String
	}
	var planted *string
	if frag.PlantedAt.Valid {
		p := frag.PlantedAt.Time.Format(time.RFC3339)
		planted = &p
	}
	var frameTag *string
	if frag.SpiderFrameTag.Valid {
		frameTag = &frag.SpiderFrameTag.String
	}
	health := "healthy"
	if frag.HealthStatus.Valid {
		health = string(frag.HealthStatus.EnvironmentalHealthStatus)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":               frag.ID.String(),
		"fragment_code":    frag.FragmentCode,
		"nickname":         nick,
		"project_id":       frag.ProjectID.String(),
		"project_name":     frag.ProjectTitle,
		"species_name":     frag.SpeciesName,
		"lifecycle_status": string(frag.LifecycleStatus),
		"health_status":    health,
		"spider_frame_tag": frameTag,
		"latest_photo_url": photo,
		"planted_at":       planted,
		"created_at":       frag.CreatedAt.Time.Format(time.RFC3339),
	})
}

func (h *Handler) handleGetCoralTimeline(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	fragID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid coral fragment UUID format.", nil)
		return
	}

	milestones, err := h.service.ListCoralMilestones(r.Context(), fragID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	type milestoneItem struct {
		MilestoneMonth int32   `json:"milestone_month"`
		PhotoURL       string  `json:"photo_url"`
		HealthStatus   string  `json:"health_status"`
		GrowthCM       float64 `json:"growth_cm"`
		Notes          *string `json:"notes"`
		VerifiedAt     string  `json:"verified_at"`
	}

	items := make([]milestoneItem, 0, len(milestones))
	for _, m := range milestones {
		var notes *string
		if m.SubtitleMetrics.Valid {
			notes = &m.SubtitleMetrics.String
		}
		health := "healthy"
		if m.HealthStatus.Valid {
			health = string(m.HealthStatus.EnvironmentalHealthStatus)
		}

		items = append(items, milestoneItem{
			MilestoneMonth: m.MilestoneMonth.Int32,
			PhotoURL:       m.MediaUrl,
			HealthStatus:   health,
			GrowthCM:       1.45,
			Notes:          notes,
			VerifiedAt:     m.CapturedAt.Time.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(items)
}

func (h *Handler) handleGetImpactSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.GetImpactSummary(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	biomass, _ := decimalutil.FromNumeric(summary.TotalBiomassRestoredKg).Float64()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"total_corals_adopted":      summary.TotalCoralsAdopted,
		"total_trees_planted":       summary.TotalTreesPlanted,
		"active_guardians_count":    summary.ActiveGuardiansCount,
		"total_biomass_restored_kg": biomass,
	})
}

func (h *Handler) handleListNews(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 50 {
			limit = 50
		} else {
			limit = l
		}
	}

	articles, err := h.service.ListNewsArticles(r.Context(), int32(limit))
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	type newsItem struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Category     string `json:"category"`
		Summary      string `json:"summary"`
		Content      string `json:"content"`
		ThumbnailURL string `json:"thumbnail_url"`
		IsFeatured   bool   `json:"is_featured"`
		PublishedAt  string `json:"published_at"`
	}

	items := make([]newsItem, 0, len(articles))
	for _, a := range articles {
		items = append(items, newsItem{
			ID:           a.ID.String(),
			Title:        a.Title,
			Category:     string(a.Category),
			Summary:      a.Summary,
			Content:      a.Content,
			ThumbnailURL: a.ThumbnailUrl,
			IsFeatured:   a.IsFeatured,
			PublishedAt:  a.PublishedAt.Time.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       items,
		"next_cursor": nil,
	})
}
