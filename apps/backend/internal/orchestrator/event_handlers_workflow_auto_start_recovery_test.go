package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// This file covers the two latent auto-start recovery defects filed as a
// sibling of WO-36.1: a replay double-launch race between the queue-promotion
// lifecycle token and the create-time auto-start opt-in (Gap A), and a
// pre-session StartTask failure that stranded the create-time opt-in with no
// durable marker (Gap B). Both are fixed by the same mechanism: the opt-in
// is claimed synchronously by the launch attempt itself (autoStartTaskForLoadedStep,
// via claimAutoStartOnCreateForLaunch), not merely observed, and restored on
// failure (handleAutoStartFailure) exactly like the existing queue-promotion
// token.

// TestRecoverTaskLifecycleAttemptConcurrentPromotionAndAutoStartOnCreateLaunchesOnce
// covers Gap A: recoverTaskLifecycleAttempt used to evaluate
// autoStartOnCreateActionable against the SAME in-memory task it loaded at
// the top of the function, without re-fetching after the queue-promotion
// branch ran. handleTaskQueuePromoted's no-session path schedules a launch
// through autoStartTaskForLoadedStep, but that launch's actual session
// creation happens in a detached goroutine — so a stale read could still see
// MetaKeyAutoStartOnCreate present and schedule a second, redundant launch via
// recoverAutoStartOnCreate.
//
// The fix makes the race structural rather than timing-dependent:
// autoStartTaskForLoadedStep claims MetaKeyAutoStartOnCreate synchronously,
// before spawning its launch goroutine, and recoverTaskLifecycleAttempt
// re-fetches the task after the promotion branch. So by the time
// autoStartOnCreateActionable evaluates, the token is already gone.
func TestRecoverTaskLifecycleAttemptConcurrentPromotionAndAutoStartOnCreateLaunchesOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	metadata := map[string]interface{}{
		models.MetaKeyQueuePromotionPending: true,
		models.MetaKeyAutoStartOnCreate:     true,
		models.MetaKeyAgentProfileID:        "routine-assignee",
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-race",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		WIPAdmitted:    true,
		Title:          "Concurrent tokens",
		Description:    "prompt",
		State:          v1.TaskStateTODO,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Auto Start Step", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-race"] = &v1.Task{
		ID:          "t-race",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Description: "prompt",
		State:       v1.TaskStateTODO,
		Metadata:    metadata,
	}
	launched := make(chan string, 4)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
			launched <- agentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	svc.recoverTaskLifecycleAttempt(ctx, "t-race")

	select {
	case got := <-launched:
		if got != "routine-assignee" {
			t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the auto-start launch")
	}
	// Settle window: prove no second launch follows the first, rather than
	// just reading one value off the channel and declaring victory.
	select {
	case got := <-launched:
		t.Fatalf("unexpected second launch for agent profile %q", got)
	case <-time.After(300 * time.Millisecond):
	}

	sessions, err := repo.ListTaskSessions(ctx, "t-race")
	requireNoError(t, err)
	if len(sessions) != 1 {
		t.Fatalf("ListTaskSessions(t-race) = %d sessions, want 1", len(sessions))
	}
}

