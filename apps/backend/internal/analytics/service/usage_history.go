package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/kdlbs/kandev/pluginsdk"
)

func (s *Service) ListUsageCollectionTargets(ctx context.Context, collector string, query pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error) {
	repo, ok := s.repo.(repository.UsageHistoryRepository)
	if !ok {
		return pluginsdk.UsageCollectionTargetPageV1{}, errors.New("retained usage repository is unavailable")
	}
	return repo.ListUsageCollectionTargets(ctx, collector, query)
}

func (s *Service) ReportUsageObservations(ctx context.Context, collector string, batch pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error) {
	repo, ok := s.repo.(repository.UsageHistoryRepository)
	if !ok {
		return pluginsdk.UsageObservationBatchResultV1{}, errors.New("retained usage repository is unavailable")
	}
	return repo.ReportUsageObservations(ctx, collector, batch)
}

func (s *Service) ListCanonicalSessionUsagePage(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error) {
	if repo, ok := s.repo.(repository.RetainedUsagePageReader); ok {
		return repo.ListRetainedCanonicalUsagePage(ctx, filter)
	}
	if repo, ok := s.repo.(repository.UsageProjectionReader); ok {
		return repo.ListCanonicalUsagePage(ctx, filter)
	}
	rows, err := s.ListCanonicalSessionUsageMeasurements(ctx, filter)
	return rows, models.UsageProjectionState{Status: "unknown"}, err
}

func (s *Service) ListUsageDailySummaries(ctx context.Context, filter models.SessionUsageFilter) ([]models.UsageDailySummary, models.UsageProjectionMetadata, error) {
	if repo, ok := s.repo.(repository.UsageSummaryReader); ok {
		return repo.ListUsageDailySummaries(ctx, filter)
	}
	return nil, models.UsageProjectionMetadata{}, errors.New("retained usage summaries are unavailable")
}

func (s *Service) ListUsageCanonicalFacts(ctx context.Context, filter models.SessionUsageFilter) ([]models.UsageCanonicalFact, models.UsageProjectionMetadata, error) {
	if repo, ok := s.repo.(repository.UsageFactReader); ok {
		return repo.ListUsageCanonicalFacts(ctx, filter)
	}
	return nil, models.UsageProjectionMetadata{}, errors.New("retained usage facts are unavailable")
}
