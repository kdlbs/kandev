package repository

import (
	"context"
	"github.com/kandev/kandev/internal/analytics"

	"github.com/kandev/kandev/internal/analytics/models"
)

var ErrUsageProjectionChanged = analytics.ErrUsageProjectionChanged

// UsageRepository is the source-aware usage persistence/query contract. It is
// separate from Repository so existing analytics consumers can add this
// feature without changing their test doubles.
type UsageRepository interface {
	ListSessionUsage(ctx context.Context, filter models.SessionUsageFilter) (*models.TokenUsageReport, error)
	ListCanonicalSessionUsageMeasurements(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error)
}
