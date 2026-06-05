package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestAutoStartTransientError_BootReadyDrainsOrphanedQueue is the regression
// test for the production stuck-queue symptom on task 9378f7cf.
//
// Setup mirrors the production scenario verbatim:
//   - Step "Fixup" has auto_start_agent + on_turn_complete=move_to_next.
//   - Step "Merge" has only auto_start_agent (no on_turn_complete).
//   - Task at Fixup, session.State=RUNNING (Fixup turn in flight).
//   - The underlying agent stream drops mid-prompt — PromptAgent returns
//     "agent stream disconnected", which isTransientPromptError matches.
//
// What happens without the boot-ready drain (the bug):
//  1. handleAgentReady → applyEngineTransition flips state, transitions
//     Fixup→Merge, spawns processOnEnter(Merge) in a goroutine.
//  2. processOnEnter → autoStartStepPrompt:
//     a. recordAutoStartMessage records the Merge prompt as a user msg.
//     b. PromptTask → executor.Prompt → PromptAgent returns the transient
//     error after ~5 s in production.
//     c. handlePromptError reverts state + completeTurnForSession (this
//     is the 5-second "ghost turn" observed in the production DB).
//     d. autoStartStepPrompt's retry loop matches isTransientPromptError
//     and queues the same Merge prompt with queued_by=workflow.
//  3. The queue sits forever because:
//     - The agent never produced an agent.ready for this prompt (the
//     stream dropped first), so handleAgentReady's drain never fires.
//     - The user manually resumes the session → handleAgentBootReady
//     fires → BEFORE the fix, that handler only flipped state and
//     returned, leaving the queue orphaned.
//
// With the boot-ready drain fix in handleAgentBootReady, step 3 drains the
// queue when the session is resumed — the test asserts both halves end-to-end:
// the duplicate is created on transient failure, then boot_ready drains it.
func TestAutoStartTransientError_BootReadyDrainsOrphanedQueue(t *testing.T) {
	ctx := context.Background()
	const (
		taskID       = "task-1"
		sessionID    = "sess-1"
		executionID  = "exec-1"
		fixupStepID  = "step-fixup"
		mergeStepID  = "step-merge"
		profile      = "profile-impl"
		taskWorkflow = "wf1"
	)

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: taskWorkflow, WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps[fixupStepID] = &wfmodels.WorkflowStep{
		ID: fixupStepID, WorkflowID: taskWorkflow, Name: "Fixup after PR", Position: 1,
		AgentProfileID: profile,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": mergeStepID}},
			},
		},
	}
	stepGetter.steps[mergeStepID] = &wfmodels.WorkflowStep{
		ID: mergeStepID, WorkflowID: taskWorkflow, Name: "Merge", Position: 2,
		AgentProfileID: profile,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkflowID: taskWorkflow, WorkflowStepID: fixupStepID,
		Title: "Test", Description: "Test task",
		State:     v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:               sessionID,
		TaskID:           taskID,
		AgentProfileID:   profile,
		AgentExecutionID: executionID,
		State:            models.TaskSessionStateRunning, // mid-turn for Fixup
		IsPrimary:        true,
		StartedAt:        now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkflowID: taskWorkflow, State: v1.TaskStateInProgress,
	}

	// Stream-disconnected error matches isTransientPromptError → autoStartStepPrompt
	// records the user msg, then queues on the retry path. promptDone closes on
	// the first PromptAgent call so the test can sync on it without time.Sleep.
	firstPromptCalled := make(chan struct{})
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true, // skip ensureSessionRunning's resume
		promptErr:              errors.New("agent stream disconnected mid-prompt"),
		promptDone:             firstPromptCalled,
	}

	msgCreator := &mockMessageCreator{}

	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc := &Service{
		logger:         testLogger(),
		repo:           repo,
		taskRepo:       taskRepo,
		agentManager:   agentMgr,
		messageQueue:   messagequeue.NewServiceMemory(testLogger()),
		executor:       exec,
		messageCreator: msgCreator,
	}
	svc.SetWorkflowStepGetter(stepGetter)

	// --- Phase 1: agent.ready triggers the transient-failure auto-start path ---

	doneFired := make(chan struct{})
	go func() {
		defer close(doneFired)
		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
			AgentProfileID:   profile,
		})
	}()
	select {
	case <-doneFired:
	case <-time.After(3 * time.Second):
		t.Fatalf("handleAgentReady did not return within 3s")
	}

	// Sync on PromptAgent's first call (closes firstPromptCalled). After that,
	// the goroutine returns from PromptAgent and runs handlePromptError +
	// autoStartStepPrompt's queue branch — a fully synchronous sequence that
	// ends with messageQueue.QueueMessageWithMetadata. A tight poll covers
	// the few-microsecond gap between PromptAgent returning and the queue
	// row being committed (an unbuffered channel isn't available there).
	select {
	case <-firstPromptCalled:
	case <-time.After(3 * time.Second):
		t.Fatalf("PromptAgent was not called within 3s")
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if svc.messageQueue.GetStatus(ctx, sessionID).Count > 0 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Verify the workflow transitioned to Merge.
	updatedTask, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if updatedTask.WorkflowStepID != mergeStepID {
		t.Errorf("task step = %q, want %q", updatedTask.WorkflowStepID, mergeStepID)
	}

	// The transient-failure path records the user msg AND queues the prompt
	// — both for the same Merge auto-start. This is the production data shape
	// (one chat row + one queue entry, both with workflow_step_name=Merge).
	queueStatus := svc.messageQueue.GetStatus(ctx, sessionID)
	if queueStatus.Count != 1 {
		t.Fatalf("expected exactly 1 queued message after transient failure, got %d", queueStatus.Count)
	}
	queued := queueStatus.Entries[0]
	if queued.QueuedBy != messagequeue.QueuedByWorkflow {
		t.Errorf("queued_by = %q, want %q", queued.QueuedBy, messagequeue.QueuedByWorkflow)
	}
	if name, _ := queued.Metadata["workflow_step_name"].(string); name != "Merge" {
		t.Errorf("queued metadata workflow_step_name = %q, want Merge", name)
	}
	mergeUserMsgs := 0
	for _, m := range msgCreator.userMessages {
		if name, _ := m.metadata["workflow_step_name"].(string); name == "Merge" {
			mergeUserMsgs++
		}
	}
	if mergeUserMsgs != 1 {
		t.Errorf("expected exactly 1 Merge user_message (from recordAutoStartMessage before PromptTask failed), got %d", mergeUserMsgs)
	}

	// --- Phase 2: user resumes the session — boot_ready must drain the queue ---

	// Clear the transient error so the drain's executeQueuedMessage → PromptTask
	// → PromptAgent succeeds. Otherwise the goroutine that runs after the queue
	// take would re-enter the same transient-retry path and re-queue the message
	// immediately, racing the queue-empty assertion below. Also swap in a fresh
	// promptDone channel to signal the second PromptAgent call (the original one
	// was already closed by Phase 1).
	secondPromptCalled := make(chan struct{})
	agentMgr.mu.Lock()
	agentMgr.promptErr = nil
	agentMgr.promptDone = secondPromptCalled
	// Reset capturedPrompts' bookkeeping so the channel re-fires; the mock's
	// `first := len(m.capturedPrompts) == 0` guard would otherwise skip the close.
	agentMgr.capturedPrompts = agentMgr.capturedPrompts[:0]
	agentMgr.capturedPromptCalls = agentMgr.capturedPromptCalls[:0]
	agentMgr.mu.Unlock()

	// Simulate the resume: agentctl's ACP session has re-initialized so the
	// lifecycle manager fires events.AgentBootReady. Without the drain fix,
	// the queue stays orphaned. With the fix, it gets drained.
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:    taskID,
		SessionID: sessionID,
	})

	// drainQueuedMessageForPromptableSession pops the queue synchronously and fires
	// `go executeQueuedMessage(...)`. With the user_message_recorded flag set
	// at queue time (see autoStartStepPrompt's retry branch), the goroutine
	// SKIPS its CreateUserMessage and just calls PromptTask → PromptAgent.
	// The mock closes secondPromptCalled on that call, so the assertion below
	// has a deterministic sync point.
	select {
	case <-secondPromptCalled:
	case <-time.After(3 * time.Second):
		t.Fatalf("boot_ready drain did not reach PromptAgent within 3s")
	}

	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
		t.Errorf("queue not drained after boot_ready: %d messages still queued (the orphaned-queue bug is back)", got)
	}

	// After the drain: exactly ONE Merge user message must exist. Phase 1
	// inserted it via recordAutoStartMessage; Phase 2 (executeQueuedMessage)
	// must not double-insert. Without the user_message_recorded flag, this
	// would be 2 — the symptom reported on the ACP-removal task.
	mergeUserMsgsAfterDrain := 0
	for _, m := range msgCreator.userMessages {
		if name, _ := m.metadata["workflow_step_name"].(string); name == "Merge" {
			mergeUserMsgsAfterDrain++
		}
	}
	if mergeUserMsgsAfterDrain != 1 {
		t.Errorf("expected exactly 1 Merge user_message after boot_ready drain, got %d (duplicate-prompt bug is back)", mergeUserMsgsAfterDrain)
	}
}

