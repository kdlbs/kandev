package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.6
func TestEnsureSession_ForkReviewAwaitsManualStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-fork-review-open"
	const stepID = "step-fork-review-open"

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Review", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	metadata := map[string]interface{}{
		taskmodels.MetaKeyAgentProfileID:            testAgentProfileID,
		taskmodels.MetaKeyForkPRRequiresManualStart: true,
	}
	if err := repo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepID,
		Title: "Review fork PR", State: v1.TaskStateInProgress,
		Metadata: metadata, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAgentProfileID:            testAgentProfileID,
			taskmodels.MetaKeyForkPRRequiresManualStart: true,
			taskmodels.MetaKeyAutoStartGuard:            true,
		},
	}
	var launchCalls atomic.Int32
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls.Add(1)
			return &executor.LaunchAgentResponse{AgentExecutionID: "unexpected-launch"}, nil
		},
	}
	service := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)

	response, err := service.EnsureSession(ctx, taskID, EnsureSessionOptions{
		ActivationSource: LaunchActivationSourceSessionOpen,
	})
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	if response == nil || !response.Success {
		t.Fatalf("EnsureSession response = %+v, want successful session-less response", response)
	}
	if response.SessionID != "" || response.Source != "awaiting_manual_start" {
		t.Fatalf("EnsureSession response = %+v, want no session and source awaiting_manual_start", response)
	}
	if response.ActivationDisposition != activationDispositionSuppressed || response.ActivationReason != "fork_pr_manual_start" {
		t.Fatalf("activation disposition/reason = %q/%q, want suppressed/fork_pr_manual_start", response.ActivationDisposition, response.ActivationReason)
	}
	explicitAutoStart := true
	if _, err := service.EnsureSession(ctx, taskID, EnsureSessionOptions{
		AutoStart:        &explicitAutoStart,
		ActivationSource: LaunchActivationSourceSessionOpen,
	}); err != errForkPRManualStartRequired {
		t.Fatalf("explicit automatic EnsureSession error = %v, want %v", err, errForkPRManualStartRequired)
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions after passive open = %d, want 0", len(sessions))
	}
	if got := launchCalls.Load(); got != 0 {
		t.Fatalf("LaunchAgent calls = %d, want 0", got)
	}
}

// @covers AC-INTEGRATIONS-GITHUB-FORK-REVIEW-START-001.6
func TestEnsureSession_ForkReviewPrepareStepPassiveOpenAwaitsManualStart(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	const taskID = "task-fork-review-prepare-open"
	const stepID = "step-fork-review-prepare-open"

	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf1", Name: "Review", Position: 0,
	}
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &taskmodels.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &taskmodels.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepID,
		Title: "Review fork PR", State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAgentProfileID:            testAgentProfileID,
			taskmodels.MetaKeyForkPRRequiresManualStart: true,
		},
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, State: v1.TaskStateInProgress,
		Metadata: map[string]interface{}{
			taskmodels.MetaKeyAgentProfileID:            testAgentProfileID,
			taskmodels.MetaKeyForkPRRequiresManualStart: true,
		},
	}
	var launchCalls atomic.Int32
	agentManager := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalls.Add(1)
			return &executor.LaunchAgentResponse{AgentExecutionID: "unexpected-launch"}, nil
		},
	}
	service := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentManager)

	for _, autoStart := range []*bool{nil, new(bool)} {
		response, err := service.EnsureSession(ctx, taskID, EnsureSessionOptions{
			AutoStart: autoStart, ActivationSource: LaunchActivationSourceSessionOpen,
		})
		if err != nil {
			t.Fatalf("EnsureSession(AutoStart=%v): %v", autoStart, err)
		}
		if response == nil || response.SessionID != "" || response.Source != "awaiting_manual_start" {
			t.Fatalf("EnsureSession(AutoStart=%v) = %+v, want session-less manual-start response", autoStart, response)
		}
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions after passive opens = %d, want 0", len(sessions))
	}
	if got := launchCalls.Load(); got != 0 {
		t.Fatalf("LaunchAgent calls = %d, want 0", got)
	}
}
