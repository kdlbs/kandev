package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type queuedModelSwitchPauseFixture struct {
	svc       *Service
	repo      *sqliterepo.Repository
	manager   *mockAgentManager
	identity  messagequeue.QueueSessionIdentity
	queuedID  string
	taskID    string
	sessionID string
}

func newQueuedModelSwitchPauseFixture(
	t *testing.T,
	inPlace bool,
	switchEntered chan<- struct{},
	switchRelease <-chan struct{},
) *queuedModelSwitchPauseFixture {
	t.Helper()
	ctx := context.Background()
	const taskID, sessionID = "task1", "session-model-switch-pause"
	const oldExecution, newExecution = "execution-model-switch-pause-old", "execution-model-switch-pause-new"
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session model: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, oldExecution)
	manager := &mockAgentManager{
		isAgentRunning:           true,
		repoForExecutionLookup:   repo,
		setSessionModelSupported: inPlace,
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: newExecution, Status: "starting",
			}); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: newExecution}, nil
		},
	}
	if switchEntered != nil {
		manager.setSessionModelFunc = func(context.Context, string, string) error {
			close(switchEntered)
			<-switchRelease
			return nil
		}
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = &repoTurnService{repo: repo}
	svc.messageQueue = newAuthoritativeMemoryQueue(repo, testLogger())
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		t.Fatalf("resolve queue identity: %v", err)
	}
	if err := svc.messageQueue.SetAutoRun(ctx, sessionID, true); err != nil {
		t.Fatalf("enable auto-run: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessageWithMetadata(
		ctx, sessionID, taskID, "queued model-switch prompt", "", messagequeue.QueuedByUser, false, nil, nil,
	); err != nil {
		t.Fatalf("queue prompt: %v", err)
	}
	queued, ok, _, err := svc.messageQueue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !ok || queued == nil {
		t.Fatalf("reserve queued prompt: queued=%+v ok=%v err=%v", queued, ok, err)
	}
	reservation := svc.markQueuedDispatchInFlightWithIdentityLocked(identity, queued.ID, queued)
	if reservation == nil {
		t.Fatal("mark queued dispatch in flight returned nil")
	}
	if claimed, err := svc.claimQueuedDispatchForExecution(sessionID, queued.ID, reservation); err != nil || !claimed {
		t.Fatalf("claim queued dispatch: claimed=%v err=%v", claimed, err)
	}
	return &queuedModelSwitchPauseFixture{
		svc: svc, repo: repo, manager: manager, identity: identity, queuedID: queued.ID,
		taskID: taskID, sessionID: sessionID,
	}
}

