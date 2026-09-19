package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// repoTurnService is a minimal TurnService used by lifecycle tests. It mirrors
// the production task service's behavior for the three methods the orchestrator
// uses, while sharing the same sqlite repo as the rest of the test setup so
// that DB-backed assertions stay coherent across components.
type repoTurnService struct {
	repo *sqliterepo.Repository
}

type failingReservedTurnPublisher struct {
	TurnService
	err error
}

type completedReservedTurnPublisher struct {
	TurnService
}

type blockingReservedTurnMarker struct {
	TurnService
	entered chan struct{}
	release chan struct{}
}

type failingPromptTurnReconciler struct {
	TurnService
	err error
}

type failingReservedTurnRollback struct {
	TurnService
	err error
}

type failingActiveTurnLookup struct {
	TurnService
	err        error
	turn       *models.Turn
	startCalls int
}

func (s failingReservedTurnPublisher) PublishReservedTurn(context.Context, *models.Turn) error {
	return s.err
}

func (s completedReservedTurnPublisher) PublishReservedTurn(_ context.Context, turn *models.Turn) error {
	completedAt := time.Now().UTC()
	turn.CompletedAt = &completedAt
	return nil
}

func (s blockingReservedTurnMarker) MarkReservedTurnDispatchAttempted(
	ctx context.Context,
	turn *models.Turn,
) error {
	close(s.entered)
	<-s.release
	return s.TurnService.MarkReservedTurnDispatchAttempted(ctx, turn)
}

func (s failingPromptTurnReconciler) ReconcileUnpublishedPromptTurns(context.Context) (int, error) {
	return 0, s.err
}

func (s failingReservedTurnRollback) RollbackReservedTurn(context.Context, string, string) (bool, error) {
	return false, s.err
}

func (s *failingActiveTurnLookup) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return s.turn, s.err
}

func (s *failingActiveTurnLookup) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	s.startCalls++
	return s.TurnService.StartTurn(ctx, sessionID)
}

func (failingReservedTurnPublisher) GetActiveTurn(context.Context, string) (*models.Turn, error) {
	return nil, nil
}

func (a *repoTurnService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	now := time.Now().UTC()
	turn := &models.Turn{
		ID:            uuid.New().String(),
		TaskSessionID: sessionID,
		TaskID:        "task1", // matches the taskID seedSession uses
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := a.repo.CreateTurn(ctx, turn); err != nil {
		return nil, err
	}
	return turn, nil
}

func (a *repoTurnService) ReserveTurn(
	ctx context.Context,
	sessionID string,
	_ *models.PromptDispatchRecovery,
) (*models.Turn, error) {
	return a.StartTurn(ctx, sessionID)
}

func (a *repoTurnService) PublishReservedTurn(context.Context, *models.Turn) error { return nil }

func (a *repoTurnService) MarkReservedTurnDispatchAttempted(context.Context, *models.Turn) error {
	return nil
}

func (a *repoTurnService) RollbackReservedTurn(
	ctx context.Context,
	sessionID, turnID string,
) (bool, error) {
	return a.repo.DeleteTurnIfUnreferenced(ctx, sessionID, turnID)
}

func (a *repoTurnService) ReconcileUnpublishedPromptTurns(ctx context.Context) (int, error) {
	return a.repo.ReconcileUnpublishedPromptTurns(ctx)
}

func (a *repoTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	return a.repo.CompleteTurn(ctx, turnID)
}

func (a *repoTurnService) GetTurn(ctx context.Context, turnID string) (*models.Turn, error) {
	return a.repo.GetTurn(ctx, turnID)
}

func (a *repoTurnService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := a.repo.GetActiveTurnBySessionID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return turn, err
}

func (a *repoTurnService) UpdateTurn(ctx context.Context, turn *models.Turn) error {
	return a.repo.UpdateTurn(ctx, turn)
}

func (a *repoTurnService) PatchTurnMetadata(
	ctx context.Context,
	sessionID, turnID string,
	updates map[string]interface{},
) error {
	updated, _, err := a.repo.PatchTurnMetadata(ctx, sessionID, turnID, updates)
	if err == nil && !updated {
		return sql.ErrNoRows
	}
	return err
}

func (a *repoTurnService) AbandonOpenTurns(ctx context.Context, sessionID string) error {
	for {
		turn, err := a.GetActiveTurn(ctx, sessionID)
		if err != nil || turn == nil {
			return err
		}
		if err := a.repo.AbandonTurn(ctx, turn.ID); err != nil {
			return err
		}
	}
}

func openTurnCount(t *testing.T, repo *sqliterepo.Repository, sessionID string) int {
	t.Helper()
	turns, err := repo.ListTurnsBySession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	open := 0
	for _, turn := range turns {
		if turn.CompletedAt == nil {
			open++
		}
	}
	return open
}

func newTurnLifecycleTestService(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task1", "session1", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.turnService = &repoTurnService{repo: repo}
	return svc, repo
}

func TestApplyRouteActionRejectsActiveTurn(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	ctx := context.Background()
	if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
		t.Fatalf("start turn: %v", err)
	}
	svc.SetRouteActionHandler(func(context.Context, RouteActionRequest) (*RouteActionResult, error) {
		t.Fatal("route action handler must not run while a turn is active")
		return nil, nil
	})

	_, err := svc.ApplyRouteAction(ctx, RouteActionRequest{
		SessionID: "session1",
		Action:    RouteActionRetry,
	})
	if !errors.Is(err, ErrRouteActionActiveTurn) {
		t.Fatalf("ApplyRouteAction error = %v, want %v", err, ErrRouteActionActiveTurn)
	}
}

