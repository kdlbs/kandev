package sqlite

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/startup"
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
	for i, entry := range logs.All() {
		if entry.ContextMap()["step"] != string(wantStep) {
			continue
		}
		switch entry.Message {
		case "Startup step started":
			startedAt = i
		case "Startup step completed":
			completedAt = i
		}
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
