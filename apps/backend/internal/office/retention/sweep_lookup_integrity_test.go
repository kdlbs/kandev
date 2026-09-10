package retention

import (
	"context"
	"testing"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestGetActiveRunForFingerprint_UnaffectedByRetentionSweep proves
// AC-OFFICE-RUN-HISTORY-RETENTION-005.3: after a sweep deletes unrelated
// history rows, including ones sharing the same routine and fingerprint,
// the concurrency gate's fingerprint lookup still returns exactly the live
// task_created run it would have returned had no sweep run.
func TestGetActiveRunForFingerprint_UnaffectedByRetentionSweep(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	repo, err := officesqlite.NewWithDB(conn, conn, nil)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRunWithFingerprintAndLinkedTask(t, conn, newID(), "r-1", "done", &old, old, "fp-1", "")
	seedRoutineRunWithFingerprintAndLinkedTask(t, conn, newID(), "r-1", "failed", &old, old, "fp-1", "")

	activeID := newID()
	seedRoutineRunWithFingerprintAndLinkedTask(t, conn, activeID, "r-1", "task_created", nil, daysAgo(1), "fp-1", "")

	before, err := repo.GetActiveRunForFingerprint(ctx, "r-1", "fp-1")
	if err != nil {
		t.Fatalf("GetActiveRunForFingerprint (before): %v", err)
	}
	if before == nil || before.ID != activeID {
		t.Fatalf("GetActiveRunForFingerprint (before) = %+v, want id %s", before, activeID)
	}

	sweeper.RunSweep(ctx) // preview pass
	sweeper.RunSweep(ctx) // deleting pass

	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE status IN ('done', 'failed')`); n != 0 {
		t.Fatalf("old rows remaining = %d, want 0 (sweep should have deleted them)", n)
	}

	after, err := repo.GetActiveRunForFingerprint(ctx, "r-1", "fp-1")
	if err != nil {
		t.Fatalf("GetActiveRunForFingerprint (after): %v", err)
	}
	if after == nil || after.ID != activeID {
		t.Fatalf("GetActiveRunForFingerprint (after sweep) = %+v, want the same active run %s", after, activeID)
	}
}

// TestGetRoutineRunByLinkedTaskID_DeletedRunResolvesToNothing proves
// AC-OFFICE-RUN-HISTORY-RETENTION-005.4: once a sweep deletes the routine
// run linked to a task, the task-closure lookup for that task ID resolves
// to nothing rather than to an unrelated run.
func TestGetRoutineRunByLinkedTaskID_DeletedRunResolvesToNothing(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	repo, err := officesqlite.NewWithDB(conn, conn, nil)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	deletedID := newID()
	seedRoutineRunWithFingerprintAndLinkedTask(t, conn, deletedID, "r-1", "done", &old, old, "fp-1", "task-1")

	// A second, unrelated run under a different task must not be
	// mistaken for task-1's closed-out run.
	other := daysAgo(1)
	otherID := newID()
	seedRoutineRunWithFingerprintAndLinkedTask(t, conn, otherID, "r-1", "done", &other, other, "fp-2", "task-2")

	before, err := repo.GetRoutineRunByLinkedTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetRoutineRunByLinkedTaskID (before): %v", err)
	}
	if before == nil || before.ID != deletedID {
		t.Fatalf("GetRoutineRunByLinkedTaskID (before) = %+v, want id %s", before, deletedID)
	}

	sweeper.RunSweep(ctx) // preview pass
	sweeper.RunSweep(ctx) // deleting pass

	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, deletedID); n != 0 {
		t.Fatalf("deleted run rows = %d, want 0", n)
	}

	after, err := repo.GetRoutineRunByLinkedTaskID(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetRoutineRunByLinkedTaskID (after): %v", err)
	}
	if after != nil {
		t.Fatalf("GetRoutineRunByLinkedTaskID (after sweep) = %+v, want nil", after)
	}

	// task-2's own run must be unaffected by task-1's deletion.
	stillThere, err := repo.GetRoutineRunByLinkedTaskID(ctx, "task-2")
	if err != nil {
		t.Fatalf("GetRoutineRunByLinkedTaskID (task-2): %v", err)
	}
	if stillThere == nil || stillThere.ID != otherID {
		t.Fatalf("GetRoutineRunByLinkedTaskID (task-2) = %+v, want id %s", stillThere, otherID)
	}
}
