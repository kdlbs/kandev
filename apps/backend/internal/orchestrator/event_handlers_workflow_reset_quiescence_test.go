package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type lifecycleClaimSignalRepo struct {
	repoStore
	claimed chan<- struct{}
}

func (r lifecycleClaimSignalRepo) ClaimPromptableTaskSessionIfActive(
	ctx context.Context,
	sessionID string,
) (models.PromptableTaskSessionClaim, error) {
	claim, err := r.repoStore.ClaimPromptableTaskSessionIfActive(ctx, sessionID)
	if err == nil && claim.Status == models.PromptableTaskSessionClaimed {
		r.claimed <- struct{}{}
	}
	return claim, err
}

type orderedResetAgentManager struct {
	*mockAgentManager
	events chan string
}

func (m *orderedResetAgentManager) CancelAgent(ctx context.Context, sessionID string) error {
	m.events <- "cancel"
	return m.mockAgentManager.CancelAgent(ctx, sessionID)
}

func (m *orderedResetAgentManager) ResetAgentContext(ctx context.Context, executionID string) error {
	m.events <- "reset"
	return m.mockAgentManager.ResetAgentContext(ctx, executionID)
}

func newActiveResetTestService(t *testing.T) (*Service, *sqliterepo.Repository, *orderedResetAgentManager, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	seedExecutorRunning(t, repo, "session1", "task1", "execution1")

	baseManager := &mockAgentManager{repoForExecutionLookup: repo}
	manager := &orderedResetAgentManager{
		mockAgentManager: baseManager,
		events:           make(chan string, 4),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.turnService = &repoTurnService{repo: repo}
	turn, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start active turn: %v", err)
	}
	svc.activeTurns.Store("session1", turn.ID)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	return svc, repo, manager, session
}

func requireResetEvents(t *testing.T, events <-chan string, want ...string) {
	t.Helper()
	for _, expected := range want {
		select {
		case got := <-events:
			if got != expected {
				t.Fatalf("event order = %q, want next event %q", got, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %q", expected)
		}
	}
}

func TestResetAgentContext_QuiescesActiveTurnBeforeProviderReset(t *testing.T) {
	svc, _, manager, session := newActiveResetTestService(t)
	messages := &mockMessageCreator{}
	svc.messageCreator = messages

	if !svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned false")
	}

	requireResetEvents(t, manager.events, "cancel", "reset")
	if len(messages.sessionMessages) != 0 {
		t.Fatalf("internal reset cancellation created %d session messages", len(messages.sessionMessages))
	}
}

func TestResetAgentContext_CancellationConflictStopsProviderReset(t *testing.T) {
	svc, _, manager, session := newActiveResetTestService(t)
	operation, owner := svc.claimCancellation("session1", cancellationKindExplicit)
	if !owner {
		t.Fatal("test setup failed to claim explicit cancellation")
	}
	t.Cleanup(func() {
		svc.finishCancellation("session1", operation, nil)
	})

	if svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned true while explicit cancellation owned the session")
	}
	select {
	case event := <-manager.events:
		t.Fatalf("provider operation %q started despite cancellation conflict", event)
	default:
	}
}

func TestResetAgentContext_CancellationConflictWithoutActiveTurnStopsProviderReset(t *testing.T) {
	svc, _, manager, session := newActiveResetTestService(t)
	turnIDValue, ok := svc.activeTurns.Load(session.ID)
	if !ok {
		t.Fatal("test setup did not create an active turn")
	}
	turnID, ok := turnIDValue.(string)
	if !ok || turnID == "" {
		t.Fatalf("active turn cache value = %#v, want turn ID", turnIDValue)
	}
	if err := svc.turnService.CompleteTurn(context.Background(), turnID); err != nil {
		t.Fatalf("complete active turn: %v", err)
	}
	svc.activeTurns.Delete(session.ID)

	operation, owner := svc.claimCancellation(session.ID, cancellationKindExplicit)
	if !owner {
		t.Fatal("test setup failed to claim explicit cancellation")
	}
	t.Cleanup(func() {
		svc.finishCancellation(session.ID, operation, nil)
	})

	if svc.resetAgentContext(context.Background(), session.TaskID, session, "Successor") {
		t.Fatal("resetAgentContext returned true while explicit cancellation owned an idle session")
	}
	select {
	case event := <-manager.events:
		t.Fatalf("provider operation %q started despite cancellation conflict", event)
	default:
	}
}

func TestHasActiveResetTurn_ReservedPromptOnly(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	svc.reservedPromptTurns.Store("session1", newReservedPromptTurn("reserved-turn"))

	active, err := svc.hasActiveResetTurn(context.Background(), "session1")
	if err != nil {
		t.Fatalf("hasActiveResetTurn: %v", err)
	}
	if !active {
		t.Fatal("reserved prompt turn was not treated as active")
	}
}

