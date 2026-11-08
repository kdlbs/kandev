package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/db"
)

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21
func TestCircuitFailureCountAndUTCTimesRoundTrip(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	zone := time.FixedZone("UTC+9", 9*60*60)
	until := time.Date(2026, 10, 20, 9, 30, 0, 0, zone)
	if err := repo.SaveCircuit(ctx, dynamicruntime.CircuitSnapshot{
		Key: "profile:candidate-a", State: dynamicruntime.CircuitOpen,
		Until: until, Code: routingerr.CodeProviderUnavailable, Failures: 3,
	}); err != nil {
		t.Fatalf("SaveCircuit: %v", err)
	}
	snapshots, err := repo.LoadCircuits(ctx)
	if err != nil {
		t.Fatalf("LoadCircuits: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("LoadCircuits = %#v, want one snapshot", snapshots)
	}
	loaded := snapshots[0]
	if loaded.Failures != 3 {
		t.Fatalf("Failures = %d, want 3", loaded.Failures)
	}
	if !loaded.Until.Equal(until) || loaded.Until.Location() != time.UTC {
		t.Fatalf("Until = %v (%v), want %v in UTC", loaded.Until, loaded.Until.Location(), until.UTC())
	}
}

// A successful probe closes the circuit but keeps its count; a restart must
// still restore that count, so the closed row is loaded too.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21
func TestClosedCircuitFailureCountSurvivesRestart(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	options := []dynamicruntime.CircuitRegistryOption{
		dynamicruntime.WithCircuitPersistence(repo), dynamicruntime.WithCircuitClock(clock),
	}
	before := dynamicruntime.NewCircuitRegistry(options...)
	before.OpenFailure("profile:candidate-a", dynamicruntime.FailureOpen{Code: routingerr.CodeRateLimited, Now: now})
	now = now.Add(2 * time.Minute)
	lease, ok := before.AcquireProbe("profile:candidate-a", time.Second)
	if !ok {
		t.Fatal("AcquireProbe failed after expiry")
	}
	before.ReleaseProbe(lease, true, time.Minute)

	after := dynamicruntime.NewCircuitRegistry(options...)
	if err := after.Restore(ctx); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if after.IsOpen("profile:candidate-a", now) {
		t.Fatal("the restored circuit is open after a successful probe")
	}
	if got := restoredCircuitFailures(t, after, "profile:candidate-a"); got != 1 {
		t.Fatalf("restored failures = %d, want 1", got)
	}
	if !after.ResetFailures("profile:candidate-a") {
		t.Fatal("ResetFailures did not clear the restored count")
	}
	snapshots, err := repo.LoadCircuits(ctx)
	if err != nil {
		t.Fatalf("LoadCircuits: %v", err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("LoadCircuits after reset = %#v, want no rows to restore", snapshots)
	}
}

// An existing database whose circuit table predates the failure count gains
// the column with a zero default, and its open circuits still load.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.21
func TestCircuitFailureCountMigrationAddsColumnToLegacyTable(t *testing.T) {
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	until := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	if _, err := sqlxDB.Exec(`
		CREATE TABLE dynamic_resource_circuits (
			resource_key TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			until_at TIMESTAMP,
			code TEXT NOT NULL DEFAULT '',
			probe_until TIMESTAMP,
			updated_at TIMESTAMP NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy circuit table: %v", err)
	}
	if _, err := sqlxDB.Exec(`
		INSERT INTO dynamic_resource_circuits (resource_key, state, until_at, code, updated_at)
		VALUES (?, ?, ?, ?, ?)`, "profile:legacy", "open", until, "rate_limited", until); err != nil {
		t.Fatalf("seed legacy circuit: %v", err)
	}

	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("NewWithDB on legacy schema: %v", err)
	}
	snapshots, err := repo.LoadCircuits(context.Background())
	if err != nil {
		t.Fatalf("LoadCircuits: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].Key != "profile:legacy" || snapshots[0].Failures != 0 ||
		!snapshots[0].Until.Equal(until) {
		t.Fatalf("migrated circuits = %#v, want the legacy row with zero failures", snapshots)
	}
	if _, err := NewWithDB(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("schema replay: %v", err)
	}
}

func restoredCircuitFailures(t *testing.T, registry *dynamicruntime.CircuitRegistry, key string) int {
	t.Helper()
	for _, snapshot := range registry.Snapshots() {
		if snapshot.Key == key {
			return snapshot.Failures
		}
	}
	t.Fatalf("circuit %s missing", key)
	return 0
}