// TestRecoverTaskLifecycleAttemptConcurrentPromotionLaunchesOnceWithoutWorkspaceBindingElection
// covers the same Gap A race, but on the session-create route that does NOT
// go through CreateTaskSessionWithWorkspaceBinding's election
// (executor_execute.go: bindWorkspace && !taskUsesDeferredEnvironmentInheritance).
// Today that election is what accidentally limits the race to one launch on
// the ordinary path: the loser's session-create call fails with "workspace is
// preparing", and handleAutoStartFailure restores the promotion token for the
// next sweep to converge. A task using workspace.mode=inherit_parent takes the
// CreateTaskSessionWithInitialRuntimeSeed / plain CreateTaskSession route
// instead, neither of which enforces per-task session uniqueness — so this is
// the test that proves the actual fix (the synchronous token claim), not the
// incidental mitigation.
func TestRecoverTaskLifecycleAttemptConcurrentPromotionLaunchesOnceWithoutWorkspaceBindingElection(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "t-race-parent", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		Title: "Parent", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}))
	// ExecutorType intentionally left empty: the child launch below resolves
	// no executor either (no MetaKeyExecutorID, no default configured in this
	// test harness), and LaunchPreparedSession rejects an inherited
	// environment whose non-empty ExecutorType disagrees with the launch's
	// resolved one (executor_execute.go's workspace-reuse-unsafe guard).
	requireNoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-parent", TaskID: "t-race-parent",
		Status: models.TaskEnvironmentStatusReady,
	}))
	requireNoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ps1", TaskID: "t-race-parent", State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "env-parent", StartedAt: now, UpdatedAt: now,
	}))

	metadata := map[string]interface{}{
		models.MetaKeyQueuePromotionPending: true,
		models.MetaKeyAutoStartOnCreate:     true,
		models.MetaKeyAgentProfileID:        "routine-assignee",
		"workspace":                         map[string]interface{}{"mode": "inherit_parent"},
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-race-child",
		ParentID:       "t-race-parent",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		WIPAdmitted:    true,
		Title:          "Concurrent tokens, inherited workspace",
		Description:    "prompt",
		State:          v1.TaskStateTODO,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Auto Start Step", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-race-child"] = &v1.Task{
		ID:          "t-race-child",
		ParentID:    "t-race-parent",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Description: "prompt",
		State:       v1.TaskStateTODO,
		Metadata:    metadata,
	}
	launched := make(chan string, 4)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
			launched <- agentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	svc.recoverTaskLifecycleAttempt(ctx, "t-race-child")

	select {
	case got := <-launched:
		if got != "routine-assignee" {
			t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the auto-start launch")
	}
	select {
	case got := <-launched:
		t.Fatalf("unexpected second launch for agent profile %q", got)
	case <-time.After(300 * time.Millisecond):
	}

	sessions, err := repo.ListTaskSessions(ctx, "t-race-child")
	requireNoError(t, err)
	if len(sessions) != 1 {
		t.Fatalf("ListTaskSessions(t-race-child) = %d sessions, want 1", len(sessions))
	}
	if sessions[0].TaskEnvironmentID != "env-parent" {
		t.Fatalf("session TaskEnvironmentID = %q, want env-parent (inherited, non-bindWorkspace route confirms this isn't the workspace-binding election doing the work)", sessions[0].TaskEnvironmentID)
	}
}

// TestHandleTaskCreatedRestoresAutoStartOnCreateOnPreSessionFailure covers Gap
// B: handleTaskCreated claims (removes) MetaKeyAutoStartOnCreate synchronously
// before handing the launch to autoStartTaskForLoadedStep's detached
// goroutine. Before this fix, handleAutoStartFailure restored
// MetaKeyAutoStartGuard and MetaKeyQueuePromotionPending on failure but never
// MetaKeyAutoStartOnCreate, so a StartTask failure before a session exists
// stranded the task with no durable marker for the next startup sweep to
// find.
//
// getTaskErr fails scheduler.GetTask, which startTask calls after resolving
// the target step but before prepareSessionForStart creates any session row
// — a deterministic pre-session failure, no sleep-and-hope.
func TestHandleTaskCreatedRestoresAutoStartOnCreateOnPreSessionFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	metadata := map[string]interface{}{
		models.MetaKeyAutoStartOnCreate: true,
		models.MetaKeyAgentProfileID:    "routine-assignee",
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-pre-session-failure",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		Title:          "Routine run",
		Description:    "prompt",
		State:          v1.TaskStateCreated,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	launched := make(chan string, 2)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
			launched <- agentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.getTaskErr = errors.New("boom: transient task lookup failure")
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t-pre-session-failure"})

	waitForAutoStartFailedMarker(t, repo, ctx, "t-pre-session-failure")

	select {
	case got := <-launched:
		t.Fatalf("unexpected launch despite pre-session task lookup failure: %q", got)
	default:
	}

	reloaded, err := repo.GetTask(ctx, "t-pre-session-failure")
	requireNoError(t, err)
	if !models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("MetaKeyAutoStartOnCreate was not restored after a pre-session StartTask failure; the task is stranded with no durable marker for the next startup sweep")
	}
	sessions, err := repo.ListTaskSessions(ctx, "t-pre-session-failure")
	requireNoError(t, err)
	if len(sessions) != 0 {
		t.Fatalf("ListTaskSessions = %d, want 0 (the injected failure must happen before session persistence)", len(sessions))
	}

	// Clear the injected failure and let a subsequent startup sweep retry
	// using the restored token. Locked because the first launch attempt's
	// detached goroutine (see autoStartTaskForLoadedStep) can still be
	// finishing unrelated work on taskRepo after waitForAutoStartFailedMarker
	// observes the marker on the separate repo above; GetTask takes the same
	// lock on its read side.
	taskRepo.mu.Lock()
	taskRepo.getTaskErr = nil
	taskRepo.tasks["t-pre-session-failure"] = &v1.Task{
		ID:          "t-pre-session-failure",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Description: "prompt",
		State:       v1.TaskStateCreated,
		Metadata:    metadata,
	}
	taskRepo.mu.Unlock()

	svc.reconcileTaskLifecycleTokens(ctx)

	select {
	case got := <-launched:
		if got != "routine-assignee" {
			t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the retried auto-start launch")
	}
}

