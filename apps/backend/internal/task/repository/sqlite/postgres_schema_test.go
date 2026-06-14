package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresFreshSchemaInitializes(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	if _, err := NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}
}

func TestPostgresSkipsLegacyTaskEnvironmentBackfill(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init fresh postgres schema: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), "task-orphaned", "Orphaned task", now, now); err != nil {
		t.Fatalf("insert orphaned task: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, state, started_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "session-orphaned", "task-orphaned", "CREATED", now, now); err != nil {
		t.Fatalf("insert orphaned session: %v", err)
	}

	if err := repo.backfillTaskEnvironments(); err != nil {
		t.Fatalf("backfill task environments: %v", err)
	}

	var count int
	if err := db.Get(&count, db.Rebind(`
		SELECT COUNT(*) FROM task_environments WHERE task_id = ?
	`), "task-orphaned"); err != nil {
		t.Fatalf("count task environments: %v", err)
	}
	if count != 0 {
		t.Fatalf("task environment count = %d, want 0", count)
	}
}
