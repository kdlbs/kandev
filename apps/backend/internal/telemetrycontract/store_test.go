package telemetrycontract

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func memDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestNewWithDBCreatesTableIdempotently(t *testing.T) {
	db := memDB(t)
	if _, err := NewWithDB(db, db); err != nil {
		t.Fatalf("first NewWithDB: %v", err)
	}
	if _, err := NewWithDB(db, db); err != nil {
		t.Fatalf("second NewWithDB (replay): %v", err)
	}
}

func TestActivateWritesOneRowPerRegisteredContract(t *testing.T) {
	db := memDB(t)
	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()
	if err := store.Activate(ctx); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM telemetry_activations`); err != nil {
		t.Fatalf("count activations: %v", err)
	}
	if count != len(Registry()) {
		t.Fatalf("activation row count = %d, want %d (one per registered contract)", count, len(Registry()))
	}

	for _, c := range Registry() {
		var version int
		if err := db.Get(&version, db.Rebind(`
			SELECT contract_version FROM telemetry_activations
			WHERE contract_key = ? AND contract_version = ?
		`), c.Key, c.Version); err != nil {
			t.Fatalf("activation row for %s missing: %v", c.Key, err)
		}
	}
}

func TestActivateLeavesExistingRowUnchanged(t *testing.T) {
	db := memDB(t)
	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()
	if err := store.Activate(ctx); err != nil {
		t.Fatalf("first Activate: %v", err)
	}

	var firstActivatedAt string
	if err := db.Get(&firstActivatedAt, db.Rebind(`
		SELECT activated_at FROM telemetry_activations WHERE contract_key = ? AND contract_version = ?
	`), Registry()[0].Key, Registry()[0].Version); err != nil {
		t.Fatalf("read first activation: %v", err)
	}

	if err := store.Activate(ctx); err != nil {
		t.Fatalf("second Activate: %v", err)
	}

	var secondActivatedAt string
	if err := db.Get(&secondActivatedAt, db.Rebind(`
		SELECT activated_at FROM telemetry_activations WHERE contract_key = ? AND contract_version = ?
	`), Registry()[0].Key, Registry()[0].Version); err != nil {
		t.Fatalf("read second activation: %v", err)
	}

	if firstActivatedAt != secondActivatedAt {
		t.Fatalf("activated_at changed across boots: %q -> %q, want unchanged", firstActivatedAt, secondActivatedAt)
	}
}

func TestActivateAppendsRowOnVersionBump(t *testing.T) {
	db := memDB(t)
	if _, err := NewWithDB(db, db); err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()

	key := "test.versioned_contract"
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO telemetry_activations (contract_key, contract_version, activated_at)
		VALUES (?, 1, ?)
	`), key, "2026-01-01T00:00:00Z"); err != nil {
		t.Fatalf("seed version-1 row: %v", err)
	}
	if _, err := db.ExecContext(ctx, db.Rebind(`
		INSERT INTO telemetry_activations (contract_key, contract_version, activated_at)
		VALUES (?, 2, ?)
	`), key, "2026-02-01T00:00:00Z"); err != nil {
		t.Fatalf("seed version-2 row: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM telemetry_activations WHERE contract_key = ?`), key); err != nil {
		t.Fatalf("count rows for key: %v", err)
	}
	if count != 2 {
		t.Fatalf("rows for %s = %d, want 2 (a version bump appends rather than overwrites)", key, count)
	}
}
