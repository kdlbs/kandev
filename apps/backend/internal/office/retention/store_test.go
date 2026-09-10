package retention

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// testDB builds a fresh in-memory SQLite database carrying the real office
// schema (including the retention indexes), so eligibility queries run
// against the genuine table shapes rather than a hand-rolled fixture.
func testDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := officesqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("init office schema: %v", err)
	}
	return conn
}

func seedRoutine(t *testing.T, conn *sqlx.DB, id string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at)
		VALUES (?, 'ws-1', ?, ?, ?)
	`), id, id, now, now); err != nil {
		t.Fatalf("seed routine %s: %v", id, err)
	}
}

func seedRoutineRun(t *testing.T, conn *sqlx.DB, id, routineID, status string, completedAt *time.Time, createdAt time.Time) {
	t.Helper()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO office_routine_runs (id, routine_id, source, status, completed_at, created_at)
		VALUES (?, ?, 'trigger', ?, ?, ?)
	`), id, routineID, status, completedAt, createdAt); err != nil {
		t.Fatalf("seed routine run %s: %v", id, err)
	}
}

func seedRoutineRunWithFingerprintAndLinkedTask(
	t *testing.T, conn *sqlx.DB, id, routineID, status string,
	completedAt *time.Time, createdAt time.Time, fingerprint, linkedTaskID string,
) {
	t.Helper()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO office_routine_runs (id, routine_id, source, status, completed_at, created_at, dispatch_fingerprint, linked_task_id)
		VALUES (?, ?, 'trigger', ?, ?, ?, ?, ?)
	`), id, routineID, status, completedAt, createdAt, fingerprint, linkedTaskID); err != nil {
		t.Fatalf("seed routine run %s: %v", id, err)
	}
}

func seedRun(t *testing.T, conn *sqlx.DB, id, agentProfileID, status string, finishedAt *time.Time, requestedAt time.Time) {
	t.Helper()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO runs (id, agent_profile_id, reason, status, requested_at, finished_at)
		VALUES (?, ?, 'test', ?, ?, ?)
	`), id, agentProfileID, status, requestedAt, finishedAt); err != nil {
		t.Fatalf("seed run %s: %v", id, err)
	}
}

func seedRunEvent(t *testing.T, conn *sqlx.DB, runID string, seq int) {
	t.Helper()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO run_events (run_id, seq, event_type, created_at)
		VALUES (?, ?, 'test', ?)
	`), runID, seq, time.Now().UTC()); err != nil {
		t.Fatalf("seed run event for %s: %v", runID, err)
	}
}

func seedRouteAttempt(t *testing.T, conn *sqlx.DB, runID string, seq int) {
	t.Helper()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO office_run_route_attempts (run_id, seq, provider_id, model, tier, outcome, started_at)
		VALUES (?, ?, 'p', 'm', 't', 'ok', ?)
	`), runID, seq, time.Now().UTC()); err != nil {
		t.Fatalf("seed route attempt for %s: %v", runID, err)
	}
}

func seedRunSkill(t *testing.T, conn *sqlx.DB, runID, skillID string) {
	t.Helper()
	if _, err := conn.Exec(conn.Rebind(`
		INSERT INTO office_run_skills (run_id, skill_id, version, content_hash, materialized_path)
		VALUES (?, ?, 'v1', 'hash', '/path')
	`), runID, skillID); err != nil {
		t.Fatalf("seed run skill for %s: %v", runID, err)
	}
}

func countRows(t *testing.T, conn *sqlx.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := conn.Get(&n, conn.Rebind(query), args...); err != nil {
		t.Fatalf("count query %q: %v", query, err)
	}
	return n
}

func daysAgo(n int) time.Time { return time.Now().UTC().AddDate(0, 0, -n) }

func newID() string { return uuid.New().String() }

