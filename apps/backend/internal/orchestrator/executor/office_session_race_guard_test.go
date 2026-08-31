package executor

// Tests for the office-session live-pair guard in the in-process fallback
// creator (Change 1b) and bounded recovery in EnsureSessionForAgentWithCreation
// (Change 3). See
// docs/specs/office/{requirements,system-design}/task-session-identity*.md.
// mockRepository does not implement officeTaskSessionCreator, so every
// EnsureSessionForAgent* test in this package already exercises
// persistOfficeSessionFallback rather than a repository-native creator.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// TestPersistOfficeSessionFallbackRefusesSecondLivePair drives the fallback
// creator directly (bypassing the lookup-then-create dance in
// EnsureSessionForAgentWithCreation) to prove it — not just the SQLite
// repository — enforces the live-pair guard: the first call for a pair
// succeeds, a second call for the SAME pair while the first is still live is
// refused with ErrOfficeSessionRaceConflict, and refusal writes nothing.
func TestPersistOfficeSessionFallbackRefusesSecondLivePair(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()
	const taskID = "task-office-fallback-live-pair"
	agent := "agent-fallback-live-pair"

	first := &models.TaskSession{ID: "fallback-live-pair-1", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated}
	if err := exec.persistOfficeSessionFallback(ctx, taskID, first); err != nil {
		t.Fatalf("first persistOfficeSessionFallback: %v", err)
	}

	second := &models.TaskSession{ID: "fallback-live-pair-2", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated}
	err := exec.persistOfficeSessionFallback(ctx, taskID, second)
	if !errors.Is(err, taskrepo.ErrOfficeSessionRaceConflict) {
		t.Fatalf("second persistOfficeSessionFallback error = %v, want ErrOfficeSessionRaceConflict", err)
	}

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions for task = %d, want 1 (refused call must not have written a row)", len(sessions))
	}
}

// TestPersistOfficeSessionFallbackAllowsCreateWhenExistingPairRowIsTerminal
// mirrors the repository-layer guard's terminal-retry allowance: a pair
// whose only existing row is terminal must not block a fresh create.
func TestPersistOfficeSessionFallbackAllowsCreateWhenExistingPairRowIsTerminal(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()
	const taskID = "task-office-fallback-terminal-retry"
	agent := "agent-fallback-terminal-retry"

	repo.sessions["fallback-terminal-retry-old"] = &models.TaskSession{
		ID: "fallback-terminal-retry-old", TaskID: taskID, AgentProfileID: agent,
		State: models.TaskSessionStateCompleted, StartedAt: time.Now().UTC(),
	}

	fresh := &models.TaskSession{ID: "fallback-terminal-retry-new", TaskID: taskID, AgentProfileID: agent, State: models.TaskSessionStateCreated}
	if err := exec.persistOfficeSessionFallback(ctx, taskID, fresh); err != nil {
		t.Fatalf("persistOfficeSessionFallback: %v", err)
	}

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions for task = %d, want 2", len(sessions))
	}
}

// TestPersistOfficeSessionFallbackSkipsGuardForEmptyAgentProfileID proves the
// fallback guard is bypassed for kanban-shaped rows with no agent profile,
// exactly like the repository-layer guard.
func TestPersistOfficeSessionFallbackSkipsGuardForEmptyAgentProfileID(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()
	const taskID = "task-office-fallback-empty-agent"

	first := &models.TaskSession{ID: "fallback-empty-agent-1", TaskID: taskID, State: models.TaskSessionStateCreated}
	if err := exec.persistOfficeSessionFallback(ctx, taskID, first); err != nil {
		t.Fatalf("first persistOfficeSessionFallback: %v", err)
	}
	second := &models.TaskSession{ID: "fallback-empty-agent-2", TaskID: taskID, State: models.TaskSessionStateCreated}
	if err := exec.persistOfficeSessionFallback(ctx, taskID, second); err != nil {
		t.Fatalf("second persistOfficeSessionFallback: %v", err)
	}

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions for task = %d, want 2 (guard must not apply to empty agent_profile_id)", len(sessions))
	}
}