func TestApplyRouteActionRunsAfterTurnSettles(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	ctx := context.Background()
	turn, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("start turn: %v", err)
	}
	if err := svc.turnService.CompleteTurn(ctx, turn.ID); err != nil {
		t.Fatalf("complete turn: %v", err)
	}
	want := &RouteActionResult{SessionID: "session1", State: "starting"}
	svc.SetRouteActionHandler(func(_ context.Context, request RouteActionRequest) (*RouteActionResult, error) {
		if request.Action != RouteActionTryNext {
			t.Fatalf("route action = %q, want %q", request.Action, RouteActionTryNext)
		}
		return want, nil
	})

	got, err := svc.ApplyRouteAction(ctx, RouteActionRequest{
		SessionID: "session1",
		Action:    RouteActionTryNext,
	})
	if err != nil {
		t.Fatalf("ApplyRouteAction: %v", err)
	}
	if got != want {
		t.Fatalf("ApplyRouteAction result = %#v, want %#v", got, want)
	}
}

func TestStartTurnDropsPromptAfterActiveTurnLookupFailure(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	lookupErr := errors.New("active turn read failed")
	failing := &failingActiveTurnLookup{
		TurnService: svc.turnService,
		err:         lookupErr,
		turn:        &models.Turn{ID: "partial-active-turn"},
	}
	svc.turnService = failing

	turnID, created := svc.startTurnForSessionWithOwnership(context.Background(), "session1")
	if turnID != "" || created {
		t.Fatalf("start after lookup failure = (%q, %v), want empty drop", turnID, created)
	}
	if failing.startCalls != 0 {
		t.Fatalf("StartTurn calls = %d, want 0 after lookup failure", failing.startCalls)
	}
	turns, err := repo.ListTurnsBySession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	if len(turns) != 0 {
		t.Fatalf("turns after lookup failure = %#v, want none", turns)
	}
}

func TestReservedPromptCallbackOwnerCancelsAndDrains(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	svc.resetReservedPromptCallbacks()
	t.Cleanup(svc.stopReservedPromptCallbacks)
	reservation := newReservedPromptTurn("turn-callback-owner")
	entered := make(chan struct{})
	finished := make(chan struct{})
	if !svc.deferReservedPromptCallback(reservation, func(ctx context.Context) {
		close(entered)
		<-ctx.Done()
		close(finished)
	}) {
		t.Fatal("failed to defer reservation callback")
	}
	svc.reservedPromptTurns.Store("session1", reservation)
	if !svc.resolveReservedPromptTurn("session1", reservation.id, true) {
		t.Fatal("failed to resolve reservation")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reserved callback did not start")
	}

	stopDone := make(chan struct{})
	go func() {
		svc.stopReservedPromptCallbacks()
		close(stopDone)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("reserved callback did not observe owner cancellation")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("reserved callback owner did not drain before returning")
	}
}

func TestAgentReadyDetachedContextPreservesReservedCallbackShutdown(t *testing.T) {
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	callbackCtx, cancelCallback := reservedPromptCallbackContext(ownerCtx, context.Background())
	t.Cleanup(cancelCallback)
	detached := agentReadyDetachedContext(callbackCtx)
	cancelOwner()
	select {
	case <-detached.Done():
	case <-time.After(time.Second):
		t.Fatal("agent-ready detached context discarded callback-owner cancellation")
	}
}

