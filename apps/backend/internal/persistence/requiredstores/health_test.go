package requiredstores

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func TestHealthCheckMarksMissingTableUnhealthyAndRecovers(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	pool := db.NewPool(conn, conn)
	t.Cleanup(func() { _ = pool.Close() })
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create first table: %v", err)
	}

	tracker, err := NewTracker([]Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	health := NewHealth(tracker, pool, nil)
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial Check: %v", err)
	}
	if !health.Healthy() {
		t.Fatal("health is unhealthy after a successful check")
	}
	if _, err := conn.Exec("DROP TABLE first"); err != nil {
		t.Fatalf("drop first table: %v", err)
	}
	if err := health.Check(context.Background()); err == nil {
		t.Fatal("Check after dropping table returned nil")
	}
	if health.Healthy() || tracker.AggregateState() != StateUnhealthy {
		t.Fatalf("health did not become unhealthy: health=%v state=%q", health.Healthy(), tracker.AggregateState())
	}
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("recreate first table: %v", err)
	}
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("recovery Check: %v", err)
	}
	if !health.Healthy() {
		t.Fatal("health did not recover")
	}
}

func TestProbeTablesHonorsContextDeadline(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create first table: %v", err)
	}

	tracker, err := NewTracker([]Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	health := NewHealth(tracker, db.NewPool(conn, conn), nil)
	tx, err := conn.Beginx()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	err = health.probeTables(ctx, tracker.catalog[0])
	if err == nil {
		t.Fatal("probeTables() error = nil, want context deadline error")
	}
	if ctx.Err() == nil {
		t.Fatalf("probeTables() error = %v, context did not expire", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("probeTables() took %s after deadline", elapsed)
	}
}