func TestCountEligibleRoutineRuns_RespectsStatusWindowAndFloor(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()
	routineID := newID()
	seedRoutine(t, conn, routineID)

	cutoff := daysAgo(30)
	// 3 history rows older than the window, floor 50: all inside the floor,
	// none eligible.
	for i := 0; i < 3; i++ {
		completed := daysAgo(40 + i)
		seedRoutineRun(t, conn, newID(), routineID, "coalesced", &completed, completed)
	}
	// A task_created (live-state) row, ancient: never eligible regardless of
	// age (AC-OFFICE-RUN-HISTORY-RETENTION-001.1).
	ancient := daysAgo(3650)
	seedRoutineRun(t, conn, newID(), routineID, "task_created", nil, ancient)

	count, err := store.CountEligibleRoutineRuns(ctx, conn, cutoff, 50)
	if err != nil {
		t.Fatalf("CountEligibleRoutineRuns: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (all 3 history rows within the floor of 50)", count)
	}

	// Add 50 more, all older than window: total history rows now 53, floor
	// 50, so exactly 3 are eligible (the 3 oldest, since floor keeps the
	// newest 50 of the 53).
	for i := 0; i < 50; i++ {
		completed := daysAgo(35 + i)
		seedRoutineRun(t, conn, newID(), routineID, "done", &completed, completed)
	}
	count, err = store.CountEligibleRoutineRuns(ctx, conn, cutoff, 50)
	if err != nil {
		t.Fatalf("CountEligibleRoutineRuns: %v", err)
	}
	if count != 3 {
		t.Fatalf("count = %d, want 3 (53 history rows, floor 50)", count)
	}
}

func TestDeleteRoutineRunsBatch_OldestFirstAndOrderedByNamedColumns(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()
	routineID := newID()
	seedRoutine(t, conn, routineID)

	cutoff := daysAgo(30)
	var oldest, middle, newest string
	oldest, middle, newest = newID(), newID(), newID()
	oldC, midC, newC := daysAgo(90), daysAgo(60), daysAgo(45)
	seedRoutineRun(t, conn, oldest, routineID, "done", &oldC, oldC)
	seedRoutineRun(t, conn, middle, routineID, "done", &midC, midC)
	seedRoutineRun(t, conn, newest, routineID, "done", &newC, newC)

	// floor 0 so all three are eligible; batch limit 2 -> the two oldest go,
	// the newest survives as backlog.
	deleted, err := store.DeleteRoutineRunsBatch(ctx, conn, cutoff, 0, 2)
	if err != nil {
		t.Fatalf("DeleteRoutineRunsBatch: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}
	remaining := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, newest)
	if remaining != 1 {
		t.Fatalf("newest row was deleted; oldest-first ordering violated")
	}
	for _, id := range []string{oldest, middle} {
		if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, id); n != 0 {
			t.Fatalf("row %s (older) still present after batch limit 2", id)
		}
	}
}

func TestDeleteRoutineRunsBatch_FloorReassertedAtDeleteTime(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()
	routineID := newID()
	seedRoutine(t, conn, routineID)

	cutoff := daysAgo(30)
	c := daysAgo(90)
	id := newID()
	seedRoutineRun(t, conn, id, routineID, "done", &c, c)

	// Floor 1 (>= the single row present) means the row is protected: it
	// is the newest (and only) row for its routine.
	deleted, err := store.DeleteRoutineRunsBatch(ctx, conn, cutoff, 1, 100)
	if err != nil {
		t.Fatalf("DeleteRoutineRunsBatch: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted = %d, want 0 (row is inside the floor)", deleted)
	}
}

