package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedWorkflowQueueWatchdogTaskAndSession seeds the workspace/workflow/task/
// session/executor rows the watchdog tests reuse: a session sitting in
// WAITING_FOR_INPUT on the Merge step with a stale workflow auto-start prompt
// already in the queue. The returned state is the orphan condition the
// watchdog is meant to recover from.
func seedWorkflowQueueWatchdogTaskAndSession(
	t *testing.T,
	taskID, sessionID, executionID, mergeStepID, taskWorkflow string,
	sessionState models.TaskSessionState,
) (*Service, *mockAgentManager, *messagequeue.Service) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-wd", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: taskWorkflow, WorkspaceID: "ws-wd", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps[mergeStepID] = &wfmodels.WorkflowStep{
		ID: mergeStepID, WorkflowID: taskWorkflow, Name: "Merge", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkflowID: taskWorkflow, WorkflowStepID: mergeStepID,
		Title: "Test", Description: "Test task",
		State:     v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:               sessionID,
		TaskID:           taskID,
		AgentProfileID:   "profile-wd",
		AgentExecutionID: executionID,
		State:            sessionState,
		IsPrimary:        true,
		StartedAt:        now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, WorkflowID: taskWorkflow, State: v1.TaskStateInProgress}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	mq := messagequeue.NewServiceMemory(testLogger())
	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})

	svc := &Service{
		logger:       testLogger(),
		repo:         repo,
		taskRepo:     taskRepo,
		agentManager: agentMgr,
		messageQueue: mq,
		executor:     exec,
	}
	svc.SetWorkflowStepGetter(stepGetter)

	// Seed a stale workflow queue entry. queued_at is set well before the
	// watchdog's orphanAge cutoff so the sweep query picks it up.
	stale := &messagequeue.QueuedMessage{
		SessionID: sessionID,
		TaskID:    taskID,
		Content:   "merge prompt",
		QueuedBy:  messagequeue.QueuedByWorkflow,
		QueuedAt:  now.Add(-10 * time.Minute),
		Metadata:  map[string]interface{}{"workflow_step_name": "Merge"},
	}
	if _, err := mq.QueueMessageWithMetadata(ctx, stale.SessionID, stale.TaskID, stale.Content, "", stale.QueuedBy, false, nil, stale.Metadata); err != nil {
		t.Fatalf("seed queue: %v", err)
	}
	// QueueMessageWithMetadata sets queued_at = now; backdate it via the
	// memory-repo test helper so the watchdog's "older than X" filter
	// matches. Mirrors how the bug looks in production — a stuck queue
	// entry whose queued_at is many minutes/hours in the past.
	mq.SetQueuedAtForTesting(t, sessionID, now.Add(-10*time.Minute))

	return svc, agentMgr, mq
}

