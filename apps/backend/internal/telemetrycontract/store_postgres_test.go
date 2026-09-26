package telemetrycontract

// Postgres parity coverage for telemetry_activations: the composite primary
// key relies on ON CONFLICT (contract_key, contract_version) DO NOTHING,
// which both dialects support but is worth pinning per ADR 0027.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresActivateAndReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	store, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("NewWithDB: %v", err)
	}
	ctx := context.Background()
	if err := store.Activate(ctx); err != nil {
		t.Fatalf("first Activate: %v", err)
	}

	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM telemetry_activations`); err != nil {
		t.Fatalf("count activations: %v", err)
	}
	if count != len(Registry()) {
		t.Fatalf("activation row count = %d, want %d", count, len(Registry()))
	}

	// Replay: NewWithDB and Activate must both be no-ops on a second boot.
	store2, err := NewWithDB(db, db)
	if err != nil {
		t.Fatalf("replay NewWithDB: %v", err)
	}
	if err := store2.Activate(ctx); err != nil {
		t.Fatalf("replay Activate: %v", err)
	}
	var countAfterReplay int
	if err := db.Get(&countAfterReplay, `SELECT COUNT(*) FROM telemetry_activations`); err != nil {
		t.Fatalf("count activations after replay: %v", err)
	}
	if countAfterReplay != count {
		t.Fatalf("activation row count after replay = %d, want unchanged %d", countAfterReplay, count)
	}
}
