package routines_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// AC-OFFICE-LOOP-LIVENESS-001.1: last_run_at advances on the coalesced
// and skipped concurrency-policy paths too, not only on a fresh
// dispatch — the call site runs before applyConcurrencyPolicy can
// short-circuit the return.
func TestDispatch_AdvancesLastRunAtOnSkippedAndCoalescedPaths(t *testing.T) {
	t.Run("skipped", func(t *testing.T) {
		svc := newTestRoutineService(t)
		ctx := context.Background()
		routine := createTestRoutine(t, svc, "Skip Last Run", "skip_if_active")

		if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
			t.Fatalf("first fire: %v", err)
		}
		afterFirst, err := svc.GetRoutine(ctx, routine.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if afterFirst.LastRunAt == nil {
			t.Fatal("expected last_run_at set after first fire")
		}

		time.Sleep(2 * time.Millisecond)
		run2, err := svc.FireManual(ctx, routine.ID, nil)
		if err != nil {
			t.Fatalf("second fire: %v", err)
		}
		if run2.Status != "skipped" {
			t.Fatalf("second fire status = %q, want skipped", run2.Status)
		}
		afterSecond, err := svc.GetRoutine(ctx, routine.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !afterSecond.LastRunAt.After(*afterFirst.LastRunAt) {
			t.Fatalf("last_run_at did not advance on skipped fire: before %v, after %v",
				afterFirst.LastRunAt, afterSecond.LastRunAt)
		}
	})

	t.Run("coalesced", func(t *testing.T) {
		svc := newTestRoutineService(t)
		ctx := context.Background()
		routine := createTestRoutine(t, svc, "Coalesce Last Run", "coalesce_if_active")

		if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
			t.Fatalf("first fire: %v", err)
		}
		afterFirst, err := svc.GetRoutine(ctx, routine.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}

		time.Sleep(2 * time.Millisecond)
		run2, err := svc.FireManual(ctx, routine.ID, nil)
		if err != nil {
			t.Fatalf("second fire: %v", err)
		}
		if run2.Status != "coalesced" {
			t.Fatalf("second fire status = %q, want coalesced", run2.Status)
		}
		afterSecond, err := svc.GetRoutine(ctx, routine.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if !afterSecond.LastRunAt.After(*afterFirst.LastRunAt) {
			t.Fatalf("last_run_at did not advance on coalesced fire: before %v, after %v",
				afterFirst.LastRunAt, afterSecond.LastRunAt)
		}
	})
}

// failingTouchRepo wraps the real sqlite repository but forces
// TouchRoutineLastRun to fail, so the dispatch's error-tolerance
// (AC-OFFICE-LOOP-LIVENESS-001.7) can be exercised without a real
// storage failure.
type failingTouchRepo struct {
	*sqlite.Repository
}

func (f *failingTouchRepo) TouchRoutineLastRun(context.Context, string, time.Time) error {
	return errors.New("simulated last_run_at write failure")
}

// AC-OFFICE-LOOP-LIVENESS-001.7: a TouchRoutineLastRun error does not
// fail the dispatch — the routine still fires and its run is created.
func TestDispatch_SurvivesLastRunAtWriteFailure(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := routines.NewRoutineService(&failingTouchRepo{Repository: repo}, logger.Default(), &noopActivity{})

	routine := createTestRoutine(t, svc, "Failing Touch", "always_create")
	run, err := svc.FireManual(context.Background(), routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual with failing touch: %v", err)
	}
	if run.Status != "task_created" {
		t.Fatalf("status = %q, want task_created", run.Status)
	}
}
