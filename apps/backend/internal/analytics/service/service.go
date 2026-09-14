// Package service provides the business-logic seam between callers (HTTP
// handlers, and — per ADR 0043 — the plugin Host data API) and the analytics
// repository. Callers that need analytics/session aggregation data should
// depend on Service rather than reaching into repository.Repository
// directly, so future access rules or derived fields have one place to live.
package service

import (
	"context"
	"fmt"
	"strings"

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

// UpsertSessionUsage persists a batch after the caller has assigned its source
// namespace. Host callers should use UpsertPluginSessionUsage so a plugin
// cannot impersonate another source.
func (s *Service) UpsertSessionUsage(ctx context.Context, records []models.SessionUsageMeasurement) ([]models.SessionUsageUpsertResult, error) {
	usageRepo, err := s.usageRepository()
	if err != nil {
		return nil, err
	}
	return usageRepo.UpsertSessionUsage(ctx, records)
}

// UpsertPluginSessionUsage derives the source identity at the Host boundary.
func (s *Service) UpsertPluginSessionUsage(
	ctx context.Context,
	pluginID, workspaceID string,
	records []models.SessionUsageMeasurement,
) ([]models.SessionUsageUpsertResult, error) {
	pluginID = strings.TrimSpace(pluginID)
	workspaceID = strings.TrimSpace(workspaceID)
	if pluginID == "" || workspaceID == "" {
		return nil, fmt.Errorf("plugin id and workspace id are required")
	}
	prepared := make([]models.SessionUsageMeasurement, len(records))
	for i, record := range records {
		prepared[i] = record
		prepared[i].Source = "plugin:" + pluginID
		prepared[i].WorkspaceID = workspaceID
	}
	return s.UpsertSessionUsage(ctx, prepared)
}

func (s *Service) ListSessionUsage(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	usageRepo, err := s.usageRepository()
	if err != nil {
		return nil, err
	}
	return usageRepo.ListSessionUsage(ctx, filter)
}

// ListSessionUsageMeasurements returns persisted source observations for the
// plugin Host API. It deliberately excludes native ledger events because a
// plugin can only read back measurements it wrote through this contract.
func (s *Service) ListSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	usageRepo, err := s.usageRepository()
	if err != nil {
		return nil, err
	}
	return usageRepo.ListSessionUsageMeasurements(ctx, filter)
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

// ListPluginSessionUsageMeasurements scopes a Host read to the source
// namespace derived from the connected plugin. Callers cannot supply or
// override this value through the public usage DTO.
func (s *Service) ListPluginSessionUsageMeasurements(
	ctx context.Context,
	pluginID string,
	filter models.SessionUsageFilter,
) ([]models.SessionUsageMeasurement, error) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return nil, fmt.Errorf("plugin id is required")
	}
	filter.Source = "plugin:" + pluginID
	return s.ListSessionUsageMeasurements(ctx, filter)
}
