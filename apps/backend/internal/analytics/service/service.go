// Package service provides the business-logic seam between callers (HTTP
// handlers, and — per ADR 0043 — the plugin Host data API) and the analytics
// repository. Callers that need analytics/session aggregation data should
// depend on Service rather than reaching into repository.Repository
// directly, so future access rules or derived fields have one place to live.
package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
)

// Service exposes analytics/session aggregation operations backed by a
// repository.Repository.
type Service struct {
	repo repository.Repository
}

// New creates a Service wrapping the given repository.
func New(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// ListSessionCodeStats returns per-session committed and peak-pending
// lines-of-code stats matching filter. See models.SessionCodeStats and
// models.SessionCodeStatsFilter for field semantics.
func (s *Service) ListSessionCodeStats(
	ctx context.Context,
	filter models.SessionCodeStatsFilter,
) ([]*models.SessionCodeStats, error) {
	return s.repo.ListSessionCodeStats(ctx, filter)
}

func (s *Service) usageRepository() (repository.UsageRepository, error) {
	usageRepo, ok := s.repo.(repository.UsageRepository)
	if !ok || usageRepo == nil {
		return nil, fmt.Errorf("analytics usage repository is not configured")
	}
	return usageRepo, nil
}

func (s *Service) ListSessionUsage(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	usageRepo, err := s.usageRepository()
	if err != nil {
		return nil, err
	}
	return usageRepo.ListSessionUsage(ctx, filter)
}

// ListCanonicalSessionUsageMeasurements returns the host-authorized canonical
// view for a session. Unlike the plugin observation read, it may include the
// native ledger and another collector's accepted value.
func (s *Service) ListCanonicalSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	usageRepo, err := s.usageRepository()
	if err != nil {
		return nil, err
	}
	return usageRepo.ListCanonicalSessionUsageMeasurements(ctx, filter)
}
