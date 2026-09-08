package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
)

// newChildrenCompletedTestScheduler wires a real *service.Service (not
// nil, unlike newReactivityTestScheduler) because these tests exercise
// QueueRunCtx end to end — QueueRun's guardAgentStatus calls
// svc.GetAgentFromConfig, which panics on a nil Service.
func newChildrenCompletedTestScheduler(t *testing.T, repo *officesqlite.Repository) *SchedulerService {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log})
	return NewSchedulerService(repo, log, svc)
}

// createChildrenCompletedAgent registers an idle agent instance so
// guardAgentStatus (paused/stopped/pending-approval checks) passes.
func createChildrenCompletedAgent(t *testing.T, repo *officesqlite.Repository, id string) {
	t.Helper()
	if err := repo.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID:          id,
		WorkspaceID: "ws-1",
		Name:        id,
		Status:      models.AgentStatusIdle,
	}); err != nil {
		t.Fatalf("create agent instance %s: %v", id, err)
	}
}

// newChildrenCompletedQueue returns a queue closure that persists through
// the real QueueRunCtx path (mirroring the closure ApplyTaskMutation
// builds), so tests can assert against actually-persisted `runs` rows
// instead of a summary — QueueRun returns nil both when a run is
// persisted and when it is skipped as an idempotent duplicate
// (scheduler/run.go, "run skipped (idempotent)"), so a summary or a
// bare "no error" check false-greens on the exact defect these tests
// guard against.
func newChildrenCompletedQueue(t *testing.T, ss *SchedulerService) func(string, RunContext) {
	t.Helper()
	return func(agentID string, c RunContext) {
		if err := ss.QueueRunCtx(context.Background(), agentID, c); err != nil {
			t.Fatalf("QueueRunCtx: %v", err)
		}
	}
}

func insertChildTask(t *testing.T, ss *SchedulerService, id, parentID, state string) {
	t.Helper()
	if _, err := ss.repo.ExecRaw(context.Background(), `
		INSERT INTO tasks (id, workspace_id, parent_id, state)
		VALUES (?, 'ws-1', ?, ?)
	`, id, parentID, state); err != nil {
		t.Fatalf("insert child task %s: %v", id, err)
	}
}

func runsCountForReason(t *testing.T, ss *SchedulerService, reason string) int {
	t.Helper()
	var count int
	row := ss.repo.ReaderDB().QueryRowx(
		ss.repo.ReaderDB().Rebind(`SELECT COUNT(*) FROM runs WHERE reason = ?`), reason)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	return count
}

// ageRunsRequestedAt pushes every persisted run's requested_at back by
// the given duration, so tests can exercise the >24h idempotency-window
// path (IdempotencyWindowHours) without a real sleep.
func ageRunsRequestedAt(t *testing.T, ss *SchedulerService, by time.Duration) {
	t.Helper()
	// Bind a native time.Time (not a pre-formatted string): the sqlite
	// driver's own time encoding is what CreateRun's INSERT and
	// CoalesceRun's SELECT both use, and a hand-formatted string that
	// looks "earlier" can still sort later once TEXT-compared against a
	// driver-formatted cutoff, since the two encodings aren't identical.
	cutoff := time.Now().UTC().Add(-by)
	if _, err := ss.repo.ExecRaw(context.Background(),
		`UPDATE runs SET requested_at = ?`, cutoff); err != nil {
		t.Fatalf("age runs: %v", err)
	}
}