func TestReservedPromptCallbackContextCanBeReattachedAfterRetry(t *testing.T) {
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	firstCtx, cancelFirst := reservedPromptCallbackContext(ownerCtx, context.Background())
	retryCtx := context.WithoutCancel(firstCtx)
	cancelFirst()

	secondCtx, cancelSecond := reservedPromptCallbackContext(ownerCtx, retryCtx)
	t.Cleanup(cancelSecond)
	select {
	case <-secondCtx.Done():
		t.Fatal("reattached callback inherited the prior invocation's cancellation")
	default:
	}

	cancelOwner()
	select {
	case <-secondCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("reattached callback did not preserve owner cancellation")
	}
}

// TestStartTurnAdoptsExistingDBTurn covers the dual-creation leak that left
// zombie turns whenever service.CreateMessage lazily started a turn for an
// inbound user message and the orchestrator's PromptTask then started another.
// Now startTurnForSession adopts the open DB turn instead of creating a second.
func TestStartTurnAdoptsExistingDBTurn(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	// Simulate service.CreateMessage having created a turn behind the
	// orchestrator's back (DB row only, not in activeTurns).
	preexisting, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}

	// PromptTask path calls startTurnForSession after setSessionRunning. With
	// the fix it must adopt the preexisting DB turn rather than create another.
	adopted := svc.startTurnForSession(ctx, "session1")
	if adopted != preexisting.ID {
		t.Fatalf("expected adoption of existing turn %q, got %q", preexisting.ID, adopted)
	}

	turns, err := repo.ListTurnsBySession(ctx, "session1")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d (zombies: %d)", len(turns), openTurnCount(t, repo, "session1"))
	}
}

// TestStartTurnIsIdempotentInMemory verifies that repeated calls do not stack
// turns when activeTurns already tracks one.
func TestStartTurnIsIdempotentInMemory(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	first := svc.startTurnForSession(ctx, "session1")
	second := svc.startTurnForSession(ctx, "session1")
	if first == "" {
		t.Fatal("expected a turn to be created")
	}
	if first != second {
		t.Fatalf("expected same turn ID, got %q then %q", first, second)
	}

	turns, err := repo.ListTurnsBySession(ctx, "session1")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
}

func TestReservedTurnCannotBeAdoptedBeforePublication(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	turnID, created, reserved, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	if err != nil || !created || reserved == nil || turnID == "" {
		t.Fatalf("reserve turn: id=%q created=%v reserved=%v err=%v", turnID, created, reserved, err)
	}
	if cached, ok := svc.activeTurns.Load("session1"); ok {
		t.Fatalf("unpublished reserved turn entered active cache: %v", cached)
	}

	adoptedID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", false, nil)
	if !errors.Is(err, ErrAgentPromptInProgress) || adoptedID != "" {
		t.Fatalf("second prompt adopted unpublished turn: id=%q err=%v", adoptedID, err)
	}
	if got := svc.getActiveTurnID("session1"); got != turnID {
		t.Fatalf("message turn ID = %q, want reserved turn %q", got, turnID)
	}
	if open := openTurnCount(t, repo, "session1"); open != 1 {
		t.Fatalf("open turns before publication = %d, want 1", open)
	}

	svc.promptDispatchCallback(ctx, "task1", "session1", reserved, nil, &promptDispatchOutcome{})()
	cached, ok := svc.activeTurns.Load("session1")
	if !ok || cached != turnID {
		t.Fatalf("published turn cache = %v, %v; want %q", cached, ok, turnID)
	}
}

func TestAcceptedReservationStaysActiveWhenPublicationWriteFails(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	reserved := &models.Turn{ID: "turn-accepted", TaskSessionID: "session1", TaskID: "task1"}
	svc.turnService = failingReservedTurnPublisher{
		TurnService: svc.turnService,
		err:         errors.New("turn metadata write failed"),
	}
	svc.reservedPromptTurns.Store("session1", newReservedPromptTurn(reserved.ID))

	svc.promptDispatchCallback(
		context.Background(), "task1", "session1", reserved, nil, &promptDispatchOutcome{},
	)()

	active, ok := svc.activeTurns.Load("session1")
	if !ok || active != reserved.ID {
		t.Fatalf("accepted reservation cache = %v, %v; want %q", active, ok, reserved.ID)
	}
	if pending := svc.reservedPromptTurnID("session1"); pending != "" {
		t.Fatalf("private reservation cache = %q, want cleared after agentctl acceptance", pending)
	}
}

