package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresCostEventContractMigration is the PostgreSQL twin of
// TestCostEventContractMigration_LegacyRowsGetNullColumns
// (docs/specs/office/requirements/costs.md, ADR 0027): a pre-migration office_cost_events
// table gets the cache-split / provenance / turn-attribution columns added
// nullable, a legacy row is left NULL (never 0), the partial unique index
// exists, and the migration replays cleanly on a second boot. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
//
// The test inserts a cost event row that references a task_id, so the tasks
// table must exist first — initialize via taskrepo, mirroring production boot
// order (see internal/office/repository/sqlite/workflow_test.go).
func TestPostgresCostEventContractMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	if _, err := db.ExecContext(ctx, `
		CREATE TABLE office_cost_events (
			id TEXT PRIMARY KEY,
			session_id TEXT DEFAULT '',
			task_id TEXT DEFAULT '',
			agent_profile_id TEXT DEFAULT '',
			project_id TEXT DEFAULT '',
			model TEXT DEFAULT '',
			provider TEXT DEFAULT '',
			tokens_in BIGINT DEFAULT 0,
			tokens_cached_in BIGINT DEFAULT 0,
			tokens_out BIGINT DEFAULT 0,
			cost_subcents BIGINT NOT NULL DEFAULT 0,
			estimated BOOLEAN NOT NULL DEFAULT false,
			occurred_at TIMESTAMPTZ NOT NULL,
			created_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		t.Fatalf("seed legacy office_cost_events: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO office_cost_events (
			id, task_id, tokens_in, tokens_cached_in, tokens_out,
			cost_subcents, estimated, occurred_at, created_at
		) VALUES (
			'legacy-1', 'task-legacy', 100, 40, 50,
			10, false, now(), now()
		)`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("first boot: %v", err)
	}

	newColumns := []string{
		"tokens_cached_read", "tokens_cached_write", "turn_id", "usage_event_id",
		"cost_source", "rate_input_per_million", "rate_cached_read_per_million",
		"rate_cached_write_per_million", "rate_output_per_million",
		"pricing_catalog_version", "cost_contract_version",
	}
	for _, col := range newColumns {
		var got *string
		if err := db.QueryRowContext(ctx,
			`SELECT `+col+`::text FROM office_cost_events WHERE id = 'legacy-1'`,
		).Scan(&got); err != nil {
			t.Fatalf("select %s on legacy row: %v", col, err)
		}
		if got != nil {
			t.Errorf("legacy row %s = %v, want NULL (never 0)", col, *got)
		}
	}

	var indexName string
	if err := db.QueryRowContext(ctx, `
		SELECT indexname FROM pg_indexes
		WHERE tablename = 'office_cost_events' AND indexname = 'uniq_office_cost_usage_event'
	`).Scan(&indexName); err != nil {
		t.Fatalf("uniq_office_cost_usage_event index is missing: %v", err)
	}

	// Second boot: the migration must replay without error.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second boot (replay): %v", err)
	}
}
