package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/analytics/models"
)

// ListCanonicalUsagePage returns the last committed projection generation.
func (r *Repository) ListCanonicalUsagePage(ctx context.Context, filter models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error) {
	return r.listProjectedUsagePage(ctx, filter)
}