func TestResetAgentContext_ActiveTurnAllowsSuccessorPrompt(t *testing.T) {
	svc, repo, manager, session := newActiveResetTestService(t)
	manager.isAgentRunning = true
	manager.promptAgentFunc = func(
		_ context.Context,
		_ string,
		_ string,
		_ []v1.MessageAttachment,
		_ bool,
	) (*executor.PromptResult, error) {
		manager.events <- "prompt"
		return &executor.PromptResult{}, nil
	}
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	if !svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned false")
	}
	resetSession, err := svc.repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("reload reset session: %v", err)
	}
	step := &wfmodels.WorkflowStep{
		ID:         "step2",
		WorkflowID: "wf1",
		Name:       "Successor",
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterResetAgentContext},
		}},
	}
	if err := svc.autoStartStepPrompt(
		context.Background(), "task1", resetSession, step, "successor prompt", false, false,
	); err != nil {
		t.Fatalf("auto-start successor prompt: %v", err)
	}

	requireResetEvents(t, manager.events, "cancel", "reset", "prompt")
}

func TestResetAgentContext_CancelFailureStopsProviderReset(t *testing.T) {
	svc, _, manager, session := newActiveResetTestService(t)
	manager.cancelAgentErr = errors.New("cancel failed")

	if svc.resetAgentContext(context.Background(), "task1", session, "Successor") {
		t.Fatal("resetAgentContext returned true after cancellation failure")
	}

	requireResetEvents(t, manager.events, "cancel")
	if got := len(manager.restartProcessCalls); got != 0 {
		t.Fatalf("provider reset calls = %d, want 0", got)
	}
}

// TestResetAgentContext_ResetMarkerPrecedesCancellationWait fixes the ordering
// that made TestResetAgentContext_SerializesPromptAdmission flaky. Once reset
// has published its marker and yielded the session guard for cancellation, a
// prompt must reject the reset before it considers waiting on that cancellation.
func TestResetAgentContext_ResetMarkerPrecedesCancellationWait(t *testing.T) {
	ctx := context.Background()
	svc, _, manager, session := newActiveResetTestService(t)
	cancelEntered := make(chan struct{}, 1)
	cancelRelease := make(chan struct{})
	manager.cancelAgentEntered = cancelEntered
	manager.cancelAgentBlock = cancelRelease
	var releaseCancellation sync.Once
	releaseCancel := func() { releaseCancellation.Do(func() { close(cancelRelease) }) }
	t.Cleanup(releaseCancel)

	resetDone := make(chan bool, 1)
	go func() {
		resetDone <- svc.resetAgentContext(ctx, session.TaskID, session, "Successor")
	}()
	<-cancelEntered

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, _, _, _, _, err := svc.claimSessionRunningForPrompt(
		cancelledCtx, session.TaskID, session.ID, "", false, nil, nil, "", false,
	)
	if !errors.Is(err, ErrSessionResetInProgress) {
		t.Fatalf("prompt admission error = %v, want %v", err, ErrSessionResetInProgress)
	}

	releaseCancel()
	if resetOK := <-resetDone; !resetOK {
		t.Fatal("resetAgentContext returned false")
	}
}

func TestResetAgentContext_ResetMarkerPrecedesLifecycleCancellationWait(t *testing.T) {
	ctx := context.Background()
	svc, _, manager, session := newActiveResetTestService(t)
	cancelEntered := make(chan struct{}, 1)
	cancelRelease := make(chan struct{})
	manager.cancelAgentEntered = cancelEntered
	manager.cancelAgentBlock = cancelRelease
	var releaseCancellation sync.Once
	releaseCancel := func() { releaseCancellation.Do(func() { close(cancelRelease) }) }
	t.Cleanup(releaseCancel)

	resetDone := make(chan bool, 1)
	go func() {
		resetDone <- svc.resetAgentContext(ctx, session.TaskID, session, "Successor")
	}()
	<-cancelEntered

	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, _, _, _, err := svc.claimLifecycleSessionRunning(
		cancelledCtx, session.TaskID, session.ID, "",
	)
	if !errors.Is(err, ErrSessionResetInProgress) {
		t.Fatalf("lifecycle prompt admission error = %v, want %v", err, ErrSessionResetInProgress)
	}

	releaseCancel()
	if resetOK := <-resetDone; !resetOK {
		t.Fatal("resetAgentContext returned false")
	}
}

