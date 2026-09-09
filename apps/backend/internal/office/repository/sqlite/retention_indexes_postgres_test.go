package sqlite_test

import (
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresRetentionIndexes_CreatedFreshAndReplaySafe is the PostgreSQL
// half of TestRetentionIndexes_CreatedFreshAndReplaySafe: the two
// expression indexes the retention sweep depends on must exist there too,
// with identical CREATE INDEX IF NOT EXISTS replay safety. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresRetentionIndexes_CreatedFreshAndReplaySafe(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	conn := testutil.OpenIsolatedPostgres(t, dsn)

	// tasks is created by the task repository's schema init, mirroring
	// production boot order (see child_summaries_postgres_test.go).
	if _, err := taskrepo.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := sqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("fresh NewWithDB: %v", err)
	}
	assertPostgresIndexExists(t, conn, "idx_office_routine_runs_retention")
	assertPostgresIndexExists(t, conn, "idx_runs_retention")

	if _, err := sqlite.NewWithDB(conn, conn, nil); err != nil {
		t.Fatalf("replay NewWithDB: %v", err)
	}
	assertPostgresIndexExists(t, conn, "idx_office_routine_runs_retention")
	assertPostgresIndexExists(t, conn, "idx_runs_retention")
}

func assertPostgresIndexExists(t *testing.T, conn interface {
	Get(dest interface{}, query string, args ...interface{}) error
}, name string) {
	t.Helper()
	var count int
	if err := conn.Get(&count,
		`SELECT COUNT(*) FROM pg_indexes WHERE indexname = $1`, name,
	); err != nil {
		t.Fatalf("query pg_indexes for %s: %v", name, err)
	}
	if count != 1 {
		t.Fatalf("index %s: found %d, want 1", name, count)
	}
}
