package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
)

// exhaustAutoRetryRoute leaves the route waiting without an eligible
// candidate: concrete-a is current and concrete-b's circuit is open.
func exhaustAutoRetryRoute(t *testing.T) (*ProfileExecutionResolver, int64) {
	t.Helper()
	resolver, generation := newAutoTryNextResolver(t)
	resolver.engine.Circuits().Open(
		dynamic.ResourceKey(dynamic.ScopeProfile, "concrete-b"), time.Now().Add(time.Minute), routingerr.CodeRateLimited,
	)
	_, err := resolver.ResolveRouteAction(
		context.Background(), "session-auto", "dynamic-profile", "concrete-a", generation, "try_next",
	)
	var noCandidate *dynamic.NoEligibleCandidateError
	if !errors.As(err, &noCandidate) {
		t.Fatalf("try_next error = %v, want no eligible candidate", err)
	}
	return resolver, noCandidate.Generation
}

func TestResolveRouteActionRetryReselectsWaitingRoute(t *testing.T) {
	for _, tc := range []struct {
		action, reason string
	}{
		{"retry", routeReasonManualRetry},
		{RouteActionAutoRetry, RouteReasonAutoFailedRouteRetry},
	} {
		t.Run(tc.action, func(t *testing.T) {
			resolver, waiting := exhaustAutoRetryRoute(t)
			execution, err := resolver.ResolveRouteAction(
				context.Background(), "session-auto", "dynamic-profile", "concrete-a", waiting, tc.action,
			)
			if err != nil {
				t.Fatalf("ResolveRouteAction(%s): %v", tc.action, err)
			}
			if execution.ExecutionProfileID != "concrete-a" || execution.Generation != waiting+1 ||
				execution.Decision.Reason != tc.reason {
				t.Fatalf("execution = %#v, want concrete-a at generation %d with reason %s", execution, waiting+1, tc.reason)
			}
		})
	}
}

func TestResolveRouteActionAutoRetryResumesActionRequiredRoute(t *testing.T) {
	resolver, generation := newAutoTryNextResolver(t)
	ctx := context.Background()
	if _, err := resolver.engine.MarkActionRequired(ctx, "session-auto", generation, "rate_limited"); err != nil {
		t.Fatalf("MarkActionRequired: %v", err)
	}
	execution, err := resolver.ResolveRouteAction(
		ctx, "session-auto", "dynamic-profile", "concrete-a", generation, RouteActionAutoRetry,
	)
	if err != nil {
		t.Fatalf("ResolveRouteAction: %v", err)
	}
	if execution.ExecutionProfileID != "concrete-a" || execution.Generation != generation ||
		execution.Decision.Status != "retrying" || execution.Decision.Reason != RouteReasonAutoFailedRouteRetry {
		t.Fatalf("execution = %#v, want the same candidate and generation retrying with the automatic reason", execution)
	}
}

func TestResolveRouteActionAutoRetryIsGenerationFenced(t *testing.T) {
	resolver, waiting := exhaustAutoRetryRoute(t)
	_, err := resolver.ResolveRouteAction(
		context.Background(), "session-auto", "dynamic-profile", "concrete-a", waiting-1, RouteActionAutoRetry,
	)
	if !errors.Is(err, dynamic.ErrStaleGeneration) {
		t.Fatalf("ResolveRouteAction error = %v, want stale generation", err)
	}
}

func TestScheduleFailedRouteRetryWaitsForTheOpenCircuit(t *testing.T) {
	resolver, waiting := exhaustAutoRetryRoute(t)
	ctx := context.Background()
	// concrete-a failed too, so both candidates are open; b expires first.
	aExpiry := time.Now().Add(5 * time.Minute)
	resolver.engine.Circuits().Open(dynamic.ResourceKey(dynamic.ScopeProfile, "concrete-a"), aExpiry, routingerr.CodeRateLimited)
	bExpiry, _ := resolver.engine.Circuits().OpenUntil(dynamic.ResourceKey(dynamic.ScopeProfile, "concrete-b"))
	failure := dynamic.FailedRouteFailure{ExecutionProfileID: "concrete-a", Code: routingerr.CodeRateLimited}
	pending, scheduled, err := resolver.ScheduleFailedRouteRetry(ctx, "session-auto", "dynamic-profile", waiting, failure, 1, 30*time.Second)
	if err != nil || !scheduled {
		t.Fatalf("ScheduleFailedRouteRetry = %v scheduled=%v", err, scheduled)
	}
	if !pending.Due.Equal(bExpiry) || pending.Generation != waiting || pending.Failure != failure {
		t.Fatalf("pending = %#v, want due at concrete-b's circuit expiry %v", pending, bExpiry)
	}
}
