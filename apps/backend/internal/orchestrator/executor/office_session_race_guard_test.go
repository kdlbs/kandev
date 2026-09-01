package executor

// Tests for the office-session live-pair guard in the in-process fallback
// creator (Change 1b) and bounded recovery in EnsureSessionForAgentWithCreation
// (Change 3). See
// docs/specs/office/{requirements,system-design}/task-session-identity*.md.
// mockRepository does not implement officeTaskSessionCreator, so every
// EnsureSessionForAgent* test in this package that uses it directly exercises
// persistOfficeSessionFallback rather than a repository-native creator.
// officeTaskSessionCreatingRepository (below) is the one exception: it wraps
// mockRepository to add officeTaskSessionCreator, for tests that must prove
// persistOfficeSession's other branch — the one taken in production, where
// the repository's own transaction is the only serialization and the
// executor's per-task fallback lock is never acquired at all.

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

// TestPersistOfficeSessionFallbackIgnoresLiveSessionForDifferentAgentOnSameTask
// proves the fallback guard restricts to the (task, agent_profile_id) PAIR
// before it ever asks whether an existing row is terminal: a different
// agent's live (non-terminal) session on the same task must not block
// creating a session for a new agent. Without the pair restriction running
// first, a task-wide "any non-terminal session exists" check would also
// reject this call — and every other fallback test in this file seeds either
// no other agent's session or only a terminal one, so none of them would
// catch that regression.
func TestPersistOfficeSessionFallbackIgnoresLiveSessionForDifferentAgentOnSameTask(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()
	const taskID = "task-office-fallback-other-agent-live"

	repo.sessions["fallback-other-agent-live"] = &models.TaskSession{
		ID: "fallback-other-agent-live", TaskID: taskID, AgentProfileID: "agent-a",
		State: models.TaskSessionStateRunning, StartedAt: time.Now().UTC(),
	}

	fresh := &models.TaskSession{
		ID: "fallback-other-agent-new", TaskID: taskID, AgentProfileID: "agent-b",
		State: models.TaskSessionStateCreated,
	}
	if err := exec.persistOfficeSessionFallback(ctx, taskID, fresh); err != nil {
		t.Fatalf("persistOfficeSessionFallback: %v, want nil (a different agent's live session must not block creation)", err)
	}

	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions for task = %d, want 2", len(sessions))
	}
}