func TestAcceptedReservationDoesNotRestoreTerminalOrMissingCache(t *testing.T) {
	for _, tc := range []struct {
		name      string
		publisher func(TurnService) TurnService
	}{
		{
			name: "terminal",
			publisher: func(delegate TurnService) TurnService {
				return completedReservedTurnPublisher{TurnService: delegate}
			},
		},
		{
			name: "missing",
			publisher: func(delegate TurnService) TurnService {
				return failingReservedTurnPublisher{TurnService: delegate, err: sql.ErrNoRows}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTurnLifecycleTestService(t)
			reserved := &models.Turn{ID: "turn-accepted", TaskSessionID: "session1", TaskID: "task1"}
			svc.turnService = tc.publisher(svc.turnService)
			svc.reservedPromptTurns.Store("session1", newReservedPromptTurn(reserved.ID))

			svc.promptDispatchCallback(
				context.Background(), "task1", "session1", reserved, nil, &promptDispatchOutcome{},
			)()

			if active, ok := svc.activeTurns.Load("session1"); ok {
				t.Fatalf("terminal or missing reservation restored active cache: %v", active)
			}
			if pending := svc.reservedPromptTurnID("session1"); pending != "" {
				t.Fatalf("private reservation cache = %q, want cleared", pending)
			}
		})
	}
}

func TestReservedTurnAttemptMarkingHoldsCancellationGuard(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	if err := repo.UpdateTaskSessionState(
		ctx,
		"session1",
		models.TaskSessionStateWaitingForInput,
		"",
	); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}
	marker := blockingReservedTurnMarker{
		TurnService: svc.turnService,
		entered:     make(chan struct{}),
		release:     make(chan struct{}),
	}
	var releaseOnce sync.Once
	releaseMarker := func() { releaseOnce.Do(func() { close(marker.release) }) }
	t.Cleanup(releaseMarker)
	svc.turnService = marker

	claimDone := make(chan error, 1)
	go func() {
		_, _, err := svc.claimPromptDispatch(
			ctx,
			"task1",
			"session1",
			"",
			false,
			true,
			&models.PromptDispatchRecovery{},
			nil,
			nil,
			"",
			false,
		)
		claimDone <- err
	}()
	select {
	case <-marker.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reserved-turn attempt marker")
	}

	guardAcquired := make(chan struct{})
	go func() {
		guard, release := svc.acquireCancelInFlightGuard("session1")
		defer release()
		guard.Lock()
		close(guardAcquired)
		guard.Unlock()
	}()
	select {
	case <-guardAcquired:
		t.Fatal("cancellation guard was released before attempt marking completed")
	case <-time.After(100 * time.Millisecond):
	}

	releaseMarker()
	select {
	case err := <-claimDone:
		if err != nil {
			t.Fatalf("claimPromptDispatch: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for prompt claim")
	}
	select {
	case <-guardAcquired:
	case <-time.After(time.Second):
		t.Fatal("cancellation guard remained blocked after attempt marking")
	}
}

func TestAgentReadyWaitsForReservedPromptGeneration(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	agentMgr := svc.agentManager.(*mockAgentManager)
	agentMgr.currentPromptExecutionID = "exec1"
	agentMgr.currentPromptGeneration.Store(1)

	turnID, _, reserved, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	t.Cleanup(func() { svc.resolveReservedPromptTurn("session1", turnID, false) })
	if err != nil {
		t.Fatalf("reserve successor turn: %v", err)
	}
	reserved.Metadata = map[string]interface{}{
		models.TurnMetaKeyPromptDispatchPending:   true,
		models.TurnMetaKeyPromptDispatchAttempted: true,
	}
	if err := repo.UpdateTurn(ctx, reserved); err != nil {
		t.Fatalf("mark successor dispatch attempted: %v", err)
	}

	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID: "task1", SessionID: "session1",
			AgentExecutionID: "exec1", PromptGeneration: 1,
		})
		close(readyDone)
	}()
	select {
	case <-readyDone:
		t.Fatal("predecessor ready completed the reserved successor before dispatch resolution")
	case <-time.After(200 * time.Millisecond):
	}

	agentMgr.currentPromptGeneration.Store(2)
	svc.promptDispatchCallback(ctx, "task1", "session1", reserved, nil, &promptDispatchOutcome{})()
	select {
	case <-readyDone:
	case <-time.After(time.Second):
		t.Fatal("predecessor ready did not resume after dispatch resolution")
	}
	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("load successor after predecessor ready: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatalf("predecessor ready completed reserved successor at %v", turn.CompletedAt)
	}
}

