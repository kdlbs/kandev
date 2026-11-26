package repository

import (
	"context"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
)

var ErrUsageNotVisible = usagehistory.ErrNotVisible
var ErrInvalidUsageBatch = usagehistory.ErrInvalidBatch

type UsageHistoryRepository interface {
	ListUsageCollectionTargets(context.Context, string, pluginsdk.UsageCollectionTargetQueryV1) (pluginsdk.UsageCollectionTargetPageV1, error)
	ReportUsageObservations(context.Context, string, pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error)
}

type UsageProjectionReader interface {
	ListCanonicalUsagePage(context.Context, models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error)
}

type RetainedUsagePageReader interface {
	ListRetainedCanonicalUsagePage(context.Context, models.SessionUsageFilter) ([]models.SessionUsageMeasurement, models.UsageProjectionState, error)
}

type UsageSummaryReader interface {
	ListUsageDailySummaries(context.Context, models.SessionUsageFilter) ([]models.UsageDailySummary, models.UsageProjectionMetadata, error)
}

type UsageFactReader interface {
	ListUsageCanonicalFacts(context.Context, models.SessionUsageFilter) ([]models.UsageCanonicalFact, models.UsageProjectionMetadata, error)
}
