package telemetrycontract

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func observerLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}

func TestLogHealthEmitsOneLinePerRegisteredContract(t *testing.T) {
	db := memDB(t)
	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	// task_session_turns doesn't exist in this isolated test DB, so its
	// existence check should report exists=false rather than erroring the
	// whole health pass.
	log, logs := observerLogger(t)
	store.LogHealth(context.Background(), log)

	entries := logs.FilterMessage("telemetry.contract.health").All()
	if len(entries) != len(Registry()) {
		t.Fatalf("health lines = %d, want %d (one per registered contract)", len(entries), len(Registry()))
	}
	for i, c := range Registry() {
		ctxMap := entries[i].ContextMap()
		if ctxMap["contract_key"] != c.Key {
			t.Errorf("entry %d contract_key = %v, want %v", i, ctxMap["contract_key"], c.Key)
		}
		if exists, ok := ctxMap["exists"].(bool); !ok || exists {
			t.Errorf("entry %d exists = %v, want false (backing table absent in isolated test DB)", i, ctxMap["exists"])
		}
	}
}

func TestLogHealthReportsExistingTableAndRowCount(t *testing.T) {
	db := memDB(t)
	if _, err := db.Exec(`
		CREATE TABLE task_session_turns (
			id TEXT PRIMARY KEY,
			metadata TEXT,
			started_at TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create backing table: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO task_session_turns (id, metadata, started_at)
		VALUES ('turn-1', '{"workflow_step_id_at_start":"step-a"}', '2026-01-01T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed stamped turn: %v", err)
	}

	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	log, logs := observerLogger(t)
	store.LogHealth(context.Background(), log)

	entries := logs.FilterMessage("telemetry.contract.health").All()
	if len(entries) != len(Registry()) {
		t.Fatalf("health lines = %d, want %d", len(entries), len(Registry()))
	}
	var found bool
	for _, e := range entries {
		ctxMap := e.ContextMap()
		if ctxMap["contract_key"] != "turn.workflow_step_id_at_start" {
			continue
		}
		found = true
		if exists, ok := ctxMap["exists"].(bool); !ok || !exists {
			t.Fatalf("exists = %v, want true", ctxMap["exists"])
		}
		if rowCount, ok := ctxMap["row_count"].(int64); !ok || rowCount != 1 {
			t.Fatalf("row_count = %v, want 1", ctxMap["row_count"])
		}
	}
	if !found {
		t.Fatal("no health line for turn.workflow_step_id_at_start")
	}
}

func TestLogHealthReportsTaskStepTransitionsTableAndRowCount(t *testing.T) {
	db := memDB(t)
	if _, err := db.Exec(`
		CREATE TABLE task_step_transitions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id TEXT NOT NULL,
			occurred_at TIMESTAMP NOT NULL
		)
	`); err != nil {
		t.Fatalf("create backing table: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO task_step_transitions (task_id, occurred_at) VALUES
			('task-1', '2026-01-01T00:00:00Z'),
			('task-2', '2026-01-02T00:00:00Z')
	`); err != nil {
		t.Fatalf("seed ledger rows: %v", err)
	}

	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	log, logs := observerLogger(t)
	store.LogHealth(context.Background(), log)

	entries := logs.FilterMessage("telemetry.contract.health").All()
	var found bool
	for _, e := range entries {
		ctxMap := e.ContextMap()
		if ctxMap["contract_key"] != "task_step_transitions" {
			continue
		}
		found = true
		if exists, ok := ctxMap["exists"].(bool); !ok || !exists {
			t.Fatalf("exists = %v, want true", ctxMap["exists"])
		}
		if rowCount, ok := ctxMap["row_count"].(int64); !ok || rowCount != 2 {
			t.Fatalf("row_count = %v, want 2", ctxMap["row_count"])
		}
		if ctxMap["most_recent_occurred_at"] == nil {
			t.Fatal("most_recent_occurred_at field is missing")
		}
	}
	if !found {
		t.Fatal("no health line for task_step_transitions")
	}
}

func TestLogHealthReportsActivatedAt(t *testing.T) {
	// Activate first so telemetry_activations has a row for every registered
	// contract, then assert LogHealth actually surfaces it: activated_at is
	// built from contractHealth.activatedAt and only appended to the log
	// fields when non-nil (store.go:96-98), but until this test nothing read
	// the field back, so a regression that stopped populating it (e.g. an
	// error swallowed by activatedAt, or the field silently dropped from the
	// zap.Field slice) would leave the whole suite green.
	db := memDB(t)
	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	if err := store.Activate(context.Background()); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	log, logs := observerLogger(t)
	store.LogHealth(context.Background(), log)

	entries := logs.FilterMessage("telemetry.contract.health").All()
	if len(entries) != len(Registry()) {
		t.Fatalf("health lines = %d, want %d", len(entries), len(Registry()))
	}
	for i, c := range Registry() {
		ctxMap := entries[i].ContextMap()
		activatedAt, ok := ctxMap["activated_at"].(time.Time)
		if !ok {
			t.Fatalf("entry %d (%s) activated_at missing or wrong type: %v", i, c.Key, ctxMap["activated_at"])
		}
		if activatedAt.IsZero() {
			t.Fatalf("entry %d (%s) activated_at is zero, want a real activation timestamp", i, c.Key)
		}
	}
}

func TestLogHealthNoopWhenLoggerNil(t *testing.T) {
	db := memDB(t)
	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	// Must not panic.
	store.LogHealth(context.Background(), nil)
}
