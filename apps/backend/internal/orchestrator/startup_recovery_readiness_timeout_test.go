package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type startupRecoveryHarness struct {
	ctx                     context.Context
	repo                    *sqliterepo.Repository
	svc                     *Service
	agentMgr                *mockAgentManager
	taskRepo                *mockTaskRepo
	task                    *models.Task
	session                 *models.TaskSession
	launchCalls             atomic.Int32
	controlTaskUpdatedAt    time.Time
	controlSessionUpdatedAt time.Time
}

type successorOnReadyCheckRepo struct {
	sessionExecutorStore
	base  *sqliterepo.Repository
	armed atomic.Bool
	once  sync.Once
}

func (r *successorOnReadyCheckRepo) GetExecutorRunningBySessionID(
	ctx context.Context,
	sessionID string,
) (*models.ExecutorRunning, error) {
	running, err := r.base.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || running == nil || !r.armed.Load() || running.AgentExecutionID != "execution-resumed" {
		return running, err
	}
	snapshot := *running
	var replaceErr error
	r.once.Do(func() {
		successor := *running
		successor.AgentExecutionID = "execution-successor"
		successor.UpdatedAt = time.Now().UTC()
		replaceErr = r.base.UpsertExecutorRunning(ctx, &successor)
	})
	if replaceErr != nil {
		return nil, replaceErr
	}
	return &snapshot, nil
}

