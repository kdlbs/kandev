package sqlite_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/persistence"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

const runOutcomeActivationKey = "telemetry.run_outcome.activated_at"

// TestRunOutcomeMigration_AddsNullableColumn covers the Migration and
// activation scenario: a database created before this feature gets
// runs.outcome, and it is NULL on every pre-existing row (docs/specs/
// task-delivery-ledger/spec.md, "Scenarios § Migration and activation").
func TestRunOutcomeMigration_AddsNullableColumn(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	seedAgentProfile(t, db, "agent-outcome-migration", "ws-outcome-migration")
	ctx := context.Background()

	run := &models.Run{
		AgentProfileID: "agent-outcome-migration",
		Reason:         "task_assigned",
		Payload:        `{}`,
		Status:         "queued",
		CoalescedCount: 1,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}

	var outcome sql.NullString
	if err := db.Get(&outcome, db.Rebind(`SELECT outcome FROM runs WHERE id = ?`), run.ID); err != nil {
		t.Fatalf("select outcome: %v", err)
	}
	if outcome.Valid {
		t.Fatalf("outcome on freshly-queued run = %q, want NULL", outcome.String)
	}
}

// TestRunOutcomeActivation_WrittenOnceAfterSchemaProbe covers "Activation
// points": the key is written only after a positive probe confirms
// runs.outcome exists, and it is never overwritten on replay.
func TestRunOutcomeActivation_WrittenOnceAfterSchemaProbe(t *testing.T) {
	_, db := newTestRepoWithDB(t)

	val, err := persistence.ReadMetaKey(db, runOutcomeActivationKey)
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected telemetry.run_outcome.activated_at to be written after first boot")
	}
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		t.Fatalf("activation value %q is not RFC3339: %v", val, err)
	}

	// Overwrite with a sentinel to prove a replayed boot never re-writes
	// (spec: "never overwritten on replay").
	if _, err := db.Exec(db.Rebind(
		`UPDATE kandev_meta SET value = ? WHERE key = ?`,
	), "sentinel-value", runOutcomeActivationKey); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay init: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay init twice: %v", err)
	}

	got, err := persistence.ReadMetaKey(db, runOutcomeActivationKey)
	if err != nil {
		t.Fatalf("read activation key after replay: %v", err)
	}
	if got != "sentinel-value" {
		t.Fatalf("activation key = %q after replay, want unchanged sentinel-value", got)
	}
}

// TestPostgresRunOutcomeMigration_AddsColumnAndActivates is the PostgreSQL
// twin required by ADR 0027: fresh init and replay both succeed and the
// runs.outcome column and activation key exist. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
//
// The office schema's runs table has a foreign key onto tasks, so the tasks
// table must exist first — initialize via taskrepo, mirroring production
// boot order (see internal/office/repository/sqlite/workflow_test.go).
func TestPostgresRunOutcomeMigration_AddsColumnAndActivates(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	// Replay against the same database.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay postgres schema: %v", err)
	}

	var dataType string
	err := db.QueryRow(`
		SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'runs' AND column_name = 'outcome'
	`).Scan(&dataType)
	if err != nil {
		t.Fatalf("inspect runs.outcome: %v", err)
	}
	if dataType != "text" {
		t.Fatalf("runs.outcome data type = %q, want text", dataType)
	}

	val, err := persistence.ReadMetaKey(db, runOutcomeActivationKey)
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected activation key to be written on postgres")
	}
}
