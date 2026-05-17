package scheduler

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
)

// publishProviderHealthChanged emits a provider_health_changed event so
// the office WS broadcaster forwards it to connected clients.
func (ss *SchedulerService) publishProviderHealthChanged(
	ctx context.Context, h models.ProviderHealth,
) {
	if ss.eb == nil {
		return
	}
	var retryAt string
	if h.RetryAt != nil {
		retryAt = h.RetryAt.UTC().Format(time.RFC3339)
	}
	payload := map[string]interface{}{
		"workspace_id": h.WorkspaceID,
		"provider_id":  h.ProviderID,
		"scope":        h.Scope,
		"scope_value":  h.ScopeValue,
		"state":        h.State,
		"error_code":   h.ErrorCode,
		"retry_at":     retryAt,
	}
	ev := bus.NewEvent(events.OfficeProviderHealthChanged, "office-scheduler", payload)
	if err := ss.eb.Publish(ctx, events.OfficeProviderHealthChanged, ev); err != nil {
		ss.logger.Warn("publish provider_health_changed failed",
			zap.String("workspace_id", h.WorkspaceID),
			zap.String("provider_id", h.ProviderID),
			zap.Error(err))
	}
}

// publishProviderHealthyChanged emits the healthy-flip variant when the
// caller does not have a full ProviderHealth row in hand.
func (ss *SchedulerService) publishProviderHealthy(
	ctx context.Context, workspaceID, providerID, scope, scopeValue string,
) {
	ss.publishProviderHealthChanged(ctx, models.ProviderHealth{
		WorkspaceID: workspaceID,
		ProviderID:  providerID,
		Scope:       models.ProviderHealthScope(scope),
		ScopeValue:  scopeValue,
		State:       "healthy",
	})
}

// publishRouteAttemptAppended emits a route_attempt_appended event with
// the same DTO shape the run-detail HTTP layer surfaces.
func (ss *SchedulerService) publishRouteAttemptAppended(
	ctx context.Context, runID string, attempt models.RouteAttempt,
) {
	if ss.eb == nil {
		return
	}
	payload := map[string]interface{}{
		"run_id":  runID,
		"attempt": attempt,
	}
	ev := bus.NewEvent(events.OfficeRouteAttemptAppended, "office-scheduler", payload)
	if err := ss.eb.Publish(ctx, events.OfficeRouteAttemptAppended, ev); err != nil {
		ss.logger.Warn("publish route_attempt_appended failed",
			zap.String("run_id", runID), zap.Error(err))
	}
}
