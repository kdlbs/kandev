package sqlite_test

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestMigrate_RoutineCatchUpPolicyDefaultRebuildPostgres is the Postgres
// counterpart to TestMigrate_RoutineCatchUpPolicyDefaultRebuild
// (catchup_migration_replay_test.go). The SQLite migration detects
// staleness via sqlite_master/PRAGMA table_info, neither of which exist on
// Postgres, so without a dedicated Postgres path the rebuild silently never
// runs there and an upgraded Postgres install's office_routines.catch_up_policy
// column keeps the retired 'enqueue_missed_with_cap' DEFAULT forever
// (AC-OFFICE-ROUTINE-CATCHUP-003.3: "the column's stored default shall be
// summarize_missed on an upgraded install ... Inspecting the live schema
// ... shall not show the retired value").
//
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestMigrate_RoutineCatchUpPolicyDefaultRebuildPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	// office_routines' schema init runs after the task repository's, mirroring
	// production boot order (see runs_inflight_postgres_test.go).
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}

	// Fresh init creates office_routines with the current (correct) default.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo (fresh schema): %v", err)
	}

	// Simulate an upgraded install: force the column's DEFAULT back to the
	// retired literal, as if this schema predated the rename.
	if _, err := db.Exec(
		`ALTER TABLE office_routines ALTER COLUMN catch_up_policy SET DEFAULT 'enqueue_missed_with_cap'`,
	); err != nil {
		t.Fatalf("seed legacy default: %v", err)
	}

	// Re-opening the repository must detect and correct the stale default.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init office repo (replay migrations): %v", err)
	}

	dflt := currentCatchUpPolicyDefault(t, db)
	if strings.Contains(dflt, "enqueue_missed_with_cap") {
		t.Fatalf("catch_up_policy DEFAULT still names the retired value: %q", dflt)
	}
	if !strings.Contains(dflt, "summarize_missed") {
		t.Fatalf("catch_up_policy DEFAULT = %q, want it to contain summarize_missed", dflt)
	}

	// Idempotency: a second replay against an already-correct default must
	// not error, and must leave it unchanged.
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("second init should be idempotent: %v", err)
	}
	if got := currentCatchUpPolicyDefault(t, db); !strings.Contains(got, "summarize_missed") {
		t.Fatalf("catch_up_policy DEFAULT after replay = %q, want it to contain summarize_missed", got)
	}
}

func currentCatchUpPolicyDefault(t *testing.T, db interface {
	Get(dest interface{}, query string, args ...interface{}) error
}) string {
	t.Helper()
	var dflt string
	if err := db.Get(&dflt, `
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'office_routines'
		  AND column_name = 'catch_up_policy'
	`); err != nil {
		t.Fatalf("read column_default: %v", err)
	}
	return dflt
}