// TestHandleTaskCreatedWithQueuePromotionPendingRestoresAutoStartOnCreateOnPreSessionFailure
// covers Review Round 1's F1: a task carrying BOTH MetaKeyAutoStartOnCreate and
// MetaKeyQueuePromotionPending reaches handleTaskCreated (as recoverAutoStartOnCreate's
// replay does), which claims MetaKeyAutoStartOnCreate and calls autoStartTaskForStep
// with autoStartOnCreateClaimed=true. autoStartTaskForStep sees the promotion
// token and redirects into the queue-promotion handler; before the fix that
// redirect dropped autoStartOnCreateClaimed, so claimAutoStartOnCreateForLaunch
// saw the (already-removed) key as unclaimed and a pre-session StartTask
// failure restored MetaKeyQueuePromotionPending but never
// MetaKeyAutoStartOnCreate, stranding the task on exactly the recovery path
// this card exists to close.
func TestHandleTaskCreatedWithQueuePromotionPendingRestoresAutoStartOnCreateOnPreSessionFailure(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	metadata := map[string]interface{}{
		models.MetaKeyAutoStartOnCreate:     true,
		models.MetaKeyQueuePromotionPending: true,
		models.MetaKeyAgentProfileID:        "routine-assignee",
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-promotion-and-create",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		WIPAdmitted:    true,
		Title:          "Promoted routine run",
		Description:    "prompt",
		State:          v1.TaskStateTODO,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	launched := make(chan string, 2)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
			launched <- agentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.getTaskErr = errors.New("boom: transient task lookup failure")
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)

	svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t-promotion-and-create"})

	waitForAutoStartFailedMarker(t, repo, ctx, "t-promotion-and-create")

	select {
	case got := <-launched:
		t.Fatalf("unexpected launch despite pre-session task lookup failure: %q", got)
	default:
	}

	reloaded, err := repo.GetTask(ctx, "t-promotion-and-create")
	requireNoError(t, err)
	if !models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("MetaKeyAutoStartOnCreate was not restored after a pre-session StartTask failure reached through the queue-promotion redirect")
	}
	if _, pending := reloaded.Metadata[models.MetaKeyQueuePromotionPending]; !pending {
		t.Fatal("MetaKeyQueuePromotionPending was not restored after the pre-session StartTask failure")
	}
	sessions, err := repo.ListTaskSessions(ctx, "t-promotion-and-create")
	requireNoError(t, err)
	if len(sessions) != 0 {
		t.Fatalf("ListTaskSessions = %d, want 0 (the injected failure must happen before session persistence)", len(sessions))
	}
}

