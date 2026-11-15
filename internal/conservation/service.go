package conservation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Drewron0/bioverse-backend/internal/platform/decimalutil"
	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

var (
	// ErrProjectNotFound indicates the conservation project was not found.
	ErrProjectNotFound = errors.New("conservation project not found")
	// ErrNoOpenFrameAvailable indicates all frames in the zone are at capacity.
	ErrNoOpenFrameAvailable = errors.New("no open spider frame currently available in this zone")
)

// Service provides business logic for conservation zones, projects, and coral fragments.
type Service struct {
	pool  *pgxpool.Pool
	store store.Querier
}

// NewService constructs a new conservation service.
func NewService(pool *pgxpool.Pool, q store.Querier) *Service {
	return &Service{
		pool:  pool,
		store: q,
	}
}

// ListProjects returns all active conservation projects open for donation.
func (s *Service) ListProjects(ctx context.Context) ([]store.ListConservationProjectsRow, error) {
	return s.store.ListConservationProjects(ctx)
}

// GetProject returns a single conservation project by its UUID.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (*store.GetConservationProjectByIDRow, error) {
	proj, err := s.store.GetConservationProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to get conservation project: %w", err)
	}
	return &proj, nil
}

// AllocateFragments allocates coral fragments into open spider frames using row-level locking.
func (s *Service) AllocateFragments(
	ctx context.Context,
	tx pgx.Tx,
	zoneID uuid.UUID,
	projectID uuid.UUID,
	userID uuid.UUID,
	donationID uuid.UUID,
	quantity int,
	speciesName string,
	nickname string,
) ([]store.CreateCoralFragmentRow, error) {
	txStore := store.New(tx)

	frame, err := txStore.GetOpenSpiderFrameForZone(ctx, zoneID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoOpenFrameAvailable
		}
		return nil, fmt.Errorf("failed to lock open spider frame: %w", err)
	}

	availableCapacity := int(frame.MaxCapacityFragments - frame.FragmentsPlanted)
	unitsToPlant := quantity
	if unitsToPlant > availableCapacity {
		unitsToPlant = availableCapacity
	}

	_, err = txStore.IncrementSpiderFrameFragmentsPlanted(ctx, store.IncrementSpiderFrameFragmentsPlantedParams{
		ID:               frame.ID,
		FragmentsPlanted: int32(unitsToPlant),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to increment frame capacity: %w", err)
	}

	_, err = txStore.IncrementProjectFundedUnits(ctx, store.IncrementProjectFundedUnitsParams{
		ID:                projectID,
		FundedUnitsCount: int32(quantity),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to increment project funded units: %w", err)
	}

	createdFragments := make([]store.CreateCoralFragmentRow, 0, quantity)
	for i := 0; i < quantity; i++ {
		fragID, _ := uuid.NewV7()
		fragCode := fmt.Sprintf("CF-%s-%04d", frame.FrameCode, frame.FragmentsPlanted+int32(i+1))

		fragNickname := nickname
		if fragNickname == "" {
			fragNickname = fmt.Sprintf("Adopted Coral #%d", i+1)
		}

		frag, err := txStore.CreateCoralFragment(ctx, store.CreateCoralFragmentParams{
			ID:                 fragID,
			FragmentCode:       fragCode,
			Nickname:           pgtype.Text{String: fragNickname, Valid: true},
			ProjectID:          projectID,
			DonationID:         pgtype.UUID{Bytes: donationID, Valid: true},
			SpiderFrameID:      pgtype.UUID{Bytes: frame.ID, Valid: true},
			AdoptedByUserID:    pgtype.UUID{Bytes: userID, Valid: true},
			SpeciesName:        speciesName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create coral fragment record: %w", err)
		}
		createdFragments = append(createdFragments, frag)
	}

	return createdFragments, nil
}

// ListUserAdoptedCorals returns all coral fragments adopted by the specified user.
func (s *Service) ListUserAdoptedCorals(ctx context.Context, userID uuid.UUID) ([]store.ListCoralFragmentsByUserIDRow, error) {
	return s.store.ListCoralFragmentsByUserID(ctx, pgtype.UUID{Bytes: userID, Valid: true})
}

// GetCoralFragment returns details for a single coral fragment.
func (s *Service) GetCoralFragment(ctx context.Context, id uuid.UUID) (*store.GetCoralFragmentByIDRow, error) {
	frag, err := s.store.GetCoralFragmentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("coral fragment not found")
		}
		return nil, fmt.Errorf("failed to query coral fragment: %w", err)
	}
	return &frag, nil
}

