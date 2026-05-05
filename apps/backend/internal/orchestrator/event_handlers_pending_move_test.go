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
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
	sc := buildPendingMoveScenario(t)

	// Snapshot the workflow_step_id history by sampling at intervals. We expect
	// exactly one change: stepInReviewID → stepInProgressID. Anything else
	// (e.g. stepInProgressID → stepInReviewID right after, or skipping ahead
	// to stepReviewedID) means the bug has fired.
	historyDone, stepHistory := sc.startStepHistorySampler(t, 2*time.Second)

	// Fire the QA session's agent.ready — this is what handleAgentReady receives
	// when MarkReady is called from handleCompleteEventMarkState after the QA
	// agent's turn ends.
	sc.svc.handleAgentReady(sc.ctx, watcher.AgentEventData{
		TaskID:           "task-1",
		SessionID:        sc.reviewSessionID,
		AgentExecutionID: "ae-review",
		AgentProfileID:   profileReview,
	})

	// Give the async processStepExitAndEnter goroutine time to complete.
	// Then drain the history collector.
	time.Sleep(1 * time.Second)
	<-historyDone

	sc.assertOneTransitionToInProgress(t, *stepHistory)
}

// --- Pending-move scenario builder & assertions ---

const (
	stepInProgressID = "step-in-progress"
	stepInReviewID   = "step-in-review"
	stepReviewedID   = "step-reviewed"

	profileImpl   = "profile-impl"
	profileReview = "profile-review"
)

// pendingMoveScenario is the seeded fixture used by deferred-move tests.
// It owns the repo + service + mock agent manager so a single value carries
// every reference an assertion needs without long parameter lists.
type pendingMoveScenario struct {
	ctx              context.Context
	svc              *Service
	repo             *sqliterepo.Repository
	agentMgr         *mockAgentManager
	stepGetter       *mockStepGetter
	implSessionID    string
	reviewSessionID  string
	implRelaunchExec string
}

// buildPendingMoveScenario sets up the full repro scenario:
//   - 3 workflow steps: In Progress (auto_start, on_turn_complete → Review),
//     In Review (auto_start, on_turn_complete → Reviewed), Reviewed (terminal).
//   - Task currently at "In Review", with two sessions: an Impl session that
//     was completed earlier (revivable — has executors_running), and a Review
//     session that's currently RUNNING and primary.
//   - PendingMove + hand-off prompt seeded as if the QA agent just called
//     move_task_kandev mid-turn.
//   - Mock LaunchAgent that fires the boot signal asynchronously so the
//     resume path can complete in tests without a real agent process.
func buildPendingMoveScenario(t *testing.T) *pendingMoveScenario {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	repo := setupTestRepo(t)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Test WF", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	stepGetter := newPendingMoveStepGetter()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkflowID: "wf1", WorkflowStepID: stepInReviewID,
		Title: "Test", Description: "Implement a python buggy fibonnacci",
		State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	implSessionID := seedImplSession(t, repo, now)
	reviewSessionID := seedReviewSession(t, repo, now)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task-1"] = &v1.Task{
		ID: "task-1", WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test", Description: "Implement a python buggy fibonnacci",
		State: v1.TaskStateInProgress,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	log := testLogger()
	exec := executor.NewExecutor(agentMgr, repo, log, executor.ExecutorConfig{})
	sched := scheduler.NewScheduler(queue.NewTaskQueue(100), exec, taskRepo, log, scheduler.SchedulerConfig{})

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

	const implRelaunchExec = "ae-impl-relaunch"
	wireBootReadySimulator(svc, agentMgr, implRelaunchExec)

	const handoffPrompt = "You were moved to this step with the following message: " +
		"The file fibonacci.py has two bugs — fix them."
	if _, err := svc.messageQueue.QueueMessage(
		ctx, reviewSessionID, "task-1", handoffPrompt, "", "mcp-move-task", false, nil,
	); err != nil {
		t.Fatalf("queue hand-off prompt: %v", err)
	}
	svc.messageQueue.SetPendingMove(ctx, reviewSessionID, &messagequeue.PendingMove{
		TaskID:         "task-1",
		WorkflowID:     "wf1",
		WorkflowStepID: stepInProgressID,
	})

	return &pendingMoveScenario{
		ctx:              ctx,
		svc:              svc,
		repo:             repo,
		agentMgr:         agentMgr,
		stepGetter:       stepGetter,
		implSessionID:    implSessionID,
		reviewSessionID:  reviewSessionID,
		implRelaunchExec: implRelaunchExec,
	}
}

