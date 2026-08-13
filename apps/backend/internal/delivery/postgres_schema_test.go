package delivery_test

import (
	"testing"

	"github.com/kandev/kandev/internal/delivery"
	"github.com/kandev/kandev/internal/persistence"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresDeliveryLedgerMigration_FreshAndReplay is the PostgreSQL
// twin ADR 0027 requires for every schema-touching change: fresh init and
// replay both succeed, the table and its foreign keys land as declared,
// and the activation key is written. The task schema (tasks,
// repositories — the ledger's FK targets) must exist before
// delivery.NewWithDB runs: unlike SQLite, PostgreSQL requires FK target
// tables to exist at CREATE TABLE time. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresDeliveryLedgerMigration_FreshAndReplay(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, _, err := taskrepo.Provide(db, db, nil); err != nil {
		t.Fatalf("init task schema: %v", err)
	}

	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init delivery schema: %v", err)
	}
	// Replay against the same database.
	if _, err := delivery.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay delivery schema: %v", err)
	}

	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = 'task_delivery_ledger'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("inspect task_delivery_ledger: %v", err)
	}
	if !exists {
		t.Fatal("task_delivery_ledger table missing after init")
	}

	var fkCount int
	err = db.QueryRow(`
		SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE table_schema = current_schema()
		  AND table_name = 'task_delivery_ledger'
		  AND constraint_type = 'FOREIGN KEY'
	`).Scan(&fkCount)
	if err != nil {
		t.Fatalf("inspect foreign keys: %v", err)
	}
	if fkCount != 2 {
		t.Fatalf("foreign key count = %d, want 2 (task_id, repository_id)", fkCount)
	}

	val, err := persistence.ReadKey(db, "telemetry.delivery_ledger.activated_at")
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected activation key to be written on postgres")
	}
}