// TestDeleteRoutineRunsBatch_FloorHeldIndependentlyPerRoutine proves
// AC-OFFICE-RUN-HISTORY-RETENTION-001.4's floor is per-owner: every seed
// helper elsewhere in this suite uses a single routine, so a regression
// that dropped routineRunEligibleSubquery's PARTITION BY routine_id
// (turning a per-owner floor into one shared across every routine) would
// otherwise leave the whole suite green.
func TestDeleteRoutineRunsBatch_FloorHeldIndependentlyPerRoutine(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	routineA, routineB := newID(), newID()
	seedRoutine(t, conn, routineA)
	seedRoutine(t, conn, routineB)

	cutoff := daysAgo(30)
	var newestA, newestB string
	for i := 0; i < 5; i++ {
		completed := daysAgo(90 - i) // i=0 oldest (day 90) .. i=4 newest (day 86)
		idA, idB := newID(), newID()
		seedRoutineRun(t, conn, idA, routineA, "done", &completed, completed)
		seedRoutineRun(t, conn, idB, routineB, "done", &completed, completed)
		if i == 4 {
			newestA, newestB = idA, idB
		}
	}

	// Floor 3 per routine: each routine has 5 history rows, so 2 are
	// eligible per routine, 4 total. A floor shared across both routines
	// (10 rows, floor 3) would instead delete 7 and could delete either
	// routine's newest row.
	deleted, err := store.DeleteRoutineRunsBatch(ctx, conn, cutoff, 3, 100)
	if err != nil {
		t.Fatalf("DeleteRoutineRunsBatch: %v", err)
	}
	if deleted != 4 {
		t.Fatalf("deleted = %d, want 4 (2 eligible per routine, floor 3 held independently)", deleted)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = ?`, routineA); n != 3 {
		t.Fatalf("routineA remaining = %d, want 3 (its own floor)", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE routine_id = ?`, routineB); n != 3 {
		t.Fatalf("routineB remaining = %d, want 3 (its own floor)", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, newestA); n != 1 {
		t.Fatalf("routineA's newest row was deleted; its floor should have protected it")
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs WHERE id = ?`, newestB); n != 1 {
		t.Fatalf("routineB's newest row was deleted; its floor should have protected it")
	}
}

func TestDeleteRunBatch_DeletesSatellitesAtomicallyWithRun(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	runID := newID()
	finished := daysAgo(60)
	seedRun(t, conn, runID, "agent-1", "finished", &finished, finished)
	seedRunEvent(t, conn, runID, 0)
	seedRunEvent(t, conn, runID, 1)
	seedRouteAttempt(t, conn, runID, 0)
	seedRunSkill(t, conn, runID, "skill-1")

	result, err := store.DeleteRunBatch(ctx, conn, daysAgo(30), 0, 100)
	if err != nil {
		t.Fatalf("DeleteRunBatch: %v", err)
	}
	if result.RunsDeleted != 1 || result.RunEventsDeleted != 2 || result.RouteAttemptsDeleted != 1 || result.RunSkillsDeleted != 1 {
		t.Fatalf("result = %+v, want RunsDeleted=1 RunEventsDeleted=2 RouteAttemptsDeleted=1 RunSkillsDeleted=1", result)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM run_events WHERE run_id = ?`, runID); n != 0 {
		t.Fatalf("%d run_events rows remain referencing a deleted run", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_run_route_attempts WHERE run_id = ?`, runID); n != 0 {
		t.Fatalf("%d route attempt rows remain referencing a deleted run", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_run_skills WHERE run_id = ?`, runID); n != 0 {
		t.Fatalf("%d run skill rows remain referencing a deleted run", n)
	}
}

// TestDeleteRunBatch_SurvivingRunKeepsEveryEvent proves
// AC-OFFICE-RUN-HISTORY-RETENTION-001.6: run_events is never deleted for a
// run that is not itself being deleted in the same transaction, no matter
// how old those events are.
func TestDeleteRunBatch_SurvivingRunKeepsEveryEvent(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	liveRunID := newID()
	// queued: live state, never eligible at any age.
	seedRun(t, conn, liveRunID, "agent-1", "queued", nil, daysAgo(400))
	for i := 0; i < 5; i++ {
		seedRunEvent(t, conn, liveRunID, i)
	}

	result, err := store.DeleteRunBatch(ctx, conn, daysAgo(30), 0, 100)
	if err != nil {
		t.Fatalf("DeleteRunBatch: %v", err)
	}
	if result.RunsDeleted != 0 {
		t.Fatalf("RunsDeleted = %d, want 0 (queued run is live state)", result.RunsDeleted)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM run_events WHERE run_id = ?`, liveRunID); n != 5 {
		t.Fatalf("run_events for a surviving run = %d, want 5 untouched", n)
	}
}

// TestDeleteRunBatch_RunResurrectedBeforeSelectionIsNeverSelected proves a
// run resurrected to "queued" (as ScheduleRetry does, clearing finished_at)
// before DeleteRunBatch runs at all is excluded by the eligibility
// selection itself, leaving it and its satellites untouched. This is the
// simple case; the delete-time re-assertion this package's AC-002.4
// re-assertion actually catches — a resurrection landing between
// selection and the delete statement, inside one attempt — is covered by
// TestDeleteRunBatch_MidTransactionResurrectionRetriesThenSurvives below.
func TestDeleteRunBatch_RunResurrectedBeforeSelectionIsNeverSelected(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	runID := newID()
	finished := daysAgo(60)
	seedRun(t, conn, runID, "agent-1", "finished", &finished, finished)
	seedRunEvent(t, conn, runID, 0)

	// Simulate the resurrection race by racing a resurrecting UPDATE
	// against DeleteRunBatch using a second connection to the same
	// in-memory database (SQLite's single-writer serializes them, but the
	// delete's re-assertion inside the transaction is what must catch the
	// now-live row regardless of interleaving — resurrecting up front is
	// the deterministic way to exercise that same code path without
	// depending on goroutine scheduling).
	if _, err := conn.Exec(conn.Rebind(`
		UPDATE runs SET status = 'queued', finished_at = NULL WHERE id = ?
	`), runID); err != nil {
		t.Fatalf("resurrect run: %v", err)
	}

	result, err := store.DeleteRunBatch(ctx, conn, daysAgo(30), 0, 100)
	if err != nil {
		t.Fatalf("DeleteRunBatch: %v", err)
	}
	if result.RunsDeleted != 0 || result.Abandoned {
		t.Fatalf("result = %+v, want a clean no-op (resurrected run was never selected)", result)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE id = ?`, runID); n != 1 {
		t.Fatalf("resurrected run was deleted")
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM run_events WHERE run_id = ?`, runID); n != 1 {
		t.Fatalf("resurrected run's event was deleted")
	}
}

// TestDeleteRunBatch_MidTransactionResurrectionRetriesThenSurvives drives
// the retry path deterministically via testAfterSelectEligibleRunIDs: the
// row is eligible at selection time (so it enters attempt 0's id list),
// resurrected by the hook immediately after that selection (before
// deleteRunBatchOnce's own DELETE runs), so the re-assertion inside the
// transaction detects the mismatch, rolls back, and attempt 1 re-selects
// against the now-live row and finds nothing to do. The run and its
// satellite survive throughout.
func TestDeleteRunBatch_MidTransactionResurrectionRetriesThenSurvives(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	runID := newID()
	finished := daysAgo(60)
	seedRun(t, conn, runID, "agent-1", "finished", &finished, finished)
	seedRunEvent(t, conn, runID, 0)

	t.Cleanup(func() { testAfterSelectEligibleRunIDs = nil })
	testAfterSelectEligibleRunIDs = func(attempt int, ids []string) {
		if attempt != 0 {
			return
		}
		if _, err := conn.Exec(conn.Rebind(
			`UPDATE runs SET status = 'queued', finished_at = NULL WHERE id = ?`,
		), runID); err != nil {
			t.Fatalf("resurrect run mid-transaction: %v", err)
		}
	}

	result, err := store.DeleteRunBatch(ctx, conn, daysAgo(30), 0, 100)
	if err != nil {
		t.Fatalf("DeleteRunBatch: %v", err)
	}
	if result.RunsDeleted != 0 || result.Abandoned {
		t.Fatalf("result = %+v, want a clean no-op after the retry re-selects nothing", result)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE id = ?`, runID); n != 1 {
		t.Fatalf("resurrected run was deleted despite the retry")
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM run_events WHERE run_id = ?`, runID); n != 1 {
		t.Fatalf("resurrected run's event was deleted despite the retry")
	}
}

// TestDeleteRunBatch_AbandonsAfterTwoConsecutiveMismatches forces the
// resurrection race to land on *both* attempts via
// testAfterSelectEligibleRunIDs: the row is repeatedly resurrected right
// after each selection, so both attempts' delete re-assertions mismatch.
// DeleteRunBatch must give up rather than loop forever, reporting the
// batch as Abandoned — which the sweep records as that table's failure,
// not backlog (AC-OFFICE-RUN-HISTORY-RETENTION-002.7).
func TestDeleteRunBatch_AbandonsAfterTwoConsecutiveMismatches(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	runID := newID()
	finished := daysAgo(60)
	seedRun(t, conn, runID, "agent-1", "finished", &finished, finished)
	seedRunEvent(t, conn, runID, 0)

	t.Cleanup(func() {
		testBeforeSelectEligibleRunIDs = nil
		testAfterSelectEligibleRunIDs = nil
	})
	beforeCalls, afterCalls := 0, 0
	// Before each attempt's selection, make sure the row reads terminal
	// again (undoing the previous attempt's resurrection) so it is
	// selected every time, not just on attempt 0.
	testBeforeSelectEligibleRunIDs = func(attempt int) {
		beforeCalls++
		if attempt == 0 {
			return // already terminal from seeding
		}
		if _, err := conn.Exec(conn.Rebind(
			`UPDATE runs SET status = 'finished', finished_at = ? WHERE id = ?`,
		), finished, runID); err != nil {
			t.Fatalf("re-terminalize run before attempt %d: %v", attempt, err)
		}
	}
	// After each attempt's selection (which just proved the row was
	// terminal), flip it live so that attempt's own delete re-assertion
	// mismatches.
	testAfterSelectEligibleRunIDs = func(attempt int, ids []string) {
		afterCalls++
		if _, err := conn.Exec(conn.Rebind(
			`UPDATE runs SET status = 'queued', finished_at = NULL WHERE id = ?`,
		), runID); err != nil {
			t.Fatalf("resurrect run mid-transaction (attempt %d): %v", attempt, err)
		}
	}

	result, err := store.DeleteRunBatch(ctx, conn, daysAgo(30), 0, 100)
	if err != nil {
		t.Fatalf("DeleteRunBatch: %v", err)
	}
	if !result.Abandoned {
		t.Fatalf("result = %+v, want Abandoned=true after two consecutive mismatches", result)
	}
	if result.RunsDeleted != 0 || result.RunEventsDeleted != 0 {
		t.Fatalf("result = %+v, want every count zero on an abandoned batch", result)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE id = ?`, runID); n != 1 {
		t.Fatalf("run was deleted despite an abandoned batch")
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM run_events WHERE run_id = ?`, runID); n != 1 {
		t.Fatalf("run_events was deleted despite an abandoned batch")
	}
	if beforeCalls != 2 || afterCalls != 2 {
		t.Fatalf("before/after hooks invoked %d/%d times, want exactly 2/2 (one per attempt)", beforeCalls, afterCalls)
	}
}

// TestDeleteRunBatch_FloorHeldIndependentlyPerAgentProfile is the runs-table
// equivalent of TestDeleteRoutineRunsBatch_FloorHeldIndependentlyPerRoutine:
// every other DeleteRunBatch test in this file uses a single
// "agent-1" owner, so a regression that dropped runEligibleSubquery's
// PARTITION BY agent_profile_id would otherwise leave the whole suite green.
func TestDeleteRunBatch_FloorHeldIndependentlyPerAgentProfile(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()

	cutoff := daysAgo(30)
	var newestA, newestB string
	for i := 0; i < 5; i++ {
		finished := daysAgo(90 - i) // i=0 oldest (day 90) .. i=4 newest (day 86)
		idA, idB := newID(), newID()
		seedRun(t, conn, idA, "agent-a", "finished", &finished, finished)
		seedRun(t, conn, idB, "agent-b", "finished", &finished, finished)
		if i == 4 {
			newestA, newestB = idA, idB
		}
	}

	// Floor 3 per agent profile: each owns 5 history rows, so 2 are
	// eligible per owner, 4 total.
	result, err := store.DeleteRunBatch(ctx, conn, cutoff, 3, 100)
	if err != nil {
		t.Fatalf("DeleteRunBatch: %v", err)
	}
	if result.RunsDeleted != 4 || result.Abandoned {
		t.Fatalf("result = %+v, want 4 deleted (2 eligible per agent profile, floor 3 held independently)", result)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE agent_profile_id = ?`, "agent-a"); n != 3 {
		t.Fatalf("agent-a remaining = %d, want 3 (its own floor)", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE agent_profile_id = ?`, "agent-b"); n != 3 {
		t.Fatalf("agent-b remaining = %d, want 3 (its own floor)", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE id = ?`, newestA); n != 1 {
		t.Fatalf("agent-a's newest run was deleted; its floor should have protected it")
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs WHERE id = ?`, newestB); n != 1 {
		t.Fatalf("agent-b's newest run was deleted; its floor should have protected it")
	}
}

func TestCountRunEvents_PlainCount(t *testing.T) {
	conn := testDB(t)
	store := NewStore(db.NewPool(conn, conn))
	ctx := context.Background()
	runID := newID()
	finished := daysAgo(1)
	seedRun(t, conn, runID, "agent-1", "finished", &finished, finished)
	for i := 0; i < 4; i++ {
		seedRunEvent(t, conn, runID, i)
	}
	count, err := store.CountRunEvents(ctx, conn)
	if err != nil {
		t.Fatalf("CountRunEvents: %v", err)
	}
	if count != 4 {
		t.Fatalf("CountRunEvents() = %d, want 4", count)
	}
}
