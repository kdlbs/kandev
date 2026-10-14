package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresRecordDeferredAssignment_UpsertsOnConflict is the PostgreSQL
// twin of the SQLite-covered insert path in paused_assignment_replay_test.go.
// RecordDeferredAssignment uses the same insert-then-catch-
// isUniqueConstraintErr-then-UPDATE fallback that originally broke
// CreateWorkspacePauseWithActivity on Postgres (see
// TestPostgresCreateWorkspacePauseWithActivity_RejectsSecondPause):
// isUniqueConstraintErr matched only SQLite's error text, so a Postgres
// primary-key conflict surfaced as a raw driver error instead of falling
// through to the UPDATE. Running the real conflict path against Postgres
// makes a regression on that dialect gap fail loudly for this table too.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRecordDeferredAssignment_UpsertsOnConflict(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	if _, err := repo.RecordDeferredAssignment(ctx, "pg-task-1", "pg-ws-1", "agent-1", 1, "pg-pause-1"); err != nil {
		t.Fatalf("first record: %v", err)
	}

	// Resolve the pending row so the second call's reset-to-pending effect
	// (resolved_at/outcome cleared) is observable, not just "still NULL".
	resolved, err := repo.ResolveDeferredAssignment(ctx, "pg-task-1", 1, "agent-1", "pg-pause-1", "dropped")
	if err != nil {
		t.Fatalf("resolve before conflict: %v", err)
	}
	if !resolved {
		t.Fatal("expected the resolve to win the CAS")
	}

	// Same task_id again: must hit the UPDATE fallback on the PK conflict,
	// not error, and must reset resolved_at/outcome back to pending with
	// the new agent/generation/pause.
	if _, err := repo.RecordDeferredAssignment(ctx, "pg-task-1", "pg-ws-1", "agent-2", 2, "pg-pause-2"); err != nil {
		t.Fatalf("second record (conflict fallback): %v", err)
	}

	pending, err := repo.ListPendingDeferredAssignmentsForWorkspace(ctx, "pg-ws-1")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending count = %d, want 1: %+v", len(pending), pending)
	}
	row := pending[0]
	if row.AgentProfileID != "agent-2" || row.AssignmentGeneration != 2 || row.PauseID != "pg-pause-2" {
		t.Fatalf("row not overwritten by conflict fallback: %+v", row)
	}
	if row.ResolvedAt != nil || row.Outcome != "" {
		t.Fatalf("expected reset to pending, got resolved_at=%v outcome=%q", row.ResolvedAt, row.Outcome)
	}
}

// TestPostgresListReplayablePendingDeferredAssignments_ExcludesActivePause
// proves the NOT EXISTS correlated subquery against office_workspace_pauses
// behaves the same on Postgres as on SQLite: a pending deferred assignment
// is excluded while its workspace has an active (unreleased) pause, and
// becomes replayable once that pause is released.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresListReplayablePendingDeferredAssignments_ExcludesActivePause(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	pause := &models.WorkspacePause{
		WorkspaceID:   "pg-ws-2",
		Reason:        "incident",
		CreatedBy:     "user-1",
		CreatedByKind: "user",
	}
	pauseActivity := &models.ActivityEntry{
		WorkspaceID: "pg-ws-2",
		ActorType:   models.ActivityActorUser,
		ActorID:     "user-1",
		Action:      models.ActivityActionWorkspacePaused,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    "pg-ws-2",
		Details:     "incident",
	}
	if err := repo.CreateWorkspacePauseWithActivity(ctx, pause, pauseActivity); err != nil {
		t.Fatalf("create pause: %v", err)
	}

	if _, err := repo.RecordDeferredAssignment(ctx, "pg-task-2", "pg-ws-2", "agent-1", 1, pause.ID); err != nil {
		t.Fatalf("record deferred assignment: %v", err)
	}

	replayable, err := repo.ListReplayablePendingDeferredAssignments(ctx, 100)
	if err != nil {
		t.Fatalf("list replayable while paused: %v", err)
	}
	for _, row := range replayable {
		if row.TaskID == "pg-task-2" {
			t.Fatalf("task should not be replayable while its workspace pause is active: %+v", row)
		}
	}

	if _, err := repo.ReleaseWorkspacePauseWithActivity(ctx, pause.ID, "pg-ws-2", "user-1", "user", "resolved"); err != nil {
		t.Fatalf("release pause: %v", err)
	}

	replayable, err = repo.ListReplayablePendingDeferredAssignments(ctx, 100)
	if err != nil {
		t.Fatalf("list replayable after release: %v", err)
	}
	found := false
	for _, row := range replayable {
		if row.TaskID == "pg-task-2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected pg-task-2 to be replayable after the pause released: %+v", replayable)
	}
}