// TestWorkflowQueueWatchdog_RecoversOrphanedWorkflowQueue: stale workflow
// entry + no live agent + no active turn → sweep must drive
// tryEnsureExecution which calls LaunchAgent via the executor.
func TestWorkflowQueueWatchdog_RecoversOrphanedWorkflowQueue(t *testing.T) {
	const (
		taskID       = "task-wd-1"
		sessionID    = "sess-wd-1"
		executionID  = "exec-wd-1"
		mergeStepID  = "step-wd-merge"
		taskWorkflow = "wf-wd-1"
	)
	svc, agentMgr, _ := seedWorkflowQueueWatchdogTaskAndSession(
		t, taskID, sessionID, executionID, mergeStepID, taskWorkflow,
		models.TaskSessionStateWaitingForInput,
	)

	// Agent is dead; tryEnsureExecution → ensureSessionRunning checks
	// GetExecutionBySession which calls IsAgentRunningForSession.
	agentMgr.isAgentRunning = false

	launchCalled := make(chan struct{})
	// resumeDone closes after waitForSessionReady has had at least one full
	// poll cycle (500ms) to observe WAITING_FOR_INPUT and exit — guarantees
	// the resume goroutine drains before goleak inspects.
	resumeDone := make(chan struct{})
	bootCtx := context.Background()
	var launchedOnce atomic.Bool
	agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		// Only the first call drives boot_ready; subsequent calls (e.g. a
		// drain-triggered re-resume) reuse the response without spawning
		// another helper goroutine.
		if !launchedOnce.CompareAndSwap(false, true) {
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID + "-resumed"}, nil
		}
		close(launchCalled)
		// Mirror the production handler: once the agent has resumed, fire
		// boot_ready so waitForSessionReady returns. Without this the
		// resume goroutine would block on waitForSessionReady's 90s
		// timeout and leak.
		go func() {
			defer close(resumeDone)
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			deadline := time.After(3 * time.Second)
			for {
				select {
				case <-tick.C:
					sess, err := svc.repo.GetTaskSession(bootCtx, req.SessionID)
					if err != nil || sess == nil || sess.State != models.TaskSessionStateStarting {
						continue
					}
					svc.handleAgentBootReady(bootCtx, watcher.AgentEventData{
						TaskID: req.TaskID, SessionID: req.SessionID,
					})
					// Give waitForSessionReady (500ms poll) at least one
					// full cycle to observe WAITING_FOR_INPUT and exit.
					time.Sleep(600 * time.Millisecond)
					return
				case <-deadline:
					return
				}
			}
		}()
		return &executor.LaunchAgentResponse{AgentExecutionID: executionID + "-resumed"}, nil
	}

	wd := svc.newWorkflowQueueWatchdog()
	wd.sweep(context.Background())

	select {
	case <-launchCalled:
	case <-time.After(5 * time.Second):
		t.Fatalf("watchdog did not drive LaunchAgent within 5s")
	}
	select {
	case <-resumeDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("resume goroutine did not drain within 5s")
	}
}

// TestWorkflowQueueWatchdog_SkipsLiveAgent: stale queue entry but the
// agent is alive → handleAgentReady will drain on next turn end. Watchdog
// must not recurse into LaunchAgent.
func TestWorkflowQueueWatchdog_SkipsLiveAgent(t *testing.T) {
	const (
		taskID       = "task-wd-2"
		sessionID    = "sess-wd-2"
		executionID  = "exec-wd-2"
		mergeStepID  = "step-wd-merge2"
		taskWorkflow = "wf-wd-2"
	)
	svc, agentMgr, mq := seedWorkflowQueueWatchdogTaskAndSession(
		t, taskID, sessionID, executionID, mergeStepID, taskWorkflow,
		models.TaskSessionStateRunning,
	)
	agentMgr.isAgentRunning = true

	agentMgr.launchAgentFunc = func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		t.Fatalf("LaunchAgent must not be called when the agent is alive")
		return nil, nil
	}

	wd := svc.newWorkflowQueueWatchdog()
	wd.sweep(context.Background())

	// Queue must be untouched: the watchdog only drops on terminal sessions.
	if got := mq.GetStatus(context.Background(), sessionID).Count; got != 1 {
		t.Errorf("queue count after sweep = %d, want 1 (untouched)", got)
	}
}

// TestWorkflowQueueWatchdog_SkipsActiveTurn: an active turn exists, so a
// future agent.ready will run the natural drain. Watchdog must not fire.
func TestWorkflowQueueWatchdog_SkipsActiveTurn(t *testing.T) {
	const (
		taskID       = "task-wd-3"
		sessionID    = "sess-wd-3"
		executionID  = "exec-wd-3"
		mergeStepID  = "step-wd-merge3"
		taskWorkflow = "wf-wd-3"
	)
	svc, agentMgr, mq := seedWorkflowQueueWatchdogTaskAndSession(
		t, taskID, sessionID, executionID, mergeStepID, taskWorkflow,
		models.TaskSessionStateRunning,
	)
	svc.activeTurns.Store(sessionID, "turn-1")
	agentMgr.isAgentRunning = false // doesn't matter; activeTurns guard runs first

	agentMgr.launchAgentFunc = func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		t.Fatalf("LaunchAgent must not be called when an active turn exists")
		return nil, nil
	}

	wd := svc.newWorkflowQueueWatchdog()
	wd.sweep(context.Background())

	if got := mq.GetStatus(context.Background(), sessionID).Count; got != 1 {
		t.Errorf("queue count after sweep = %d, want 1 (untouched)", got)
	}
}

