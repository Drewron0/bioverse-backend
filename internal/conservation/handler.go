package conservation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/identity"
	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/httperr"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
func (h *Handler) RegisterRoutes(r chi.Router, authMiddleware, adminMiddleware func(http.Handler) http.Handler) {
	r.Route("/v1/app/conservation", func(cr chi.Router) {
		cr.Get("/projects", h.handleListProjects)
		cr.Get("/projects/{id}", h.handleGetProject)

		cr.Group(func(protected chi.Router) {
			protected.Use(authMiddleware)
			protected.Get("/corals/my", h.handleListMyCorals)
			protected.Get("/corals/{id}", h.handleGetCoral)
			protected.Get("/corals/{id}/timeline", h.handleGetCoralTimeline)
			protected.Get("/units/my", h.handleListMyUnits)
		})
	})

	r.Get("/v1/app/impact/summary", h.handleGetImpactSummary)
	r.Get("/v1/app/news", h.handleListNews)

	r.Group(func(adminRouter chi.Router) {
		adminRouter.Use(adminMiddleware)

		adminRouter.Group(func(ops chi.Router) {
			ops.Use(identity.RequireAdminRole("super_admin", "operations"))
			ops.Post("/v1/admin/conservation/zones", h.handleAdminCreateZone)
			ops.Get("/v1/admin/conservation/zones", h.handleAdminListZones)
			ops.Put("/v1/admin/conservation/zones/{id}", h.handleAdminUpdateZone)
			ops.Post("/v1/admin/conservation/projects", h.handleAdminCreateProject)
			ops.Put("/v1/admin/conservation/projects/{id}", h.handleAdminUpdateProject)
			ops.Post("/v1/admin/conservation/frames", h.handleAdminCreateFrame)
			ops.Get("/v1/admin/conservation/frames", h.handleAdminListFrames)
		})

		adminRouter.Group(func(content chi.Router) {
			content.Use(identity.RequireAdminRole("super_admin", "content", "operations"))
			content.Post("/v1/admin/content/news", h.handleAdminCreateNews)
			content.Put("/v1/admin/content/news/{id}", h.handleAdminUpdateNews)
			content.Delete("/v1/admin/content/news/{id}", h.handleAdminDeleteNews)
		})
	})
}

type projectResponse struct {
	ID                    string  `json:"id"`
	Title                 string  `json:"title"`
	Description           string  `json:"description"`
	Objectives            string  `json:"objectives"`
	Category              string  `json:"category"`
	SpeciesOrTypeName     string  `json:"species_or_type_name"`
	Status                string  `json:"status"`
	PhysicalHealthStatus  string  `json:"physical_health_status"`
	LocationName          string  `json:"location_name"`
	Latitude              float64 `json:"latitude"`
	Longitude             float64 `json:"longitude"`
	ContributorsCount     int64   `json:"contributors_count"`
	CurrencyCode          string  `json:"currency_code"`
	PricePerUnitMoney     string  `json:"price_per_unit_money"`
	PricePerUnitPoints    int64   `json:"price_per_unit_points"`
	TargetAmount          string  `json:"target_amount"`
	TargetUnits           int32   `json:"target_units"`
	FundedUnitsCount      int32   `json:"funded_units_count"`
	PlantedUnitsCount     int32   `json:"planted_units_count"`
	SurvivingUnitsCount   int32   `json:"surviving_units_count"`
	DonationStartDate     string  `json:"donation_start_date"`
	DonationEndDate       string  `json:"donation_end_date"`
	EstimatedPlantingDate string  `json:"estimated_planting_date"`
	ImageURL              string  `json:"image_url"`
	IsUrgent              bool    `json:"is_urgent"`
	CreatedAt             string  `json:"created_at"`
}

