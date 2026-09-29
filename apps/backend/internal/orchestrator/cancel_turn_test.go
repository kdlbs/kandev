package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type cancelTurnFixture struct {
	svc      *Service
	repo     *sqliterepo.Repository
	agentMgr *mockAgentManager
}

func newCancelTurnFixture(t *testing.T) *cancelTurnFixture {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: "turn-1", TaskID: "t1", TaskSessionID: "s1", StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	agentMgr.currentPromptExecutionID = "exec-1"
	agentMgr.currentPromptGeneration.Store(3)
	agentMgr.currentPromptActivityEpoch.Store(7)
	svc := createEngineService(t, repo, newMockStepGetter(), agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	return &cancelTurnFixture{svc: svc, repo: repo, agentMgr: agentMgr}
}

func (f *cancelTurnFixture) activeTurnID(t *testing.T) string {
	t.Helper()
	turn, err := f.repo.GetActiveTurnBySessionID(context.Background(), "s1")
	if err != nil {
		return ""
	}
	return turn.ID
}

func TestCancelTurn_MatchingTurnIsCancelledOnceAndReconciledSilently(t *testing.T) {
	f := newCancelTurnFixture(t)
	var gotExec string
	var gotGen, gotEpoch uint64
	f.agentMgr.cancelAgentForPromptFunc = func(_ context.Context, _, exec string, gen, epoch uint64) error {
		gotExec, gotGen, gotEpoch = exec, gen, epoch
		return nil
	}

	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if n := f.agentMgr.cancelAgentForPromptCalls.Load(); n != 1 {
		t.Fatalf("prompt-fenced cancels = %d, want 1", n)
	}
	if gotExec != "exec-1" || gotGen != 3 || gotEpoch != 7 {
		t.Fatalf("cancel fenced on (%q,%d,%d), want (exec-1,3,7)", gotExec, gotGen, gotEpoch)
	}
	if id := f.activeTurnID(t); id != "" {
		t.Fatalf("turn %q is still active after the cancel", id)
	}
	session, err := f.repo.GetTaskSession(t.Context(), "s1")
	if err != nil {
		t.Fatal(err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want WAITING_FOR_INPUT", session.State)
	}
	if f.svc.isCancelInFlight("s1") {
		t.Fatal("cancellation claim was not released")
	}
}

func TestCancelTurn_MismatchAndNoActiveTurnCancelNothing(t *testing.T) {
	f := newCancelTurnFixture(t)
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-other"); !errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("mismatch err = %v", err)
	}
	if err := f.repo.CompleteTurn(t.Context(), "turn-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); !errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("no active turn err = %v", err)
	}
	if n := f.agentMgr.cancelAgentForPromptCalls.Load() + f.agentMgr.cancelAgentCalls.Load(); n != 0 {
		t.Fatalf("agent cancelled %d times for a fenced-off call", n)
	}
	if f.svc.isCancelInFlight("s1") {
		t.Fatal("a pre-claim mismatch took the cancellation claim")
	}
}

// swappingTurnService reports the expected turn on the first active-turn read
// (the pre-claim peek) and a manager's successor turn afterwards, the drain
// race between the peek and the guard.
type swappingTurnService struct {
	*repoTurnService
	first     string
	successor *models.Turn
	reads     atomic.Int32
}

func (s *swappingTurnService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	if s.reads.Add(1) == 1 {
		return &models.Turn{ID: s.first, TaskSessionID: sessionID}, nil
	}
	return s.successor, nil
}

func TestCancelTurn_SuccessorTurnInsideGuardIsNotCancelled(t *testing.T) {
	f := newCancelTurnFixture(t)
	f.svc.turnService = &swappingTurnService{
		repoTurnService: &repoTurnService{repo: f.repo},
		first:           "turn-1",
		successor:       &models.Turn{ID: "turn-manager", TaskSessionID: "s1"},
	}
	err := f.svc.CancelTurn(t.Context(), "s1", "turn-1")
	if !errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("err = %v, want ErrTurnNotActive", err)
	}
	if n := f.agentMgr.cancelAgentForPromptCalls.Load() + f.agentMgr.cancelAgentCalls.Load(); n != 0 {
		t.Fatalf("the manager's turn was cancelled (%d calls)", n)
	}
	if f.svc.isCancelInFlight("s1") {
		t.Fatal("claim not released after a mismatch")
	}
}

func TestCancelTurn_OtherCancellationInFlightIsRefusedNotJoined(t *testing.T) {
	f := newCancelTurnFixture(t)
	_, owner, _, accepted := f.svc.claimCancellationWithActionExclusive("s1", cancellationKindExplicit, nil)
	if !owner || !accepted {
		t.Fatal("test setup could not claim the session")
	}
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); !errors.Is(err, ErrCancelInFlight) {
		t.Fatalf("err = %v, want ErrCancelInFlight", err)
	}
	if n := f.agentMgr.cancelAgentForPromptCalls.Load() + f.agentMgr.cancelAgentCalls.Load(); n != 0 {
		t.Fatalf("joined the other cancellation and cancelled %d times", n)
	}
}