func (f *queuedModelSwitchPauseFixture) promptTask(ctx context.Context) error {
	_, err := f.svc.promptTask(
		ctx, f.taskID, f.sessionID, "queued model-switch prompt", "new-model", false, nil, true,
		launchOriginAutomatic,
		promptTaskOptions{claimEntryID: f.queuedID, expectedSessionIdentity: &f.identity},
	)
	return err
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.7
// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.8
func TestResumeAttempt_ModelSwitchFallbackCancellationBeforeInitialPromptAcceptance(t *testing.T) {
	ctx := context.Background()
	const (
		taskID       = "task-model-switch-before-acceptance"
		sessionID    = "session-model-switch-before-acceptance"
		oldExecution = "execution-model-switch-before-old"
		newExecution = "execution-model-switch-before-new"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentProfileSnapshot = map[string]interface{}{"model": "old-model"}
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session model: %v", err)
	}
	seedExecutorRunning(t, repo, sessionID, taskID, oldExecution)

	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
				ID: sessionID, SessionID: sessionID, TaskID: taskID,
				AgentExecutionID: newExecution, Status: "starting",
			}); err != nil {
				return nil, err
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: newExecution}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	attempt, owner, err := svc.beginResumeAttempt(ctx, taskID, sessionID)
	if err != nil || !owner {
		t.Fatalf("begin model-switch attempt: attempt=%v owner=%v err=%v", attempt, owner, err)
	}
	attempt.setExecutionID(oldExecution)
	t.Cleanup(func() { attempt.finish(svc.resumeAttemptStore()) })

	beforeAdmission, acceptAdmission, releaseAdmission := svc.newModelSwitchAdmissionGate(
		context.WithoutCancel(ctx), taskID, sessionID, session, promptTaskOptions{}, attempt, nil,
	)
	_, handled, err := svc.trySwitchModelForPromptWithAdmission(
		ctx, taskID, sessionID, "new-model", "model-switch prompt", session,
		&foregroundDispatch{}, beforeAdmission, acceptAdmission, releaseAdmission, attempt,
	)
	if err != nil {
		t.Fatalf("model-switch fallback: %v", err)
	}
	if !handled {
		t.Fatal("model-switch fallback was not handled")
	}
	manager.mu.Lock()
	onDispatched := manager.initialPromptDispatchCallback
	beforeInitialAdmission := manager.initialPromptAdmissionCallback
	onFailure := manager.initialPromptFailureCallback
	started := append([]string(nil), manager.startAgentProcessCalls...)
	manager.mu.Unlock()
	if len(started) != 1 || started[0] != newExecution {
		t.Fatalf("StartAgentProcess calls = %#v, want [%q]", started, newExecution)
	}
	if onDispatched == nil {
		t.Fatal("model-switch fallback did not retain the initial-prompt callback")
	}
	if beforeInitialAdmission == nil {
		t.Fatal("model-switch fallback did not retain its final admission callback")
	}
	// promptTask returns before lifecycle's asynchronous initial prompt callback.
	// Its normal deferred finish must leave the attempt cancellable in this gap.
	attempt.finish(svc.resumeAttemptStore())
	if _, active := svc.resumeAttemptStore().current(sessionID); !active {
		t.Fatal("prompt-task completion removed the pending model-switch attempt before acceptance")
	}

	registry := svc.resumeAttemptStore()
	registry.mu.Lock()
	acceptedBeforeCancel := attempt.accepted
	registry.mu.Unlock()
	if acceptedBeforeCancel {
		t.Fatal("startup ownership transferred before the provider accepted the initial prompt")
	}
	svc.invalidateResumeAttempt(sessionID)
	svc.cleanupCancelledResumeAttempt(attempt)
	if err := beforeInitialAdmission(); !errors.Is(err, ErrResumeAttemptCancelled) {
		t.Fatalf("late initial-prompt admission error = %v, want cancelled resume attempt", err)
	}
	if onFailure != nil {
		onFailure()
	}

	manager.mu.Lock()
	stops := append([]stopAgentCall(nil), manager.stopAgentWithReasonArgs...)
	manager.mu.Unlock()
	if len(stops) != 1 {
		t.Fatalf("forced cleanup calls = %#v, want one call", stops)
	}
	if stops[0].ExecutionID != newExecution || !stops[0].Force {
		t.Fatalf("forced cleanup call = %#v, want execution %q with force", stops[0], newExecution)
	}

	// A delayed lifecycle callback from the cancelled startup cannot revive
	// provider ownership after exact-execution cleanup has won.
	onDispatched()
	registry.mu.Lock()
	acceptedAfterLateCallback := attempt.accepted
	registry.mu.Unlock()
	if acceptedAfterLateCallback {
		t.Fatal("late initial-prompt callback revived a cancelled resume attempt")
	}
}

