package sqlite

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestBackfillPromptSeqReportsOpaqueStep covers the
// task.prompt_seq.backfill half of AC-PLATFORM-STARTUP-PROGRESS-005.2/.8:
// the opaque step brackets the whole statement pair (backfill UPDATE plus
// counter seed) with a start/completed pair, every boot, since the backfill
// itself is already idempotent (WHERE prompt_seq = 0) and has no separate
// runtime skip condition to respect.
func TestBackfillPromptSeqReportsOpaqueStep(t *testing.T) {
	repo := newRepoForSessionTests(t)
	reporter, logs := newObservedMigrationStepReporter(t)
	ctx := startup.WithReporter(context.Background(), reporter)

	if err := repo.backfillPromptSeq(ctx); err != nil {
		t.Fatalf("backfillPromptSeq: %v", err)
	}

	assertNoPhaseMismatchWarnings(t, logs)
	assertOpaqueStepBracket(t, logs, startup.StepPromptSeqBackfill)
	if snap := reporter.Snapshot(); snap.Step != nil {
		t.Fatalf("Step after backfillPromptSeq = %+v, want nil (step closed)", snap.Step)
	}
}

// TestMessageTimestampsBackfillReportsOpaqueStep covers the
// task.message_timestamps.backfill half of the same ACs: runMigrations
// brackets its single backfill UPDATE the same way, every boot, since the
// statement is already idempotent (WHERE updated_at IS NULL) with no
// runtime skip condition.
func TestMessageTimestampsBackfillReportsOpaqueStep(t *testing.T) {
	repo := newRepoForSessionTests(t)
	reporter, logs := newObservedMigrationStepReporter(t)
	ctx := startup.WithReporter(context.Background(), reporter)

	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	assertNoPhaseMismatchWarnings(t, logs)
	assertOpaqueStepBracket(t, logs, startup.StepMessageTimestampsBackfill)
}

// TestMigrateSubagentContextBackfillOpensStepOnlyOnFirstBoot covers F78's
// rule for a registered step whose work is conditionally skipped at
// runtime: migrateSubagentContextBackfill's guard means the real
// INSERT...SELECT (and therefore the step) only runs once per installation
// (AC-23a). The first call — the one that actually does the scan — must
// open and close the step; a later call, once both activation keys are
// already written, must never open it at all (matching
// TestInitConversationJournalSchemaSkipsStepsOnceBackfilled's rule for the
// journal steps).
func TestMigrateSubagentContextBackfillOpensStepOnlyOnFirstBoot(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)

	firstReporter, firstLogs := newObservedMigrationStepReporter(t)
	firstCtx := startup.WithReporter(context.Background(), firstReporter)
	repo.migrateSubagentContextBackfill(firstCtx)
	assertNoPhaseMismatchWarnings(t, firstLogs)
	assertOpaqueStepBracket(t, firstLogs, startup.StepSubagentContextBackfill)

	secondReporter, secondLogs := newObservedMigrationStepReporter(t)
	secondCtx := startup.WithReporter(context.Background(), secondReporter)
	repo.migrateSubagentContextBackfill(secondCtx)
	for _, entry := range secondLogs.All() {
		if entry.Message == "Startup step started" {
			t.Fatalf("unexpected step start on an already-activated boot: %+v", entry.ContextMap())
		}
	}
}

// assertOpaqueStepBracket asserts exactly one "Startup step started" and one
// "Startup step completed" entry for wantStep, started before completed.
func assertOpaqueStepBracket(t *testing.T, logs *observer.ObservedLogs, wantStep startup.StepID) {
	t.Helper()
	var startedAt, completedAt = -1, -1
	var startedCount, completedCount int
	for i, entry := range logs.All() {
		if entry.ContextMap()["step"] != string(wantStep) {
			continue
		}
		switch entry.Message {
		case "Startup step started":
			startedCount++
			startedAt = i
		case "Startup step completed":
			completedCount++
			completedAt = i
		}
	}
	if startedCount != 1 {
		t.Fatalf("startup step %q started %d times, want 1", wantStep, startedCount)
	}
	if completedCount != 1 {
		t.Fatalf("startup step %q completed %d times, want 1", wantStep, completedCount)
	}
	if startedAt == -1 {
		t.Fatalf("no \"Startup step started\" entry for step %q", wantStep)
	}
	if completedAt == -1 {
		t.Fatalf("no \"Startup step completed\" entry for step %q", wantStep)
	}
	if completedAt < startedAt {
		t.Fatalf("step %q completed (index %d) before it started (index %d)", wantStep, completedAt, startedAt)
	}
}

// newObservedMigrationStepReporter returns a reporter already in the
// applying_migrations phase, plus the observed log every step assertion in
// this file reads.
func newObservedMigrationStepReporter(t *testing.T) (*startup.Reporter, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	reporter := startup.New(log)
	reporter.Set(startup.ApplyingMigrations)
	return reporter, logs
}

