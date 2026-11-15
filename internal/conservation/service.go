package conservation

import (
	"context"
	"errors"
	"fmt"

	"github.com/Drewron0/bioverse-backend/internal/platform/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
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
