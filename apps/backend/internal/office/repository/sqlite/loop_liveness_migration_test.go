package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/persistence"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

const loopLivenessActivationKey = "telemetry.office_loop_liveness.activated_at"

// TestLoopLivenessMigration_AddsCausationIDColumns covers
// REQ-OFFICE-LOOP-LIVENESS-002's Persistence section: all three
// causation_id columns exist and default to ” (not NULL) on a
// freshly-migrated database.
func TestLoopLivenessMigration_AddsCausationIDColumns(t *testing.T) {
	_, db := newTestRepoWithDB(t)

	for _, table := range []string{"office_routine_runs", "agent_wakeup_requests", "runs"} {
		var notnull int
		var dflt sql.NullString
		rows, err := db.Queryx("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		found := false
		for rows.Next() {
			var cid, nn, pk int
			var name, ctype string
			var d sql.NullString
			if err := rows.Scan(&cid, &name, &ctype, &nn, &d, &pk); err != nil {
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			if name == "causation_id" {
				found = true
				notnull = nn
				dflt = d
			}
		}
		_ = rows.Close()
		if !found {
			t.Fatalf("%s.causation_id column missing", table)
		}
		if notnull != 1 {
			t.Fatalf("%s.causation_id NOT NULL = %d, want 1", table, notnull)
		}
		if !dflt.Valid || dflt.String != "''" {
			t.Fatalf("%s.causation_id default = %v, want ''", table, dflt)
		}
	}
}

// TestLoopLivenessActivation_WrittenOnceAfterSchemaProbe mirrors
// TestRunOutcomeActivation_WrittenOnceAfterSchemaProbe: the key is
// written only after a positive probe of all three columns, and a
// replayed boot never re-writes it.
func TestLoopLivenessActivation_WrittenOnceAfterSchemaProbe(t *testing.T) {
	_, db := newTestRepoWithDB(t)

	val, err := persistence.ReadMetaKey(db, loopLivenessActivationKey)
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected telemetry.office_loop_liveness.activated_at to be written after first boot")
	}
	if _, err := time.Parse(time.RFC3339, val); err != nil {
		t.Fatalf("activation value %q is not RFC3339: %v", val, err)
	}

	if _, err := db.Exec(db.Rebind(
		`UPDATE kandev_meta SET value = ? WHERE key = ?`,
	), "sentinel-value", loopLivenessActivationKey); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}

	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay init: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay init twice: %v", err)
	}

	got, err := persistence.ReadMetaKey(db, loopLivenessActivationKey)
	if err != nil {
		t.Fatalf("read activation key after replay: %v", err)
	}
	if got != "sentinel-value" {
		t.Fatalf("activation key = %q after replay, want unchanged sentinel-value", got)
	}
}

// TestLoopLivenessActivation_ReadHelperRoundTrips covers
// Repository.LoopLivenessActivation: published=true with the parsed
// instant after boot, and published=false when the key is absent.
func TestLoopLivenessActivation_ReadHelperRoundTrips(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)

	at, published := repo.LoopLivenessActivation()
	if !published {
		t.Fatal("expected activation to be published after boot")
	}
	if at.IsZero() {
		t.Fatal("expected a non-zero activation instant")
	}
}

// TestPostgresLoopLivenessMigration_AddsColumnsAndActivates is the
// PostgreSQL twin required by ADR 0027. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresLoopLivenessMigration_AddsColumnsAndActivates(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	if _, err := sqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("replay postgres schema: %v", err)
	}

	for _, table := range []string{"office_routine_runs", "agent_wakeup_requests", "runs"} {
		var dataType string
		err := db.QueryRow(`
			SELECT data_type FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = 'causation_id'
		`, table).Scan(&dataType)
		if err != nil {
			t.Fatalf("inspect %s.causation_id: %v", table, err)
		}
		if dataType != "text" {
			t.Fatalf("%s.causation_id data type = %q, want text", table, dataType)
		}
	}

	val, err := persistence.ReadMetaKey(db, loopLivenessActivationKey)
	if err != nil {
		t.Fatalf("read activation key: %v", err)
	}
	if val == "" {
		t.Fatal("expected activation key to be written on postgres")
	}
}
