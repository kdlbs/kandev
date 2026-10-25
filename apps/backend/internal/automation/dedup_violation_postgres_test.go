package automation

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresCreateRun_SecondConcurrentDedupKeyReportsUniqueViolation is the
// PostgreSQL twin of TestCreateRun_SecondConcurrentDedupKeyReportsUniqueViolation.
// IsDedupKeyUniqueViolation's PostgreSQL branch reads pgErr.ConstraintName off a
// typed pgconn.PgError — unlike the SQLite string-match branch, that assumption
// has never run against a real PostgreSQL error until this test. It also
// exercises initSchema's migration ordering (dedupeAutomationRunDuplicateKeys
// then migrateRunDedupUniqueIndexSQL) against Postgres's own CREATE UNIQUE
// INDEX ... WHERE partial-index syntax. Skips unless KANDEV_TEST_POSTGRES_DSN
// is set.
func TestPostgresCreateRun_SecondConcurrentDedupKeyReportsUniqueViolation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("init automation store: %v", err)
	}

	a := &Automation{WorkspaceID: "ws-1", Name: "x", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	first := &AutomationRun{AutomationID: a.ID, TriggerID: "t-1", TriggerType: TriggerTypeWebhook,
		Status: RunStatusTriggered, DedupKey: "webhook:alert-1"}
	if err := store.CreateRun(ctx, first); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := &AutomationRun{AutomationID: a.ID, TriggerID: "t-2", TriggerType: TriggerTypeWebhook,
		Status: RunStatusTriggered, DedupKey: "webhook:alert-1"}
	err = store.CreateRun(ctx, second)
	if err == nil {
		t.Fatal("expected the second concurrent insert with the same dedup key to fail")
	}
	if !IsDedupKeyUniqueViolation(err) {
		t.Fatalf("expected IsDedupKeyUniqueViolation to recognize the error, got %v", err)
	}
}

// TestPostgresCreateRun_EmptyDedupKeyIsNotConstrained is the PostgreSQL twin of
// TestCreateRun_EmptyDedupKeyIsNotConstrained, proving the partial index's
// WHERE dedup_key != ” predicate is honored by Postgres the same way it is by
// SQLite: many undeduplicated rows may share an empty dedup_key.
func TestPostgresCreateRun_EmptyDedupKeyIsNotConstrained(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("init automation store: %v", err)
	}

	a := &Automation{WorkspaceID: "ws-1", Name: "x", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		run := &AutomationRun{AutomationID: a.ID, TriggerID: "t", TriggerType: TriggerTypeWebhook,
			Status: RunStatusTriggered, DedupKey: ""}
		if err := store.CreateRun(ctx, run); err != nil {
			t.Fatalf("insert %d with empty dedup key: %v", i, err)
		}
	}
}

// TestPostgresDedupeAutomationRunDuplicateKeys_ClearsAllButOnePerPair is the
// PostgreSQL twin of TestDedupeAutomationRunDuplicateKeys_ClearsAllButOnePerPair,
// proving the pre-index cleanup pass (required so an upgrading database with
// pre-existing duplicates doesn't fail to boot when the unique index is
// created) uses only ANSI-portable SQL (MIN/GROUP BY, no SQLite-only syntax).
func TestPostgresDedupeAutomationRunDuplicateKeys_ClearsAllButOnePerPair(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("init automation store: %v", err)
	}
	a := &Automation{WorkspaceID: "ws-1", Name: "x", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`DROP INDEX idx_automation_runs_dedup_unique`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run-1", "run-2"} {
		if _, err := db.Exec(
			`INSERT INTO automation_runs (id, automation_id, trigger_id, trigger_type, status, dedup_key, trigger_data, created_at)
			VALUES ($1, $2, 't', 'webhook', 'triggered', 'webhook:dup', '{}', CURRENT_TIMESTAMP)`,
			id, a.ID); err != nil {
			t.Fatalf("seed duplicate %s: %v", id, err)
		}
	}

	if err := store.dedupeAutomationRunDuplicateKeys(); err != nil {
		t.Fatalf("dedupe: %v", err)
	}
	if _, err := db.Exec(schemaSQLForDriver(migrateRunDedupUniqueIndexSQL, db.DriverName())); err != nil {
		t.Fatalf("re-create unique index after cleanup: %v", err)
	}

	var withKey int
	if err := db.Get(&withKey, `SELECT COUNT(*) FROM automation_runs WHERE dedup_key = 'webhook:dup'`); err != nil {
		t.Fatal(err)
	}
	if withKey != 1 {
		t.Fatalf("expected exactly one row to retain the duplicate key, got %d", withKey)
	}
}
