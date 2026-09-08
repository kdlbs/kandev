package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
)

func TestRepairDynamicRouteAfterLaunchFailureSurfacesMarkerError(t *testing.T) {
	session := &models.TaskSession{
		ID:              "session-recovery",
		RouteGeneration: 7,
		RouteReason:     orchestrator.RouteActionLaunchFailedReason,
	}
	markerErr := errors.New("database unavailable")
	calls := 0
	err := repairDynamicRouteAfterLaunchFailure(
		context.Background(), session, session.RouteGeneration,
		func(context.Context, string, int64) error {
			calls++
			return markerErr
		},
	)
	if !errors.Is(err, markerErr) {
		t.Fatalf("repair error = %v, want marker error", err)
	}
	if calls != 1 {
		t.Fatalf("marker calls = %d, want one retry", calls)
	}
}

func TestRepairDynamicRouteAfterLaunchFailureIgnoresUnrelatedProjection(t *testing.T) {
	tests := []struct {
		name       string
		reason     string
		generation int64
	}{
		{name: "different reason", reason: "manual_retry", generation: 7},
		{name: "stale generation", reason: orchestrator.RouteActionLaunchFailedReason, generation: 8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			session := &models.TaskSession{
				ID:              "session-recovery",
				RouteGeneration: 7,
				RouteReason:     tt.reason,
			}
			err := repairDynamicRouteAfterLaunchFailure(
				context.Background(), session, tt.generation,
				func(context.Context, string, int64) error {
					calls++
					return nil
				},
			)
			if err != nil {
				t.Fatalf("repair error = %v, want nil", err)
			}
			if calls != 0 {
				t.Fatalf("marker calls = %d, want zero", calls)
			}
		})
	}
}