func TestAgentReadyRevalidatesAfterReservedPromptRollback(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	stepGetter := svc.workflowStepGetter.(*mockStepGetter)
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1"}
	agentMgr := svc.agentManager.(*mockAgentManager)
	agentMgr.currentPromptExecutionID = "exec1"
	agentMgr.currentPromptGeneration.Store(1)

	turnID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	t.Cleanup(func() { svc.resolveReservedPromptTurn("session1", turnID, false) })
	if err != nil {
		t.Fatalf("reserve successor turn: %v", err)
	}
	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID: "task1", SessionID: "session1",
			AgentExecutionID: "exec1", PromptGeneration: 1,
		})
		close(readyDone)
	}()
	select {
	case <-readyDone:
		t.Fatal("predecessor ready returned before reservation resolution")
	case <-time.After(200 * time.Millisecond):
	}

	svc.rollbackReservedPromptTurn(ctx, "session1", turnID)
	select {
	case <-readyDone:
	case <-time.After(time.Second):
		t.Fatal("predecessor ready did not resume after reservation rollback")
	}
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session after ready: %v", err)
	}
	if session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting {
		t.Fatalf("rolled-back reservation stranded predecessor in %q", session.State)
	}
}

func TestAgentReadyDetachesDeliveryCancellationAfterReservedPromptRollback(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	stepGetter := svc.workflowStepGetter.(*mockStepGetter)
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1"}
	agentMgr := svc.agentManager.(*mockAgentManager)
	agentMgr.currentPromptExecutionID = "exec1"
	agentMgr.currentPromptGeneration.Store(1)

	turnID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(context.Background(), "session1", true, nil)
	t.Cleanup(func() { svc.resolveReservedPromptTurn("session1", turnID, false) })
	if err != nil {
		t.Fatalf("reserve successor turn: %v", err)
	}
	deliveryCtx, cancelDelivery := context.WithCancel(context.Background())
	t.Cleanup(cancelDelivery)
	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(deliveryCtx, watcher.AgentEventData{
			TaskID: "task1", SessionID: "session1",
			AgentExecutionID: "exec1", PromptGeneration: 1,
		})
		close(readyDone)
	}()
	select {
	case <-readyDone:
		t.Fatal("predecessor ready returned before reservation resolution")
	case <-time.After(200 * time.Millisecond):
	}

	cancelDelivery()
	svc.rollbackReservedPromptTurn(context.Background(), "session1", turnID)
	select {
	case <-readyDone:
	case <-time.After(time.Second):
		t.Fatal("predecessor ready did not resume after reservation rollback")
	}
	session, err := repo.GetTaskSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("load session after ready: %v", err)
	}
	if session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateStarting {
		t.Fatalf("canceled delivery context stranded predecessor in %q", session.State)
	}
}

func TestAgentReadyReconcilesWhenReservedPromptRollbackFinishesAfterWaitTimeout(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	svc.agentReadyReservationWaitTimeout = 20 * time.Millisecond
	ctx := context.Background()
	stepGetter := svc.workflowStepGetter.(*mockStepGetter)
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1"}
	agentMgr := svc.agentManager.(*mockAgentManager)
	agentMgr.currentPromptExecutionID = "exec1"
	agentMgr.currentPromptGeneration.Store(1)

	turnID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	t.Cleanup(func() { svc.resolveReservedPromptTurn("session1", turnID, false) })
	if err != nil {
		t.Fatalf("reserve successor turn: %v", err)
	}
	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID: "task1", SessionID: "session1",
			AgentExecutionID: "exec1", PromptGeneration: 1,
		})
		close(readyDone)
	}()
	select {
	case <-readyDone:
	case <-time.After(time.Second):
		t.Fatal("ready handler did not return after reservation wait timeout")
	}

	svc.rollbackReservedPromptTurn(ctx, "session1", turnID)
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		session, loadErr := repo.GetTaskSession(ctx, "session1")
		if loadErr != nil {
			t.Fatalf("load session after late rollback: %v", loadErr)
		}
		if session.State != models.TaskSessionStateRunning && session.State != models.TaskSessionStateStarting {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("late rollback stranded predecessor in %q", session.State)
		case <-ticker.C:
		}
	}
}

func TestAgentReadyWaitsForReservedTurnThenDropsGenerationlessEventOnRollback(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	agentMgr := svc.agentManager.(*mockAgentManager)
	agentMgr.currentPromptExecutionID = "exec1"

	turnID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	t.Cleanup(func() { svc.resolveReservedPromptTurn("session1", turnID, false) })
	if err != nil {
		t.Fatalf("reserve successor turn: %v", err)
	}
	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{
			TaskID: "task1", SessionID: "session1",
			AgentExecutionID: "exec1", PromptGeneration: 0,
		})
		close(readyDone)
	}()
	select {
	case <-readyDone:
		t.Fatal("generationless ready returned before reservation resolution")
	case <-time.After(200 * time.Millisecond):
	}

	svc.rollbackReservedPromptTurn(ctx, "session1", turnID)
	select {
	case <-readyDone:
	case <-time.After(time.Second):
		t.Fatal("generationless ready did not resume after reservation rollback")
	}
	if _, err := repo.GetTurn(ctx, turnID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rolled-back reserved turn remains: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("load session after generationless ready: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("generationless ready changed session state to %q", session.State)
	}
}