// TestEnsureSessionForAgentWithCreation_ConcurrentCallersConvergeOnOneSession
// is the executor-level convergence test: N concurrent callers for the same
// (task, agent) pair, none of which observe an existing row at lookup time
// (simulated by starting them all from an empty repo and letting the
// per-task mutex in persistOfficeSession serialize the real race), must
// converge on exactly one row — the losers recover via re-read-and-reuse
// rather than surfacing the conflict to their caller (AC-003.1, AC-003.2).
func TestEnsureSessionForAgentWithCreation_ConcurrentCallersConvergeOnOneSession(t *testing.T) {
	const n = 4
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	results := make([]*models.TaskSession, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session, _, err := exec.EnsureSessionForAgentWithCreation(
				context.Background(), task, "agent-convergence", "profile-1", "exec-1", "",
			)
			results[i] = session
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d: %v", i, err)
		}
		if results[i] == nil {
			t.Fatalf("caller %d: session = nil, want non-nil", i)
		}
	}
	firstID := results[0].ID
	for i, session := range results {
		if session.ID != firstID {
			t.Fatalf("caller %d converged on %q, want %q (all callers must share one row)", i, session.ID, firstID)
		}
	}

	sessions, err := repo.ListTaskSessions(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions for task = %d, want exactly 1", len(sessions))
	}
	if len(repo.createTaskSessionCalls) != 1 {
		t.Fatalf("CreateTaskSession calls = %d, want exactly 1 (no duplicate CREATED event)", len(repo.createTaskSessionCalls))
	}
}

// TestEnsureSessionForAgentWithCreation_RecoveryStopsAfterTwoAttempts proves
// the bounded-retry contract (AC-003.3): if every create attempt loses the
// race AND the recovery re-read also fails to observe a winner (a
// pathological case that should not happen in practice, but the retry must
// still be bounded rather than looping), EnsureSessionForAgentWithCreation
// makes at most 2 create attempts total and returns the conflict rather than
// retrying forever.
func TestEnsureSessionForAgentWithCreation_RecoveryStopsAfterTwoAttempts(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	attempts := 0
	repo.createTaskSessionFunc = func(_ context.Context, _ *models.TaskSession) error {
		attempts++
		return fmt.Errorf("%w: perpetual loser", taskrepo.ErrOfficeSessionRaceConflict)
	}

	session, err := exec.EnsureSessionForAgent(
		context.Background(), task, "agent-bounded-retry", "profile-1", "exec-1", "",
	)
	if err == nil {
		t.Fatal("expected error when every create attempt and every recovery lookup fails")
	}
	if session != nil {
		t.Fatalf("session = %#v, want nil on unrecoverable conflict", session)
	}
	if !errors.Is(err, taskrepo.ErrOfficeSessionRaceConflict) {
		t.Fatalf("error = %v, want errors.Is match against ErrOfficeSessionRaceConflict", err)
	}
	if attempts != 2 {
		t.Fatalf("create attempts = %d, want exactly 2 (bounded retry)", attempts)
	}
}

// TestEnsureSessionForAgentWithCreation_NonConflictCreateFailureNotLaundered
// proves a genuine (non-conflict) create failure is returned as itself, not
// masked as a race conflict or silently swallowed by the recovery branch
// (AC-003.7).
func TestEnsureSessionForAgentWithCreation_NonConflictCreateFailureNotLaundered(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	wantErr := errors.New("disk full")
	repo.createTaskSessionFunc = func(_ context.Context, _ *models.TaskSession) error {
		return wantErr
	}

	session, err := exec.EnsureSessionForAgent(
		context.Background(), task, "agent-non-conflict-failure", "profile-1", "exec-1", "",
	)
	if session != nil {
		t.Fatalf("session = %#v, want nil", session)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want errors.Is match against the underlying failure", err)
	}
	if errors.Is(err, taskrepo.ErrOfficeSessionRaceConflict) {
		t.Fatalf("non-conflict failure %v must not classify as ErrOfficeSessionRaceConflict", err)
	}
}
