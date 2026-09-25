package routines_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// legacyLightweightFingerprint mirrors computeFingerprint("", "", assignee)
// (service.go): a lightweight routine always renders an empty title and
// description, so its fingerprint depends only on the assignee and is
// identical on every fire.
func legacyLightweightFingerprint(assignee string) string {
	h := sha256.Sum256([]byte("|" + "|" + assignee))
	return fmt.Sprintf("%x", h[:16])
}

// seedLegacyUnlinkedTaskCreatedRun inserts a task_created row with an
// empty linked_task_id directly into office_routine_runs, bypassing the
// service entirely. Pre-#3488 code wrote rows in this exact shape;
// current code never does (materialiseHeavyRoutineRun always sets a
// non-empty LinkedTaskID in the same write that sets task_created), so
// this row can only be reproduced by direct insertion, standing in for
// the fossil an upgraded install carries forward (Office Beta ISSUE-1).
func seedLegacyUnlinkedTaskCreatedRun(t *testing.T, db *sqlx.DB, id, routineID, fingerprint string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO office_routine_runs
			(id, routine_id, source, status, linked_task_id, dispatch_fingerprint, created_at)
		 VALUES (?, ?, 'cron', 'task_created', '', ?, ?)`,
		id, routineID, fingerprint, time.Now().UTC().AddDate(0, -2, 0),
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
}

func newLegacyGateLightweightRoutine(t *testing.T, svc *routines.RoutineService, name string, policy models.RoutineConcurrencyPolicy) *models.Routine {
	t.Helper()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   name,
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-legacy",
		Status:                 "active",
		ConcurrencyPolicy:      policy,
	}
	if err := svc.CreateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return routine
}

func queryRunStatus(t *testing.T, db *sqlx.DB, runID string) (status string, completedAtSet bool) {
	t.Helper()
	var completedAt *time.Time
	if err := db.QueryRow(
		`SELECT status, completed_at FROM office_routine_runs WHERE id = ?`, runID,
	).Scan(&status, &completedAt); err != nil {
		t.Fatalf("query run %s: %v", runID, err)
	}
	return status, completedAt != nil
}

func countCoalescedInto(t *testing.T, db *sqlx.DB, runID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM office_routine_runs WHERE coalesced_into_run_id = ?`, runID,
	).Scan(&n); err != nil {
		t.Fatalf("count coalesced-into rows: %v", err)
	}
	return n
}

