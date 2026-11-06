package orchestrator

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	staleCreatedTaskID      = "task-stale-created"
	staleCreatedSessionID   = "session-stale-created"
	staleCreatedExecutionID = "exec-stale-created"
	staleCreatedPlanStep    = "step-plan"
	staleCreatedOtherStep   = "step-review"
	staleCreatedPrompt      = "Plan the work"
)

type staleCreatedAutoStartFixture struct {
	svc      *Service
	repo     *sqliterepo.Repository
	agentMgr *mockAgentManager
	taskRepo *mockTaskRepo
	step     *wfmodels.WorkflowStep
	stale    *models.TaskSession
	launches atomic.Int32
}

// newStaleCreatedAutoStartFixture seeds the production shape: a CREATED
// session on the auto-start step whose workspace is already prepared, and the
// CREATED snapshot the workflow dispatch carries. advance runs once, when the
// launch resolves the prepared workspace's execution: after the launch reloaded
// the session and before it persists STARTING, the window in which production
// saw a concurrent step move put the session in WAITING_FOR_INPUT.
func newStaleCreatedAutoStartFixture(
	t *testing.T,
	advance func(ctx context.Context),
) *staleCreatedAutoStartFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, staleCreatedTaskID, staleCreatedSessionID, models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, staleCreatedTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = staleCreatedPlanStep
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("place task on plan step: %v", err)
	}
	seedExecutorRunning(t, repo, staleCreatedSessionID, staleCreatedTaskID, staleCreatedExecutionID)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[staleCreatedTaskID] = &v1.Task{
		ID: staleCreatedTaskID, Title: "Stale created", State: v1.TaskStateInProgress,
	}
	f := &staleCreatedAutoStartFixture{repo: repo, taskRepo: taskRepo}
	var advanced atomic.Bool
	f.agentMgr = &mockAgentManager{
		repoForExecutionLookup: repo,
		getExecutionIDForSessionFunc: func(lookupCtx context.Context, sessionID string) (string, error) {
			if advance != nil && !advanced.Swap(true) {
				advance(context.WithoutCancel(lookupCtx))
			}
			running, err := repo.GetExecutorRunningBySessionID(lookupCtx, sessionID)
			if err != nil || running == nil {
				return "", err
			}
			return running.AgentExecutionID, nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			f.launches.Add(1)
			return &executor.LaunchAgentResponse{AgentExecutionID: "mock-launch-" + req.SessionID}, nil
		},
	}

	f.step = &wfmodels.WorkflowStep{ID: staleCreatedPlanStep, WorkflowID: "wf1", Name: "Plan"}
	stepGetter := newMockStepGetter()
	stepGetter.steps[f.step.ID] = f.step
	stepGetter.steps[staleCreatedOtherStep] = &wfmodels.WorkflowStep{
		ID: staleCreatedOtherStep, WorkflowID: "wf1", Name: "Review",
	}

	f.svc = createTestServiceWithScheduler(repo, stepGetter, taskRepo, f.agentMgr)
	f.svc.messageCreator = &mockMessageCreator{}

	session, err := repo.GetTaskSession(ctx, staleCreatedSessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.AgentProfileID = "profile-stale-created"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session profile: %v", err)
	}
	stale := *session
	f.stale = &stale
	return f
}

// setSessionWaiting mirrors the concurrent path's WAITING_FOR_INPUT write.
func (f *staleCreatedAutoStartFixture) setSessionWaiting(t *testing.T, ctx context.Context) {
	t.Helper()
	if err := f.repo.UpdateTaskSessionState(
		ctx, staleCreatedSessionID, models.TaskSessionStateWaitingForInput, "",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}
}

// moveTaskAway mirrors the production manual move off the auto-start step.
func (f *staleCreatedAutoStartFixture) moveTaskAway(t *testing.T, ctx context.Context) {
	t.Helper()
	task, err := f.repo.GetTask(ctx, staleCreatedTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.WorkflowStepID = staleCreatedOtherStep
	if err := f.repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("move task: %v", err)
	}
	f.setSessionWaiting(t, ctx)
}

func (f *staleCreatedAutoStartFixture) assertNotFailed(t *testing.T, ctx context.Context) {
	t.Helper()
	session, err := f.repo.GetTaskSession(ctx, staleCreatedSessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s (error %q), want WAITING_FOR_INPUT: a lost CREATED race is not a launch failure",
			session.State, session.ErrorMessage)
	}
	task, err := f.repo.GetTask(ctx, staleCreatedTaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.State == v1.TaskStateFailed {
		t.Fatal("task state = FAILED, want the lost CREATED race to leave the task unfailed")
	}
	f.taskRepo.mu.Lock()
	mockState := f.taskRepo.tasks[staleCreatedTaskID].State
	f.taskRepo.mu.Unlock()
	if mockState == v1.TaskStateFailed {
		t.Fatal("scheduler task state = FAILED, want the lost CREATED race to leave the task unfailed")
	}
}

