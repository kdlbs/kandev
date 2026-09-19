package requiredstores

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/maintenance"
)

func newSQLiteHealthFixture(t *testing.T, descriptors []Descriptor) (*sqlx.DB, *db.Pool, *Tracker, *Health) {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetMaxOpenConns(1)

	tracker, err := NewTracker(descriptors)
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	pool := db.NewPool(conn, conn)
	health := NewHealth(tracker, pool, nil)
	return conn, pool, tracker, health
}

func holdWriter(t *testing.T, conn *sqlx.DB) *sqlx.Tx {
	t.Helper()
	tx, err := conn.Beginx()
	if err != nil {
		t.Fatalf("begin writer transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return tx
}

func TestRuntimeHealthDefersDuringMaintenance(t *testing.T) {
	conn, pool, tracker, health := newSQLiteHealthFixture(t, []Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"},
	}})
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create first table: %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial Check: %v", err)
	}
	before := tracker.Snapshot()

	release, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("maintenance lease unavailable")
	}
	defer release()
	holdWriter(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	deferred, err := health.checkRuntime(ctx)
	if err != nil {
		t.Fatalf("checkRuntime error = %v, want nil while maintenance owns the pool", err)
	}
	if !deferred {
		t.Fatal("checkRuntime deferred = false, want true while maintenance owns the pool")
	}
	after := tracker.Snapshot()
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("tracker changed during deferred probe: before=%#v after=%#v", before, after)
	}
}

func TestRuntimeHealthDeferralPreservesMixedStoreStates(t *testing.T) {
	conn, pool, tracker, health := newSQLiteHealthFixture(t, []Descriptor{
		{ID: "healthy", OwnerPackage: "owner/healthy", RequiredTables: []string{"healthy"}},
		{ID: "unhealthy", OwnerPackage: "owner/unhealthy", RequiredTables: []string{"unhealthy"}},
	})
	if _, err := conn.Exec("CREATE TABLE healthy (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create healthy table: %v", err)
	}
	for _, id := range []string{"healthy", "unhealthy"} {
		if err := tracker.RecordSuccess(id); err != nil {
			t.Fatalf("RecordSuccess(%q): %v", id, err)
		}
	}
	if err := health.Check(context.Background()); err == nil {
		t.Fatal("initial Check returned nil with a missing table")
	}
	before := tracker.Snapshot()

	release, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("maintenance lease unavailable")
	}
	defer release()
	holdWriter(t, conn)

	deferred, err := health.checkRuntime(context.Background())
	if err != nil {
		t.Fatalf("checkRuntime error = %v, want nil while maintenance owns the pool", err)
	}
	if !deferred {
		t.Fatal("checkRuntime deferred = false, want true while maintenance owns the pool")
	}
	after := tracker.Snapshot()
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("mixed tracker state changed during deferred probe: before=%#v after=%#v", before, after)
	}
}

func TestStartupHealthDoesNotDefer(t *testing.T) {
	conn, pool, tracker, health := newSQLiteHealthFixture(t, []Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"},
	}})
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create first table: %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial Check: %v", err)
	}
	before, ok := tracker.Status("first")
	if !ok {
		t.Fatal("missing first status")
	}

	release, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("maintenance lease unavailable")
	}
	defer release()
	holdWriter(t, conn)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := health.Check(ctx); err == nil {
		t.Fatal("startup Check returned nil while its writer probe was blocked")
	}
	after, ok := tracker.Status("first")
	if !ok {
		t.Fatal("missing first status after Check")
	}
	if after.State != StateUnhealthy {
		t.Fatalf("startup Check state = %q, want %q", after.State, StateUnhealthy)
	}
	if !after.LastCheckedAt.After(before.LastCheckedAt) {
		t.Fatalf("startup Check did not record a newer check time: before=%v after=%v", before.LastCheckedAt, after.LastCheckedAt)
	}
}

func TestRuntimeHealthResumesAfterMaintenance(t *testing.T) {
	conn, pool, tracker, health := newSQLiteHealthFixture(t, []Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"},
	}})
	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("create first table: %v", err)
	}
	if err := tracker.RecordSuccess("first"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := health.Check(context.Background()); err != nil {
		t.Fatalf("initial Check: %v", err)
	}

	release, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("maintenance lease unavailable")
	}
	deferred, err := health.checkRuntime(context.Background())
	if err != nil || !deferred {
		t.Fatalf("checkRuntime during maintenance = deferred %v, err %v; want true, nil", deferred, err)
	}
	release()

	if _, err := conn.Exec("DROP TABLE first"); err != nil {
		t.Fatalf("drop first table: %v", err)
	}
	deferred, err = health.checkRuntime(context.Background())
	if deferred {
		t.Fatal("checkRuntime deferred after maintenance released")
	}
	if err == nil {
		t.Fatal("checkRuntime returned nil with a missing table")
	}
	if health.Healthy() {
		t.Fatal("health remained healthy after a real post-maintenance failure")
	}
	probeLease, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("failed probe leaked maintenance admission")
	}
	probeLease()

	if _, err := conn.Exec("CREATE TABLE first (id TEXT PRIMARY KEY)"); err != nil {
		t.Fatalf("recreate first table: %v", err)
	}
	deferred, err = health.checkRuntime(context.Background())
	if deferred || err != nil {
		t.Fatalf("recovery check = deferred %v, err %v; want false, nil", deferred, err)
	}
	if !health.Healthy() {
		t.Fatal("health did not recover after the missing table was repaired")
	}
	successLease, ok := maintenance.ForPool(pool).TryAcquire()
	if !ok {
		t.Fatal("successful probe leaked maintenance admission")
	}
	successLease()
}

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
