package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) GetRecordedModelUsage(ctx context.Context, wsID string, since, until time.Time) (items []models.RecordedModelUsage, err error) {
	parentCtx := ctx
	ctx, release, err := r.beginAnalyticsOperation(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { err = normalizeAnalyticsOperationError(parentCtx, ctx, err); release() }()
	query := `SELECT e.model,e.provider,e.measurement_source,e.usage_completeness,e.cost_source,e.estimated,
	 COUNT(*) AS event_count,SUM(e.tokens_in) AS tokens_in,SUM(e.tokens_total) AS tokens_total,
	 CASE WHEN COUNT(e.tokens_out)=COUNT(*) THEN SUM(e.tokens_out) END AS tokens_out,
	 CASE WHEN COUNT(e.tokens_cached_read)=COUNT(*) THEN SUM(e.tokens_cached_read) END AS tokens_cached_read,
	 CASE WHEN COUNT(e.tokens_cached_write)=COUNT(*) THEN SUM(e.tokens_cached_write) END AS tokens_cached_write,
	 SUM(e.cost_subcents) AS cost_subcents
	 FROM task_usage_events e JOIN tasks t ON t.id=e.task_id
	 WHERE t.workspace_id=? AND t.is_ephemeral=0` + andNotAutomationOriginT + `
	 AND e.occurred_at>=? AND e.occurred_at<=?
	 GROUP BY e.model,e.provider,e.measurement_source,e.usage_completeness,e.cost_source,e.estimated
	 ORDER BY event_count DESC,e.model,e.provider,e.measurement_source,e.usage_completeness,e.cost_source,e.estimated`
	items = []models.RecordedModelUsage{}
	err = r.ro.SelectContext(ctx, &items, r.ro.Rebind(query), wsID, since, until)
	return items, err
}
