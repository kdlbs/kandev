package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestApplyGuardedTransitionLifecycle_OfficeRejectLeg_DoesNotPromptDeciderSession
// is the two-session regression proof for the office reject leg (plan unit
// 7). wait_for_quorum guards are exclusively an Office-workflow feature
// (config/workflows/office-default.yml), and applyGuardedTransitionLifecycle
// is reached only from the engine's guarded-transition re-evaluation
// (quorum.go's applyFirstSatisfiedGuardedTransition -> ApplyTransitionIfAtStep),
// with sessionID set to whichever session RecordParticipantDecision resolved
// for the decider (the reviewer), not the task's assignee.
//
// Before the fix, applyGuardedTransitionLifecycle dispatched on_enter
// (triggerOnEnter=true), and Work's on_enter (auto_start_agent, no
// agent_profile_id of its own) fell through to processOnEnter's session
// continuation, prompting the reviewer's session directly into Work — the
// wrong agent. Office's own reactivity (office/dashboard.
// runReactivityForDecision) is the mechanism that correctly resolves and
// wakes the assignee; this test proves the orchestrator's guarded-transition
// bridge no longer also fires a second, wrongly-targeted prompt through
// session continuation. With one session in play this failure mode is
// unobservable — hence two distinct sessions (reviewer vs. assignee agent).
func TestApplyGuardedTransitionLifecycle_OfficeRejectLeg_DoesNotPromptDeciderSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "review",
		// ProjectID non-empty marks the task as Office-owned (IsFromOfficePredicate).
		ProjectID: "proj-1", AssigneeAgentProfileID: "assignee-agent",
		Title: "T", Description: "T", State: v1.TaskStateReview,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// The reviewer's own session — the identity RecordParticipantDecision
	// resolves and passes through as sessionID (AC-16a / officeSessionIdentity).
	// It is a distinct agent from the task's assignee and must never be the
	// one auto-started into Work.
	reviewerSession := &models.TaskSession{
		ID: "reviewer-sess", TaskID: "t1", AgentProfileID: "reviewer-agent",
		AgentExecutionID: "reviewer-exec",
		State:            models.TaskSessionStateWaitingForInput, IsPrimary: true,
		StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, reviewerSession); err != nil {
		t.Fatalf("create reviewer session: %v", err)
	}
	seedExecutorRunning(t, repo, reviewerSession.ID, "t1", reviewerSession.AgentExecutionID)

	stepGetter := newMockStepGetter()
	stepGetter.steps["review"] = &wfmodels.WorkflowStep{ID: "review", WorkflowID: "wf1", Position: 1}
	stepGetter.steps["work"] = &wfmodels.WorkflowStep{
		ID: "work", WorkflowID: "wf1", Position: 0,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateReview}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	svc := &Service{
		logger: log, repo: repo, workflowStepGetter: stepGetter, taskRepo: taskRepo, agentManager: agentMgr,
		messageQueue: messagequeue.NewServiceMemory(log), executor: exec,
		workflowStore: newWorkflowStore(repo, stepGetter, agentMgr, noopPublisher, log),
	}
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	applied, err := svc.applyGuardedTransitionLifecycle(
		ctx, "t1", reviewerSession.ID, "review", "work", engine.TriggerOnTurnComplete,
	)
	if err != nil {
		t.Fatalf("applyGuardedTransitionLifecycle: %v", err)
	}
	if !applied {
		t.Fatal("applyGuardedTransitionLifecycle() = false, want true (task should move to work)")
	}

	storedTask, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if storedTask.WorkflowStepID != "work" {
		t.Fatalf("workflow_step_id = %q, want work", storedTask.WorkflowStepID)
	}

	// If on_enter was dispatched (pre-fix behavior), processOnEnter runs in a
	// detached goroutine and signals completion here; wait for it before
	// checking capturedPromptCalls so the assertion isn't racing the
	// goroutine. Post-fix, on_enter is never dispatched for this path, so the
	// channel never fires and the short timeout is the expected case.
	select {
	case <-onEnterDone:
	case <-time.After(300 * time.Millisecond):
	}

	agentMgr.mu.Lock()
	promptCalls := len(agentMgr.capturedPromptCalls)
	agentMgr.mu.Unlock()
	if promptCalls != 0 {
		t.Fatalf("reviewer session was prompted into Work (%d calls) — the reject leg must wake the assignee via Office reactivity, not by continuing the decider's session", promptCalls)
	}
}
