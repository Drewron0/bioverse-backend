package catalog

import (
	"context"

	"github.com/Drewron01/bioverse-backend/internal/platform/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Service provides business logic for the digital item shop and subscription catalog.
type Service struct {
	pool  *pgxpool.Pool
	store store.Querier
}

// NewService constructs a new catalog service.
func NewService(pool *pgxpool.Pool, q store.Querier) *Service {
	return &Service{
		pool:  pool,
		store: q,
	}
}

// ListDigitalProducts returns all active products with pricing in the requested currency.
func (s *Service) ListDigitalProducts(ctx context.Context, currencyCode string) ([]store.ListActiveDigitalProductsRow, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	return s.store.ListActiveDigitalProducts(ctx, currencyCode)
}

// ListSubscriptionPlans returns all active subscription plans with pricing.
func (s *Service) ListSubscriptionPlans(ctx context.Context, currencyCode string) ([]store.ListActiveSubscriptionPlansRow, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	return s.store.ListActiveSubscriptionPlans(ctx, currencyCode)
}
