package requiredstores

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/startup"
)

func TestPersistenceContentionFixture(t *testing.T) {
	for _, test := range []struct {
		name       string
		stage      string
		poolToHold string
	}{
		{name: "writer pool", stage: "writer_ping", poolToHold: "writer"},
		{name: "reader pool", stage: "reader_ping", poolToHold: "reader"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := openSingleConnectionSQLite(t)
			reader := openSingleConnectionSQLite(t)
			pool := db.NewPool(writer, reader)
			t.Cleanup(func() { _ = pool.Close() })
			if _, err := writer.Exec("CREATE TABLE required_probe (id TEXT PRIMARY KEY)"); err != nil {
				t.Fatalf("create required table: %v", err)
			}
			tracker, err := NewTracker([]Descriptor{{
				ID: "fixture", OwnerPackage: "fixture", RequiredTables: []string{"required_probe"},
				Sweep: startup.StepStoresRepositories,
			}})
			if err != nil {
				t.Fatalf("NewTracker: %v", err)
			}
			if err := tracker.RecordSuccess("fixture"); err != nil {
				t.Fatalf("RecordSuccess: %v", err)
			}
			health := NewHealth(tracker, pool, nil)
			if err := health.Check(context.Background()); err != nil {
				t.Fatalf("initial health check: %v", err)
			}

			blocked := writer
			if test.poolToHold == "reader" {
				blocked = reader
			}
			tx, err := blocked.Beginx()
			if err != nil {
				t.Fatalf("hold %s connection: %v", test.poolToHold, err)
			}
			t.Cleanup(func() { _ = tx.Rollback() })
			before := blocked.Stats()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			started := time.Now()
			diagnostic, probeErr := health.check(ctx)
			elapsed := time.Since(started)
			cancel()
			after := blocked.Stats()
			if probeErr == nil {
				t.Fatal("health check passed while the selected pool connection was held")
			}
			if ctx.Err() != context.DeadlineExceeded {
				t.Fatalf("context error = %v, want deadline exceeded", ctx.Err())
			}
			if diagnostic == nil || diagnostic.stage != test.stage {
				t.Fatalf("probe diagnostic = %#v, want stage %q", diagnostic, test.stage)
			}
			waitCountDelta := after.WaitCount - before.WaitCount
			waitDuration := after.WaitDuration - before.WaitDuration
			if waitCountDelta == 0 || waitDuration <= 0 {
				t.Fatalf("pool wait count delta = %d, duration = %s", waitCountDelta, waitDuration)
			}
			if health.Healthy() {
				t.Fatal("health remained healthy after the bounded pool wait")
			}
			t.Logf("held_operation=transaction_holding_single_connection probe_stage=%s queue_wait_count_delta=%d queue_wait_duration=%s probe_elapsed=%s", diagnostic.stage, waitCountDelta, waitDuration, elapsed)

			if err := tx.Rollback(); err != nil {
				t.Fatalf("release %s connection: %v", test.poolToHold, err)
			}
			if err := health.Check(context.Background()); err != nil {
				t.Fatalf("health recovery check: %v", err)
			}
			if !health.Healthy() {
				t.Fatal("health did not recover after releasing the held connection")
			}
			t.Log("recovery=successful")
		})
	}
}

func openSingleConnectionSQLite(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
