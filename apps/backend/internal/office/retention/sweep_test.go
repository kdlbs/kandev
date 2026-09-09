package retention

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

// newTestSweeper builds a Sweeper over one in-memory SQLite database
// carrying both the office schema (routine runs, plain runs, satellites)
// and the settings schema, so a sweep's table deletes and its
// settings/preview reads share one connection exactly as they do on the
// writer pool in production.
func newTestSweeper(t *testing.T) (*Sweeper, *sqlx.DB) {
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
	pool := db.NewPool(conn, conn)
	settingsRaw, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("init settings schema: %v", err)
	}

	store := NewStore(pool)
	settingsStore := NewSettingsStore(settingsRaw)
	previewMarker := NewPreviewMarkerStore(settingsRaw)
	return NewSweeper(pool, store, settingsStore, previewMarker), conn
}

func TestRunSweep_FirstPassPreviewsBothTablesWithoutDeleting(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)
	seedRun(t, conn, newID(), "agent-1", "finished", &old, old)

	sweeper.RunSweep(ctx)

	last, ok := sweeper.LastSweepSnapshot()
	if !ok {
		t.Fatal("LastSweepSnapshot: ok = false, want true")
	}
	if !last.OfficeRoutineRuns.Previewed || last.OfficeRoutineRuns.WouldDelete != 1 {
		t.Fatalf("office_routine_runs = %+v, want previewed with WouldDelete=1", last.OfficeRoutineRuns)
	}
	if last.OfficeRoutineRuns.Deleted != 0 {
		t.Fatalf("office_routine_runs.Deleted = %d, want 0 on a preview pass", last.OfficeRoutineRuns.Deleted)
	}
	if !last.Runs.Previewed || last.Runs.WouldDelete != 1 {
		t.Fatalf("runs = %+v, want previewed with WouldDelete=1", last.Runs)
	}
	if last.Runs.Deleted != 0 {
		t.Fatalf("runs.Deleted = %d, want 0 on a preview pass", last.Runs.Deleted)
	}

	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs`); n != 1 {
		t.Fatalf("office_routine_runs rows after preview = %d, want 1 (nothing deleted)", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs`); n != 1 {
		t.Fatalf("runs rows after preview = %d, want 1 (nothing deleted)", n)
	}
}

func TestRunSweep_SecondPassDeletesAfterPreview(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)
	runID := newID()
	seedRun(t, conn, runID, "agent-1", "finished", &old, old)
	seedRunEvent(t, conn, runID, 1)

	sweeper.RunSweep(ctx) // preview pass
	sweeper.RunSweep(ctx) // deleting pass

	last, ok := sweeper.LastSweepSnapshot()
	if !ok {
		t.Fatal("LastSweepSnapshot: ok = false, want true")
	}
	if last.OfficeRoutineRuns.Previewed {
		t.Fatal("office_routine_runs: second sweep should not be a preview pass")
	}
	if last.OfficeRoutineRuns.Deleted != 1 {
		t.Fatalf("office_routine_runs.Deleted = %d, want 1", last.OfficeRoutineRuns.Deleted)
	}
	if last.Runs.Previewed {
		t.Fatal("runs: second sweep should not be a preview pass")
	}
	if last.Runs.Deleted != 1 {
		t.Fatalf("runs.Deleted = %d, want 1", last.Runs.Deleted)
	}
	if last.RunEvents.Deleted != 1 {
		t.Fatalf("run_events.Deleted = %d, want 1", last.RunEvents.Deleted)
	}

	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs`); n != 0 {
		t.Fatalf("office_routine_runs rows after delete = %d, want 0", n)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM runs`); n != 0 {
		t.Fatalf("runs rows after delete = %d, want 0", n)
	}
}

func TestRunSweep_BacklogFlaggedWhenEligibleExceedsBatchLimit(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()

	const eligibleRows = 105
	const batchLimit = 100 // minBatchLimit; AC-004.3 forbids going lower

	seedRoutine(t, conn, "r-1")
	for i := 0; i < eligibleRows; i++ {
		old := daysAgo(60 + i)
		seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)
	}

	settings := DefaultSettings()
	settings.BatchLimit = batchLimit
	settings.RoutineRuns.FloorPerOwner = 0
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	sweeper.RunSweep(ctx) // preview pass, no deletion, no batch limit involved
	sweeper.RunSweep(ctx) // deleting pass: 105 eligible, batch limit 100

	last, ok := sweeper.LastSweepSnapshot()
	if !ok {
		t.Fatal("LastSweepSnapshot: ok = false")
	}
	if last.OfficeRoutineRuns.Deleted != batchLimit {
		t.Fatalf("Deleted = %d, want %d (capped by batch limit)", last.OfficeRoutineRuns.Deleted, batchLimit)
	}
	if !last.OfficeRoutineRuns.Backlog {
		t.Fatal("Backlog = false, want true (105 eligible > batch limit 100)")
	}
}

func TestRunSweep_DisabledSkipsSweepWithoutRecordingSkip(t *testing.T) {
	sweeper, _ := newTestSweeper(t)
	ctx := context.Background()

	settings := DefaultSettings()
	settings.Enabled = false
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	sweeper.RunSweep(ctx)

	if _, ok := sweeper.LastSweepSnapshot(); ok {
		t.Fatal("LastSweepSnapshot: ok = true, want false (disabled means no sweep ran)")
	}
	count, _ := sweeper.SkipSnapshot()
	if count != 0 {
		t.Fatalf("skip count = %d, want 0 (disabled is not a recorded skip)", count)
	}
}