// TestEnsureSessionForAgentWithCreation_ConcurrentCallersConvergeOnOneSession
// is the executor-level convergence test: N concurrent callers for the same
// (task, agent) pair, none of which observe an existing row at lookup time
// (simulated by starting them all from an empty repo and letting the
// per-task mutex in persistOfficeSession serialize the real race), must
// converge on exactly one row — the losers recover via re-read-and-reuse
// rather than surfacing the conflict to their caller (AC-003.1, AC-003.2).
// AC-003.2 is specifically a one-creator/N-1-reusers contract, not just a
// same-ID contract, so this also captures and asserts the "created by this
// call" boolean each caller receives: exactly one true among the n results.
func TestEnsureSessionForAgentWithCreation_ConcurrentCallersConvergeOnOneSession(t *testing.T) {
	const n = 4
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	results := make([]*models.TaskSession, n)
	created := make([]bool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session, wasCreated, err := exec.EnsureSessionForAgentWithCreation(
				context.Background(), task, "agent-convergence", "profile-1", "exec-1", "",
			)
			results[i] = session
			created[i] = wasCreated
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

	creatorCount := 0
	for i, wasCreated := range created {
		if wasCreated {
			creatorCount++
		}
		t.Logf("caller %d created=%v", i, wasCreated)
	}
	if creatorCount != 1 {
		t.Fatalf("callers reporting created=true = %d, want exactly 1 (one creator, %d reusers)", creatorCount, n-1)
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

// TestTryReuseExistingSession_IdleCASMismatchDoesNotResurrectTerminalSession
// proves the IDLE→RUNNING flip in tryReuseExistingSession is guarded by a CAS
// on the observed state (SEC-001): a caller that read the row as IDLE before
// a concurrent terminal transition (COMPLETED/FAILED/CANCELLED) landed must
// not have its stale write resurrect the now-terminal row to RUNNING. Drives
// the CAS mismatch directly — the mock's underlying row is already COMPLETED
// while the argument passed to tryReuseExistingSession still reflects the
// stale IDLE read — rather than relying on goroutine timing.
func TestTryReuseExistingSession_IdleCASMismatchDoesNotResurrectTerminalSession(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()

	const taskID = "task-idle-cas-mismatch"
	current := &models.TaskSession{ID: "idle-cas-mismatch", TaskID: taskID, State: models.TaskSessionStateCompleted}
	repo.sessions[current.ID] = current

	staleView := &models.TaskSession{ID: current.ID, TaskID: taskID, State: models.TaskSessionStateIdle}
	result, decision := exec.tryReuseExistingSession(ctx, staleView)

	if decision != reuseDecisionTerminal {
		t.Fatalf("decision = %v, want reuseDecisionTerminal", decision)
	}
	if result != nil {
		t.Fatalf("result = %#v, want nil", result)
	}
	stored := repo.sessions[current.ID]
	if stored.State != models.TaskSessionStateCompleted {
		t.Fatalf("stored session state = %v, want unchanged COMPLETED (must not be resurrected to RUNNING)", stored.State)
	}
}

// TestTryReuseExistingSession_IdleFlipsToRunningWhenStateStillMatches proves
// the happy path still works with the CAS write: an IDLE row whose state has
// not changed since the read flips to RUNNING and is returned as reused.
func TestTryReuseExistingSession_IdleFlipsToRunningWhenStateStillMatches(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx := context.Background()

	const taskID = "task-idle-cas-match"
	current := &models.TaskSession{ID: "idle-cas-match", TaskID: taskID, State: models.TaskSessionStateIdle}
	repo.sessions[current.ID] = current

	view := &models.TaskSession{ID: current.ID, TaskID: taskID, State: models.TaskSessionStateIdle}
	result, decision := exec.tryReuseExistingSession(ctx, view)

	if decision != reuseDecisionReused {
		t.Fatalf("decision = %v, want reuseDecisionReused", decision)
	}
	if result == nil || result.State != models.TaskSessionStateRunning {
		t.Fatalf("result = %#v, want non-nil session with State RUNNING", result)
	}
	if repo.updateTaskSessionIfCurrentCalls != 1 {
		t.Fatalf("UpdateTaskSessionIfCurrentState calls = %d, want 1", repo.updateTaskSessionIfCurrentCalls)
	}
	stored := repo.sessions[current.ID]
	if stored.State != models.TaskSessionStateRunning {
		t.Fatalf("stored session state = %v, want RUNNING", stored.State)
	}
}

// officeTaskSessionCreatingRepository wraps mockRepository to additionally
// implement officeTaskSessionCreator, mirroring the live-pair-then-create
// transaction in the real Repository.CreateOfficeTaskSession (session.go):
// count live (non-terminal) sessions for the (task, agent_profile) pair,
// refuse with ErrOfficeSessionRaceConflict if any exist, otherwise mark
// task-initial origin on the first session for the task and persist. The
// whole check-then-write runs under creatingMu, standing in for the real
// implementation's DB transaction — persistOfficeSession takes this branch
// precisely when it will NOT also acquire the executor's own per-task lock,
// so this double's serialization must come from the same place production's
// does: the "transaction," not caller-side locking.
type officeTaskSessionCreatingRepository struct {
	*mockRepository
	creatingMu               sync.Mutex
	createOfficeTaskSessionN int
}

func newOfficeTaskSessionCreatingRepository() *officeTaskSessionCreatingRepository {
	return &officeTaskSessionCreatingRepository{mockRepository: newMockRepository()}
}

func (r *officeTaskSessionCreatingRepository) CreateOfficeTaskSession(
	ctx context.Context, session *models.TaskSession,
) error {
	r.creatingMu.Lock()
	defer r.creatingMu.Unlock()
	r.createOfficeTaskSessionN++

	r.mu.Lock()
	var sessionCount, liveCount int
	for _, existing := range r.sessions {
		if existing.TaskID != session.TaskID {
			continue
		}
		sessionCount++
		if session.AgentProfileID != "" && existing.AgentProfileID == session.AgentProfileID &&
			!isStopTerminalSessionState(existing.State) {
			liveCount++
		}
	}
	r.mu.Unlock()

	if liveCount > 0 {
		return taskrepo.ErrOfficeSessionRaceConflict
	}
	if sessionCount == 0 {
		if session.Metadata == nil {
			session.Metadata = make(map[string]interface{})
		}
		session.Metadata[models.SessionMetaKeyOrigin] = models.SessionOriginTaskInitial
	}
	return r.CreateTaskSession(ctx, session)
}

// TestEnsureSessionForAgentWithCreation_ConvergesThroughRepositoryNativeCreator
// is TestEnsureSessionForAgentWithCreation_ConcurrentCallersConvergeOnOneSession's
// counterpart for the officeTaskSessionCreator branch of persistOfficeSession:
// N concurrent callers for the same (task, agent) pair, racing against a
// repository double that implements officeTaskSessionCreator, must still
// converge on exactly one row via that branch specifically — not the
// in-process fallback lock, which is never acquired on this path. Proves the
// interface-detection routing in persistOfficeSession is exercised end to
// end, not just the mock-fallback convergence covered elsewhere in this file.
func TestEnsureSessionForAgentWithCreation_ConvergesThroughRepositoryNativeCreator(t *testing.T) {
	const n = 4
	repo := newOfficeTaskSessionCreatingRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	results := make([]*models.TaskSession, n)
	created := make([]bool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			session, wasCreated, err := exec.EnsureSessionForAgentWithCreation(
				context.Background(), task, "agent-native-creator-convergence", "profile-1", "exec-1", "",
			)
			results[i] = session
			created[i] = wasCreated
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

	creatorCount := 0
	for _, wasCreated := range created {
		if wasCreated {
			creatorCount++
		}
	}
	if creatorCount != 1 {
		t.Fatalf("callers reporting created=true = %d, want exactly 1 (one creator, %d reusers)", creatorCount, n-1)
	}

	// A caller whose initial GetTaskSessionByTaskAndAgent lookup lands after
	// the winner's row is already committed reuses it directly and never
	// reaches CreateOfficeTaskSession at all, so the call count is not
	// pinned to n — only bounded by it. What must hold regardless of
	// interleaving: at least one call happened (the interface branch was
	// actually exercised) and no caller needed the bounded-recovery loop's
	// second create attempt (which would push the count past n).
	if repo.createOfficeTaskSessionN < 1 || repo.createOfficeTaskSessionN > n {
		t.Fatalf("CreateOfficeTaskSession calls = %d, want between 1 and %d (repository-native branch exercised, no runaway recovery retries)",
			repo.createOfficeTaskSessionN, n)
	}
	if len(repo.createTaskSessionCalls) != 1 {
		t.Fatalf("underlying CreateTaskSession calls = %d, want exactly 1 (no duplicate CREATED event)", len(repo.createTaskSessionCalls))
	}
	sessions, err := repo.ListTaskSessions(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions for task = %d, want exactly 1", len(sessions))
	}
}

// TestEnsureSessionForAgentWithCreation_RecoveryLookupFailurePropagatesAsItself
// exercises the AC-003.7 re-read-error arm in
// createOfficeSessionWithBoundedRecovery: after a create attempt loses the
// race (ErrOfficeSessionRaceConflict), the recovery re-read via
// GetTaskSessionByTaskAndAgent can itself fail for an unrelated reason (a
// transient DB error, say). That failure must surface as itself — wrapped,
// not laundered into ErrOfficeSessionRaceConflict and not swallowed into a
// nil-session/nil-error result — exactly like the sibling non-conflict
// create-failure case already covered above.
func TestEnsureSessionForAgentWithCreation_RecoveryLookupFailurePropagatesAsItself(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	repo.createTaskSessionFunc = func(_ context.Context, _ *models.TaskSession) error {
		return taskrepo.ErrOfficeSessionRaceConflict
	}
	wantErr := errors.New("db unavailable")
	var lookups int
	repo.getTaskSessionByTaskAndAgentFunc = func(_ context.Context, _, _ string) (*models.TaskSession, error) {
		lookups++
		if lookups == 1 {
			// EnsureSessionForAgentWithCreation's initial lookup: no existing
			// row, so it proceeds to create (which will lose the race above).
			return nil, nil
		}
		// The bounded-recovery re-read after losing the create race: this is
		// the AC-003.7 arm under test.
		return nil, wantErr
	}

	session, wasCreated, err := exec.EnsureSessionForAgentWithCreation(
		context.Background(), task, "agent-recovery-lookup-failure", "profile-1", "exec-1", "",
	)
	if session != nil {
		t.Fatalf("session = %#v, want nil", session)
	}
	if wasCreated {
		t.Fatal("wasCreated = true, want false")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want errors.Is match against the underlying lookup failure", err)
	}
	if errors.Is(err, taskrepo.ErrOfficeSessionRaceConflict) {
		t.Fatalf("recovery lookup failure %v must not classify as ErrOfficeSessionRaceConflict", err)
	}
}

// TestEnsureSessionForAgentWithCreation_RecoveryReReadFindsTerminalWinnerRetriesCreate
// covers createOfficeSessionWithBoundedRecovery's terminal-retry arm
// (executor_office.go), which had zero coverage: when the post-conflict
// recovery re-read finds a row that has itself gone terminal by the time
// tryReuseExistingSession inspects it, the loop must retry the create rather
// than returning the terminal row. This is distinct from the already-covered
// raced == nil shape (RecoveryStopsAfterTwoAttempts) and the lookup-error
// shape (RecoveryLookupFailurePropagatesAsItself) above - here the re-read
// itself succeeds and returns a real, terminal row.
//
// Left as-is, a regression collapsing `if decision != reuseDecisionTerminal {
// return reused, false, nil }` to an unconditional return would leave this
// package green while reproducing a reachable production nil-pointer panic:
// task_operations.go:1601 dereferences the returned session without a nil
// check on the (nil, false, nil) shape this arm would otherwise produce.
func TestEnsureSessionForAgentWithCreation_RecoveryReReadFindsTerminalWinnerRetriesCreate(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	var createAttempts int
	repo.createTaskSessionFunc = func(_ context.Context, session *models.TaskSession) error {
		createAttempts++
		if createAttempts == 1 {
			return taskrepo.ErrOfficeSessionRaceConflict
		}
		repo.mu.Lock()
		repo.sessions[session.ID] = session
		repo.mu.Unlock()
		return nil
	}

	terminalWinner := &models.TaskSession{
		ID: "recovery-terminal-winner", TaskID: task.ID, AgentProfileID: "agent-recovery-terminal",
		State: models.TaskSessionStateCompleted, StartedAt: time.Now().UTC(),
	}
	var lookups int
	repo.getTaskSessionByTaskAndAgentFunc = func(_ context.Context, _, _ string) (*models.TaskSession, error) {
		lookups++
		if lookups == 1 {
			// Initial lookup: no existing row, so it proceeds to create
			// (which will lose the race below).
			return nil, nil
		}
		// The bounded-recovery re-read after losing the create race: the
		// winner has already gone terminal by the time we observe it.
		return terminalWinner, nil
	}

	session, wasCreated, err := exec.EnsureSessionForAgentWithCreation(
		context.Background(), task, "agent-recovery-terminal", "profile-1", "exec-1", "",
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if !wasCreated {
		t.Fatal("wasCreated = false, want true (the loop must retry the create rather than reuse the terminal row)")
	}
	if session == nil || session.ID == terminalWinner.ID {
		t.Fatalf("session = %#v, want a freshly created row distinct from the terminal winner %q", session, terminalWinner.ID)
	}
	if createAttempts != 2 {
		t.Fatalf("create attempts = %d, want exactly 2 (first loses the race, second retries after the terminal re-read)", createAttempts)
	}
	if lookups != 2 {
		t.Fatalf("recovery lookups = %d, want exactly 2 (initial lookup + post-conflict re-read)", lookups)
	}
}

// TestEnsureSessionForAgentWithCreation_RecoveryReReadFindsLiveWinnerReusesWithoutRetry
// is the mirror of the terminal-retry test above and closes two related
// AC-003.7 gaps flagged by Review entry 13:
//
//  1. "No retry spent" is otherwise unasserted: the concurrent convergence
//     test proves one creator and N-1 reusers but never asserts that a
//     caller which reuses via the recovery re-read stops after exactly the
//     one (failed) create attempt that lost the race, rather than looping to
//     spend the bounded loop's second attempt as well.
//  2. The concurrent convergence test doesn't reliably prove the
//     bounded-recovery re-read path was traversed at all - sampling found
//     roughly 1-in-5 runs where every goroutine happened to win or lose
//     without any of them observing a live winner through the recovery
//     re-read. This test drives that exact interleaving deterministically,
//     independent of goroutine scheduling.
func TestEnsureSessionForAgentWithCreation_RecoveryReReadFindsLiveWinnerReusesWithoutRetry(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	task := officeTestTask()

	var createAttempts int
	repo.createTaskSessionFunc = func(_ context.Context, _ *models.TaskSession) error {
		createAttempts++
		return taskrepo.ErrOfficeSessionRaceConflict
	}

	liveWinner := &models.TaskSession{
		ID: "recovery-live-winner", TaskID: task.ID, AgentProfileID: "agent-recovery-live",
		State: models.TaskSessionStateRunning, StartedAt: time.Now().UTC(),
		// Matches the execution profile passed to EnsureSessionForAgentWithCreation
		// below so rebindOfficeSessionExecutionProfile's no-op fast path (the
		// row is already on the right profile) applies - this test targets
		// tryReuseExistingSession's outcome, not the rebind CAS update, and the
		// winner row was never written to the mock's session store.
		ExecutionProfileID: "profile-1",
	}
	var lookups int
	repo.getTaskSessionByTaskAndAgentFunc = func(_ context.Context, _, _ string) (*models.TaskSession, error) {
		lookups++
		if lookups == 1 {
			return nil, nil
		}
		// The recovery re-read: the winner has already committed and is
		// still live (not terminal), so this caller must reuse it.
		return liveWinner, nil
	}

	session, wasCreated, err := exec.EnsureSessionForAgentWithCreation(
		context.Background(), task, "agent-recovery-live", "profile-1", "exec-1", "",
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if wasCreated {
		t.Fatal("wasCreated = true, want false (this caller reused the live winner, it did not create)")
	}
	if session == nil || session.ID != liveWinner.ID {
		t.Fatalf("session = %#v, want the live winner row %q", session, liveWinner.ID)
	}
	if createAttempts != 1 {
		t.Fatalf("create attempts = %d, want exactly 1 (reusing a live winner must not spend the bounded loop's second attempt)", createAttempts)
	}
	if lookups != 2 {
		t.Fatalf("recovery lookups = %d, want exactly 2 (initial lookup + post-conflict re-read), proving the recovery branch was actually traversed", lookups)
	}
}