// TestResetAgentContext_SerializesPromptAdmission stages a prompt immediately
// before its final guarded claim, then starts reset while that claim is paused.
// If reset wins the shared guard, the marker rejects the prompt while reset is
// waiting on cancellation. If the prompt wins, reset must observe that turn
// and cancel it before replacing the provider session.
func TestResetAgentContext_SerializesPromptAdmission(t *testing.T) {
	ctx := context.Background()
	svc, repo, manager, session := newActiveResetTestService(t)
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting for input: %v", err)
	}
	cancelEntered := make(chan struct{}, 1)
	cancelRelease := make(chan struct{})
	manager.cancelAgentEntered = cancelEntered
	manager.cancelAgentBlock = cancelRelease
	var releaseCancellation sync.Once
	releaseCancel := func() { releaseCancellation.Do(func() { close(cancelRelease) }) }
	t.Cleanup(releaseCancel)

	guard, releaseGuard := svc.acquireCancelInFlightGuard(session.ID)
	guard.Lock()
	promptStarted := make(chan struct{})
	promptDone := make(chan error, 1)
	go func() {
		close(promptStarted)
		_, _, _, _, _, err := svc.claimSessionRunningForPrompt(
			ctx, session.TaskID, session.ID, "", false, nil, nil, "", false,
		)
		promptDone <- err
	}()
	<-promptStarted

	resetStarted := make(chan struct{})
	resetDone := make(chan bool, 1)
	go func() {
		close(resetStarted)
		resetDone <- svc.resetAgentContext(ctx, session.TaskID, session, "Successor")
	}()
	<-resetStarted

	guard.Unlock()
	releaseGuard()

	var promptErr error
	resetWonGuard := false
	select {
	case promptErr = <-promptDone:
	case <-cancelEntered:
		// Reset won the admission guard and is now waiting for its internal
		// cancellation. The prompt must observe the still-published marker.
		resetWonGuard = true
		select {
		case promptErr = <-promptDone:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for reset-blocked prompt admission")
		}
	}
	if resetWonGuard && !errors.Is(promptErr, ErrSessionResetInProgress) {
		t.Fatalf("prompt admission after reset won guard = %v, want %v", promptErr, ErrSessionResetInProgress)
	}
	releaseCancel()
	select {
	case resetOK := <-resetDone:
		if !resetOK {
			t.Fatal("resetAgentContext returned false")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for context reset")
	}

	if !resetWonGuard && promptErr != nil && !errors.Is(promptErr, ErrSessionResetInProgress) {
		t.Fatalf("prompt admission error = %v, want reset sentinel or successful claim", promptErr)
	}
	// A prompt that won the guard must have been visible as the active turn to
	// reset. The provider replacement is therefore ordered after cancellation.
	requireResetEvents(t, manager.events, "cancel", "reset")
}

func TestResetAgentContext_SeesLifecycleTurnBeforeProviderReset(t *testing.T) {
	ctx := context.Background()
	svc, repo, manager, session := newActiveResetTestService(t)
	turnIDValue, ok := svc.activeTurns.Load(session.ID)
	if !ok {
		t.Fatal("test setup did not create an active turn")
	}
	turnID, ok := turnIDValue.(string)
	if !ok || turnID == "" {
		t.Fatalf("active turn cache value = %#v, want turn ID", turnIDValue)
	}
	if err := svc.turnService.CompleteTurn(ctx, turnID); err != nil {
		t.Fatalf("complete setup turn: %v", err)
	}
	svc.activeTurns.Delete(session.ID)
	if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput

	claimed := make(chan struct{}, 1)
	svc.repo = lifecycleClaimSignalRepo{repoStore: repo, claimed: claimed}
	svc.taskRuntimeStateMu.Lock()

	promptDone := make(chan error, 1)
	go func() {
		_, _, err := svc.claimLifecyclePromptDispatch(ctx, session.TaskID, session.ID, "", nil)
		promptDone <- err
	}()
	<-claimed

	resetDone := make(chan bool, 1)
	go func() {
		resetDone <- svc.resetAgentContext(ctx, session.TaskID, session, "Successor")
	}()

	// The lifecycle claim is now blocked before its task-state reconciliation.
	// Reset must still see and cancel its turn before replacing provider context;
	// the lifecycle claim must then reject the cancelled turn.
	requireResetEvents(t, manager.events, "cancel", "reset")
	if resetOK := <-resetDone; !resetOK {
		t.Fatal("resetAgentContext returned false")
	}

	svc.taskRuntimeStateMu.Unlock()
	select {
	case err := <-promptDone:
		if !errors.Is(err, ErrSessionResetInProgress) {
			t.Fatalf("lifecycle prompt claim error = %v, want %v", err, ErrSessionResetInProgress)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for lifecycle prompt claim")
	}
}
