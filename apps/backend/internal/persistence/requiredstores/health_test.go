package requiredstores

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/startup"
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
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories,
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
		{ID: "healthy", OwnerPackage: "owner/healthy", RequiredTables: []string{"healthy"}, Sweep: startup.StepStoresRepositories},
		{ID: "unhealthy", OwnerPackage: "owner/unhealthy", RequiredTables: []string{"unhealthy"}, Sweep: startup.StepStoresRepositories},
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
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories,
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
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories,
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

func TestRuntimeHealthProbeFailureLogsBoundedStageAndRecovers(t *testing.T) {
	for _, test := range []struct {
		name      string
		stage     string
		blockedDB string
	}{
		{name: "writer ping", stage: "writer_ping", blockedDB: "writer"},
		{name: "reader ping", stage: "reader_ping", blockedDB: "reader"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer, err := sqlx.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatalf("open writer: %v", err)
			}
			writer.SetMaxOpenConns(1)
			reader, err := sqlx.Open("sqlite3", ":memory:")
			if err != nil {
				_ = writer.Close()
				t.Fatalf("open reader: %v", err)
			}
			reader.SetMaxOpenConns(1)
			pool := db.NewPool(writer, reader)
			t.Cleanup(func() { _ = pool.Close() })
			for _, table := range []string{"first", "second"} {
				if _, err := writer.Exec("CREATE TABLE " + table + " (id TEXT PRIMARY KEY)"); err != nil {
					t.Fatalf("create %s table: %v", table, err)
				}
			}

			descriptors := []Descriptor{
				{ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories},
				{ID: "second", OwnerPackage: "owner/second", RequiredTables: []string{"second"}, Sweep: startup.StepStoresRepositories},
			}
			tracker, err := NewTracker(descriptors)
			if err != nil {
				t.Fatalf("NewTracker: %v", err)
			}
			for _, descriptor := range descriptors {
				if err := tracker.RecordSuccess(descriptor.ID); err != nil {
					t.Fatalf("RecordSuccess(%s): %v", descriptor.ID, err)
				}
			}

			core, observed := observer.New(zapcore.WarnLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("NewFromZap: %v", err)
			}
			health := NewHealth(tracker, pool, log)
			if err := health.Check(context.Background()); err != nil {
				t.Fatalf("initial Check: %v", err)
			}

			blocked := writer
			if test.blockedDB == "reader" {
				blocked = reader
			}
			tx, err := blocked.Beginx()
			if err != nil {
				t.Fatalf("begin blocking transaction: %v", err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })

			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			defer cancel()
			done := make(chan struct{})
			go health.run(ctx, 200*time.Millisecond, done)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("health probe loop did not stop after its context was canceled")
			}

			entries := observed.FilterMessage("required persistence probe failed").All()
			if len(entries) != 1 {
				t.Fatalf("failure warnings = %d, want one bounded warning", len(entries))
			}
			fields := entries[0].ContextMap()
			if got := fields["stage"]; got != test.stage {
				t.Fatalf("failure stage = %v, want %q", got, test.stage)
			}
			if got := fields["error_class"]; got != "deadline_exceeded" {
				t.Fatalf("error_class = %v, want deadline_exceeded", got)
			}
			if got, ok := fields["elapsed_ms"].(float64); !ok || got <= 0 {
				t.Fatalf("elapsed_ms = %v, want a positive number", fields["elapsed_ms"])
			}
			for _, field := range []string{
				"writer_open_connections", "writer_in_use", "writer_wait_count", "writer_wait_duration_ms",
				"reader_open_connections", "reader_in_use", "reader_wait_count", "reader_wait_duration_ms",
			} {
				if _, ok := fields[field]; !ok {
					t.Fatalf("failure warning omitted %q: %#v", field, fields)
				}
			}
			waitField := test.blockedDB + "_wait_count"
			if got, ok := fields[waitField].(int64); !ok || got == 0 {
				t.Fatalf("%s = %v, want evidence of a pool wait", waitField, fields[waitField])
			}
			if _, ok := fields["error"]; ok {
				t.Fatalf("failure warning exposed raw error: %#v", fields["error"])
			}
			if health.Healthy() {
				t.Fatal("health remained healthy after a timed-out probe")
			}

			if err := tx.Rollback(); err != nil {
				t.Fatalf("release blocked connection: %v", err)
			}
			if err := health.Check(context.Background()); err != nil {
				t.Fatalf("recovery Check: %v", err)
			}
			if !health.Healthy() {
				t.Fatal("health did not recover after the blocked pool became available")
			}
		})
	}
}

func TestHealthCheckCapturesTableProbeFailure(t *testing.T) {
	_, _, _, health := newSQLiteHealthFixture(t, []Descriptor{{
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"missing"}, Sweep: startup.StepStoresRepositories,
	}})

	diagnostic, err := health.check(context.Background())
	if err == nil {
		t.Fatal("health check returned nil with a missing required table")
	}
	if diagnostic == nil {
		t.Fatal("health check returned no probe diagnostic")
	}
	if diagnostic.stage != "table_probe" {
		t.Fatalf("diagnostic stage = %q, want table_probe", diagnostic.stage)
	}
	if diagnostic.errorClass != "required_table_missing" {
		t.Fatalf("diagnostic error class = %q, want required_table_missing", diagnostic.errorClass)
	}
	if diagnostic.storeID != "first" {
		t.Fatalf("diagnostic store ID = %q, want first", diagnostic.storeID)
	}
	if diagnostic.elapsed < 0 {
		t.Fatalf("diagnostic elapsed = %s, want a non-negative duration", diagnostic.elapsed)
	}
	for _, field := range diagnostic.logFields() {
		if field.Key == "error" || field.Key == "table" {
			t.Fatalf("table probe diagnostic contains an unsafe field %q", field.Key)
		}
	}
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
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories,
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
		ID: "first", OwnerPackage: "owner/first", RequiredTables: []string{"first"}, Sweep: startup.StepStoresRepositories,
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
