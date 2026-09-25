package automation

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// TestCreateRun_SecondConcurrentDedupKeyReportsUniqueViolation empirically
// confirms both the shape of go-sqlite3's error message for
// idx_automation_runs_dedup_unique (a partial, composite unique index) and
// that IsDedupKeyUniqueViolation recognizes it. A second insert with the same
// (automation_id, dedup_key) is exactly what two instances racing
// admitTriggerLocked's check-then-insert window would each produce.
func TestCreateRun_SecondConcurrentDedupKeyReportsUniqueViolation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

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
	err := store.CreateRun(ctx, second)
	if err == nil {
		t.Fatal("expected the second concurrent insert with the same dedup key to fail")
	}
	if !IsDedupKeyUniqueViolation(err) {
		t.Fatalf("expected IsDedupKeyUniqueViolation to recognize the error, got %v", err)
	}
}

// A blank dedup_key never resolved (see DedupBinding) is legitimately shared
// across many rows — every undeduplicated firing of the same automation
// writes one. The index is declared WHERE dedup_key != ” precisely so this
// is not a violation.
func TestCreateRun_EmptyDedupKeyIsNotConstrained(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

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

// dedupeAutomationRunDuplicateKeys must clear a pre-existing duplicate so the
// unique index this test's own store setup just created (via NewStore ->
// initSchema) does not fail to apply on an upgrading database. This exercises
// the cleanup directly against a table it populates by bypassing CreateRun
// (which would itself now reject the duplicate) with a raw insert, simulating
// data written before this index existed.
func TestDedupeAutomationRunDuplicateKeys_ClearsAllButOnePerPair(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	a := &Automation{WorkspaceID: "ws-1", Name: "x", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	// Insert two duplicates directly, bypassing the (already-live) unique
	// index by writing straight into the table underneath it is not
	// possible once the index exists — so this test instead re-derives the
	// pre-migration shape: drop the index, insert the duplicate, then run
	// the cleanup plus re-creation exactly as initSchema does on next boot.
	if _, err := db.Exec(`DROP INDEX idx_automation_runs_dedup_unique`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"run-1", "run-2"} {
		if _, err := db.Exec(
			`INSERT INTO automation_runs (id, automation_id, trigger_id, trigger_type, status, dedup_key, trigger_data, created_at)
			VALUES (?, ?, 't', 'webhook', 'triggered', 'webhook:dup', '{}', CURRENT_TIMESTAMP)`,
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
