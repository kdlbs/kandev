package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
)

var errFailedRouteRetryNotConfigured = errors.New("dynamic profile execution is not configured")

// ScheduleFailedRouteRetry records one automatic retry for a route that a
// classified failure left in manual recovery. The profile's candidates and
// their circuits decide the earliest due time. See
// dynamic.Engine.ScheduleFailedRouteRetry.
func (r *ProfileExecutionResolver) ScheduleFailedRouteRetry(
	ctx context.Context,
	sessionID, logicalProfileID string,
	expectedGeneration int64,
	failure dynamic.FailedRouteFailure,
	maxAttempts int,
	minDelay time.Duration,
) (dynamic.PendingFailedRouteRetry, bool, error) {
	if r.engine == nil || r.profiles == nil {
		return dynamic.PendingFailedRouteRetry{}, false, errFailedRouteRetryNotConfigured
	}
	profile, err := r.loadDynamicProfile(ctx, logicalProfileID)
	if err != nil {
		return dynamic.PendingFailedRouteRetry{}, false, err
	}
	return r.engine.ScheduleFailedRouteRetry(
		ctx, sessionID, profile, expectedGeneration, failure, maxAttempts, minDelay,
	)
}

// ClaimFailedRouteRetry consumes a due automatic retry schedule just before
// the retry route action is submitted.
func (r *ProfileExecutionResolver) ClaimFailedRouteRetry(
	ctx context.Context,
	sessionID string,
	expectedGeneration int64,
) (dynamic.PendingFailedRouteRetry, error) {
	if r.engine == nil {
		return dynamic.PendingFailedRouteRetry{}, errFailedRouteRetryNotConfigured
	}
	return r.engine.ClaimFailedRouteRetry(ctx, sessionID, expectedGeneration)
}

// ClearFailedRouteRetry ends a session's automatic retry chain after a
// successful turn.
func (r *ProfileExecutionResolver) ClearFailedRouteRetry(ctx context.Context, sessionID string) error {
	if r.engine == nil {
		return nil
	}
	return r.engine.ClearFailedRouteRetry(ctx, sessionID)
}