func (f *staleCreatedAutoStartFixture) assertNoAgentStarted(t *testing.T) {
	t.Helper()
	f.agentMgr.mu.Lock()
	starts := len(f.agentMgr.startAgentProcessCalls)
	prompts := len(f.agentMgr.capturedPrompts)
	f.agentMgr.mu.Unlock()
	if starts != 0 || f.launches.Load() != 0 {
		t.Fatalf("agent starts = %d, launches = %d, want none for a session another path advanced",
			starts, f.launches.Load())
	}
	if prompts != 0 {
		t.Fatalf("prompts delivered = %d, want 0 after the task left the auto-start step", prompts)
	}
}

// Production sequence (task 4ff6ce79, session 6517fa73): a queue promotion
// dispatched the plan step's auto-start for a CREATED session with a prepared
// workspace; while the launch ran, a manual move left the plan step and put the
// session in WAITING_FOR_INPUT. The launch's CREATED-guarded persistence lost
// the race and the session and task were marked FAILED.
func TestStaleCreatedAutoStart_LostRaceAfterStepMoveIsNotFailure(t *testing.T) {
	ctx := context.Background()
	var f *staleCreatedAutoStartFixture
	f = newStaleCreatedAutoStartFixture(t, func(advanceCtx context.Context) {
		f.moveTaskAway(t, advanceCtx)
	})

	err := f.svc.autoStartStepPrompt(ctx, staleCreatedTaskID, f.stale, f.step, staleCreatedPrompt, false, true, nil)
	f.assertNotFailed(t, ctx)
	if err != nil {
		t.Fatalf("autoStartStepPrompt returned %v, want a no-op for a session another path advanced", err)
	}
	f.assertNoAgentStarted(t)
}

// The same lost race while the task stays on the auto-start step, after the
// concurrent path left a live agent waiting for input: the losing launch
// started no agent, so the step prompt is handed once to the waiting-session
// prompt path instead of being dropped or launched twice.
func TestStaleCreatedAutoStart_LostRaceOnSameStepDeliversPromptOnce(t *testing.T) {
	ctx := context.Background()
	var f *staleCreatedAutoStartFixture
	var liveAgent atomic.Bool
	f = newStaleCreatedAutoStartFixture(t, func(advanceCtx context.Context) {
		f.setSessionWaiting(t, advanceCtx)
		liveAgent.Store(true)
	})
	f.agentMgr.isAgentRunningFn = func(context.Context, string) bool { return liveAgent.Load() }

	err := f.svc.autoStartStepPrompt(ctx, staleCreatedTaskID, f.stale, f.step, staleCreatedPrompt, false, true, nil)
	session, getErr := f.repo.GetTaskSession(ctx, staleCreatedSessionID)
	if getErr != nil {
		t.Fatalf("get session: %v", getErr)
	}
	if session.State == models.TaskSessionStateFailed {
		t.Fatalf("session state = FAILED (%q), want the lost race not to fail the session", session.ErrorMessage)
	}
	if err != nil {
		t.Fatalf("autoStartStepPrompt returned %v, want the prompt delivered after the lost race", err)
	}
	if got := f.svc.messageQueue.GetStatus(ctx, staleCreatedSessionID).Count; got != 0 {
		t.Fatalf("queued messages = %d, want the prompt delivered instead of left queued", got)
	}
	f.agentMgr.mu.Lock()
	prompts := append([]string(nil), f.agentMgr.capturedPrompts...)
	f.agentMgr.mu.Unlock()
	if len(prompts) != 1 {
		t.Fatalf("prompts delivered = %d, want exactly 1", len(prompts))
	}
	if !strings.Contains(prompts[0], staleCreatedPrompt) {
		t.Fatalf("delivered prompt %q does not carry the step prompt", prompts[0])
	}
}

// A replay whose CREATED snapshot is already stale when the launch reloads the
// session does not launch once the task has left the auto-start step.
func TestStaleCreatedAutoStart_ReplayAfterSessionWaitingDoesNotLaunch(t *testing.T) {
	ctx := context.Background()
	f := newStaleCreatedAutoStartFixture(t, nil)
	f.moveTaskAway(t, ctx)

	err := f.svc.autoStartStepPrompt(ctx, staleCreatedTaskID, f.stale, f.step, staleCreatedPrompt, false, true, nil)
	f.assertNotFailed(t, ctx)
	if err != nil {
		t.Fatalf("autoStartStepPrompt returned %v, want a no-op for a stale CREATED replay", err)
	}
	f.assertNoAgentStarted(t)
}
