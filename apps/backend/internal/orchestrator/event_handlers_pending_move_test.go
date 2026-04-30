package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestPendingMove_ReviewToInProgress_OneTransitionOnly reproduces the production bug
// observed at task a99d863e ("buggy fibo"): a QA agent calls move_task_kandev to send
// the task back to "In Progress" with a hand-off prompt, but the deferred-move flow
// triggers spurious additional transitions and the task ends up at "Reviewed" instead.
//
// Workflow (simplified, matches the user's actual setup):
//
//	[In Progress] --on_turn_complete-->  [In Review] --on_turn_complete-->  [Reviewed]
//	  on_enter: auto_start_agent              on_enter: auto_start_agent
//	  profile-impl                            profile-review
//
// Both on_turn_complete rules are unconditional — any agent.ready event triggers
// a transition. That's the workflow author's choice, but the orchestrator must
// not feed it spurious ready events. The deferred-move feature must produce
// exactly one transition: "In Review" → "In Progress". Anything else (e.g.
// "In Progress" → "In Review", or worse, "In Review" → "Reviewed" via a stale
// ready) is the bug.
//
// Scenario the test sets up:
//   - Task is currently at "In Review" (the QA step).
//   - Two sessions exist: an "In Progress" session (profile-impl, completed earlier
//     when the workflow first transitioned to Review) and an "In Review" session
//     (profile-review, currently RUNNING, primary).
//   - QA called move_task_kandev mid-turn → handleMoveTask set a PendingMove
//     pointing at "In Progress" and queued the hand-off prompt.
//   - QA's turn ends → agent.ready fires → handleAgentReady is invoked.
//
// Expected outcome:
//   - Task workflow_step_id == "In Progress" step ID.
//   - The "In Progress" session is the primary (revived from COMPLETED).
//   - The "In Review" session is COMPLETED.
//   - No subsequent transition fires.
//
// The test deliberately stubs PromptAgent / LaunchAgent so we don't need a real
// agent process. The bug we're chasing is in the orchestrator's transition
// logic, not in the executor — so an executor that returns success deterministically
// is sufficient to expose multiple transitions if they occur.
func TestPendingMove_ReviewToInProgress_OneTransitionOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)

	// Workspace + workflow scaffolding.
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// Build the three workflow steps. Both auto_start steps have UNCONDITIONAL
	// on_turn_complete rules, mirroring the user's bug repro workflow.
	const (
		stepInProgressID = "step-in-progress"
		stepInReviewID   = "step-in-review"
		stepReviewedID   = "step-reviewed"

		profileImpl   = "profile-impl"
		profileReview = "profile-review"
	)
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepInProgressID] = &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1,
		AgentProfileID: profileImpl,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepInReviewID}},
			},
		},
	}
	stepGetter.steps[stepInReviewID] = &wfmodels.WorkflowStep{
		ID: stepInReviewID, WorkflowID: "wf1", Name: "In Review", Position: 2,
		AgentProfileID: profileReview,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepReviewedID}},
			},
		},
	}
	stepGetter.steps[stepReviewedID] = &wfmodels.WorkflowStep{
		ID: stepReviewedID, WorkflowID: "wf1", Name: "Reviewed", Position: 3,
	}

	// Task at the "In Review" step.
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkflowID: "wf1", WorkflowStepID: stepInReviewID,
		Title: "Test", Description: "Implement a python buggy fibonnacci",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// "In Progress" session — completed earlier when the workflow first
	// transitioned away from In Progress to In Review. Has an executors_running
	// record (so reuseSessionForStep will revive it as WAITING_FOR_INPUT, not
	// CREATED — matching the real-world scenario where this session has been
	// launched before and has a resume token).
	completedAt := now.Add(-1 * time.Minute)
	implSession := &models.TaskSession{
		ID:                "session-impl",
		TaskID:            "task-1",
		AgentProfileID:    profileImpl,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-impl-original",
		State:             models.TaskSessionStateCompleted,
		IsPrimary:         false,
		CompletedAt:       &completedAt,
		StartedAt:         now.Add(-2 * time.Minute),
		UpdatedAt:         completedAt,
	}
	if err := repo.CreateTaskSession(ctx, implSession); err != nil {
		t.Fatalf("create impl session: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "session-impl", SessionID: "session-impl", TaskID: "task-1",
		ResumeToken: "resume-token-impl", AgentExecutionID: "ae-impl-original",
		CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: completedAt,
	}); err != nil {
		t.Fatalf("upsert executors_running for impl: %v", err)
	}

	// "In Review" session — currently active, primary, RUNNING. The QA agent
	// is mid-turn; it just called move_task_kandev which set the PendingMove.
	reviewSession := &models.TaskSession{
		ID:                "session-review",
		TaskID:            "task-1",
		AgentProfileID:    profileReview,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-review",
		State:             models.TaskSessionStateRunning,
		IsPrimary:         true,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.CreateTaskSession(ctx, reviewSession); err != nil {
		t.Fatalf("create review session: %v", err)
	}

	// Build the orchestrator service. We use the real repo + workflow engine
	// (so transitions actually persist) and a mock agent manager that records
	// PromptAgent calls and returns success for LaunchAgent. PromptTask's
	// ensureSessionRunning will see no in-memory execution and call ResumeSession;
	// we make LaunchAgent return a fresh execution ID without firing any events.
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-1"] = &v1.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Implement a python buggy fibonnacci",
		State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})

	// Track every call to processOnTurnCompleteViaEngine so we can assert that
	// no spurious turn-complete evaluation runs after the deferred move applies.
	// (The bug manifests as additional ready events firing on_turn_complete
	// against the new step, ping-ponging the task forward.) Wrapping the method
	// is awkward; instead we count transition writes to the task's
	// workflow_step_id by polling repo state.
	svc := &Service{
		logger:             log,
		repo:               repo,
		workflowStepGetter: stepGetter,
		taskRepo:           taskRepo,
		agentManager:       agentMgr,
		messageQueue:       messagequeue.NewService(log),
		executor:           exec,
		scheduler:          sched,
	}
	svc.SetWorkflowStepGetter(stepGetter)

	// Simulate a real agent boot: when ResumeSession launches the impl agent,
	// fire the boot signal a beat later (after persistResumeState writes
	// state=STARTING). handleAgentBootReady flips state to WAITING_FOR_INPUT —
	// unblocking waitForSessionReady — without ever evaluating on_turn_complete.
	agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		newExecID := "ae-impl-relaunch"
		go func() {
			time.Sleep(50 * time.Millisecond)
			svc.handleAgentBootReady(context.Background(), watcher.AgentEventData{
				TaskID:           req.TaskID,
				SessionID:        req.SessionID,
				AgentExecutionID: newExecID,
				AgentProfileID:   req.AgentProfileID,
			})
		}()
		return &executor.LaunchAgentResponse{
			AgentExecutionID: newExecID,
			ContainerID:      "container-relaunch",
			Status:           v1.AgentStatusReady,
		}, nil
	}

	// Set the PendingMove + queue the hand-off prompt the way handleMoveTask
	// would when the QA agent calls move_task_kandev mid-turn.
	const handoffPrompt = "You were moved to this step with the following message: " +
		"The file fibonacci.py has two bugs — fix them."
	if _, err := svc.messageQueue.QueueMessage(
		ctx, reviewSession.ID, "task-1", handoffPrompt, "", "mcp-move-task", false, nil,
	); err != nil {
		t.Fatalf("queue hand-off prompt: %v", err)
	}
	svc.messageQueue.SetPendingMove(ctx, reviewSession.ID, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	// Snapshot the workflow_step_id history by sampling at intervals. We expect
	// exactly one change: stepInReviewID → stepInProgressID. Anything else
	// (e.g. stepInProgressID → stepInReviewID right after, or skipping ahead
	// to stepReviewedID) means the bug has fired.
	historyDone := make(chan struct{})
	var stepHistory []string
	go func() {
		defer close(historyDone)
		seen := stepInReviewID
		stepHistory = append(stepHistory, seen)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			task, err := repo.GetTask(ctx, "task-1")
			if err == nil && task.WorkflowStepID != seen {
				seen = task.WorkflowStepID
				stepHistory = append(stepHistory, seen)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Fire the QA session's agent.ready — this is what handleAgentReady receives
	// when MarkReady is called from handleCompleteEventMarkState after the QA
	// agent's turn ends.
	svc.handleAgentReady(ctx, watcher.AgentEventData{
		TaskID:           "task-1",
		SessionID:        reviewSession.ID,
		AgentExecutionID: "ae-review",
		AgentProfileID:   profileReview,
	})

	// Give the async processStepExitAndEnter goroutine time to complete.
	// Then drain the history collector.
	time.Sleep(1 * time.Second)
	<-historyDone

	// --- Assertions ---

	finalTask, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}

	dedup := dedupConsecutive(stepHistory)
	t.Logf("workflow_step_id transition history: %v", stepNamesFromIDs(dedup, stepGetter))

	// 1) Task must end at "In Progress" — the step the agent explicitly moved it to.
	if finalTask.WorkflowStepID != stepInProgressID {
		t.Errorf("final workflow_step_id = %q, want %q (In Progress)", finalTask.WorkflowStepID, stepInProgressID)
	}

	// 2) Exactly one transition: In Review → In Progress.
	expected := []string{stepInReviewID, stepInProgressID}
	if !sliceEqual(dedup, expected) {
		t.Errorf("transition history = %v, want %v\n  (this means the deferred-move triggered spurious additional transitions — the bug)",
			stepNamesFromIDs(dedup, stepGetter), stepNamesFromIDs(expected, stepGetter))
	}

	// 3) The In Review session must be COMPLETED.
	rev, err := repo.GetTaskSession(ctx, reviewSession.ID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	if rev.State != models.TaskSessionStateCompleted {
		t.Errorf("review session state = %q, want COMPLETED (it's been parked by the profile switch)", rev.State)
	}
	if rev.IsPrimary {
		t.Error("review session must no longer be primary (the impl session takes over)")
	}

	// 4) The Impl session must be primary again.
	impl, err := repo.GetTaskSession(ctx, implSession.ID)
	if err != nil {
		t.Fatalf("load impl session: %v", err)
	}
	if !impl.IsPrimary {
		t.Error("impl session must be primary after the deferred move applies")
	}
	if impl.State == models.TaskSessionStateCompleted {
		t.Errorf("impl session state = %q, expected non-terminal (revived for a new turn)", impl.State)
	}

	// 5) The hand-off prompt must have been delivered (or be queued for delivery)
	//    on the impl session — not lost, not delivered to the QA session.
	implPrompts := capturedPromptsForSession(agentMgr, "ae-impl-relaunch")
	implQueued := svc.messageQueue.GetStatus(ctx, implSession.ID)
	if len(implPrompts) == 0 && !implQueued.IsQueued {
		t.Error("hand-off prompt was neither delivered to the impl session nor queued for it")
	}
	for _, p := range implPrompts {
		if strings.Contains(p, "fibonacci.py has two bugs") {
			return // delivered — good
		}
	}
	if implQueued.IsQueued && implQueued.Message != nil &&
		strings.Contains(implQueued.Message.Content, "fibonacci.py has two bugs") {
		return // queued for delivery — also acceptable
	}
}

// --- Helpers ---

func capturedPromptsForSession(agentMgr *mockAgentManager, _ string) []string {
	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	out := make([]string, len(agentMgr.capturedPrompts))
	copy(out, agentMgr.capturedPrompts)
	return out
}

func dedupConsecutive(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := []string{in[0]}
	for i := 1; i < len(in); i++ {
		if in[i] != in[i-1] {
			out = append(out, in[i])
		}
	}
	return out
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stepNamesFromIDs(ids []string, sg *mockStepGetter) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if step, ok := sg.steps[id]; ok {
			out = append(out, step.Name)
		} else {
			out = append(out, id)
		}
	}
	return out
}

// TestHandleAgentBootReady_DoesNotTriggerOnTurnComplete locks in the post-fix
// invariant: a boot-ready signal (agent's ACP session has just initialized,
// no turn has run yet) must NEVER step the workflow. The lifecycle layer now
// publishes events.AgentBootReady — distinct from events.AgentReady — and the
// orchestrator routes it to handleAgentBootReady which only flips the session
// to WAITING_FOR_INPUT.
//
// Before this split, both signals shared events.AgentReady and the
// orchestrator tried to disambiguate them with the resumeInProgressSessions
// flag. That flag had a race: when the boot ready arrived BEFORE
// persistResumeState wrote state=STARTING, handleAgentReady's state guard
// returned without consuming the flag, leaking it to the next event and
// firing on_turn_complete against the wrong session.
//
// This test fires the boot signal directly into handleAgentBootReady to
// confirm: (a) no on_turn_complete evaluation runs, (b) the session ends up
// WAITING_FOR_INPUT regardless of what state it was in.
func TestHandleAgentBootReady_DoesNotTriggerOnTurnComplete(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	// One step with an unconditional on_turn_complete. If a boot-ready
	// somehow reaches the turn-end path, this rule fires and the task moves —
	// the user-visible symptom of the original bug.
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-current"] = &wfmodels.WorkflowStep{
		ID: "step-current", WorkflowID: "wf1", Name: "Current", Position: 1,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step-next"}},
			},
		},
	}
	stepGetter.steps["step-next"] = &wfmodels.WorkflowStep{
		ID: "step-next", WorkflowID: "wf1", Name: "Next", Position: 2,
	}

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkflowID: "wf1", WorkflowStepID: "step-current",
		Title: "T", Description: "D", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Two scenarios the boot signal must handle correctly:
	//   - state=STARTING (the textbook case: persistResumeState wrote it)
	//   - state=WAITING_FOR_INPUT (the racy case: boot signal beat
	//     persistResumeState, or reviveReusedSession left it WAITING)
	cases := []struct {
		name     string
		startSt  models.TaskSessionState
		expectSt models.TaskSessionState
	}{
		{"STARTING", models.TaskSessionStateStarting, models.TaskSessionStateWaitingForInput},
		{"WAITING_FOR_INPUT (race-with-persistResumeState)", models.TaskSessionStateWaitingForInput, models.TaskSessionStateWaitingForInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sessionID := "s1-" + tc.name
			if err := repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: "task-1", AgentProfileID: "profile-impl",
				AgentExecutionID: "ae-current",
				State:            tc.startSt,
				IsPrimary:        true,
				StartedAt:        now, UpdatedAt: now,
			}); err != nil {
				t.Fatalf("create session: %v", err)
			}

			taskRepo := newMockTaskRepo()
			taskRepo.tasks["task-1"] = &v1.Task{
				ID: "task-1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
			}

			agentMgr := &mockAgentManager{}
			log := testLogger()
			exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
			svc := &Service{
				logger:             log,
				repo:               repo,
				workflowStepGetter: stepGetter,
				taskRepo:           taskRepo,
				agentManager:       agentMgr,
				messageQueue:       messagequeue.NewService(log),
				executor:           exec,
			}
			svc.SetWorkflowStepGetter(stepGetter)

			// Reset task to step-current in case a prior subtest moved it.
			tk, _ := repo.GetTask(ctx, "task-1")
			tk.WorkflowStepID = "step-current"
			_ = repo.UpdateTask(ctx, tk)

			// Fire the new boot-only event. The handler must NOT run on_turn_complete.
			svc.handleAgentBootReady(ctx, watcher.AgentEventData{
				TaskID: "task-1", SessionID: sessionID,
				AgentExecutionID: "ae-current",
				AgentProfileID:   "profile-impl",
			})

			finalTask, err := repo.GetTask(ctx, "task-1")
			if err != nil {
				t.Fatalf("load task: %v", err)
			}
			if finalTask.WorkflowStepID != "step-current" {
				t.Errorf("workflow_step_id = %q, want %q (boot signal must not move the workflow)",
					finalTask.WorkflowStepID, "step-current")
			}

			finalSess, err := repo.GetTaskSession(ctx, sessionID)
			if err != nil {
				t.Fatalf("load session: %v", err)
			}
			if finalSess.State != tc.expectSt {
				t.Errorf("session.State = %q, want %q", finalSess.State, tc.expectSt)
			}
		})
	}
}