// ListCoralMilestones returns photographic monitoring milestones for a coral fragment.
func (s *Service) ListCoralMilestones(ctx context.Context, fragmentID uuid.UUID) ([]store.ListCoralMilestonesByFragmentIDRow, error) {
	return s.store.ListCoralMilestonesByFragmentID(ctx, pgtype.UUID{Bytes: fragmentID, Valid: true})
}

// GetImpactSummary retrieves platform-wide restoration metrics.
func (s *Service) GetImpactSummary(ctx context.Context) (*store.GetPlatformImpactSummaryRow, error) {
	res, err := s.store.GetPlatformImpactSummary(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get impact summary: %w", err)
	}
	return &res, nil
}

// ListNewsArticles retrieves recent news articles.
func (s *Service) ListNewsArticles(ctx context.Context, limit int32) ([]store.NewsArticles, error) {
	if limit <= 0 {
		limit = 10
	}
	return s.store.ListNewsArticles(ctx, limit)
}

// CreateZone creates a new conservation zone.
func (s *Service) CreateZone(ctx context.Context, name, region, province string, ecosystem store.EcosystemKind, allocation store.ZoneAllocationType, lat, lng decimal.Decimal, minDepth, maxDepth *decimal.Decimal) (*store.ConservationZones, error) {
	zoneID, _ := uuid.NewV7()
	var minD, maxD pgtype.Numeric
	if minDepth != nil {
		minD = decimalutil.ToNumeric(*minDepth)
	}
	if maxDepth != nil {
		maxD = decimalutil.ToNumeric(*maxDepth)
	}

	z, err := s.store.CreateConservationZone(ctx, store.CreateConservationZoneParams{
		ID:             zoneID,
		Name:           name,
		Region:         region,
		Province:       province,
		EcosystemType:  ecosystem,
		AllocationType: allocation,
		Latitude:       decimalutil.ToNumeric(lat),
		Longitude:      decimalutil.ToNumeric(lng),
		MinDepthMeters: minD,
		MaxDepthMeters: maxD,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create conservation zone: %w", err)
	}
	return &z, nil
}

// ListZones returns all registered conservation zones.
func (s *Service) ListZones(ctx context.Context) ([]store.ConservationZones, error) {
	return s.store.ListConservationZones(ctx)
}

// UpdateZone updates an existing conservation zone.
func (s *Service) UpdateZone(ctx context.Context, id uuid.UUID, name, region, province string, ecosystem store.EcosystemKind, allocation store.ZoneAllocationType, lat, lng decimal.Decimal, minDepth, maxDepth *decimal.Decimal) (*store.ConservationZones, error) {
	var minD, maxD pgtype.Numeric
	if minDepth != nil {
		minD = decimalutil.ToNumeric(*minDepth)
	}
	if maxDepth != nil {
		maxD = decimalutil.ToNumeric(*maxDepth)
	}

	z, err := s.store.UpdateConservationZone(ctx, store.UpdateConservationZoneParams{
		ID:             id,
		Name:           name,
		Region:         region,
		Province:       province,
		EcosystemType:  ecosystem,
		AllocationType: allocation,
		Latitude:       decimalutil.ToNumeric(lat),
		Longitude:      decimalutil.ToNumeric(lng),
		MinDepthMeters: minD,
		MaxDepthMeters: maxD,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update conservation zone: %w", err)
	}
	return &z, nil
}

// CreateProjectInput defines fields to create a project.
type CreateProjectInput struct {
	ZoneID                uuid.UUID
	Title                 string
	Description           string
	Objectives            string
	Category              store.ProjectCategory
	SpeciesOrTypeName     string
	ImageURL              string
	UnitName              string
	CurrencyCode          string
	PricePerUnitMoney     decimal.Decimal
	PricePerUnitPoints    int64
	TargetUnits           int32
	TargetAmount          decimal.Decimal
	ReplantReservePct     decimal.Decimal
	DonationStartDate     time.Time
	DonationEndDate       time.Time
	EstimatedPlantingDate time.Time
}

// CreateProject creates a new conservation project.
func (s *Service) CreateProject(ctx context.Context, in CreateProjectInput) (*store.ConservationProjects, error) {
	pID, _ := uuid.NewV7()
	if in.UnitName == "" {
		in.UnitName = "fragment"
	}
	if in.CurrencyCode == "" {
		in.CurrencyCode = "USD"
	}
	if in.ReplantReservePct.IsZero() {
		in.ReplantReservePct = decimal.NewFromInt(30)
	}

	p, err := s.store.CreateConservationProject(ctx, store.CreateConservationProjectParams{
		ID:                    pID,
		ZoneID:                in.ZoneID,
		Title:                 in.Title,
		Description:           in.Description,
		Objectives:            in.Objectives,
		Category:              in.Category,
		SpeciesOrTypeName:     in.SpeciesOrTypeName,
		ImageUrl:              in.ImageURL,
		Status:                store.ProjectStatusOpenForDonation,
		PhysicalHealthStatus:  store.EnvironmentalHealthStatusHealthy,
		UnitName:              in.UnitName,
		CurrencyCode:          in.CurrencyCode,
		PricePerUnitMoney:     decimalutil.ToNumeric(in.PricePerUnitMoney),
		PricePerUnitPoints:    in.PricePerUnitPoints,
		TargetAmount:          decimalutil.ToNumeric(in.TargetAmount),
		TargetUnits:           in.TargetUnits,
		ReplantReservePct:     decimalutil.ToNumeric(in.ReplantReservePct),
		DonationStartDate:     pgtype.Date{Time: in.DonationStartDate, Valid: true},
		DonationEndDate:       pgtype.Date{Time: in.DonationEndDate, Valid: true},
		EstimatedPlantingDate: pgtype.Date{Time: in.EstimatedPlantingDate, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create conservation project: %w", err)
	}
	return &p, nil
}

// UpdateProject updates mutable fields of a project.
func (s *Service) UpdateProject(ctx context.Context, id uuid.UUID, title, description, objectives string, status store.ProjectStatus, health store.EnvironmentalHealthStatus, priceMoney decimal.Decimal, pricePoints int64) (*store.ConservationProjects, error) {
	p, err := s.store.UpdateConservationProject(ctx, store.UpdateConservationProjectParams{
		ID:                   id,
		Title:                title,
		Description:          description,
		Objectives:           objectives,
		Status:               status,
		PhysicalHealthStatus: health,
		PricePerUnitMoney:    decimalutil.ToNumeric(priceMoney),
		PricePerUnitPoints:   pricePoints,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update conservation project: %w", err)
	}
	return &p, nil
}

// CreateSpiderFrame creates a new spider frame for coral propagation.
func (s *Service) CreateSpiderFrame(ctx context.Context, frameCode string, zoneID uuid.UUID, companyID *uuid.UUID, depth, lat, lng decimal.Decimal, maxCap int32) (*store.SpiderFrames, error) {
	fID, _ := uuid.NewV7()
	var compUUID pgtype.UUID
	if companyID != nil {
		compUUID = pgtype.UUID{Bytes: *companyID, Valid: true}
	}
	if maxCap <= 0 {
		maxCap = 15
	}

	sf, err := s.store.CreateSpiderFrame(ctx, store.CreateSpiderFrameParams{
		ID:                   fID,
		FrameCode:            frameCode,
		ZoneID:               zoneID,
		CompanyID:            compUUID,
		DepthMeters:          decimalutil.ToNumeric(depth),
		Latitude:             decimalutil.ToNumeric(lat),
		Longitude:            decimalutil.ToNumeric(lng),
		MaxCapacityFragments: maxCap,
		PhysicalHealthStatus: store.EnvironmentalHealthStatusHealthy,
		IsActive:             true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create spider frame: %w", err)
	}
	return &sf, nil
}

// ListSpiderFrames returns spider frames in a given zone.
func (s *Service) ListSpiderFrames(ctx context.Context, zoneID uuid.UUID) ([]store.SpiderFrames, error) {
	return s.store.ListSpiderFramesByZoneID(ctx, zoneID)
}

// CreateNews creates a new article.
func (s *Service) CreateNews(ctx context.Context, title string, cat store.ProjectCategory, summary, content, thumb string, isFeatured bool) (*store.NewsArticles, error) {
	nID, _ := uuid.NewV7()
	n, err := s.store.CreateNewsArticle(ctx, store.CreateNewsArticleParams{
		ID:           nID,
		Title:        title,
		Category:     cat,
		Summary:      summary,
		Content:      content,
		ThumbnailUrl: thumb,
		IsFeatured:   isFeatured,
		PublishedAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create news article: %w", err)
	}
	return &n, nil
}

// UpdateNews updates an existing news article.
func (s *Service) UpdateNews(ctx context.Context, id uuid.UUID, title string, cat store.ProjectCategory, summary, content, thumb string, isFeatured bool) (*store.NewsArticles, error) {
	n, err := s.store.UpdateNewsArticle(ctx, store.UpdateNewsArticleParams{
		ID:           id,
		Title:        title,
		Category:     cat,
		Summary:      summary,
		Content:      content,
		ThumbnailUrl: thumb,
		IsFeatured:   isFeatured,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update news article: %w", err)
	}
	return &n, nil
}

// DeleteNews removes a news article.
func (s *Service) DeleteNews(ctx context.Context, id uuid.UUID) error {
	return s.store.DeleteNewsArticle(ctx, id)
}