// newPendingMoveStepGetter builds the 3-step workflow used by the scenario.
// Both auto_start steps have UNCONDITIONAL on_turn_complete rules — that's
// the workflow shape that exposed the original ping-pong bug, so we keep it.
func newPendingMoveStepGetter() *mockStepGetter {
	sg := newMockStepGetter()
	sg.steps[stepInProgressID] = &wfmodels.WorkflowStep{
		ID: stepInProgressID, WorkflowID: "wf1", Name: "In Progress", Position: 1,
		AgentProfileID: profileImpl,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepInReviewID}},
			},
		},
	}
	sg.steps[stepInReviewID] = &wfmodels.WorkflowStep{
		ID: stepInReviewID, WorkflowID: "wf1", Name: "In Review", Position: 2,
		AgentProfileID: profileReview,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepReviewedID}},
			},
		},
	}
	sg.steps[stepReviewedID] = &wfmodels.WorkflowStep{
		ID: stepReviewedID, WorkflowID: "wf1", Name: "Reviewed", Position: 3,
	}
	return sg
}

// seedImplSession seeds the previously-completed Impl session with an
// executors_running record so reuseSessionForStep revives it as
// WAITING_FOR_INPUT (matching the real-world "previously launched" path).
func seedImplSession(t *testing.T, repo *sqliterepo.Repository, now time.Time) string {
	t.Helper()
	const id = "session-impl"
	completedAt := now.Add(-1 * time.Minute)
	sess := &models.TaskSession{
		ID:                id,
		TaskID:            "task-1",
		AgentProfileID:    profileImpl,
		ExecutorID:        "exec-local",
		ExecutorProfileID: "ep1",
		AgentExecutionID:  "ae-impl-original",
		State:             models.TaskSessionStateCompleted,
		CompletedAt:       &completedAt,
		StartedAt:         now.Add(-2 * time.Minute),
		UpdatedAt:         completedAt,
	}
	if err := repo.CreateTaskSession(context.Background(), sess); err != nil {
		t.Fatalf("create impl session: %v", err)
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: id, SessionID: id, TaskID: "task-1",
		ResumeToken: "resume-token-impl", AgentExecutionID: "ae-impl-original",
		CreatedAt: now.Add(-2 * time.Minute), UpdatedAt: completedAt,
	}); err != nil {
		t.Fatalf("upsert executors_running for impl: %v", err)
	}
	return id
}

// seedReviewSession seeds the currently-active Review session as primary,
// RUNNING — the QA agent that's about to fire its move_task_kandev call.
func seedReviewSession(t *testing.T, repo *sqliterepo.Repository, now time.Time) string {
	t.Helper()
	const id = "session-review"
	sess := &models.TaskSession{
		ID:                id,
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
	if err := repo.CreateTaskSession(context.Background(), sess); err != nil {
		t.Fatalf("create review session: %v", err)
	}
	return id
}

// wireBootReadySimulator stubs LaunchAgent to fire handleAgentBootReady ~50ms
// after returning. Real agentctl bootstrap publishes events.AgentBootReady from
// outside the LaunchAgent call; mirroring that timing here lets the resume
// path complete in unit tests without spawning a real subprocess.
func wireBootReadySimulator(svc *Service, agentMgr *mockAgentManager, newExecID string) {
	agentMgr.launchAgentFunc = func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		// Simulate the lifecycle manager's persistExecutorRunning: in production
		// the row is upserted in lockstep with executionStore.Add; here we mirror
		// that timing so the orchestrator's GetExecutionIDForSession lookup
		// resolves to the new exec id immediately.
		if svc != nil && svc.repo != nil {
			_ = svc.repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID:               req.SessionID,
				SessionID:        req.SessionID,
				TaskID:           req.TaskID,
				AgentExecutionID: newExecID,
				ContainerID:      "container-relaunch",
				Status:           "starting",
			})
		}
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
}