// erroringDependencyReader models a transient DependencyGate read failure,
// distinct from a genuine dependency block: DependencyGate itself errors, so
// dependencyBlocksAutoStart fails closed with gateErrored=true regardless of
// whether the task has any dependency edges at all.
type erroringDependencyReader struct {
	err error
}

func (r *erroringDependencyReader) DependencyGate(context.Context, string) (bool, string, error) {
	return false, "", r.err
}

func (r *erroringDependencyReader) ListDependentTaskIDs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (r *erroringDependencyReader) ListPendingDependencyLaunches(context.Context) ([]taskservice.PendingDependencyLaunch, error) {
	return nil, nil
}

// TestAutoStartTaskForStepRestoresAutoStartOnCreateOnDependencyGateError covers
// Review Round 2's F4: autoStartTaskForStep's dependency gate is its third
// early return, and unlike the other two (GetTask, GetStep failures, both
// already fixed), it did not restore MetaKeyAutoStartOnCreate when
// autoStartOnCreateClaimed was true. handleTaskCreated already removes the key
// before dispatching here, and the startup sweep discovers candidates only by
// that key's existence — so a DependencyGate read failure (not a genuine
// block) stranded the task permanently: it has no dependency edge for
// reconcileDependencyLaunchesOnStartup to find, and no other path re-lists it.
//
// A genuine block is deliberately NOT covered by this restore (see
// dependencyBlocksAutoStart's gateErrored split): it is already covered by
// evaluateDependentAfterPredecessorChange / reconcileDependencyLaunchesOnStartup
// once the dependency resolves, so restoring there would only burn
// recoverTaskLifecycleAttempt's bounded retry budget on every startup for as
// long as the task stays blocked.
func TestAutoStartTaskForStepRestoresAutoStartOnCreateOnDependencyGateError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))

	metadata := map[string]interface{}{
		models.MetaKeyAutoStartOnCreate: true,
		models.MetaKeyAgentProfileID:    "routine-assignee",
	}
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:             "t-dependency-gate-error",
		WorkspaceID:    "ws1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step1",
		Title:          "Routine run",
		Description:    "prompt",
		State:          v1.TaskStateCreated,
		Metadata:       metadata,
		CreatedAt:      now,
		UpdatedAt:      now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Routine Start", Position: 0,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}

	launched := make(chan string, 2)
	agentMgr := &mockAgentManager{
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentProfileID, _ := req.Metadata[models.MetaKeyAgentProfileID].(string)
			launched <- agentProfileID
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t-dependency-gate-error"] = &v1.Task{
		ID:          "t-dependency-gate-error",
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Description: "prompt",
		State:       v1.TaskStateCreated,
		Metadata:    metadata,
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.SetTaskDependencyReader(&erroringDependencyReader{err: errors.New("boom: transient dependency gate failure")})

	svc.handleTaskCreated(ctx, watcher.TaskEventData{TaskID: "t-dependency-gate-error"})

	select {
	case got := <-launched:
		t.Fatalf("unexpected launch despite dependency gate error: %q", got)
	default:
	}

	reloaded, err := repo.GetTask(ctx, "t-dependency-gate-error")
	requireNoError(t, err)
	if !models.HasAutoStartOnCreateIntent(reloaded.Metadata) {
		t.Fatal("MetaKeyAutoStartOnCreate was not restored after a DependencyGate error; the task is stranded with no dependency edge for reconcileDependencyLaunchesOnStartup to find and no other marker for the next sweep")
	}
	sessions, err := repo.ListTaskSessions(ctx, "t-dependency-gate-error")
	requireNoError(t, err)
	if len(sessions) != 0 {
		t.Fatalf("ListTaskSessions = %d, want 0 (the dependency gate must block before any session is created)", len(sessions))
	}

	// Clear the injected failure and let a subsequent startup sweep retry using
	// the restored token.
	svc.SetTaskDependencyReader(&resolvedDependencyReader{})

	svc.reconcileTaskLifecycleTokens(ctx)

	select {
	case got := <-launched:
		if got != "routine-assignee" {
			t.Fatalf("AgentProfileID = %q, want routine-assignee", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the retried auto-start launch")
	}
}