func TestCancelTurn_EscalatedCancelIsConfirmed(t *testing.T) {
	f := newCancelTurnFixture(t)
	f.agentMgr.cancelAgentForPromptFunc = func(context.Context, string, string, uint64, uint64) error {
		return lifecycle.ErrCancelEscalated
	}
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); err != nil {
		t.Fatalf("escalated cancel = %v, want nil", err)
	}
	if id := f.activeTurnID(t); id != "" {
		t.Fatalf("turn %q still active after an escalated cancel", id)
	}
}

func TestCancelTurn_PromptStartedAfterCaptureIsNotCancelled(t *testing.T) {
	f := newCancelTurnFixture(t)
	f.agentMgr.cancelAgentForPromptFunc = func(context.Context, string, string, uint64, uint64) error {
		return lifecycle.ErrPromptActivityNotOwned
	}
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); !errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("err = %v, want ErrTurnNotActive", err)
	}
	if id := f.activeTurnID(t); id != "turn-1" {
		t.Fatalf("active turn = %q, the turn must be left untouched", id)
	}
}

func TestCancelTurn_OtherCancelErrorIsReturnedAndLeavesTurnOpen(t *testing.T) {
	f := newCancelTurnFixture(t)
	boom := errors.New("agentctl unreachable")
	f.agentMgr.cancelAgentForPromptFunc = func(context.Context, string, string, uint64, uint64) error { return boom }
	err := f.svc.CancelTurn(t.Context(), "s1", "turn-1")
	if !errors.Is(err, boom) || errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("err = %v", err)
	}
	if id := f.activeTurnID(t); id != "turn-1" {
		t.Fatalf("active turn = %q, a failed cancel must leave the turn open", id)
	}
}

func TestCancelTurn_NoTrackedExecutionUsesThePlainCancel(t *testing.T) {
	f := newCancelTurnFixture(t)
	f.agentMgr.currentPromptExecutionID = ""
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); err != nil {
		t.Fatal(err)
	}
	if f.agentMgr.cancelAgentCalls.Load() != 1 || f.agentMgr.cancelAgentForPromptCalls.Load() != 0 {
		t.Fatalf("plain=%d fenced=%d", f.agentMgr.cancelAgentCalls.Load(), f.agentMgr.cancelAgentForPromptCalls.Load())
	}
}

func TestCancelTurn_IncompletePromptIdentityFailsClosed(t *testing.T) {
	f := newCancelTurnFixture(t)
	f.agentMgr.currentPromptGeneration.Store(0)
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); err == nil || errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("err = %v, want a hard error", err)
	}
	if n := f.agentMgr.cancelAgentForPromptCalls.Load() + f.agentMgr.cancelAgentCalls.Load(); n != 0 {
		t.Fatalf("cancelled %d times on an incomplete identity", n)
	}
}

func TestCancelTurn_FailsClosedOnMissingAuthorityAndEmptyArguments(t *testing.T) {
	f := newCancelTurnFixture(t)
	if err := f.svc.CancelTurn(t.Context(), "", "turn-1"); err == nil {
		t.Fatal("empty session id accepted")
	}
	if err := f.svc.CancelTurn(t.Context(), "s1", ""); err == nil {
		t.Fatal("empty turn id accepted")
	}
	f.svc.turnService = nil
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); err == nil || errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("nil turn service err = %v", err)
	}
	f.svc.turnService = &failingActiveTurnLookup{TurnService: &repoTurnService{repo: f.repo}, err: errors.New("db down")}
	if err := f.svc.CancelTurn(t.Context(), "s1", "turn-1"); err == nil || errors.Is(err, ErrTurnNotActive) {
		t.Fatalf("peek read error err = %v", err)
	}
}

func TestCancelTurn_CallerContextEndingDoesNotAbortTheDetachedOperation(t *testing.T) {
	f := newCancelTurnFixture(t)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	f.agentMgr.cancelAgentEntered = entered
	f.agentMgr.cancelAgentBlock = release
	f.agentMgr.cancelAgentForPromptFunc = nil
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- f.svc.CancelTurn(ctx, "s1", "turn-1") }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !f.svc.isCancelInFlight("s1") {
		t.Fatal("the detached operation stopped with its caller")
	}
	close(release)
	deadline := time.After(5 * time.Second)
	for f.svc.isCancelInFlight("s1") {
		select {
		case <-deadline:
			t.Fatal("detached operation never finished")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if id := f.activeTurnID(t); id != "" {
		t.Fatalf("turn %q still active after the detached cancel", id)
	}
}