// startStepHistorySampler polls task.WorkflowStepID and appends every change
// to a slice. Returned channel closes when sampling ends; the *[]string is
// safe to read from the caller after that.
func (sc *pendingMoveScenario) startStepHistorySampler(t *testing.T, duration time.Duration) (<-chan struct{}, *[]string) {
	t.Helper()
	done := make(chan struct{})
	history := []string{stepInReviewID}
	go func() {
		defer close(done)
		seen := stepInReviewID
		deadline := time.Now().Add(duration)
		for time.Now().Before(deadline) {
			task, err := sc.repo.GetTask(sc.ctx, "task-1")
			if err == nil && task.WorkflowStepID != seen {
				seen = task.WorkflowStepID
				history = append(history, seen)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	return done, &history
}

// assertOneTransitionToInProgress checks every postcondition of the scenario:
// task moved to In Progress, exactly one transition, sessions in the right
// state, and the hand-off prompt landed on the impl session (delivered or
// queued — either is acceptable for this regression).
func (sc *pendingMoveScenario) assertOneTransitionToInProgress(t *testing.T, stepHistory []string) {
	t.Helper()

	finalTask, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}

	dedup := dedupConsecutive(stepHistory)
	t.Logf("workflow_step_id transition history: %v", stepNamesFromIDs(dedup, sc.stepGetter))

	if finalTask.WorkflowStepID != stepInProgressID {
		t.Errorf("final workflow_step_id = %q, want %q (In Progress)", finalTask.WorkflowStepID, stepInProgressID)
	}

	expected := []string{stepInReviewID, stepInProgressID}
	if !sliceEqual(dedup, expected) {
		t.Errorf("transition history = %v, want %v\n  (this means the deferred-move triggered spurious additional transitions — the bug)",
			stepNamesFromIDs(dedup, sc.stepGetter), stepNamesFromIDs(expected, sc.stepGetter))
	}

	rev, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	if rev.State != models.TaskSessionStateCompleted {
		t.Errorf("review session state = %q, want COMPLETED (parked by the profile switch)", rev.State)
	}
	if rev.IsPrimary {
		t.Error("review session must no longer be primary (the impl session takes over)")
	}

	impl, err := sc.repo.GetTaskSession(sc.ctx, sc.implSessionID)
	if err != nil {
		t.Fatalf("load impl session: %v", err)
	}
	if !impl.IsPrimary {
		t.Error("impl session must be primary after the deferred move applies")
	}
	if impl.State == models.TaskSessionStateCompleted {
		t.Errorf("impl session state = %q, expected non-terminal (revived for a new turn)", impl.State)
	}

	sc.assertHandoffDeliveredOrQueued(t)
}

// assertHandoffDeliveredOrQueued checks the hand-off prompt landed on the impl
// session — either delivered to its agent (PromptAgent capture) or sitting in
// the queue waiting for delivery. Both are acceptable; the failure mode the
// regression catches is "lost" (neither delivered nor queued) or "delivered
// to the wrong session".
func (sc *pendingMoveScenario) assertHandoffDeliveredOrQueued(t *testing.T) {
	t.Helper()
	implPrompts := capturedPromptsForExecution(sc.agentMgr, sc.implRelaunchExec)
	implQueued := sc.svc.messageQueue.GetStatus(sc.ctx, sc.implSessionID)

	if len(implPrompts) == 0 && !implQueued.IsQueued {
		t.Error("hand-off prompt was neither delivered to the impl session nor queued for it")
		return
	}
	for _, p := range implPrompts {
		if strings.Contains(p, "fibonacci.py has two bugs") {
			return
		}
	}
	if implQueued.IsQueued && implQueued.Message != nil &&
		strings.Contains(implQueued.Message.Content, "fibonacci.py has two bugs") {
		return
	}
	t.Errorf("hand-off prompt was neither delivered nor queued with the expected content")
}

// --- Helpers ---

// capturedPromptsForExecution returns only the prompts that were sent to the
// given agent execution ID. The earlier version ignored its selector and
// returned every recorded prompt — which would let the test pass even if the
// hand-off had been delivered to the wrong session.
func capturedPromptsForExecution(agentMgr *mockAgentManager, executionID string) []string {
	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	out := make([]string, 0, len(agentMgr.capturedPromptCalls))
	for _, c := range agentMgr.capturedPromptCalls {
		if c.ExecutionID == executionID {
			out = append(out, c.Prompt)
		}
	}
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

			agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
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
