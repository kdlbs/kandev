package routines_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// newTestRoutineServiceRejectingRunStatusWrite is newTestRoutineService
// plus a trigger that aborts every UPDATE of office_routine_runs.status
// to 'task_created' or 'done', so a test can force materialiseRoutineRun's
// write to fail (on either the heavy or the lightweight path) without
// disturbing CreateRoutineRun (the write whose success is what
// office_loop_routine_run_total must count).
func newTestRoutineServiceRejectingRunStatusWrite(t *testing.T) *routines.RoutineService {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_run_status_write
		BEFORE UPDATE OF status ON office_routine_runs
		WHEN NEW.status IN ('task_created', 'done')
		BEGIN
			SELECT RAISE(ABORT, 'run status write rejected for test');
		END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	log := logger.Default()
	return routines.NewRoutineService(repo, log, &noopActivity{})
}

// AC-003.1 / AC-003.9 (Review round 3, R3-4): office_loop_routine_run_total
// must count a routine run CreateRoutineRun genuinely persisted even when
// materialiseRoutineRun errors out afterward. Before the fix,
// IncLoopRoutineRun was only called inside the applyConcurrencyPolicy and
// materialiseRoutineRun success branches — an error from either meant
// dispatchRoutineRun returned without incrementing the counter at all,
// silently undercounting a fire that genuinely created a persisted row
// (the exact defect class this capability exists to catch: the reference
// incident was 322 consecutive fires that read as healthy because
// nothing counted them correctly).
func TestDispatchRoutineRun_CountsRunEvenWhenMaterializeFails(t *testing.T) {
	svc := newTestRoutineServiceRejectingRunStatusWrite(t)
	ctx := context.Background()

	routine := newLightweightTestRoutine(t, svc)

	key := "workspace=" + routine.WorkspaceID + ";source=manual;disposition=failed"
	before := loopCounterInt(t, "office_loop_routine_run_total", key)

	if _, err := svc.FireManual(ctx, routine.ID, nil); err == nil {
		t.Fatal("expected FireManual to return an error from the rejected status write")
	}

	after := loopCounterInt(t, "office_loop_routine_run_total", key)
	if after != before+1 {
		t.Fatalf("routine_run[failed] delta = %d, want 1 (the row was genuinely created, must not go uncounted)", after-before)
	}
}