func newStartupRecoveryHarness(t *testing.T, promptReady bool) *startupRecoveryHarness {
	t.Helper()
	h := &startupRecoveryHarness{ctx: context.Background()}
	h.repo = setupTestRepo(t)
	seedTaskAndSession(t, h.repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedTaskAndSession(t, h.repo, "control-task", "control-session", models.TaskSessionStateWaitingForInput)

	var err error
	h.task, err = h.repo.GetTask(h.ctx, "task1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	h.task.WorkflowStepID = "blocked-step"
	if err := h.repo.UpdateTask(h.ctx, h.task); err != nil {
		t.Fatalf("update task: %v", err)
	}
	controlTask, err := h.repo.GetTask(h.ctx, "control-task")
	if err != nil {
		t.Fatalf("get control task: %v", err)
	}
	controlTask.WorkflowStepID = "control-step"
	if err := h.repo.UpdateTask(h.ctx, controlTask); err != nil {
		t.Fatalf("update control task: %v", err)
	}
	h.controlTaskUpdatedAt = controlTask.UpdatedAt
	controlSession, err := h.repo.GetTaskSession(h.ctx, "control-session")
	if err != nil {
		t.Fatalf("get control session: %v", err)
	}
	h.controlSessionUpdatedAt = controlSession.UpdatedAt
	h.session, err = h.repo.GetTaskSession(h.ctx, "session1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	h.session.AgentProfileID = "profile1"
	if err := h.repo.UpdateTaskSession(h.ctx, h.session); err != nil {
		t.Fatalf("update session: %v", err)
	}

	now := time.Now().UTC()
	if err := h.repo.UpsertExecutorRunning(h.ctx, &models.ExecutorRunning{
		ID:               "running1",
		SessionID:        h.session.ID,
		TaskID:           h.task.ID,
		AgentExecutionID: "execution-before-restart",
		Status:           models.ExecutorRunningStatusReady,
		Resumable:        true,
		ResumeToken:      "resume-token",
		WorktreePath:     "/tasks/task1",
		WorktreeBranch:   "feature/task1",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed executor row: %v", err)
	}

	h.agentMgr = &mockAgentManager{
		repoForExecutionLookup: h.repo,
		isAgentRunningFn: func(context.Context, string) bool {
			return false
		},
		isAgentReadyFn: func(context.Context, string) bool {
			return promptReady
		},
		launchAgentFunc: func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			h.launchCalls.Add(1)
			running, err := h.repo.GetExecutorRunningBySessionID(ctx, req.SessionID)
			if err != nil {
				return nil, err
			}
			running.AgentExecutionID = "execution-resumed"
			running.UpdatedAt = time.Now().UTC()
			if err := h.repo.UpsertExecutorRunning(ctx, running); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-resumed"}, nil
		},
	}
	h.taskRepo = newMockTaskRepo()
	h.taskRepo.tasks[h.task.ID] = &v1.Task{ID: h.task.ID, State: v1.TaskStateInProgress}
	stepGetter := newMockStepGetter()
	stepGetter.steps["blocked-step"] = &wfmodels.WorkflowStep{
		ID: "blocked-step", WorkflowID: "wf1", Name: "Blocked", Prompt: "continue",
	}
	h.svc = createTestServiceWithAgent(h.repo, stepGetter, h.taskRepo, h.agentMgr)
	h.svc.executor = executor.NewExecutor(h.agentMgr, h.repo, testLogger(), executor.ExecutorConfig{})
	h.svc.scheduler = scheduler.NewScheduler(
		queue.NewTaskQueue(10), h.svc.executor, h.taskRepo, testLogger(), scheduler.SchedulerConfig{},
	)
	return h
}

func (h *startupRecoveryHarness) assertStableIdentity(t *testing.T) {
	t.Helper()
	if got := h.launchCalls.Load(); got != 1 {
		t.Fatalf("LaunchAgent calls = %d, want 1", got)
	}
	sessions, err := h.repo.ListTaskSessions(h.ctx, h.task.ID)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != h.session.ID {
		t.Fatalf("sessions = %+v, want only %s", sessions, h.session.ID)
	}
	running, err := h.repo.GetExecutorRunningBySessionID(h.ctx, h.session.ID)
	if err != nil {
		t.Fatalf("get executor row: %v", err)
	}
	if running.ResumeToken != "resume-token" || running.WorktreePath != "/tasks/task1" ||
		running.WorktreeBranch != "feature/task1" {
		t.Fatalf("resume/worktree identity changed: %+v", running)
	}
	persistedTask, err := h.repo.GetTask(h.ctx, h.task.ID)
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if persistedTask.WorkflowStepID != "blocked-step" {
		t.Fatalf("workflow step = %q, want blocked-step", persistedTask.WorkflowStepID)
	}
	controlTask, err := h.repo.GetTask(h.ctx, "control-task")
	if err != nil {
		t.Fatalf("reload control task: %v", err)
	}
	if controlTask.WorkflowStepID != "control-step" || !controlTask.UpdatedAt.Equal(h.controlTaskUpdatedAt) {
		t.Fatalf("unrelated task changed: %+v", controlTask)
	}
	controlSession, err := h.repo.GetTaskSession(h.ctx, "control-session")
	if err != nil {
		t.Fatalf("reload control session: %v", err)
	}
	if controlSession.State != models.TaskSessionStateWaitingForInput ||
		!controlSession.UpdatedAt.Equal(h.controlSessionUpdatedAt) {
		t.Fatalf("unrelated session changed: %+v", controlSession)
	}
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestEnsureSessionRunning_StartupRecoveryReconcilesReadyExecutionAfterSessionTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, true)
		if err := h.svc.ensureSessionRunning(h.ctx, h.session.ID, h.session); err != nil {
			t.Fatalf("ensure session running: %v", err)
		}
		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateWaitingForInput {
			t.Fatalf("session state = %s, want WAITING_FOR_INPUT", persisted.State)
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestEnsureSessionRunning_StartupRecoveryTerminalizesUnreadyExecutionAfterSessionTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, false)
		if err := h.svc.ensureSessionRunning(h.ctx, h.session.ID, h.session); err == nil {
			t.Fatal("expected readiness timeout")
		}
		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateFailed {
			t.Fatalf("session state = %s, want FAILED", persisted.State)
		}
		h.svc.handleAgentBootReady(h.ctx, watcher.AgentEventData{
			TaskID: h.task.ID, SessionID: h.session.ID, AgentExecutionID: "execution-resumed",
		})
		persisted, err = h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session after delayed boot-ready: %v", err)
		}
		if persisted.State != models.TaskSessionStateFailed {
			t.Fatalf("delayed boot-ready changed session state to %s", persisted.State)
		}
		h.agentMgr.mu.Lock()
		stopCalls := append([]stopAgentCall(nil), h.agentMgr.stopAgentWithReasonArgs...)
		h.agentMgr.mu.Unlock()
		if len(stopCalls) != 1 || stopCalls[0].ExecutionID != "execution-resumed" {
			t.Fatalf("stop calls = %+v, want one stop for execution-resumed", stopCalls)
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestEnsureSessionRunning_StartupRecoveryStopFailurePreservesRetryableSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, false)
		h.agentMgr.stopAgentWithReasonErr = errors.New("runtime still active")

		if err := h.svc.ensureSessionRunning(h.ctx, h.session.ID, h.session); err == nil {
			t.Fatal("expected readiness teardown failure")
		}
		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateStarting {
			t.Fatalf("session state = %s, want retryable STARTING", persisted.State)
		}
		if got := h.taskRepo.updatedStates[h.task.ID]; got == v1.TaskStateFailed {
			t.Fatal("stop failure marked task FAILED while its execution may still be active")
		}
		if got := h.taskRepo.stateWrites[h.task.ID]; got != 0 {
			t.Fatalf("task state writes = %d, want 0 after stop failure", got)
		}
		h.agentMgr.mu.Lock()
		stopCalls := append([]stopAgentCall(nil), h.agentMgr.stopAgentWithReasonArgs...)
		h.agentMgr.mu.Unlock()
		if len(stopCalls) != 1 || stopCalls[0].ExecutionID != "execution-resumed" {
			t.Fatalf("stop calls = %+v, want one attempt for execution-resumed", stopCalls)
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestEnsureSessionRunning_StartupRecoveryDoesNotTouchSuccessorExecution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, true)
		wrapped := &successorOnReadyCheckRepo{sessionExecutorStore: h.svc.repo, base: h.repo}
		h.svc.repo = wrapped
		launch := h.agentMgr.launchAgentFunc
		h.agentMgr.launchAgentFunc = func(
			ctx context.Context,
			req *executor.LaunchAgentRequest,
		) (*executor.LaunchAgentResponse, error) {
			response, err := launch(ctx, req)
			if err == nil {
				wrapped.armed.Store(true)
			}
			return response, err
		}

		if err := h.svc.ensureSessionRunning(h.ctx, h.session.ID, h.session); err == nil {
			t.Fatal("expected readiness timeout after successor superseded the resumed execution")
		}
		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateStarting {
			t.Fatalf("session state = %s, want successor-owned STARTING", persisted.State)
		}
		running, err := h.repo.GetExecutorRunningBySessionID(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload execution: %v", err)
		}
		if running.AgentExecutionID != "execution-successor" {
			t.Fatalf("execution id = %q, want execution-successor", running.AgentExecutionID)
		}
		h.agentMgr.mu.Lock()
		stopCalls := append([]stopAgentCall(nil), h.agentMgr.stopAgentWithReasonArgs...)
		h.agentMgr.mu.Unlock()
		if len(stopCalls) != 0 {
			t.Fatalf("successor race issued stop calls: %+v", stopCalls)
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestEnsureSessionRunning_StartupRecoveryDoesNotTeardownTerminalSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, true)
		var transitionOnce sync.Once
		var transitionErr error
		h.agentMgr.isAgentReadyFn = func(context.Context, string) bool {
			transitionOnce.Do(func() {
				transitionErr = h.repo.UpdateTaskSessionState(
					h.ctx, h.session.ID, models.TaskSessionStateCancelled, "cancelled concurrently",
				)
			})
			return true
		}

		if err := h.svc.ensureSessionRunning(h.ctx, h.session.ID, h.session); err == nil {
			t.Fatal("expected readiness timeout after terminal transition won")
		}
		if transitionErr != nil {
			t.Fatalf("terminal transition: %v", transitionErr)
		}
		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateCancelled {
			t.Fatalf("session state = %s, want CANCELLED", persisted.State)
		}
		h.agentMgr.mu.Lock()
		stopCalls := append([]stopAgentCall(nil), h.agentMgr.stopAgentWithReasonArgs...)
		h.agentMgr.mu.Unlock()
		if len(stopCalls) != 0 {
			t.Fatalf("terminal race issued stop calls: %+v", stopCalls)
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestWorkflowAutoStart_ReadinessTimeoutDoesNotPublishWaitingOrCreateReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, false)
		eventBus := &mockEventBus{}
		h.svc.eventBus = eventBus
		step := &wfmodels.WorkflowStep{ID: "blocked-step", WorkflowID: "wf1", Name: "Blocked", Prompt: "continue"}

		h.svc.launchAfterOnEnterDispatch(
			h.ctx, h.task.ID, h.session, step, h.task.Description, false, true, false,
		)

		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateFailed {
			t.Fatalf("session state = %s, want FAILED", persisted.State)
		}
		for _, published := range eventBus.published() {
			if published.Subject != events.TaskSessionStateChanged {
				continue
			}
			data, ok := published.Event.Data.(map[string]interface{})
			if ok && data[metaKeyNewState] == string(models.TaskSessionStateWaitingForInput) {
				t.Fatalf("published false WAITING_FOR_INPUT event: %+v", data)
			}
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestWorkflowAutoStart_ReadinessTimeoutStopFailurePreservesRetryableSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, false)
		h.agentMgr.stopAgentWithReasonErr = errors.New("runtime still active")
		h.svc.registerBackgroundWork(h.session.ID, "background-live", "execution-resumed", "work-live")
		eventBus := &mockEventBus{}
		h.svc.eventBus = eventBus
		step := &wfmodels.WorkflowStep{ID: "blocked-step", WorkflowID: "wf1", Name: "Blocked", Prompt: "continue"}

		h.svc.launchAfterOnEnterDispatch(
			h.ctx, h.task.ID, h.session, step, h.task.Description, false, true, false,
		)

		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateStarting {
			t.Fatalf("session state = %s, want retryable STARTING", persisted.State)
		}
		if got := h.taskRepo.stateWrites[h.task.ID]; got != 0 {
			t.Fatalf("task state writes = %d, want 0 after stop failure", got)
		}
		for _, published := range eventBus.published() {
			if published.Subject != events.TaskSessionStateChanged {
				continue
			}
			data, ok := published.Event.Data.(map[string]interface{})
			if ok && data[metaKeyNewState] == string(models.TaskSessionStateWaitingForInput) {
				t.Fatalf("published false WAITING_FOR_INPUT event after stop failure: %+v", data)
			}
		}
		if !h.svc.hasBackgroundTask(h.session.ID, "background-live") {
			t.Fatal("stop failure retired activity still owned by the live execution")
		}
		if h.svc.isExecutionCompleted(h.session.ID, "execution-resumed") {
			t.Fatal("stop failure terminal-marked a still-live execution")
		}
		h.agentMgr.mu.Lock()
		stopCalls := append([]stopAgentCall(nil), h.agentMgr.stopAgentWithReasonArgs...)
		h.agentMgr.mu.Unlock()
		if len(stopCalls) != 1 || stopCalls[0].ExecutionID != "execution-resumed" {
			t.Fatalf("stop calls = %+v, want one attempt for execution-resumed", stopCalls)
		}
		h.assertStableIdentity(t)
	})
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.6
func TestWorkflowAutoStart_ReadinessTimeoutDoesNotReconcileSuccessorExecution(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newStartupRecoveryHarness(t, true)
		wrapped := &successorOnReadyCheckRepo{sessionExecutorStore: h.svc.repo, base: h.repo}
		h.svc.repo = wrapped
		launch := h.agentMgr.launchAgentFunc
		h.agentMgr.launchAgentFunc = func(
			ctx context.Context,
			req *executor.LaunchAgentRequest,
		) (*executor.LaunchAgentResponse, error) {
			response, err := launch(ctx, req)
			if err == nil {
				wrapped.armed.Store(true)
			}
			return response, err
		}
		eventBus := &mockEventBus{}
		h.svc.eventBus = eventBus
		step := &wfmodels.WorkflowStep{ID: "blocked-step", WorkflowID: "wf1", Name: "Blocked", Prompt: "continue"}

		h.svc.launchAfterOnEnterDispatch(
			h.ctx, h.task.ID, h.session, step, h.task.Description, false, true, false,
		)

		persisted, err := h.repo.GetTaskSession(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload session: %v", err)
		}
		if persisted.State != models.TaskSessionStateStarting {
			t.Fatalf("session state = %s, want successor-owned STARTING", persisted.State)
		}
		running, err := h.repo.GetExecutorRunningBySessionID(h.ctx, h.session.ID)
		if err != nil {
			t.Fatalf("reload execution: %v", err)
		}
		if running.AgentExecutionID != "execution-successor" {
			t.Fatalf("execution id = %q, want execution-successor", running.AgentExecutionID)
		}
		for _, published := range eventBus.published() {
			if published.Subject != events.TaskSessionStateChanged {
				continue
			}
			data, ok := published.Event.Data.(map[string]interface{})
			if ok && data[metaKeyNewState] == string(models.TaskSessionStateWaitingForInput) {
				t.Fatalf("published false WAITING_FOR_INPUT event for successor: %+v", data)
			}
		}
		h.agentMgr.mu.Lock()
		stopCalls := append([]stopAgentCall(nil), h.agentMgr.stopAgentWithReasonArgs...)
		h.agentMgr.mu.Unlock()
		if len(stopCalls) != 0 {
			t.Fatalf("successor race issued stop calls: %+v", stopCalls)
		}
		h.assertStableIdentity(t)
	})
}