func TestStartFailsClosedWhenPromptTurnRecoveryFails(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	recoveryErr := errors.New("recover unpublished prompt turn")
	svc.turnService = failingPromptTurnReconciler{TurnService: svc.turnService, err: recoveryErr}

	err := svc.Start(context.Background())
	if !errors.Is(err, recoveryErr) {
		t.Fatalf("Start error = %v, want %v", err, recoveryErr)
	}
	if svc.running {
		t.Fatal("service remained running after prompt-turn recovery failure")
	}
}

func TestStartFailsClosedWithoutTurnService(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	svc.turnService = nil

	err := svc.Start(context.Background())
	if err == nil {
		_ = svc.Stop()
		t.Fatal("Start error = nil, want missing turn service failure")
	}
	if svc.running {
		t.Fatal("service remained running without prompt-turn recovery")
	}
}

func TestPublishedReservedTurnCannotBeRolledBack(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	turnID, _, reserved, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	if err != nil {
		t.Fatalf("reserve turn: %v", err)
	}

	svc.promptDispatchCallback(ctx, "task1", "session1", reserved, nil, &promptDispatchOutcome{})()
	svc.rollbackReservedPromptTurn(ctx, "session1", turnID)

	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("load accepted turn after late rollback: %v", err)
	}
	if turn == nil {
		t.Fatal("late prompt failure deleted an already-published turn")
	}
}

func TestRejectedReservedTurnClearsPrivateCache(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	turnID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	if err != nil {
		t.Fatalf("reserve turn: %v", err)
	}
	svc.rollbackReservedPromptTurn(ctx, "session1", turnID)
	if reserved := svc.reservedPromptTurnID("session1"); reserved != "" {
		t.Fatalf("reserved cache after rollback = %q, want empty", reserved)
	}
	if _, ok := svc.activeTurns.Load("session1"); ok {
		t.Fatal("rolled-back reservation entered active cache")
	}
	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("open turns after rollback = %d, want 0", open)
	}

	successorID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", false, nil)
	if err != nil || successorID == "" || successorID == turnID {
		t.Fatalf("successor after rollback: id=%q rejected=%q err=%v", successorID, turnID, err)
	}
}

func TestFailedReservedTurnRollbackKeepsSessionQuarantined(t *testing.T) {
	svc, _ := newTurnLifecycleTestService(t)
	ctx := context.Background()
	turnID, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", true, nil)
	if err != nil {
		t.Fatalf("reserve turn: %v", err)
	}
	reservation := svc.reservedPromptTurn("session1")
	rollbackErr := errors.New("ambiguous rollback commit")
	svc.turnService = failingReservedTurnRollback{TurnService: svc.turnService, err: rollbackErr}

	svc.rollbackReservedPromptTurn(ctx, "session1", turnID)
	if pending := svc.reservedPromptTurnID("session1"); pending != turnID {
		t.Fatalf("reservation after failed rollback = %q, want quarantine %q", pending, turnID)
	}
	select {
	case <-reservation.done:
		t.Fatal("failed rollback resolved the reservation waiter")
	default:
	}
	if _, _, _, err := svc.startTurnForSessionWithOwnershipChecked(ctx, "session1", false, nil); !errors.Is(err, ErrAgentPromptInProgress) {
		t.Fatalf("new prompt after failed rollback error = %v, want %v", err, ErrAgentPromptInProgress)
	}
}

// TestCompleteTurnClosesUntrackedDBTurn covers the user-cancel zombie path:
// completeTurnForSession previously bailed when activeTurns was empty, leaving
// the DB row open (e.g. after a backend restart wiped activeTurns, or after
// the dual-creation drift left a turn the orchestrator never tracked).
func TestCompleteTurnClosesUntrackedDBTurn(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	// Simulate an open turn the orchestrator never tracked.
	if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	if open := openTurnCount(t, repo, "session1"); open != 1 {
		t.Fatalf("expected 1 open turn before complete, got %d", open)
	}

	svc.completeTurnForSession(ctx, "session1")

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after complete, got %d", open)
	}
}

