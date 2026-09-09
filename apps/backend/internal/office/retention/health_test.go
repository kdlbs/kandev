package retention

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/health"
)

var errCensusEvaluation = errors.New("census evaluation failed")

func newTestChecker(t *testing.T) (*Checker, *Sweeper, *sqlx.DB) {
	t.Helper()
	sweeper, conn := newTestSweeper(t)
	checker := NewChecker(sweeper.settingsStore, sweeper, sweeper.previewMarker)
	return checker, sweeper, conn
}

func issueIDs(issues []health.Issue) []string {
	ids := make([]string, 0, len(issues))
	for _, i := range issues {
		ids = append(ids, i.ID)
	}
	return ids
}

func hasIssue(issues []health.Issue, id string) bool {
	for _, i := range issues {
		if i.ID == id {
			return true
		}
	}
	return false
}

func TestChecker_NameAndCategory(t *testing.T) {
	checker, _, _ := newTestChecker(t)
	if checker.Name() != "Office run retention" {
		t.Fatalf("Name() = %q", checker.Name())
	}
	if checker.Category() != "office" {
		t.Fatalf("Category() = %q", checker.Category())
	}
}

func TestChecker_FreshInstallNoSweepNoIssues(t *testing.T) {
	checker, sweeper, _ := newTestChecker(t)
	ctx := context.Background()
	sweeper.RunCensus(ctx) // AC-003.11: counts available before any sweep

	issues := checker.Check(ctx)
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none on a fresh, empty, enabled install", issueIDs(issues))
	}
}

func TestChecker_SettingsInvalidRaisesGlobalIssue(t *testing.T) {
	checker, _, conn := newTestChecker(t)
	ctx := context.Background()

	if _, err := conn.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES ('office_run_retention', 'not json', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed unparseable settings: %v", err)
	}

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_settings_invalid") {
		t.Fatalf("issues = %v, want office_retention_settings_invalid", issueIDs(issues))
	}
}

func TestChecker_PreviewMarkerUnreadableRaisesGlobalIssue(t *testing.T) {
	checker, _, conn := newTestChecker(t)
	ctx := context.Background()

	if _, err := conn.Exec(`
		INSERT INTO settings (key, value, updated_at) VALUES ('office_run_retention_preview_completed', 'not json', CURRENT_TIMESTAMP)
	`); err != nil {
		t.Fatalf("seed unparseable preview marker: %v", err)
	}

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_preview_unreadable") {
		t.Fatalf("issues = %v, want office_retention_preview_unreadable", issueIDs(issues))
	}
}

func TestChecker_PreviewPendingNamesTablesWithNonzeroWouldDelete(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	seedRoutine(t, conn, "r-1")
	old := daysAgo(60)
	seedRoutineRun(t, conn, newID(), "r-1", "done", &old, old)
	seedRun(t, conn, newID(), "agent-1", "finished", &old, old)

	sweeper.RunSweep(ctx) // preview pass, both tables have 1 eligible row

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_preview_pending") {
		t.Fatalf("issues = %v, want office_retention_preview_pending", issueIDs(issues))
	}
}

func TestChecker_PreviewWithNothingEligibleRaisesNoWarning(t *testing.T) {
	checker, sweeper, _ := newTestChecker(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	sweeper.RunSweep(ctx) // preview pass, nothing seeded, both tables report zero

	issues := checker.Check(ctx)
	if hasIssue(issues, "office_retention_preview_pending") {
		t.Fatalf("issues = %v, want no preview_pending when the preview found nothing (AC-003.3)", issueIDs(issues))
	}
}

func TestChecker_BacklogRaisesPerSweptTable(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	const eligibleRows = 105
	const batchLimit = 100

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

	sweeper.RunSweep(ctx) // preview pass
	sweeper.RunSweep(ctx) // deleting pass, 105 eligible > batch limit 100

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_backlog:office_routine_runs") {
		t.Fatalf("issues = %v, want office_retention_backlog:office_routine_runs", issueIDs(issues))
	}
	if hasIssue(issues, "office_retention_backlog:runs") {
		t.Fatalf("issues = %v, want no backlog issue for runs (never seeded)", issueIDs(issues))
	}
}

func TestChecker_PreviewedTableNeverRaisesBacklog(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	const eligibleRows = 105
	const batchLimit = 100

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

	sweeper.RunSweep(ctx) // preview pass only: 105 eligible, never backlog (AC-002.3)

	issues := checker.Check(ctx)
	if hasIssue(issues, "office_retention_backlog:office_routine_runs") {
		t.Fatalf("issues = %v, want no backlog issue for a preview pass regardless of eligible count", issueIDs(issues))
	}
}

