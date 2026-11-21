package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresTaskSessionEnvironmentTaskIndexFreshUpgradeReplay is the
// PostgreSQL twin of the SQLite lifecycle test. Each scenario owns an isolated
// schema so a missing index or missing environment column cannot leak between
// cases. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresTaskSessionEnvironmentTaskIndexFreshUpgradeReplay(t *testing.T) {
	t.Run("fresh and replay", func(t *testing.T) {
		_, db, fixture := newPostgresSessionIndexRepo(t)
		assertPostgresTaskSessionEnvironmentIndex(t, db)
		fixture.assertPreserved(t, db)

		replayed, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("replay postgres schema: %v", err)
		}
		assertPostgresTaskSessionEnvironmentIndex(t, replayed.db)
		fixture.assertPreserved(t, db)
	})

	t.Run("existing database without index", func(t *testing.T) {
		_, db, fixture := newPostgresSessionIndexRepo(t)
		if _, err := db.Exec("DROP INDEX " + taskSessionEnvironmentTaskIndex); err != nil {
			t.Fatalf("drop postgres environment index: %v", err)
		}
		upgraded, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("upgrade postgres schema: %v", err)
		}
		assertPostgresTaskSessionEnvironmentIndex(t, upgraded.db)
		fixture.assertPreserved(t, db)
	})

	t.Run("identical operator index", func(t *testing.T) {
		_, db, fixture := newPostgresSessionIndexRepo(t)
		if _, err := db.Exec("DROP INDEX " + taskSessionEnvironmentTaskIndex); err != nil {
			t.Fatalf("drop postgres environment index: %v", err)
		}
		if _, err := db.Exec(`CREATE INDEX idx_task_sessions_environment_task
			ON task_sessions(task_environment_id, task_id)`); err != nil {
			t.Fatalf("create operator index: %v", err)
		}
		replayed, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("replay postgres operator index: %v", err)
		}
		assertPostgresTaskSessionEnvironmentIndex(t, replayed.db)
		fixture.assertPreserved(t, db)
	})

	t.Run("pre-column database", func(t *testing.T) {
		_, db, fixture := newPostgresSessionIndexRepo(t)
		if _, err := db.Exec("DROP INDEX " + taskSessionEnvironmentTaskIndex); err != nil {
			t.Fatalf("drop postgres environment index: %v", err)
		}
		if _, err := db.Exec(`ALTER TABLE task_sessions DROP COLUMN task_environment_id`); err != nil {
			t.Fatalf("drop environment column: %v", err)
		}
		upgraded, err := NewWithDB(db, db, nil)
		if err != nil {
			t.Fatalf("initialize postgres pre-column schema: %v", err)
		}
		assertPostgresTaskSessionEnvironmentIndex(t, upgraded.db)
		var columns int
		if err := db.Get(&columns, `
			SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = 'task_sessions'
			  AND column_name = 'task_environment_id'
		`); err != nil {
			t.Fatalf("inspect restored environment column: %v", err)
		}
		if columns != 1 {
			t.Fatalf("restored environment columns = %d, want 1", columns)
		}
		fixture.assertPreserved(t, db)
	})
}

func newPostgresSessionIndexRepo(t *testing.T) (*Repository, *sqlx.DB, sessionIndexFixture) {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize postgres schema: %v", err)
	}
	startedAt := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	fixture := sessionIndexFixture{
		workspaceID: "ws-session-index-pg",
		taskID:      "task-session-index-pg",
		environment: "env-session-index-pg",
		sessionID:   "session-session-index-pg",
		snapshotID:  "snapshot-session-index-pg",
		startedAt:   startedAt,
		updatedAt:   startedAt.Add(time.Minute),
	}
	fixture.seed(t, db, true)
	return repo, db, fixture
}

func assertPostgresTaskSessionEnvironmentIndex(t *testing.T, db *sqlx.DB) {
	t.Helper()
	var count int
	if err := db.Get(&count, `
		SELECT COUNT(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind = 'i'
		  AND c.relname = $1
		  AND n.nspname = current_schema()
	`, taskSessionEnvironmentTaskIndex); err != nil {
		t.Fatalf("list postgres environment indexes: %v", err)
	}
	if count != 1 {
		t.Fatalf("postgres environment index count = %d, want 1", count)
	}
	var unique bool
	var definition string
	if err := db.QueryRow(`
		SELECT i.indisunique, pg_get_indexdef(i.indexrelid)
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indexrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = $1
		  AND n.nspname = current_schema()
	`, taskSessionEnvironmentTaskIndex).Scan(&unique, &definition); err != nil {
		t.Fatalf("read postgres environment index: %v", err)
	}
	if unique {
		t.Fatalf("postgres environment index is unique: %s", definition)
	}
	normalized := strings.Join(strings.Fields(definition), " ")
	if !strings.Contains(normalized, "(task_environment_id, task_id)") {
		t.Fatalf("postgres environment index definition = %q, want ordered keys task_environment_id, task_id", definition)
	}
}