// TestCompleteTurnMopsUpMultipleZombies verifies the loop that cleans up
// pre-existing zombies from before this fix shipped.
func TestCompleteTurnMopsUpMultipleZombies(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
			t.Fatalf("seed turn %d: %v", i, err)
		}
	}
	if open := openTurnCount(t, repo, "session1"); open != 4 {
		t.Fatalf("expected 4 open turns, got %d", open)
	}

	svc.completeTurnForSession(ctx, "session1")

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after sweep, got %d", open)
	}
}

// TestCompleteTurnRespectsIterationCap verifies that the cleanup loop closes at
// most maxIterations (16) turns per call and that a subsequent call mops up the
// remainder. Locks in the cap behavior so future tweaks don't accidentally turn
// the safety bound into a footgun.
func TestCompleteTurnRespectsIterationCap(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	const seeded = 20
	for i := 0; i < seeded; i++ {
		if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
			t.Fatalf("seed turn %d: %v", i, err)
		}
	}
	if open := openTurnCount(t, repo, "session1"); open != seeded {
		t.Fatalf("expected %d open turns, got %d", seeded, open)
	}

	svc.completeTurnForSession(ctx, "session1")

	if open := openTurnCount(t, repo, "session1"); open != seeded-16 {
		t.Fatalf("expected %d open turns after first sweep (cap=16), got %d", seeded-16, open)
	}

	svc.completeTurnForSession(ctx, "session1")

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after second sweep, got %d", open)
	}
}

// TestCompleteTurnIsIdempotent covers the cancel-after-agent-already-completed
// race: the agent's stream complete event closed the turn, then CancelAgent
// runs completeTurnForSession again. Should be a no-op, not an error.
func TestCompleteTurnIsIdempotent(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	turnID := svc.startTurnForSession(ctx, "session1")
	if turnID == "" {
		t.Fatal("expected turn to be created")
	}

	svc.completeTurnForSession(ctx, "session1")
	svc.completeTurnForSession(ctx, "session1") // second call: no-op

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns, got %d", open)
	}
	turns, err := repo.ListTurnsBySession(ctx, "session1")
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	if len(turns) != 1 {
		t.Fatalf("expected exactly 1 turn (no phantom), got %d", len(turns))
	}
}

// TestAbandonOpenTurnsZeroesDuration covers the resume-orphan path: turns
// left open by a previous crash must close with completed_at = started_at so
// the UI's running timer doesn't count from a stale start, and analytics
// doesn't accumulate hours of dead time.
func TestAbandonOpenTurnsZeroesDuration(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	stale, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}

	if err := svc.turnService.AbandonOpenTurns(ctx, "session1"); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after abandon, got %d", open)
	}

	got, err := repo.GetTurn(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected completed_at to be set after abandon")
	}
	if !got.CompletedAt.Equal(got.StartedAt) {
		t.Fatalf("expected completed_at == started_at (zero duration), got started=%v completed=%v",
			got.StartedAt, *got.CompletedAt)
	}
}

// TestAbandonOpenTurnsHandlesMultipleZombies verifies the loop closes every
// open turn for the session, mirroring the behavior of completeTurnForSession.
func TestAbandonOpenTurnsHandlesMultipleZombies(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		if _, err := svc.turnService.StartTurn(ctx, "session1"); err != nil {
			t.Fatalf("seed turn %d: %v", i, err)
		}
	}
	if open := openTurnCount(t, repo, "session1"); open != 4 {
		t.Fatalf("expected 4 open turns before abandon, got %d", open)
	}

	if err := svc.turnService.AbandonOpenTurns(ctx, "session1"); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after abandon, got %d", open)
	}
}

func TestReconcileSessionsOnStartupAbandonsOpenTurns(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	stale, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session1",
		SessionID:        "session1",
		TaskID:           "task1",
		AgentExecutionID: "exec-before-restart",
		Status:           models.ExecutorRunningStatusStarting,
	}); err != nil {
		t.Fatalf("seed executors_running: %v", err)
	}

	svc.reconcileSessionsOnStartup(ctx)

	session, err := repo.GetTaskSession(ctx, "session1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want WAITING_FOR_INPUT", session.State)
	}
	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after startup reconciliation, got %d", open)
	}
	got, err := repo.GetTurn(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected startup reconciliation to set completed_at")
	}
	if !got.CompletedAt.Equal(got.StartedAt) {
		t.Fatalf("expected completed_at == started_at, got started=%v completed=%v",
			got.StartedAt, *got.CompletedAt)
	}
}