// TestBetaLegacyLightweightGate proves AC-OFFICE-SCHEDULER-001.13: a
// legacy task_created row with no linked task is not treated as an
// active run forever. Before the fix, GetActiveRunForFingerprint keeps
// returning the fossil row on every fire (its fingerprint never
// changes for a lightweight routine), and selfHealIfTaskTerminal leaves
// an empty-linked row untouched, so coalesce_if_active/skip_if_active
// gate every future fire and the routine can never launch an agent
// again (Office Beta ISSUE-1).
func TestBetaLegacyLightweightGate(t *testing.T) {
	t.Run("coalesce_if_active", func(t *testing.T) {
		testLegacyGateUnblocksLightweightFires(t, models.ConcurrencyPolicyCoalesceIfActive, true)
	})
	t.Run("skip_if_active", func(t *testing.T) {
		testLegacyGateUnblocksLightweightFires(t, models.ConcurrencyPolicySkipIfActive, true)
	})
	t.Run("always_create", func(t *testing.T) {
		// always_create never consults GetActiveRunForFingerprint (see
		// applyConcurrencyPolicy's early return), so a legacy row was
		// never able to block this policy and the fix does not touch
		// it here either. Included as a control: the fix must not
		// introduce gating where none existed.
		testLegacyGateUnblocksLightweightFires(t, models.ConcurrencyPolicyAlwaysCreate, false)
	})

	t.Run("persists_across_reopen", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "routines.db")
		ctx := context.Background()

		db1, err := sqlx.Open("sqlite3", path)
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		repo1, err := sqlite.NewWithDB(db1, db1, nil)
		if err != nil {
			t.Fatalf("new repo: %v", err)
		}
		svc1 := routines.NewRoutineService(repo1, logger.Default(), &noopActivity{})
		svc1.SetWakeupEnqueuer(&fakeWakeupEnqueuer{})
		routine := newLegacyGateLightweightRoutine(t, svc1, "Reopen", models.ConcurrencyPolicySkipIfActive)
		legacyID := "legacy-" + routine.ID
		seedLegacyUnlinkedTaskCreatedRun(t, db1, legacyID, routine.ID, legacyLightweightFingerprint(routine.AssigneeAgentProfileID))

		run1, err := svc1.FireManual(ctx, routine.ID, nil)
		if err != nil {
			t.Fatalf("first fire: %v", err)
		}
		if run1.Status != models.RoutineRunStatusDone {
			t.Fatalf("first fire status = %q, want done", run1.Status)
		}
		if err := db1.Close(); err != nil {
			t.Fatalf("close db1: %v", err)
		}

		db2, err := sqlx.Open("sqlite3", path)
		if err != nil {
			t.Fatalf("reopen db: %v", err)
		}
		t.Cleanup(func() { _ = db2.Close() })
		repo2, err := sqlite.NewWithDB(db2, db2, nil)
		if err != nil {
			t.Fatalf("reopen repo: %v", err)
		}
		svc2 := routines.NewRoutineService(repo2, logger.Default(), &noopActivity{})
		svc2.SetWakeupEnqueuer(&fakeWakeupEnqueuer{})

		status, completedAtSet := queryRunStatus(t, db2, legacyID)
		if status != string(models.RoutineRunStatusFailed) || !completedAtSet {
			t.Fatalf("legacy row after reopen = (%q, completed_at set=%v), want (failed, true) — close-out did not persist", status, completedAtSet)
		}

		run2, err := svc2.FireManual(ctx, routine.ID, nil)
		if err != nil {
			t.Fatalf("second fire (after reopen): %v", err)
		}
		if run2.Status != models.RoutineRunStatusDone {
			t.Fatalf("second fire (after reopen) status = %q, want done", run2.Status)
		}
	})

	t.Run("heavy_active_run_still_gates", func(t *testing.T) {
		for _, policy := range []models.RoutineConcurrencyPolicy{
			models.ConcurrencyPolicyCoalesceIfActive, models.ConcurrencyPolicySkipIfActive,
		} {
			t.Run(string(policy), func(t *testing.T) {
				svc, db := newTestRoutineServiceWithDB(t)
				ctx := context.Background()
				svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
				svc.SetTaskCreator(&fakeTaskCreator{})

				routine := createTestRoutine(t, svc, "Heavy still gates "+string(policy), string(policy))

				run1, err := svc.FireManual(ctx, routine.ID, nil)
				if err != nil {
					t.Fatalf("first run: %v", err)
				}
				if run1.Status != models.RoutineRunStatusTaskCreated {
					t.Fatalf("first run status = %q, want task_created", run1.Status)
				}
				if run1.LinkedTaskID == "" {
					t.Fatalf("first run has no linked task; test setup is wrong")
				}

				if _, err := db.ExecContext(ctx,
					`CREATE TABLE tasks (id TEXT PRIMARY KEY, state TEXT, archived_at TIMESTAMP)`); err != nil {
					t.Fatalf("create tasks table: %v", err)
				}
				if _, err := db.ExecContext(ctx,
					`INSERT INTO tasks (id, state) VALUES (?, ?)`, run1.LinkedTaskID, "IN_PROGRESS"); err != nil {
					t.Fatalf("insert linked task: %v", err)
				}

				run2, err := svc.FireManual(ctx, routine.ID, nil)
				if err != nil {
					t.Fatalf("second run: %v", err)
				}
				if run2.Status == models.RoutineRunStatusDone || run2.Status == models.RoutineRunStatusTaskCreated {
					t.Fatalf("second run status = %q, want skipped/coalesced (live task must keep gating)", run2.Status)
				}

				status, _ := queryRunStatus(t, db, run1.ID)
				if status != string(models.RoutineRunStatusTaskCreated) {
					t.Fatalf("live heavy run status = %q, want task_created (must not be closed out)", status)
				}
			})
		}
	})

	t.Run("fresh_lightweight_received_untouched", func(t *testing.T) {
		svc, db := newTestRoutineServiceWithDB(t)
		ctx := context.Background()
		svc.SetWakeupEnqueuer(&fakeWakeupEnqueuer{})

		routine := newLegacyGateLightweightRoutine(t, svc, "Fresh received", models.ConcurrencyPolicySkipIfActive)
		fp := legacyLightweightFingerprint(routine.AssigneeAgentProfileID)

		// A run still in "received" (mid-dispatch, not yet
		// materialized) must never be selected by
		// GetActiveRunForFingerprint (it only selects task_created),
		// so the self-heal fix must never observe or touch it.
		receivedID := "received-" + routine.ID
		if _, err := db.Exec(
			`INSERT INTO office_routine_runs
				(id, routine_id, source, status, linked_task_id, dispatch_fingerprint, created_at)
			 VALUES (?, ?, 'cron', 'received', '', ?, ?)`,
			receivedID, routine.ID, fp, time.Now().UTC(),
		); err != nil {
			t.Fatalf("seed received row: %v", err)
		}

		if _, err := svc.FireManual(ctx, routine.ID, nil); err != nil {
			t.Fatalf("fire: %v", err)
		}

		status, _ := queryRunStatus(t, db, receivedID)
		if status != "received" {
			t.Fatalf("received row status = %q, want received (untouched)", status)
		}
	})
}