// assertNoPhaseMismatchWarnings fails if any step opened under a phase other
// than the one its registry entry declares
// (AC-PLATFORM-STARTUP-PROGRESS-005.7).
func assertNoPhaseMismatchWarnings(t *testing.T, logs *observer.ObservedLogs) {
	t.Helper()
	for _, entry := range logs.All() {
		if entry.Message == "Startup step warning" && entry.ContextMap()["condition"] == "phase_mismatch" {
			t.Fatalf("unexpected phase_mismatch warning: %+v", entry.ContextMap())
		}
	}
}

func TestRunMigrationsPrunesWorkflowStepOrphansOnce(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const (
		workspaceID    = "ws-workflow-orphan-prune"
		liveWorkflowID = "wf-workflow-orphan-prune-live"
	)
	seedWorkspace(t, repo, workspaceID)
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: liveWorkflowID, WorkspaceID: workspaceID, Name: "Live",
	}))
	insertWorkflowDeleteTestStep(t, repo, "step-workflow-prune-live", liveWorkflowID)
	insertWorkflowDeleteTestStep(t, repo, "step-workflow-prune-old", "wf-workflow-prune-old")
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		DELETE FROM kandev_meta WHERE key = ?
	`), workflowStepOrphansPrunedMetaKey)
	require.NoError(t, err)

	require.NoError(t, repo.runMigrations(ctx))
	require.Equal(t, 0, countWorkflowDeleteTestSteps(t, repo, "wf-workflow-prune-old"))
	require.Equal(t, 1, countWorkflowDeleteTestSteps(t, repo, liveWorkflowID))
	require.Equal(t, 1, countWorkflowStepOrphanMarker(t, repo))

	insertWorkflowDeleteTestStep(t, repo, "step-workflow-prune-late", "wf-workflow-prune-late")
	require.NoError(t, repo.runMigrations(ctx))
	require.Equal(t, 1, countWorkflowDeleteTestSteps(t, repo, "wf-workflow-prune-late"))
}

func countWorkflowStepOrphanMarker(t *testing.T, repo *Repository) int {
	t.Helper()
	var count int
	require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`
		SELECT COUNT(*) FROM kandev_meta WHERE key = ?
	`), workflowStepOrphansPrunedMetaKey))
	return count
}

// TestPostgresWorkflowStepOrphanPruneStartsAndSerializes proves that the
// startup migration reaches the one-time orphan prune and that concurrent
// cleanups take the transaction-scoped advisory lock before reading the marker.
func TestPostgresWorkflowStepOrphanPruneStartsAndSerializes(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-pg-workflow-orphan-prune")

	insertWorkflowDeleteTestStep(t, repo, "step-pg-workflow-prune-startup", "wf-pg-workflow-prune-startup")
	clearWorkflowStepOrphanMarker(t, repo)
	require.NoError(t, repo.runMigrations(ctx))
	require.Equal(t, 0, countWorkflowDeleteTestSteps(t, repo, "wf-pg-workflow-prune-startup"))
	require.Equal(t, 1, countWorkflowStepOrphanMarker(t, repo))

	insertWorkflowDeleteTestStep(t, repo, "step-pg-workflow-prune-concurrent", "wf-pg-workflow-prune-concurrent")
	clearWorkflowStepOrphanMarker(t, repo)
	lock, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lock.Rollback() }()
	lockKey := "workflow_step_orphan_migration:" + workflowStepOrphansPrunedMetaKey
	_, err = lock.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- repo.pruneOrphanWorkflowStepsOnce()
		}()
	}
	close(start)
	if err := waitForWorkflowPruneLockWaiters(ctx, db, 2); err != nil {
		_ = lock.Rollback()
		<-results
		<-results
		t.Fatalf("wait for both cleanups to block on the advisory lock: %v", err)
	}
	require.NoError(t, lock.Commit())
	errs := []error{<-results, <-results}
	for _, err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 0, countWorkflowDeleteTestSteps(t, repo, "wf-pg-workflow-prune-concurrent"))
	require.Equal(t, 1, countWorkflowStepOrphanMarker(t, repo))
}

func clearWorkflowStepOrphanMarker(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(`
		DELETE FROM kandev_meta WHERE key = ?
	`), workflowStepOrphansPrunedMetaKey)
	require.NoError(t, err)
}

func waitForWorkflowPruneLockWaiters(ctx context.Context, db *sqlx.DB, want int) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiters int
		if err := db.GetContext(ctx, &waiters, `
			SELECT COUNT(*) FROM pg_stat_activity
			WHERE pid <> pg_backend_pid()
				AND datname = current_database()
				AND wait_event_type = 'Lock'
				AND query LIKE '%pg_advisory_xact_lock%'
		`); err != nil {
			return err
		}
		if waiters >= want {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