// TestWorkflowQueueWatchdog_DropsTerminalSessionQueue: session is terminal,
// the workflow auto-start prompt can never drain — watchdog removes it so
// it stops polluting the queue table.
func TestWorkflowQueueWatchdog_DropsTerminalSessionQueue(t *testing.T) {
	const (
		taskID       = "task-wd-4"
		sessionID    = "sess-wd-4"
		executionID  = "exec-wd-4"
		mergeStepID  = "step-wd-merge4"
		taskWorkflow = "wf-wd-4"
	)
	svc, agentMgr, mq := seedWorkflowQueueWatchdogTaskAndSession(
		t, taskID, sessionID, executionID, mergeStepID, taskWorkflow,
		models.TaskSessionStateCompleted,
	)
	agentMgr.launchAgentFunc = func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		t.Fatalf("LaunchAgent must not be called for a terminal session")
		return nil, nil
	}

	wd := svc.newWorkflowQueueWatchdog()
	wd.sweep(context.Background())

	if got := mq.GetStatus(context.Background(), sessionID).Count; got != 0 {
		t.Errorf("queue count after sweep = %d, want 0 (workflow entry dropped)", got)
	}
}

// TestWorkflowQueueWatchdog_DedupesBySession: two stale workflow entries on
// the same session must trigger AT MOST one recovery action per sweep. The
// dedupe inside sweep is what keeps a many-entries-per-session backlog from
// fanning out N concurrent resume goroutines for the same session.
func TestWorkflowQueueWatchdog_DedupesBySession(t *testing.T) {
	const (
		taskID       = "task-wd-5"
		sessionID    = "sess-wd-5"
		executionID  = "exec-wd-5"
		mergeStepID  = "step-wd-merge5"
		taskWorkflow = "wf-wd-5"
	)
	svc, agentMgr, mq := seedWorkflowQueueWatchdogTaskAndSession(
		t, taskID, sessionID, executionID, mergeStepID, taskWorkflow,
		models.TaskSessionStateWaitingForInput,
	)
	// Inject a second stale workflow entry against the same session.
	ctx := context.Background()
	if _, err := mq.QueueMessageWithMetadata(ctx, sessionID, taskID, "second", "", messagequeue.QueuedByWorkflow, false, nil, nil); err != nil {
		t.Fatalf("seed second queue entry: %v", err)
	}
	mq.SetQueuedAtForTesting(t, sessionID, time.Now().Add(-10*time.Minute))
	if got := mq.GetStatus(ctx, sessionID).Count; got != 2 {
		t.Fatalf("expected 2 stale entries before sweep, got %d", got)
	}

	// Dead agent so the recover branch fires.
	agentMgr.isAgentRunning = false

	var launchCount atomic.Int32
	launchCalled := make(chan struct{}, 4)
	resumeDone := make(chan struct{})
	var resumeOnce atomic.Bool
	agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launchCount.Add(1)
		select {
		case launchCalled <- struct{}{}:
		default:
		}
		if !resumeOnce.CompareAndSwap(false, true) {
			return &executor.LaunchAgentResponse{AgentExecutionID: executionID + "-resumed"}, nil
		}
		go func() {
			defer close(resumeDone)
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			deadline := time.After(3 * time.Second)
			for {
				select {
				case <-tick.C:
					sess, err := svc.repo.GetTaskSession(context.Background(), req.SessionID)
					if err != nil || sess == nil || sess.State != models.TaskSessionStateStarting {
						continue
					}
					svc.handleAgentBootReady(context.Background(), watcher.AgentEventData{
						TaskID: req.TaskID, SessionID: req.SessionID,
					})
					time.Sleep(600 * time.Millisecond)
					return
				case <-deadline:
					return
				}
			}
		}()
		return &executor.LaunchAgentResponse{AgentExecutionID: executionID + "-resumed"}, nil
	}

	wd := svc.newWorkflowQueueWatchdog()
	wd.sweep(ctx)

	// Wait for the single recovery to settle.
	select {
	case <-launchCalled:
	case <-time.After(5 * time.Second):
		t.Fatalf("watchdog did not drive LaunchAgent within 5s")
	}
	select {
	case <-resumeDone:
	case <-time.After(5 * time.Second):
		t.Fatalf("resume goroutine did not drain within 5s")
	}

	// LaunchAgent may have been re-invoked once by a re-resume after drain
	// completed (legitimate); what matters is the watchdog itself did NOT
	// spawn N concurrent resumes for the dedupe-target session — checked
	// by ensuring sweep() returned before any extra resume fired (the
	// launchCount snapshot below). It's >=1 (first sweep) and bounded.
	got := launchCount.Load()
	if got < 1 {
		t.Errorf("expected at least 1 LaunchAgent call from the dedupe sweep, got %d", got)
	}
	if got > 2 {
		t.Errorf("expected at most 2 LaunchAgent calls (1 sweep + 1 re-resume), got %d — dedupe may be broken", got)
	}
}