func (h *Handler) handleListProjects(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		if l > 50 {
			limit = 50
		} else {
			limit = l
		}
	}

	categoryFilter := r.URL.Query().Get("category")
	statusFilter := r.URL.Query().Get("status")
	cursor := r.URL.Query().Get("cursor")

	projects, err := h.service.ListProjects(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	filtered := make([]store.ListConservationProjectsRow, 0, len(projects))
	for _, p := range projects {
		if categoryFilter != "" && string(p.Category) != categoryFilter {
			continue
		}
		if statusFilter != "" && string(p.Status) != statusFilter {
			continue
		}
		filtered = append(filtered, p)
	}

	startIndex := 0
	if cursor != "" {
		for i, p := range filtered {
			if p.ID.String() == cursor {
				startIndex = i + 1
				break
			}
		}
	}

	items := make([]projectResponse, 0, limit)
	var nextCursor *string
	for i := startIndex; i < len(filtered); i++ {
		if len(items) >= limit {
			c := filtered[i-1].ID.String()
			nextCursor = &c
			break
		}
		p := filtered[i]

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

		var donStart, donEnd, plantEst string
		if p.DonationStartDate.Valid {
			donStart = p.DonationStartDate.Time.Format("2006-01-02")
		}
		if p.DonationEndDate.Valid {
			donEnd = p.DonationEndDate.Time.Format("2006-01-02")
		}
		if p.EstimatedPlantingDate.Valid {
			plantEst = p.EstimatedPlantingDate.Time.Format("2006-01-02")
		}

		lat, _ := decimalutil.FromNumeric(p.Latitude).Float64()
		lon, _ := decimalutil.FromNumeric(p.Longitude).Float64()

		items = append(items, projectResponse{
			ID:                    p.ID.String(),
			Title:                 p.Title,
			Description:           p.Description,
			Objectives:            p.Objectives,
			Category:              string(p.Category),
			SpeciesOrTypeName:     p.SpeciesOrTypeName,
			Status:                string(p.Status),
			PhysicalHealthStatus:  string(p.PhysicalHealthStatus),
			LocationName:          p.LocationName,
			Latitude:              lat,
			Longitude:             lon,
			ContributorsCount:     p.ContributorsCount,
			CurrencyCode:          p.CurrencyCode,
			PricePerUnitMoney:     decimalutil.FromNumeric(p.PricePerUnitMoney).StringFixed(2),
			PricePerUnitPoints:    p.PricePerUnitPoints,
			TargetAmount:          decimalutil.FromNumeric(p.TargetAmount).StringFixed(2),
			TargetUnits:           p.TargetUnits,
			FundedUnitsCount:      p.FundedUnitsCount,
			PlantedUnitsCount:     p.PlantedUnitsCount,
			SurvivingUnitsCount:   p.SurvivingUnitsCount,
			DonationStartDate:     donStart,
			DonationEndDate:       donEnd,
			EstimatedPlantingDate: plantEst,
			ImageURL:              p.ImageUrl,
			IsUrgent:              isUrgent,
			CreatedAt:             time.Now().UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items":       items,
		"next_cursor": nextCursor,
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

	lat, _ := decimalutil.FromNumeric(p.Latitude).Float64()
	lon, _ := decimalutil.FromNumeric(p.Longitude).Float64()

	resp := projectResponse{
		ID:                    p.ID.String(),
		Title:                 p.Title,
		Description:           p.Description,
		Objectives:            p.Objectives,
		Category:              string(p.Category),
		SpeciesOrTypeName:     p.SpeciesOrTypeName,
		Status:                string(p.Status),
		PhysicalHealthStatus:  string(p.PhysicalHealthStatus),
		LocationName:          p.LocationName,
		Latitude:              lat,
		Longitude:             lon,
		ContributorsCount:     p.ContributorsCount,
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

type unitMilestoneItem struct {
	MilestoneMonth    int32   `json:"milestone_month"`
	MilestoneTitle    string  `json:"milestone_title"`
	MediaURL          string  `json:"media_url"`
	IsVerifiedHealthy bool    `json:"is_verified_healthy"`
	HealthStatus      string  `json:"health_status"`
	SubtitleMetrics   *string `json:"subtitle_metrics"`
	CapturedAt        string  `json:"captured_at"`
}

type conservationUnitItem struct {
	ID              string              `json:"id"`
	UnitCode        string              `json:"unit_code"`
	Nickname        *string             `json:"nickname"`
	Category        string              `json:"category"`
	ProjectID       string              `json:"project_id"`
	ProjectName     string              `json:"project_name"`
	SpeciesName     string              `json:"species_name"`
	LocationName    string              `json:"location_name"`
	Quantity        int32               `json:"quantity"`
	LifecycleStatus string              `json:"lifecycle_status"`
	HealthStatus    string              `json:"health_status"`
	SpiderFrameTag  *string             `json:"spider_frame_tag"`
	LatestPhotoURL  *string             `json:"latest_photo_url"`
	PlantedAt       *string             `json:"planted_at"`
	CreatedAt       string              `json:"created_at"`
	Milestones      []unitMilestoneItem `json:"milestones"`
}

func (h *Handler) handleListMyUnits(w http.ResponseWriter, r *http.Request) {
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

	donationUnits, err := h.service.ListUserAdoptedDonationUnits(r.Context(), userID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	units := make([]conservationUnitItem, 0, len(corals)+len(donationUnits))

	// 1. Process Coral Units
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

		dbMilestones, _ := h.service.ListCoralMilestones(r.Context(), c.ID)
		milestones := make([]unitMilestoneItem, 0, 3)

		if len(dbMilestones) > 0 {
			for _, m := range dbMilestones {
				var sub *string
				if m.SubtitleMetrics.Valid {
					sub = &m.SubtitleMetrics.String
				}
				hStatus := health
				if m.HealthStatus.Valid {
					hStatus = string(m.HealthStatus.EnvironmentalHealthStatus)
				}
				milestones = append(milestones, unitMilestoneItem{
					MilestoneMonth:    m.MilestoneMonth.Int32,
					MilestoneTitle:    m.MilestoneTitle.String,
					MediaURL:          m.MediaUrl,
					IsVerifiedHealthy: m.IsVerifiedHealthy,
					HealthStatus:      hStatus,
					SubtitleMetrics:   sub,
					CapturedAt:        m.CapturedAt.Time.Format(time.RFC3339),
				})
			}
		} else {
			defaultPhoto := "https://images.unsplash.com/photo-1546026423-cc4642628d2b"
			if photo != nil {
				defaultPhoto = *photo
			}
			plantedTime := c.CreatedAt.Time
			if c.PlantedAt.Valid {
				plantedTime = c.PlantedAt.Time
			}

			sub1 := "Struktur Biorock aktif dialiri listrik mikro, polip karang membuka sempurna"
			sub3 := "Endapan mineral kalsium karbonat menebal, percabangan tumbuh 2.5 cm"
			sub6 := "Koloni karang mandiri, ikan karang dan anemon mulai berkoloni di sekitar fragmen"

			milestones = append(milestones,
				unitMilestoneItem{
					MilestoneMonth:    1,
					MilestoneTitle:    "Bulan 1: Penempelan & Adaptasi Substrat",
					MediaURL:          defaultPhoto,
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub1,
					CapturedAt:        plantedTime.AddDate(0, 1, 0).Format(time.RFC3339),
				},
				unitMilestoneItem{
					MilestoneMonth:    3,
					MilestoneTitle:    "Bulan 3: Pertumbuhan Cabang & Kalsifikasi",
					MediaURL:          "https://images.unsplash.com/photo-1582967788606-a171c1080cb0",
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub3,
					CapturedAt:        plantedTime.AddDate(0, 3, 0).Format(time.RFC3339),
				},
				unitMilestoneItem{
					MilestoneMonth:    6,
					MilestoneTitle:    "Bulan 6: Koloni Mandiri & Ekosistem Mikro",
					MediaURL:          "https://images.unsplash.com/photo-1682687220063-4742bd7fd538",
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub6,
					CapturedAt:        plantedTime.AddDate(0, 6, 0).Format(time.RFC3339),
				},
			)
		}

		units = append(units, conservationUnitItem{
			ID:              c.ID.String(),
			UnitCode:        c.FragmentCode,
			Nickname:        nick,
			Category:        "coral",
			ProjectID:       c.ProjectID.String(),
			ProjectName:     c.ProjectTitle,
			SpeciesName:     c.SpeciesName,
			LocationName:    c.ZoneName,
			Quantity:        1,
			LifecycleStatus: string(c.LifecycleStatus),
			HealthStatus:    health,
			SpiderFrameTag:  nil,
			LatestPhotoURL:  photo,
			PlantedAt:       planted,
			CreatedAt:       c.CreatedAt.Time.Format(time.RFC3339),
			Milestones:      milestones,
		})
	}

	// 2. Process Non-Coral Units (Mangroves, Trees, etc.)
	for _, du := range donationUnits {
		photo := du.ProjectImageUrl
		paidTime := du.CreatedAt.Time
		var plantedStr *string
		if du.PaidAt.Valid {
			paidTime = du.PaidAt.Time
			s := paidTime.Format(time.RFC3339)
			plantedStr = &s
		}

		prefix := "MNG"
		if du.ProjectCategory == "tree" {
			prefix = "TRE"
		}
		unitCode := fmt.Sprintf("%s-%s", prefix, du.ID.String()[:8])

		milestones := make([]unitMilestoneItem, 0, 3)
		if du.ProjectCategory == "mangrove" {
			sub1 := "Bibit Rhizophora ditanam di lumpur pesisir dengan ajir bambu pelindung"
			sub3 := "Akar tunjang mulai mencengkeram substrat, daun bertunas 4-6 helai baru"
			sub6 := "Sabuk hijau mangrove rapat menahan abrasi ombak dan menyerap karbon biru"
			milestones = append(milestones,
				unitMilestoneItem{
					MilestoneMonth:    1,
					MilestoneTitle:    "Bulan 1: Penanaman Bibit Mangrove",
					MediaURL:          du.ProjectImageUrl,
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub1,
					CapturedAt:        paidTime.AddDate(0, 1, 0).Format(time.RFC3339),
				},
				unitMilestoneItem{
					MilestoneMonth:    3,
					MilestoneTitle:    "Bulan 3: Perkembangan Akar Tunjang",
					MediaURL:          "https://images.unsplash.com/photo-1544551763-77ef2d0cfc6c",
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub3,
					CapturedAt:        paidTime.AddDate(0, 3, 0).Format(time.RFC3339),
				},
				unitMilestoneItem{
					MilestoneMonth:    6,
					MilestoneTitle:    "Bulan 6: Pemantapan Sabuk Hijau Pesisir",
					MediaURL:          "https://images.unsplash.com/photo-1559827291-72ee739d0d9a",
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub6,
					CapturedAt:        paidTime.AddDate(0, 6, 0).Format(time.RFC3339),
				},
			)
		} else {
			sub1 := "Bibit pohon ditanam pada bedengan lereng dengan mulsa organik"
			sub3 := "Batang mengeras dan bertambah tinggi 40 cm, perakaran menembus tanah dalam"
			sub6 := "Kanopi rimbun menaungi tanah lereng dan mengikat resapan air tanah pegunungan"
			milestones = append(milestones,
				unitMilestoneItem{
					MilestoneMonth:    1,
					MilestoneTitle:    "Bulan 1: Penanaman Pohon di Lereng",
					MediaURL:          du.ProjectImageUrl,
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub1,
					CapturedAt:        paidTime.AddDate(0, 1, 0).Format(time.RFC3339),
				},
				unitMilestoneItem{
					MilestoneMonth:    3,
					MilestoneTitle:    "Bulan 3: Penguatan Batang & Perakaran",
					MediaURL:          "https://images.unsplash.com/photo-1513836279014-a89f7a76ae86",
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub3,
					CapturedAt:        paidTime.AddDate(0, 3, 0).Format(time.RFC3339),
				},
				unitMilestoneItem{
					MilestoneMonth:    6,
					MilestoneTitle:    "Bulan 6: Pembentukan Kanopi & Konservasi Air",
					MediaURL:          "https://images.unsplash.com/photo-1502082553048-f009c37129b9",
					IsVerifiedHealthy: true,
					HealthStatus:      "healthy",
					SubtitleMetrics:   &sub6,
					CapturedAt:        paidTime.AddDate(0, 6, 0).Format(time.RFC3339),
				},
			)
		}

		units = append(units, conservationUnitItem{
			ID:              du.ID.String(),
			UnitCode:        unitCode,
			Nickname:        nil,
			Category:        du.ProjectCategory,
			ProjectID:       du.ProjectID.String(),
			ProjectName:     du.ProjectTitle,
			SpeciesName:     du.SpeciesName,
			LocationName:    du.LocationName,
			Quantity:        du.UnitQuantity,
			LifecycleStatus: "planted",
			HealthStatus:    "healthy",
			SpiderFrameTag:  nil,
			LatestPhotoURL:  &photo,
			PlantedAt:       plantedStr,
			CreatedAt:       du.CreatedAt.Time.Format(time.RFC3339),
			Milestones:      milestones,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"items": units,
	})
}

func (h *Handler) handleGetCoral(w http.ResponseWriter, r *http.Request) {
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

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

	if !frag.AdoptedByUserID.Valid || frag.AdoptedByUserID.Bytes != userID {
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
	userID, ok := identity.GetUserIDFromContext(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "Authentication required.")
		return
	}

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

	if !frag.AdoptedByUserID.Valid || frag.AdoptedByUserID.Bytes != userID {
		httperr.NotFound(w, r, "Adopted coral fragment not found.")
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

		growth := float64(m.MilestoneMonth.Int32) * 0.75
		if m.SubtitleMetrics.Valid {
			var parsed float64
			if n, _ := fmt.Sscanf(m.SubtitleMetrics.String, "%f", &parsed); n > 0 {
				growth = parsed
			}
		}

		items = append(items, milestoneItem{
			MilestoneMonth: m.MilestoneMonth.Int32,
			PhotoURL:       m.MediaUrl,
			HealthStatus:   health,
			GrowthCM:       growth,
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

type createZoneRequest struct {
	Name           string                   `json:"name"`
	Region         string                   `json:"region"`
	Province       string                   `json:"province"`
	EcosystemType  store.EcosystemKind      `json:"ecosystem_type"`
	AllocationType store.ZoneAllocationType `json:"allocation_type"`
	Latitude       decimal.Decimal          `json:"latitude"`
	Longitude      decimal.Decimal          `json:"longitude"`
	MinDepthMeters *decimal.Decimal         `json:"min_depth_meters,omitempty"`
	MaxDepthMeters *decimal.Decimal         `json:"max_depth_meters,omitempty"`
}

func (h *Handler) handleAdminCreateZone(w http.ResponseWriter, r *http.Request) {
	var req createZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.Name == "" || req.Region == "" || req.Province == "" {
		httperr.BadRequest(w, r, "name, region, and province are required", nil)
		return
	}
	if req.AllocationType == "" {
		req.AllocationType = store.ZoneAllocationTypeB2cPool
	}
	if req.EcosystemType == "" {
		req.EcosystemType = store.EcosystemKindMarineCoral
	}

	z, err := h.service.CreateZone(r.Context(), req.Name, req.Region, req.Province, req.EcosystemType, req.AllocationType, req.Latitude, req.Longitude, req.MinDepthMeters, req.MaxDepthMeters)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, z)
}

func (h *Handler) handleAdminListZones(w http.ResponseWriter, r *http.Request) {
	zones, err := h.service.ListZones(r.Context())
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}
	writeJSON(w, http.StatusOK, zones)
}

func (h *Handler) handleAdminUpdateZone(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	zID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid zone UUID", nil)
		return
	}

	var req createZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	z, err := h.service.UpdateZone(r.Context(), zID, req.Name, req.Region, req.Province, req.EcosystemType, req.AllocationType, req.Latitude, req.Longitude, req.MinDepthMeters, req.MaxDepthMeters)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, z)
}

type createProjectRequest struct {
	ZoneID                uuid.UUID             `json:"zone_id"`
	Title                 string                `json:"title"`
	Description           string                `json:"description"`
	Objectives            string                `json:"objectives"`
	Category              store.ProjectCategory `json:"category"`
	SpeciesOrTypeName     string                `json:"species_or_type_name"`
	ImageURL              string                `json:"image_url"`
	UnitName              string                `json:"unit_name"`
	CurrencyCode          string                `json:"currency_code"`
	PricePerUnitMoney     decimal.Decimal       `json:"price_per_unit_money"`
	PricePerUnitPoints    int64                 `json:"price_per_unit_points"`
	TargetUnits           int32                 `json:"target_units"`
	TargetAmount          decimal.Decimal       `json:"target_amount"`
	ReplantReservePct     decimal.Decimal       `json:"replant_reserve_pct"`
	DonationStartDate     string                `json:"donation_start_date"`
	DonationEndDate       string                `json:"donation_end_date"`
	EstimatedPlantingDate string                `json:"estimated_planting_date"`
}

func (h *Handler) handleAdminCreateProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.Title == "" || req.ZoneID == uuid.Nil {
		httperr.BadRequest(w, r, "title and zone_id are required", nil)
		return
	}

	startDate, _ := time.Parse("2006-01-02", req.DonationStartDate)
	if startDate.IsZero() {
		startDate = time.Now().UTC()
	}
	endDate, _ := time.Parse("2006-01-02", req.DonationEndDate)
	if endDate.IsZero() {
		endDate = startDate.AddDate(0, 6, 0)
	}
	plantDate, _ := time.Parse("2006-01-02", req.EstimatedPlantingDate)
	if plantDate.IsZero() {
		plantDate = endDate.AddDate(0, 1, 0)
	}

	p, err := h.service.CreateProject(r.Context(), CreateProjectInput{
		ZoneID:                req.ZoneID,
		Title:                 req.Title,
		Description:           req.Description,
		Objectives:            req.Objectives,
		Category:              req.Category,
		SpeciesOrTypeName:     req.SpeciesOrTypeName,
		ImageURL:              req.ImageURL,
		UnitName:              req.UnitName,
		CurrencyCode:          req.CurrencyCode,
		PricePerUnitMoney:     req.PricePerUnitMoney,
		PricePerUnitPoints:    req.PricePerUnitPoints,
		TargetUnits:           req.TargetUnits,
		TargetAmount:          req.TargetAmount,
		ReplantReservePct:     req.ReplantReservePct,
		DonationStartDate:     startDate,
		DonationEndDate:       endDate,
		EstimatedPlantingDate: plantDate,
	})
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, p)
}

type updateProjectRequest struct {
	Title                string                          `json:"title"`
	Description          string                          `json:"description"`
	Objectives           string                          `json:"objectives"`
	Status               store.ProjectStatus             `json:"status"`
	PhysicalHealthStatus store.EnvironmentalHealthStatus `json:"physical_health_status"`
	PricePerUnitMoney    decimal.Decimal                 `json:"price_per_unit_money"`
	PricePerUnitPoints   int64                           `json:"price_per_unit_points"`
}

func (h *Handler) handleAdminUpdateProject(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	pID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid project UUID", nil)
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	p, err := h.service.UpdateProject(r.Context(), pID, req.Title, req.Description, req.Objectives, req.Status, req.PhysicalHealthStatus, req.PricePerUnitMoney, req.PricePerUnitPoints)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, p)
}

type createFrameRequest struct {
	FrameCode   string          `json:"frame_code"`
	ZoneID      uuid.UUID       `json:"zone_id"`
	CompanyID   *uuid.UUID      `json:"company_id,omitempty"`
	DepthMeters decimal.Decimal `json:"depth_meters"`
	Latitude    decimal.Decimal `json:"latitude"`
	Longitude   decimal.Decimal `json:"longitude"`
	MaxCapacity int32           `json:"max_capacity"`
}

func (h *Handler) handleAdminCreateFrame(w http.ResponseWriter, r *http.Request) {
	var req createFrameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.FrameCode == "" || req.ZoneID == uuid.Nil {
		httperr.BadRequest(w, r, "frame_code and zone_id are required", nil)
		return
	}

	sf, err := h.service.CreateSpiderFrame(r.Context(), req.FrameCode, req.ZoneID, req.CompanyID, req.DepthMeters, req.Latitude, req.Longitude, req.MaxCapacity)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, sf)
}

func (h *Handler) handleAdminListFrames(w http.ResponseWriter, r *http.Request) {
	zoneIDStr := r.URL.Query().Get("zone_id")
	zoneID, err := uuid.Parse(zoneIDStr)
	if err != nil {
		httperr.BadRequest(w, r, "Valid zone_id query param required", nil)
		return
	}

	frames, err := h.service.ListSpiderFrames(r.Context(), zoneID)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, frames)
}

type createNewsRequest struct {
	Title        string                `json:"title"`
	Category     store.ProjectCategory `json:"category"`
	Summary      string                `json:"summary"`
	Content      string                `json:"content"`
	ThumbnailURL string                `json:"thumbnail_url"`
	IsFeatured   bool                  `json:"is_featured"`
}

func (h *Handler) handleAdminCreateNews(w http.ResponseWriter, r *http.Request) {
	var req createNewsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}
	if req.Title == "" || req.Summary == "" {
		httperr.BadRequest(w, r, "title and summary are required", nil)
		return
	}
	if req.Category == "" {
		req.Category = store.ProjectCategoryCoral
	}

	n, err := h.service.CreateNews(r.Context(), req.Title, req.Category, req.Summary, req.Content, req.ThumbnailURL, req.IsFeatured)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusCreated, n)
}

func (h *Handler) handleAdminUpdateNews(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	nID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid news UUID", nil)
		return
	}

	var req createNewsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.BadRequest(w, r, "Invalid request body", nil)
		return
	}

	n, err := h.service.UpdateNews(r.Context(), nID, req.Title, req.Category, req.Summary, req.Content, req.ThumbnailURL, req.IsFeatured)
	if err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, n)
}

func (h *Handler) handleAdminDeleteNews(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	nID, err := uuid.Parse(idStr)
	if err != nil {
		httperr.BadRequest(w, r, "Invalid news UUID", nil)
		return
	}

	if err := h.service.DeleteNews(r.Context(), nID); err != nil {
		httperr.InternalServerError(w, r)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "News article deleted"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
