package orchestrator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// repoTurnService is a minimal TurnService used by lifecycle tests. It mirrors
// the production task service's behavior for the three methods the orchestrator
// uses, while sharing the same sqlite repo as the rest of the test setup so
// that DB-backed assertions stay coherent across components.
type repoTurnService struct {
	repo *sqliterepo.Repository
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

func (a *repoTurnService) CompleteTurn(ctx context.Context, turnID string) error {
	return a.repo.CompleteTurn(ctx, turnID)
}

func (a *repoTurnService) GetActiveTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := a.repo.GetActiveTurnBySessionID(ctx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return turn, err
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