// testLegacyGateUnblocksLightweightFires fires a lightweight routine
// three times against a seeded legacy fossil row and asserts every fire
// reaches lightweight materialisation ("done"), never "coalesced" or
// "skipped". When expectLegacyClosed is true, the fix must have closed
// the fossil row to "failed" (with completed_at set) and no new run may
// have coalesced into it.
func testLegacyGateUnblocksLightweightFires(t *testing.T, policy models.RoutineConcurrencyPolicy, expectLegacyClosed bool) {
	svc, db := newTestRoutineServiceWithDB(t)
	ctx := context.Background()
	svc.SetWakeupEnqueuer(&fakeWakeupEnqueuer{})

	routine := newLegacyGateLightweightRoutine(t, svc, "Legacy gate "+string(policy), policy)
	legacyID := "legacy-" + routine.ID
	seedLegacyUnlinkedTaskCreatedRun(t, db, legacyID, routine.ID, legacyLightweightFingerprint(routine.AssigneeAgentProfileID))

	for i := 0; i < 3; i++ {
		run, err := svc.FireManual(ctx, routine.ID, nil)
		if err != nil {
			t.Fatalf("fire %d: %v", i+1, err)
		}
		if run.Status != models.RoutineRunStatusDone {
			t.Fatalf("fire %d status = %q, want done (legacy fossil must not gate this fire)", i+1, run.Status)
		}
	}

	status, completedAtSet := queryRunStatus(t, db, legacyID)
	if expectLegacyClosed {
		if status != string(models.RoutineRunStatusFailed) || !completedAtSet {
			t.Fatalf("legacy row = (%q, completed_at set=%v), want (failed, true)", status, completedAtSet)
		}
	} else if status != string(models.RoutineRunStatusTaskCreated) {
		t.Fatalf("legacy row = %q, want unchanged task_created under %s", status, policy)
	}

	if n := countCoalescedInto(t, db, legacyID); n != 0 {
		t.Fatalf("runs coalesced into legacy row = %d, want 0", n)
	}
}
