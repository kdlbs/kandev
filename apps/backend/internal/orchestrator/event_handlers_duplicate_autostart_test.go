package orchestrator

import (
	"context"
	"errors"
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
	// records the user msg, then queues on the retry path.
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true, // skip ensureSessionRunning's resume
		promptErr:              errors.New("agent stream disconnected mid-prompt"),
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

	// Wait for the async processOnEnter goroutine to record the user msg and
	// queue the prompt. The transient-error path queues on the first attempt
	// (no backoff sleeps), so this is fast.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if svc.messageQueue.GetStatus(ctx, sessionID).Count > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
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
	// immediately, racing the queue-empty assertion below.
	agentMgr.mu.Lock()
	agentMgr.promptErr = nil
	priorPromptCount := len(agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()

	// Simulate the resume: agentctl's ACP session has re-initialized so the
	// lifecycle manager fires events.AgentBootReady. Without the drain fix,
	// the queue stays orphaned. With the fix, it gets drained.
	svc.handleAgentBootReady(ctx, watcher.AgentEventData{
		TaskID:    taskID,
		SessionID: sessionID,
	})

	// drainQueuedMessageAfterTransition pops the queue synchronously and fires
	// `go executeQueuedMessage(...)`. The goroutine creates a user message and
	// calls PromptTask → PromptAgent. Wait for that to land, then assert the
	// agent received the prompt.
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		agentMgr.mu.Lock()
		got := len(agentMgr.capturedPrompts)
		agentMgr.mu.Unlock()
		if got > priorPromptCount {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	agentMgr.mu.Lock()
	postPromptCount := len(agentMgr.capturedPrompts)
	agentMgr.mu.Unlock()
	if postPromptCount <= priorPromptCount {
		t.Fatalf("boot_ready drain did not dispatch the queued prompt: PromptAgent calls before=%d after=%d", priorPromptCount, postPromptCount)
	}

	if got := svc.messageQueue.GetStatus(ctx, sessionID).Count; got != 0 {
		t.Errorf("queue not drained after boot_ready: %d messages still queued (the orphaned-queue bug is back)", got)
	}
}