// setupChildrenCompletedParent creates a parent task with a
// workflow_step_participants "runner" row so GetTaskAssignee resolves it
// to agentID (RunnerProjection's task-scoped runner fallback). An empty
// workflow_steps table is also required: RunnerProjection references it
// in a COALESCE branch that SQLite still needs to compile even when that
// branch never matches.
func setupChildrenCompletedParent(t *testing.T, ss *SchedulerService, parentID, agentID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := ss.repo.ExecRaw(ctx, `
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			agent_profile_id TEXT DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create workflow_steps table: %v", err)
	}
	if _, err := ss.repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id) VALUES (?, 'ws-1')
	`, parentID); err != nil {
		t.Fatalf("insert parent task: %v", err)
	}
	if _, err := ss.repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('p-runner', 'step-x', ?, 'runner', ?, 0, 0)
	`, parentID, agentID); err != nil {
		t.Fatalf("insert runner participant: %v", err)
	}
}

// TestCascadeChildrenCompleted_SecondWaveDifferentChildSet_PersistsNewRun
// is the regression test for the once-ever idempotency-key defect: a
// parent that delegates in two sequential waves (finish wave 1, then fan
// out wave 2) must be woken for BOTH waves, not just the first. Before
// the fix, QueueRunCtx derived the idempotency key from only
// {reason}:{taskID}:{agentID} — identical for every wave — so wave 2's
// wake collided with wave 1's already-consumed key and QueueRun silently
// returned nil without persisting a row (scheduler/run.go, "run skipped
// (idempotent)"). That is why this test counts persisted `runs` rows
// rather than checking for an error.
func TestCascadeChildrenCompleted_SecondWaveDifferentChildSet_PersistsNewRun(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	queue := newChildrenCompletedQueue(t, ss)
	ctx := context.Background()

	// Wave 1: child-1 completes, parent has no other children.
	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after wave 1: persisted runs = %d, want 1", got)
	}

	// Push wave 1's run outside CoalesceWindowSeconds (5s) so wave 2
	// creates its own row instead of merging into wave 1's still-queued
	// one — in production the two waves are minutes/hours apart (wave 2
	// only exists because the parent woke, read wave 1's result, and
	// delegated again), so this just makes that separation real in a
	// synchronous test.
	ageRunsRequestedAt(t, ss, 10*time.Second)

	// Wave 2: parent delegates again, a new child completes.
	insertChildTask(t, ss, "child-2", "parent-1", "COMPLETED")
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-2", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 2 {
		t.Fatalf("after wave 2: persisted runs = %d, want 2 (parent must be woken again for the new wave)", got)
	}
}

// TestCascadeChildrenCompleted_SameChildSetTwice_StillDedupes proves the
// fix didn't trade "once-ever" for "always fires": a duplicate cascade
// over the exact same terminal child set (e.g. a re-delivered event) must
// still dedupe to a single persisted row.
func TestCascadeChildrenCompleted_SameChildSetTwice_StillDedupes(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	queue := newChildrenCompletedQueue(t, ss)
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	task := &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}

	ss.cascadeChildrenCompleted(ctx, task, queue)
	ss.cascadeChildrenCompleted(ctx, task, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("persisted runs = %d, want exactly 1 (same child set must dedupe)", got)
	}
}

// TestCascadeChildrenCompleted_DistinctWaveBeyondIdempotencyWindow_NoInsertError
// covers the shape-change the triage evidence documented: before the fix,
// ANY second wake for the same parent+agent past IdempotencyWindowHours
// (24h) fell through CheckIdempotencyKey's time-bounded guard into
// CreateRun and hit "UNIQUE constraint failed: runs.idempotency_key",
// because the key never changed. A genuinely distinct wave (new child
// set) must insert cleanly regardless of how much time elapsed since the
// previous wave — the fix makes the key vary with content, not just with
// the guard's time window.
func TestCascadeChildrenCompleted_DistinctWaveBeyondIdempotencyWindow_NoInsertError(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	queue := newChildrenCompletedQueue(t, ss)
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	// Push wave 1's row outside the 24h idempotency window.
	ageRunsRequestedAt(t, ss, 25*time.Hour)

	// Wave 2, a distinct child set, fires more than 24h after wave 1.
	insertChildTask(t, ss, "child-2", "parent-1", "COMPLETED")
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{ID: "child-2", WorkspaceID: "ws-1", ParentID: "parent-1"}, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 2 {
		t.Fatalf("persisted runs = %d, want 2 (distinct wave past the window must not collide)", got)
	}
}

// TestCascadeChildrenCompleted_ChildTerminalStateEdited_DoesNotPersistNewRun
// is the regression test for a review finding (PR #3059): reactToStatusChange
// invokes cascadeChildrenCompleted on any transition into "done", regardless
// of the child's PRIOR state, so a sibling that is later edited from
// CANCELLED to COMPLETED (a terminal-to-terminal transition, not a new
// delegation wave) fires this cascade again. If the idempotency key digested
// each child's state string, that edit alone would change the digest and
// persist a spurious second run even though the child ID set never changed.
// The parent must be woken exactly once for an unchanged child set.
func TestCascadeChildrenCompleted_ChildTerminalStateEdited_DoesNotPersistNewRun(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	queue := newChildrenCompletedQueue(t, ss)
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "CANCELLED")
	task := &TaskSnapshot{ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1"}
	ss.cascadeChildrenCompleted(ctx, task, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after first terminal state: persisted runs = %d, want 1", got)
	}

	ageRunsRequestedAt(t, ss, 10*time.Second)

	// Same child, edited from CANCELLED to COMPLETED — still the only
	// child of parent-1, so the child ID set is unchanged.
	if _, err := ss.repo.ExecRaw(ctx, `UPDATE tasks SET state = 'COMPLETED' WHERE id = 'child-1'`); err != nil {
		t.Fatalf("edit child-1 state: %v", err)
	}
	ss.cascadeChildrenCompleted(ctx, task, queue)

	if got := runsCountForReason(t, ss, RunReasonTaskChildrenCompleted); got != 1 {
		t.Fatalf("after terminal-to-terminal edit: persisted runs = %d, want 1 (unchanged child set must not re-wake)", got)
	}
}

// TestCascadeChildrenCompleted_NonterminalSnapshotDoesNotQueueWake covers the
// inconsistent-read shape where the terminal check succeeds but the child
// snapshot contains a nonterminal row. A NULL state is normalized to an empty
// state by ListChildStates and is not terminal for the completion contract.
func TestCascadeChildrenCompleted_NonterminalSnapshotDoesNotQueueWake(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	setupChildrenCompletedParent(t, ss, "parent-1", "agent-1")
	ctx := context.Background()

	insertChildTask(t, ss, "child-1", "parent-1", "COMPLETED")
	if _, err := ss.repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, parent_id, state)
		VALUES ('child-2', 'ws-1', 'parent-1', NULL)
	`); err != nil {
		t.Fatalf("insert child-2: %v", err)
	}

	var queued int
	queue := func(_ string, _ RunContext) { queued++ }
	ss.cascadeChildrenCompleted(ctx, &TaskSnapshot{
		ID: "child-1", WorkspaceID: "ws-1", ParentID: "parent-1",
	}, queue)

	if queued != 0 {
		t.Fatalf("queued wakes = %d, want 0 when the child snapshot is nonterminal", queued)
	}
}