func TestPromptTask_ModelSwitchFallbackRejectsAdmissionAfterCompletedPause(t *testing.T) {
	ctx := context.Background()
	fixture := newQueuedModelSwitchPauseFixture(t, false, nil, nil)
	if err := fixture.promptTask(ctx); err != nil {
		t.Fatalf("model-switch prompt task: %v", err)
	}
	fixture.manager.mu.Lock()
	beforeAdmission := fixture.manager.initialPromptAdmissionCallback
	onDispatched := fixture.manager.initialPromptDispatchCallback
	onFailure := fixture.manager.initialPromptFailureCallback
	fixture.manager.mu.Unlock()
	if beforeAdmission == nil || onDispatched == nil {
		t.Fatal("restart-based model switch did not retain initial-prompt admission callbacks")
	}
	turn, err := fixture.svc.turnService.GetActiveTurn(ctx, fixture.sessionID)
	if err != nil || turn == nil {
		t.Fatalf("active model-switch turn = %+v, err=%v", turn, err)
	}
	if err := fixture.svc.CancelAgent(ctx, fixture.sessionID); err != nil {
		t.Fatalf("complete explicit pause: %v", err)
	}
	closedTurn, err := fixture.svc.turnService.GetTurn(ctx, turn.ID)
	if err != nil || closedTurn == nil || closedTurn.CompletedAt == nil {
		t.Fatalf("paused turn = %+v, err=%v, want completed", closedTurn, err)
	}
	generationBefore := fixture.manager.currentPromptGeneration.Load()
	providerPromptCalls := 0
	admissionErr := beforeAdmission()
	if admissionErr == nil {
		providerPromptCalls++
		fixture.manager.currentPromptGeneration.Add(1)
		onDispatched()
	}
	if !errors.Is(admissionErr, ErrAgentPromptInProgress) {
		t.Fatalf("replacement initial-prompt admission error = %v, want cancellation-fenced rejection", admissionErr)
	}
	if onFailure != nil {
		onFailure()
	}
	if providerPromptCalls != 0 {
		t.Fatalf("provider prompt calls after completed pause = %d, want 0", providerPromptCalls)
	}
	if got := fixture.manager.currentPromptGeneration.Load(); got != generationBefore {
		t.Fatalf("prompt generation after rejected admission = %d, want unchanged %d", got, generationBefore)
	}
	if active, err := fixture.svc.turnService.GetActiveTurn(ctx, fixture.sessionID); err != nil || active != nil {
		t.Fatalf("active successor turn after rejected admission = %+v, err=%v, want none", active, err)
	}
	turns, err := fixture.repo.ListTurnsBySession(ctx, fixture.sessionID)
	if err != nil || len(turns) != 1 {
		t.Fatalf("turns after rejected admission = %d, err=%v, want only the paused turn", len(turns), err)
	}
}

func TestPromptTask_QueuedInPlaceModelSwitchRejectsAfterCompletedPause(t *testing.T) {
	ctx := context.Background()
	switchEntered := make(chan struct{})
	switchRelease := make(chan struct{})
	fixture := newQueuedModelSwitchPauseFixture(t, true, switchEntered, switchRelease)
	fixture.manager.advancePromptGenerationOnAdmission = true
	promptDone := make(chan error, 1)
	go func() { promptDone <- fixture.promptTask(ctx) }()
	select {
	case <-switchEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("in-place model switch did not enter the controlled unlocked window")
	}
	if err := fixture.svc.CancelAgent(ctx, fixture.sessionID); err != nil {
		t.Fatalf("complete explicit pause during in-place switch: %v", err)
	}
	generationAfterPause := fixture.manager.currentPromptGeneration.Load()
	pausedTurns, err := fixture.repo.ListTurnsBySession(ctx, fixture.sessionID)
	if err != nil {
		t.Fatalf("read turns after pause: %v", err)
	}
	for _, turn := range pausedTurns {
		if turn.CompletedAt == nil {
			t.Fatalf("turn after completed pause is still active: %+v", turn)
		}
	}
	close(switchRelease)
	select {
	case promptErr := <-promptDone:
		if !errors.Is(promptErr, ErrAgentPromptInProgress) {
			t.Fatalf("queued prompt after completed in-place-switch pause = %v, want cancellation-fenced rejection", promptErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued prompt did not finish after the in-place switch was released")
	}
	if got := capturedPromptCount(fixture.manager); got != 0 {
		t.Fatalf("provider prompt calls after completed pause = %d, want 0", got)
	}
	if got := fixture.manager.currentPromptGeneration.Load(); got != generationAfterPause {
		t.Fatalf("prompt generation after rejected in-place admission = %d, want unchanged %d", got, generationAfterPause)
	}
	if active, err := fixture.svc.turnService.GetActiveTurn(ctx, fixture.sessionID); err != nil || active != nil {
		t.Fatalf("active successor turn after in-place switch = %+v, err=%v, want none", active, err)
	}
	turns, err := fixture.repo.ListTurnsBySession(ctx, fixture.sessionID)
	if err != nil || len(turns) != len(pausedTurns) {
		t.Fatalf("turns after cancelled in-place switch = %+v, err=%v, want same %d paused turns", turns, err, len(pausedTurns))
	}
	for i, turn := range turns {
		if turn.ID != pausedTurns[i].ID {
			t.Fatalf("turn %d after cancelled in-place switch = %s, want paused turn %s", i, turn.ID, pausedTurns[i].ID)
		}
	}
}