func TestRunSweep_SettingsUnreadableSkipsAndRecordsSkip(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()

	if _, err := conn.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES ('office_run_retention', 'not json', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed unparseable settings: %v", err)
	}

	sweeper.RunSweep(ctx)

	if _, ok := sweeper.LastSweepSnapshot(); ok {
		t.Fatal("LastSweepSnapshot: ok = true, want false")
	}
	count, lastAt := sweeper.SkipSnapshot()
	if count != 1 {
		t.Fatalf("skip count = %d, want 1", count)
	}
	if lastAt.IsZero() {
		t.Fatal("lastAt is zero, want a recorded skip time")
	}
}

func TestRunSweep_ConcurrentAttemptRecordsSkipWithoutRunning(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)

	// Simulate a sweep already in flight rather than racing goroutines
	// against SQLite's speed, which would make the collision
	// non-deterministic.
	sweeper.mu.Lock()
	sweeper.sweeping = true
	sweeper.mu.Unlock()

	sweeper.RunSweep(ctx)

	if _, ok := sweeper.LastSweepSnapshot(); ok {
		t.Fatal("LastSweepSnapshot: ok = true, want false (the attempt must not have run)")
	}
	count, _ := sweeper.SkipSnapshot()
	if count != 1 {
		t.Fatalf("skip count = %d, want 1", count)
	}
	if n := countRows(t, conn, `SELECT COUNT(*) FROM office_routine_runs`); n != 1 {
		t.Fatalf("rows = %d, want 1 (a blocked attempt must not preview or delete)", n)
	}
}

func TestRunSweep_AbandonedRunsBatchReportsFailureNotBacklogWithZeroSatellites(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	old := daysAgo(60)
	runID := newID()
	seedRun(t, conn, runID, "agent-1", "finished", &old, old)
	seedRunEvent(t, conn, runID, 1)

	sweeper.RunSweep(ctx) // preview pass

	// Before each attempt's selection, make sure the row reads terminal
	// again (undoing the previous attempt's resurrection) so it is
	// selected every time; after selection, flip it live so that
	// attempt's own delete re-assertion mismatches — forcing both
	// attempts to roll back and the batch to abandon.
	testBeforeSelectEligibleRunIDs = func(attempt int) {
		if attempt == 0 {
			return // already terminal from seeding
		}
		conn.MustExec(conn.Rebind(`UPDATE runs SET status = 'finished', finished_at = ? WHERE id = ?`), old, runID)
	}
	testAfterSelectEligibleRunIDs = func(int, []string) {
		conn.MustExec(conn.Rebind(`UPDATE runs SET status = 'queued', finished_at = NULL WHERE id = ?`), runID)
	}
	t.Cleanup(func() {
		testBeforeSelectEligibleRunIDs = nil
		testAfterSelectEligibleRunIDs = nil
	})

	sweeper.RunSweep(ctx) // deleting pass: forced to abandon

	last, ok := sweeper.LastSweepSnapshot()
	if !ok {
		t.Fatal("LastSweepSnapshot: ok = false")
	}
	if last.Runs.Err == "" {
		t.Fatal("runs.Err is empty, want the abandon failure recorded")
	}
	if last.Runs.Backlog {
		t.Fatal("runs.Backlog = true, want false (an abandoned batch is a failure, not backlog)")
	}
	if last.Runs.Deleted != 0 {
		t.Fatalf("runs.Deleted = %d, want 0", last.Runs.Deleted)
	}
	if last.RunEvents.Deleted != 0 {
		t.Fatalf("run_events.Deleted = %d, want 0 (rollback restored it)", last.RunEvents.Deleted)
	}
}

func TestLastSweepSnapshot_FalseBeforeFirstSweep(t *testing.T) {
	sweeper, _ := newTestSweeper(t)
	if _, ok := sweeper.LastSweepSnapshot(); ok {
		t.Fatal("ok = true before any sweep has run, want false")
	}
}

func TestRunCensus_PopulatesRetainedCountsForAllThreeTables(t *testing.T) {
	sweeper, conn := newTestSweeper(t)
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))
	runID := newID()
	seedRun(t, conn, runID, "agent-1", "finished", timePtr(daysAgo(1)), daysAgo(1))
	seedRunEvent(t, conn, runID, 1)

	sweeper.RunCensus(ctx)

	counts := sweeper.CensusSnapshot()
	if counts.OfficeRoutineRuns.State != CensusFresh || counts.OfficeRoutineRuns.RetainedCount != 1 {
		t.Fatalf("office_routine_runs census = %+v, want fresh with 1", counts.OfficeRoutineRuns)
	}
	if counts.Runs.State != CensusFresh || counts.Runs.RetainedCount != 1 {
		t.Fatalf("runs census = %+v, want fresh with 1", counts.Runs)
	}
	if counts.RunEvents.State != CensusFresh || counts.RunEvents.RetainedCount != 1 {
		t.Fatalf("run_events census = %+v, want fresh with 1", counts.RunEvents)
	}
}

// saveZeroFloorSettings drops both tables' floor to 0 so a test's single
// seeded row is eligible: the default floor of 50 protects the newest 50
// rows per owner, which a one-row fixture never exceeds.
func saveZeroFloorSettings(t *testing.T, sweeper *Sweeper) {
	t.Helper()
	settings := DefaultSettings()
	settings.RoutineRuns.FloorPerOwner = 0
	settings.Runs.FloorPerOwner = 0
	if _, err := sweeper.settingsStore.SaveSettings(context.Background(), settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
}
