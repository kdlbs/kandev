package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestLaunchSession_ProfileExplicitBypassesWorkflowStepOverride covers AC-14a:
// a caller-supplied agent profile marked ProfileExplicit must be used exactly
// as supplied, even when the destination workflow step pins a different
// profile. Without ProfileExplicit, the step's pin still silently overrides
// the caller's profile (the pre-existing, still-correct behavior for
// same-workspace starts) — asserting both cases in one test proves the gate
// actually discriminates rather than always returning the caller's value.
func TestLaunchSession_ProfileExplicitBypassesWorkflowStepOverride(t *testing.T) {
	tests := []struct {
		name            string
		profileExplicit bool
		wantProfile     string
	}{
		{
			name:            "explicit selector-backed profile wins over step pin",
			profileExplicit: true,
			wantProfile:     "explicit-profile",
		},
		{
			name:            "without ProfileExplicit the step pin still overrides",
			profileExplicit: false,
			wantProfile:     "step-pinned-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const taskID = "task-profile-explicit"
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
				Title: "Profile explicit task", Description: "run it", State: v1.TaskStateCreated,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create task: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks[taskID] = &v1.Task{
				ID: taskID, WorkspaceID: "ws1", Title: "Profile explicit task",
				Description: "run it", State: v1.TaskStateCreated,
			}

			stepGetter := newMockStepGetter()
			stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
				ID: "step1", WorkflowID: "wf1", AgentProfileID: "step-pinned-profile",
			}

			var captured *executor.LaunchAgentRequest
			agentManager := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					captured = req
					return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1", Status: v1.AgentStatusStarting}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)

			_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID:          taskID,
				Intent:          IntentStart,
				AgentProfileID:  "explicit-profile",
				WorkflowStepID:  "step1",
				ProfileExplicit: tt.profileExplicit,
				Prompt:          "do the work",
			})
			if err != nil {
				t.Fatalf("LaunchSession returned error: %v", err)
			}
			if captured == nil {
				t.Fatal("LaunchAgent was never called")
			}
			if captured.AgentProfileID != tt.wantProfile {
				t.Fatalf("launched agent profile = %q, want %q", captured.AgentProfileID, tt.wantProfile)
			}
		})
	}
}

// TestLaunchSession_ProfileExplicitBypassesSessionTargetStepOverride is the
// SessionTarget counterpart to TestLaunchSession_ProfileExplicitBypassesWorkflowStepOverride:
// AC-PROFILES-001.4/D14a requires ProfileExplicit to bypass every destination-step
// routing override, not just a plain AgentProfileID pin.
// prepareExplicitWorkflowStartRoute resolves a completely separate profile (and can
// reuse a different session) when the destination step carries SessionTarget, and
// that resolution used to run and overwrite the caller's profile unconditionally —
// a genuine gap this test closes (task_operations.go, Review round 4).
func TestLaunchSession_ProfileExplicitBypassesSessionTargetStepOverride(t *testing.T) {
	tests := []struct {
		name            string
		profileExplicit bool
		wantProfile     string
	}{
		{
			name:            "explicit selector-backed profile wins over session-target routing",
			profileExplicit: true,
			wantProfile:     "explicit-profile",
		},
		{
			name:            "without ProfileExplicit the session-target source step's profile still overrides",
			profileExplicit: false,
			wantProfile:     "session-target-profile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const taskID = "task-session-target-explicit"
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
				ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-target",
				Title: "Session target task", Description: "run it", State: v1.TaskStateCreated,
				CreatedAt: now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create task: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks[taskID] = &v1.Task{
				ID: taskID, WorkspaceID: "ws1", Title: "Session target task",
				Description: "run it", State: v1.TaskStateCreated,
			}

			stepGetter := newMockStepGetter()
			stepGetter.steps["step-source"] = &wfmodels.WorkflowStep{
				ID: "step-source", WorkflowID: "wf1", Position: 0,
				AgentProfileID: "session-target-profile",
			}
			stepGetter.steps["step-target"] = &wfmodels.WorkflowStep{
				ID: "step-target", WorkflowID: "wf1", Position: 1,
				AgentProfileID: "step-pinned-profile",
				SessionTarget:  &wfmodels.WorkflowSessionTarget{Kind: wfmodels.WorkflowSessionTargetStep, StepID: "step-source"},
			}

			var captured *executor.LaunchAgentRequest
			agentManager := &mockAgentManager{
				launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
					captured = req
					return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1", Status: v1.AgentStatusStarting}, nil
				},
			}
			svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)

			_, err := svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID:          taskID,
				Intent:          IntentStart,
				AgentProfileID:  "explicit-profile",
				WorkflowStepID:  "step-target",
				ProfileExplicit: tt.profileExplicit,
				Prompt:          "do the work",
			})
			if err != nil {
				t.Fatalf("LaunchSession returned error: %v", err)
			}
			if captured == nil {
				t.Fatal("LaunchAgent was never called")
			}
			if captured.AgentProfileID != tt.wantProfile {
				t.Fatalf("launched agent profile = %q, want %q", captured.AgentProfileID, tt.wantProfile)
			}
		})
	}
}
