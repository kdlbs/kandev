package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// newExplicitOnlyProfileExecutionResolver builds a real (non-nil)
// ProfileExecutionResolver whose backing profile store knows about
// validProfileID only. ValidateProfile therefore succeeds for validProfileID
// and fails (GetAgentProfile: sql.ErrNoRows) for any other profile id,
// including a workflow step's stale or otherwise-invalid pinned profile.
func newExplicitOnlyProfileExecutionResolver(t *testing.T, validProfileID string) *agentruntime.ProfileExecutionResolver {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &agentsettingsmodels.Agent{ID: "valid-agent", Name: "valid-agent"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateAgentProfile(ctx, &agentsettingsmodels.AgentProfile{
		ID: validProfileID, AgentID: "valid-agent", Name: "Valid", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	return agentruntime.NewProfileExecutionResolver(repo, dynamicruntime.NewEngine(), true)
}

// TestLaunchSession_ProfileExplicitPreflightIgnoresStepPinnedProfile covers
// Review round 4 Finding 1 / AC-14a: startTask's early preflight
// ValidateProfile call must not resolve through the workflow step's pinned
// profile when the caller supplied ProfileExplicit — the step's own pinned
// profile being invalid (e.g. stale or belonging to a disabled dynamic
// family) must not block a launch that never intended to use it.
func TestLaunchSession_ProfileExplicitPreflightIgnoresStepPinnedProfile(t *testing.T) {
	const explicitProfileID = "explicit-profile"
	const stepPinnedProfileID = "stale-step-pinned-profile" // deliberately never registered

	tests := []struct {
		name            string
		profileExplicit bool
		wantErr         bool
	}{
		{
			name:            "ProfileExplicit bypasses the preflight validation of the step's pinned profile",
			profileExplicit: true,
			wantErr:         false,
		},
		{
			name:            "without ProfileExplicit the preflight still validates the step's pinned profile and fails",
			profileExplicit: false,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const taskID = "task-profile-explicit-preflight"
			ctx := context.Background()
			repo := setupTestRepo(t)
			now := time.Now().UTC()
			if err := repo.CreateWorkspace(ctx, &models.Workspace{
				ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create workspace: %v", err)
			}
			if err := repo.CreateWorkflow(ctx, &models.Workflow{
				ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create workflow: %v", err)
			}
			if err := repo.CreateTask(ctx, &models.Task{
				ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
				Title: "Preflight task", Description: "run it", State: v1.TaskStateCreated,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create task: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks[taskID] = &v1.Task{
				ID: taskID, WorkspaceID: "ws1", Title: "Preflight task",
				Description: "run it", State: v1.TaskStateCreated,
			}

			stepGetter := newMockStepGetter()
			stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
				ID: "step1", WorkflowID: "wf1", AgentProfileID: stepPinnedProfileID,
			}

			var captured *executor.LaunchAgentRequest
			agentManager := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					captured = req
					return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1", Status: v1.AgentStatusStarting}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)
			svc.SetProfileExecutionResolver(newExplicitOnlyProfileExecutionResolver(t, explicitProfileID))

			_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID:          taskID,
				Intent:          IntentStart,
				AgentProfileID:  explicitProfileID,
				WorkflowStepID:  "step1",
				ProfileExplicit: tt.profileExplicit,
				Prompt:          "do the work",
			})

			if tt.wantErr {
				if err == nil {
					t.Fatal("LaunchSession succeeded, want the preflight to reject the step's invalid pinned profile")
				}
				return
			}
			if err != nil {
				t.Fatalf("LaunchSession returned error: %v", err)
			}
			if captured == nil {
				t.Fatal("LaunchAgent was never called")
			}
			if captured.AgentProfileID != explicitProfileID {
				t.Fatalf("launched agent profile = %q, want %q", captured.AgentProfileID, explicitProfileID)
			}
		})
	}
}