func TestReconcileSessionsOnStartupRestoresUnpublishedClarificationDispatch(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()
	startedAt := time.Now().UTC().Add(-time.Minute)
	completedAt := startedAt.Add(time.Second)

	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-clarification", TaskSessionID: "session1", TaskID: "task1",
		StartedAt: startedAt, CompletedAt: &completedAt,
	}); err != nil {
		t.Fatalf("create clarification turn: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-clarification", TaskSessionID: "session1", TaskID: "task1",
		TurnID: "turn-clarification", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, Content: "Continue?",
		Metadata: map[string]interface{}{
			"pending_id": "pending-restart", "question_id": "q1",
			"status": "answered", "response": map[string]interface{}{"custom_text": "yes"},
		},
		CreatedAt: startedAt,
	}); err != nil {
		t.Fatalf("create terminal clarification: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-unpublished", TaskSessionID: "session1", TaskID: "task1",
		StartedAt: startedAt.Add(time.Minute),
		Metadata: map[string]interface{}{
			"prompt_dispatch_pending":                   true,
			"prompt_dispatch_clarification_pending_id":  "pending-restart",
			"prompt_dispatch_clarification_turn_id":     "turn-clarification",
			"prompt_dispatch_clarification_message_ids": []string{"message-clarification"},
		},
	}); err != nil {
		t.Fatalf("create unpublished reservation: %v", err)
	}

	// No executors_running row exists. Recovery must still run before the
	// ordinary startup reconciler takes its current early-return path.
	svc.reconcileSessionsOnStartup(ctx)

	if _, err := repo.GetTurn(ctx, "turn-unpublished"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unpublished turn error = %v, want deleted reservation", err)
	}
	message, err := repo.GetMessage(ctx, "message-clarification")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if message.Metadata["status"] != "pending" || message.Metadata["response"] != nil {
		t.Fatalf("recovered metadata = %#v, want pending without response", message.Metadata)
	}
}

func TestReconcileTerminalSessionOnStartupAbandonsOpenTurns(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	stale, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, "session1", models.TaskSessionStateCompleted, ""); err != nil {
		t.Fatalf("mark session completed: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session1",
		SessionID:        "session1",
		TaskID:           "task1",
		AgentExecutionID: "exec-before-restart",
		Status:           models.ExecutorRunningStatusRunning,
	}); err != nil {
		t.Fatalf("seed executors_running: %v", err)
	}

	svc.reconcileSessionsOnStartup(ctx)

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after terminal startup reconciliation, got %d", open)
	}
	got, err := repo.GetTurn(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected terminal startup reconciliation to set completed_at")
	}
	if !got.CompletedAt.Equal(got.StartedAt) {
		t.Fatalf("expected completed_at == started_at, got started=%v completed=%v",
			got.StartedAt, *got.CompletedAt)
	}
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "session1"); !errors.Is(err, models.ErrExecutorRunningNotFound) {
		t.Fatalf("expected executor row cleanup, got err=%v", err)
	}
}

func TestReconcileFailedSessionOnStartupAbandonsOpenTurns(t *testing.T) {
	svc, repo := newTurnLifecycleTestService(t)
	ctx := context.Background()

	stale, err := svc.turnService.StartTurn(ctx, "session1")
	if err != nil {
		t.Fatalf("seed turn: %v", err)
	}
	if err := repo.UpdateTaskSessionState(ctx, "session1", models.TaskSessionStateFailed, "boom"); err != nil {
		t.Fatalf("mark session failed: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "session1",
		SessionID:        "session1",
		TaskID:           "task1",
		AgentExecutionID: "exec-before-restart",
		Status:           models.ExecutorRunningStatusFailed,
		Resumable:        true,
		ResumeToken:      "resume-token",
	}); err != nil {
		t.Fatalf("seed executors_running: %v", err)
	}

	svc.reconcileSessionsOnStartup(ctx)

	if open := openTurnCount(t, repo, "session1"); open != 0 {
		t.Fatalf("expected 0 open turns after failed startup reconciliation, got %d", open)
	}
	got, err := repo.GetTurn(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected failed startup reconciliation to set completed_at")
	}
	if !got.CompletedAt.Equal(got.StartedAt) {
		t.Fatalf("expected completed_at == started_at, got started=%v completed=%v",
			got.StartedAt, *got.CompletedAt)
	}
	running, err := repo.GetExecutorRunningBySessionID(ctx, "session1")
	if err != nil {
		t.Fatalf("expected resumable failed executor row to be preserved: %v", err)
	}
	if running.ResumeToken != "resume-token" {
		t.Fatalf("resume token = %q, want preserved token", running.ResumeToken)
	}
}