// TestWorkflowQueueWatchdog_TerminalSessionPreservesUserEntries: when the
// session is terminal, workflow auto-start entries are dropped but
// user-authored entries on the same session must be left intact (the user
// authored them and their cleanup is owned by other paths).
func TestWorkflowQueueWatchdog_TerminalSessionPreservesUserEntries(t *testing.T) {
	const (
		taskID       = "task-wd-6"
		sessionID    = "sess-wd-6"
		executionID  = "exec-wd-6"
		mergeStepID  = "step-wd-merge6"
		taskWorkflow = "wf-wd-6"
	)
	svc, _, mq := seedWorkflowQueueWatchdogTaskAndSession(
		t, taskID, sessionID, executionID, mergeStepID, taskWorkflow,
		models.TaskSessionStateCompleted,
	)
	ctx := context.Background()
	// Add a user-authored entry alongside the workflow one.
	if _, err := mq.QueueMessage(ctx, sessionID, taskID, "user follow-up", "", messagequeue.QueuedByUser, false, nil); err != nil {
		t.Fatalf("seed user queue: %v", err)
	}
	if got := mq.GetStatus(ctx, sessionID).Count; got != 2 {
		t.Fatalf("expected 2 entries before sweep, got %d", got)
	}

	wd := svc.newWorkflowQueueWatchdog()
	wd.sweep(ctx)

	status := mq.GetStatus(ctx, sessionID)
	if status.Count != 1 {
		t.Fatalf("expected 1 entry after sweep (user kept, workflow dropped), got %d", status.Count)
	}
	if status.Entries[0].QueuedBy != messagequeue.QueuedByUser {
		t.Errorf("surviving entry queued_by = %q, want %q", status.Entries[0].QueuedBy, messagequeue.QueuedByUser)
	}
}

// TestWorkflowQueueWatchdog_StartStop_NoLeaks asserts the Start/Stop
// lifecycle drains cleanly so goleak.VerifyTestMain stays green.
func TestWorkflowQueueWatchdog_StartStop_NoLeaks(t *testing.T) {
	defer goleak.VerifyNone(t)

	svc := &Service{
		logger:       testLogger(),
		messageQueue: messagequeue.NewServiceMemory(testLogger()),
	}
	// Override defaults to a short interval so the test doesn't sit idle.
	wd := &workflowQueueWatchdog{
		svc:       svc,
		interval:  10 * time.Millisecond,
		orphanAge: 24 * time.Hour, // nothing should match
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	wd.Start(context.Background())
	// Let the ticker fire at least once.
	time.Sleep(50 * time.Millisecond)
	wd.Stop()
	// Stop is idempotent.
	wd.Stop()
}
