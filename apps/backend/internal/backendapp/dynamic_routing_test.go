package backendapp

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
)

func TestLoadDynamicRouteActionSessionRejectsNonDynamicProfile(t *testing.T) {
	ctx := context.Background()
	_, profileRepo, repo := openTasklessDynamicRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-1", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-1", WorkspaceID: "workspace-1", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1", Title: "Task", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", AgentProfileID: "concrete-profile",
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	_, err := loadDynamicRouteActionSession(ctx, repo, profileRepo, &agentruntime.ProfileExecutionResolver{}, "session-1")
	if !errors.Is(err, orchestrator.ErrRouteActionRequiresDynamicProfile) {
		t.Fatalf("loadDynamicRouteActionSession error = %v, want non-dynamic profile rejection", err)
	}
}

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