// TestAutoStartTransientError_AutoResumesWhenAgentDead is the regression test
// for the silent-idle production symptom (task 648ca65e: 81-minute stuck queue).
//
// When PromptAgent returns a transient disconnect error, autoStartStepPrompt
// queues the Merge prompt with queued_by=workflow. Before this fix, the queue
// sat orphaned forever — there was no live agent process to fire agent.ready
// (which is what drains the queue normally), and no code re-launched the agent.
//
// scheduleAutoResumeForWorkflowQueue (added in queueAutoStartPrompt) fixes this:
// it fires tryEnsureExecution in the background when the agent is dead, driving
// ResumeSession → agent.boot_ready → handleAgentBootReady → drain. This test
// asserts that full chain end-to-end.
func TestAutoStartTransientError_AutoResumesWhenAgentDead(t *testing.T) {
	ctx := context.Background()
	const (
		taskID       = "task-ar-1"
		sessionID    = "sess-ar-1"
		executionID  = "exec-ar-1"
		fixupStepID  = "step-ar-fixup"
		mergeStepID  = "step-ar-merge"
		profile      = "profile-ar-impl"
		taskWorkflow = "wf-ar-1"
	)

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-ar", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: taskWorkflow, WorkspaceID: "ws-ar", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.steps[fixupStepID] = &wfmodels.WorkflowStep{
		ID: fixupStepID, WorkflowID: taskWorkflow, Name: "Fixup after PR", Position: 1,
		AgentProfileID: profile,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": mergeStepID}},
			},
		},
	}
	stepGetter.steps[mergeStepID] = &wfmodels.WorkflowStep{
		ID: mergeStepID, WorkflowID: taskWorkflow, Name: "Merge", Position: 2,
		AgentProfileID: profile,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkflowID: taskWorkflow, WorkflowStepID: fixupStepID,
		Title: "Test", Description: "Test task",
		State:     v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:               sessionID,
		TaskID:           taskID,
		AgentProfileID:   profile,
		AgentExecutionID: executionID,
		State:            models.TaskSessionStateRunning,
		IsPrimary:        true,
		StartedAt:        now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, executionID)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{
		ID: taskID, WorkflowID: taskWorkflow, State: v1.TaskStateInProgress,
	}

	// firstPromptCalled closes on the first PromptAgent call (the transient failure).
	// launchCalled closes when auto-resume fires LaunchAgent.
	// secondPromptCalled closes on the second PromptAgent call (queue drain after resume).
	firstPromptCalled := make(chan struct{})
	launchCalled := make(chan struct{})
	secondPromptCalled := make(chan struct{})
	// resumeDone is closed after handleAgentBootReady writes WAITING_FOR_INPUT and
	// waitForSessionReady has had one full poll cycle (500ms) to observe it — this
	// ensures the tryEnsureExecution goroutine exits before goleak runs.
	resumeDone := make(chan struct{})
	var agentResumed atomic.Bool

	// var svc declared here so the launchAgentFunc closure can capture it. By the
	// time LaunchAgent is called, svc is already constructed below.
	var svc *Service

	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		promptErr:              errors.New("agent stream disconnected mid-prompt"),
		promptDone:             firstPromptCalled,
	}

	// isAgentRunningFn models the production state transition:
	//   alive before the first PromptAgent → dead after the stream disconnect →
	//   alive again after LaunchAgent resumes the agent.
	agentMgr.isAgentRunningFn = func(_ context.Context, _ string) bool {
		if agentResumed.Load() {
			return true
		}
		select {
		case <-firstPromptCalled:
			return false // agent died after the first prompt attempt
		default:
			return true // agent alive before the first prompt
		}
	}

	// launchAgentFunc simulates a successful agent re-launch. It:
	//   1. Marks the agent as running (so subsequent IsAgentRunningForSession calls
	//      return true and PromptTask's ensureSessionRunning is a no-op).
	//   2. Signals launchCalled so the test can assert the resume path was taken.
	//   3. Spawns a goroutine that polls for STARTING (written by persistResumeState
	//      immediately after LaunchAgent returns), then calls handleAgentBootReady —
	//      mirroring the production AgentBootReady lifecycle event — which sets the
	//      session to WAITING_FOR_INPUT (unblocking waitForSessionReady) and drains
	//      the queue.
	//   4. Sleeps 600ms after handleAgentBootReady returns so that waitForSessionReady
	//      (which polls every 500ms) has at least one full cycle to pick up
	//      WAITING_FOR_INPUT and exit — preventing a goroutine leak caught by goleak.
	agentMgr.launchAgentFunc = func(lctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		agentResumed.Store(true) // mark alive before anything polls for it
		close(launchCalled)
		go func() {
			defer close(resumeDone)
			tick := time.NewTicker(5 * time.Millisecond)
			defer tick.Stop()
			timeout := time.After(5 * time.Second)
			for {
				select {
				case <-tick.C:
					sess, err := repo.GetTaskSession(context.Background(), sessionID)
					if err != nil || sess == nil || sess.State != models.TaskSessionStateStarting {
						continue
					}
					// Clear promptErr and swap in the second channel before handleAgentBootReady
					// triggers the drain — executeQueuedMessage must see nil error.
					agentMgr.mu.Lock()
					agentMgr.promptErr = nil
					agentMgr.promptDone = secondPromptCalled
					agentMgr.capturedPrompts = agentMgr.capturedPrompts[:0]
					agentMgr.capturedPromptCalls = agentMgr.capturedPromptCalls[:0]
					agentMgr.mu.Unlock()
					svc.handleAgentBootReady(context.Background(), watcher.AgentEventData{
						TaskID:    req.TaskID,
						SessionID: req.SessionID,
					})
					// handleAgentBootReady wrote WAITING_FOR_INPUT to DB.
					// waitForSessionReady polls every 500ms — sleep one full cycle so
					// it exits before goleak checks for leaked goroutines.
					time.Sleep(600 * time.Millisecond)
					return
				case <-timeout:
					return
				}
			}
		}()
		return &executor.LaunchAgentResponse{AgentExecutionID: "exec-ar-resumed"}, nil
	}

	exec := executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	msgCreator := &mockMessageCreator{}
	svc = &Service{
		logger:         testLogger(),
		repo:           repo,
		taskRepo:       taskRepo,
		agentManager:   agentMgr,
		messageQueue:   messagequeue.NewServiceMemory(testLogger()),
		executor:       exec,
		messageCreator: msgCreator,
	}
	svc.SetWorkflowStepGetter(stepGetter)

	// --- Phase 1: agent.ready → transient failure → workflow prompt queued ---
	// handleAgentReady fires the Fixup on_turn_complete → transitions to Merge →
	// processOnEnter(Merge) → autoStartStepPrompt → PromptAgent returns transient
	// error → queueAutoStartPrompt → scheduleAutoResumeForWorkflowQueue fires.

	doneFired := make(chan struct{})
	go func() {
		defer close(doneFired)
		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID:           taskID,
			SessionID:        sessionID,
			AgentExecutionID: executionID,
			AgentProfileID:   profile,
		})
	}()
	select {
	case <-doneFired:
	case <-time.After(3 * time.Second):
		t.Fatalf("handleAgentReady did not return within 3s")
	}

	select {
	case <-firstPromptCalled:
	case <-time.After(3 * time.Second):
		t.Fatalf("PromptAgent was not called within 3s")
	}
	// Poll for the queue entry (tiny gap between PromptAgent returning and the
	// queue write completing — both are synchronous in the processOnEnter goroutine).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if svc.messageQueue.GetStatus(ctx, sessionID).Count > 0 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 1 {
		t.Fatalf("expected 1 queued message after transient failure, got %d", got)
	}
	queued := svc.messageQueue.GetStatus(ctx, sessionID).Entries[0]
	if queued.QueuedBy != messagequeue.QueuedByWorkflow {
		t.Errorf("queued_by = %q, want %q", queued.QueuedBy, messagequeue.QueuedByWorkflow)
	}

	// --- Phase 2: auto-resume ---
	// scheduleAutoResumeForWorkflowQueue (called from queueAutoStartPrompt) fires
	// tryEnsureExecution in the background. Assert LaunchAgent was called, then
	// wait for the drain to reach the second PromptAgent call.

	select {
	case <-launchCalled:
	case <-time.After(5 * time.Second):
		t.Fatalf("scheduleAutoResumeForWorkflowQueue did not trigger LaunchAgent within 5s (auto-resume bug?)")
	}

	// Wait for secondPromptCalled (drain reached PromptAgent) and resumeDone
	// (waitForSessionReady exited) concurrently — both must fire before we assert.
	select {
	case <-secondPromptCalled:
	case <-time.After(5 * time.Second):
		t.Fatalf("queue was not drained after auto-resume (no second PromptAgent within 5s)")
	}
	select {
	case <-resumeDone:
	case <-time.After(3 * time.Second):
		t.Fatalf("tryEnsureExecution goroutine did not complete within 3s")
	}

	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
		t.Errorf("queue not empty after auto-resume drain: %d messages remain", got)
	}

	// Exactly one Merge user message must exist — no duplicate.
	mergeUserMsgs := 0
	for _, m := range msgCreator.userMessages {
		if name, _ := m.metadata["workflow_step_name"].(string); name == "Merge" {
			mergeUserMsgs++
		}
	}
	if mergeUserMsgs != 1 {
		t.Errorf("expected exactly 1 Merge user_message, got %d", mergeUserMsgs)
	}
}