func TestChecker_AbandonedRunsBatchRaisesFailedIssueForRunsOnly(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()
	saveZeroFloorSettings(t, sweeper)

	old := daysAgo(60)
	runID := newID()
	seedRun(t, conn, runID, "agent-1", "finished", &old, old)
	seedRunEvent(t, conn, runID, 1)

	sweeper.RunSweep(ctx) // preview pass

	testBeforeSelectEligibleRunIDs = func(attempt int) {
		if attempt == 0 {
			return
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

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_failed:runs") {
		t.Fatalf("issues = %v, want office_retention_failed:runs", issueIDs(issues))
	}
	if hasIssue(issues, "office_retention_failed:run_events") {
		t.Fatalf("issues = %v, want no failed issue for run_events (the rollback restored it, not a failure of its own)", issueIDs(issues))
	}
}

func TestChecker_UnknownStatusRaisesWhileDisabledAndBeforeAnySweep(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	settings := DefaultSettings()
	settings.Enabled = false
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "quarantined", nil, daysAgo(1))

	sweeper.RunCensus(ctx) // AC-003.11: census runs independent of the sweep/enabled state

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_unknown_status:office_routine_runs") {
		t.Fatalf("issues = %v, want office_retention_unknown_status:office_routine_runs (001.10, disabled, no sweep ever ran)", issueIDs(issues))
	}
}

func TestChecker_ThresholdExceededWhileEnabledRaisesThresholdID(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	settings := DefaultSettings()
	settings.RoutineRuns.WarnRows = 1
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(2)), daysAgo(2))

	sweeper.RunCensus(ctx)

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_threshold:office_routine_runs") {
		t.Fatalf("issues = %v, want office_retention_threshold:office_routine_runs", issueIDs(issues))
	}
	if hasIssue(issues, "office_retention_disabled:office_routine_runs") {
		t.Fatalf("issues = %v, want no disabled-variant issue while enabled", issueIDs(issues))
	}
}

func TestChecker_ThresholdExceededWhileDisabledRaisesDisabledID(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	settings := DefaultSettings()
	settings.Enabled = false
	settings.RoutineRuns.WarnRows = 1
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(2)), daysAgo(2))

	sweeper.RunCensus(ctx)

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_disabled:office_routine_runs") {
		t.Fatalf("issues = %v, want office_retention_disabled:office_routine_runs (AC-003.7)", issueIDs(issues))
	}
	if hasIssue(issues, "office_retention_threshold:office_routine_runs") {
		t.Fatalf("issues = %v, want no plain threshold issue while disabled", issueIDs(issues))
	}
}

func TestChecker_ZeroWarnRowsDisablesThresholdIssue(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	settings := DefaultSettings()
	settings.RoutineRuns.WarnRows = 0
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	seedRoutine(t, conn, "r-1")
	for i := 0; i < 5; i++ {
		seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(i+1)), daysAgo(i+1))
	}
	sweeper.RunCensus(ctx)

	issues := checker.Check(ctx)
	if hasIssue(issues, "office_retention_threshold:office_routine_runs") || hasIssue(issues, "office_retention_disabled:office_routine_runs") {
		t.Fatalf("issues = %v, want no threshold issue when warn_rows=0", issueIDs(issues))
	}
}

func TestChecker_CountFailedOnlyAfterAPriorSuccess(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))
	sweeper.RunCensus(ctx) // succeeds once

	sweeper.census.RecordRoutineRuns(TableCensus{}, errCensusEvaluation)

	issues := checker.Check(ctx)
	if !hasIssue(issues, "office_retention_count_failed:office_routine_runs") {
		t.Fatalf("issues = %v, want office_retention_count_failed:office_routine_runs after a prior success then a failure", issueIDs(issues))
	}
}

func TestChecker_NotComputedNeverRaisesCountFailed(t *testing.T) {
	checker, sweeper, _ := newTestChecker(t)
	ctx := context.Background()

	sweeper.census.RecordRoutineRuns(TableCensus{}, errCensusEvaluation) // fails, no prior success

	issues := checker.Check(ctx)
	if hasIssue(issues, "office_retention_count_failed:office_routine_runs") {
		t.Fatalf("issues = %v, want no count_failed issue before any evaluation ever succeeded (AC-003.11's not-yet-computed state)", issueIDs(issues))
	}
}

func TestChecker_IssuesSortedByID(t *testing.T) {
	checker, sweeper, conn := newTestChecker(t)
	ctx := context.Background()

	settings := DefaultSettings()
	settings.RoutineRuns.WarnRows = 1
	settings.Runs.WarnRows = 1
	if _, err := sweeper.settingsStore.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	seedRoutine(t, conn, "r-1")
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(1)), daysAgo(1))
	seedRoutineRun(t, conn, newID(), "r-1", "done", timePtr(daysAgo(2)), daysAgo(2))
	seedRun(t, conn, newID(), "agent-1", "finished", timePtr(daysAgo(1)), daysAgo(1))
	seedRun(t, conn, newID(), "agent-1", "finished", timePtr(daysAgo(2)), daysAgo(2))
	sweeper.RunCensus(ctx)

	issues := checker.Check(ctx)
	ids := issueIDs(issues)
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Fatalf("issues not sorted by id: %v", ids)
		}
	}
}