// TestPostgresResolveDeferredAssignment_CASOnlyResolvesPendingOnce proves
// the CAS UPDATE ... WHERE resolved_at IS NULL only ever resolves a row
// once under Postgres: a second resolve attempt on an already-resolved row
// affects zero rows and reports it lost the race, mirroring the
// office_workspace_pauses CAS release semantics tested above.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresResolveDeferredAssignment_CASOnlyResolvesPendingOnce(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	if _, err := repo.RecordDeferredAssignment(ctx, "pg-task-3", "pg-ws-3", "agent-1", 1, "pg-pause-3"); err != nil {
		t.Fatalf("record deferred assignment: %v", err)
	}

	won, err := repo.ResolveDeferredAssignment(ctx, "pg-task-3", 1, "agent-1", "pg-pause-3", "replayed")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if !won {
		t.Fatal("expected the first resolve to win the CAS")
	}

	won, err = repo.ResolveDeferredAssignment(ctx, "pg-task-3", 1, "agent-1", "pg-pause-3", "dropped")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if won {
		t.Fatal("expected the second resolve to lose the CAS (row already resolved)")
	}

	var outcome string
	if err := repo.ReaderDB().Get(&outcome, `SELECT outcome FROM office_deferred_assignments WHERE task_id = 'pg-task-3'`); err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	if outcome != "replayed" {
		t.Fatalf("outcome = %q, want %q (the losing CAS must not overwrite it)", outcome, "replayed")
	}
}

// TestPostgresRecordDeferredAssignment_DoesNotRegressToOlderGeneration_Pending
// is the PostgreSQL twin of TestRecordDeferredAssignment_DoesNotRegressToOlderGeneration
// (R1-F3): it proves the generation guard blocks a stale gen1 write from
// overwriting a still-*pending* gen2 row on Postgres too, and that the
// suppressed write reports recorded=false so the caller skips logging a
// misleading "deferred" activity entry for it.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRecordDeferredAssignment_DoesNotRegressToOlderGeneration_Pending(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	if _, err := repo.RecordDeferredAssignment(ctx, "pg-task-r1f3", "pg-ws-r1f3", "agent-2", 2, "pause-1"); err != nil {
		t.Fatalf("record gen2: %v", err)
	}
	// Stale gen1 write; must be a no-op because the still-pending gen2 row
	// has a higher generation.
	recorded, err := repo.RecordDeferredAssignment(ctx, "pg-task-r1f3", "pg-ws-r1f3", "agent-1", 1, "pause-1")
	if err != nil {
		t.Fatalf("record gen1 (stale): %v", err)
	}
	if recorded {
		t.Fatal("stale gen1 write must report recorded=false: the guard suppressed it")
	}

	pending, err := repo.ListPendingDeferredAssignmentsForWorkspace(ctx, "pg-ws-r1f3")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(pending) != 1 || pending[0].AssignmentGeneration != 2 || pending[0].AgentProfileID != "agent-2" {
		t.Fatalf("stale gen1 write regressed gen2 row on Postgres: %+v", pending)
	}
}
