package repository

import (
	"context"

	"github.com/kandev/kandev/internal/analytics/models"
)

// UsageRepository is the source-aware usage persistence/query contract. It is
// separate from Repository so existing analytics consumers can add this
// feature without changing their test doubles.
type UsageRepository interface {
	UpsertSessionUsage(ctx context.Context, records []models.SessionUsageMeasurement) ([]models.SessionUsageUpsertResult, error)
	ListSessionUsage(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error)
	ListSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error)
	ListCanonicalSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error)
}
