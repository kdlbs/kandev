package alertsource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresStoreSchemaReplay proves initSchema is idempotent under
// PostgreSQL (D14) and exercises a representative slice of the reservation
// lifecycle and DDL behaviors to prove dialect parity with the SQLite suite
// in store_test.go, rather than schema replay alone.
func TestPostgresStoreSchemaReplay(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("first alertsource store schema init: %v", err)
	}
	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("second alertsource store schema init: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO alert_sources (id, workspace_id, type, name, created_at, updated_at)
		VALUES ('src-1', 'ws-1', 'datadog', 's1', $1, $1)`, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO alert_watches (id, workspace_id, source_id, workflow_id, workflow_step_id, created_at, updated_at)
		VALUES ('watch-1', 'ws-1', 'src-1', 'wf-1', 'step-1', $1, $1)`, now); err != nil {
		t.Fatalf("insert watch: %v", err)
	}

	fp := Fingerprint(map[string]string{"seed": "pg-lifecycle"}, []string{"seed"})

	reserved, err := store.ReserveFingerprint(ctx, "watch-1", fp, []byte(`{"a":1}`))
	if err != nil || !reserved {
		t.Fatalf("reserve: reserved=%v err=%v", reserved, err)
	}
	reserved, err = store.ReserveFingerprint(ctx, "watch-1", fp, nil)
	if err != nil || reserved {
		t.Fatalf("duplicate reserve: expected reserved=false err=nil, got reserved=%v err=%v", reserved, err)
	}

	if err := store.AttachReservationTaskID(ctx, "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	if err := store.AttachReservationTaskID(ctx, "watch-1", fp, "task-1"); err != nil {
		t.Fatalf("idempotent re-attach: %v", err)
	}
	if err := store.AttachReservationTaskID(ctx, "watch-1", fp, "task-2"); !errors.Is(err, ErrReservationAttachedElsewhere) {
		t.Fatalf("expected ErrReservationAttachedElsewhere, got %v", err)
	}

	if err := store.ReleaseReservationForTask(ctx, "task-1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := store.AttachReservationTaskID(ctx, "watch-1", fp, "task-3"); !errors.Is(err, ErrReservationGone) {
		t.Fatalf("expected ErrReservationGone after release, got %v", err)
	}

	fp2 := Fingerprint(map[string]string{"seed": "pg-orphan"}, []string{"seed"})
	if _, err := store.ReserveFingerprint(ctx, "watch-1", fp2, nil); err != nil {
		t.Fatalf("reserve fp2: %v", err)
	}
	if err := store.DeleteReservation(ctx, "watch-1", fp2); err != nil {
		t.Fatalf("delete fp2: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM alert_reservations WHERE watch_id = $1 AND fingerprint = $2`,
		"watch-1", fp2); err != nil {
		t.Fatalf("count fp2: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected fp2 deleted, got %d rows", count)
	}

	fp3 := Fingerprint(map[string]string{"seed": "pg-release-orphan"}, []string{"seed"})
	if _, err := store.ReserveFingerprint(ctx, "watch-1", fp3, nil); err != nil {
		t.Fatalf("reserve fp3: %v", err)
	}
	if err := store.ReleaseOrphanedReservation(ctx, "watch-1", fp3); err != nil {
		t.Fatalf("release orphaned fp3: %v", err)
	}
	var releasedAt *time.Time
	if err := db.Get(&releasedAt, `SELECT released_at FROM alert_reservations WHERE watch_id = $1 AND fingerprint = $2`,
		"watch-1", fp3); err != nil {
		t.Fatalf("read released_at: %v", err)
	}
	if releasedAt == nil {
		t.Fatal("expected fp3 to be released")
	}
}

// TestPostgresStore_SchemaBehaviors covers the DDL-level constraints that
// are most likely to diverge silently between SQLite and PostgreSQL: FK
// actions, CHECK constraints, the UNIQUE constraint, and — per D16.3 —
// case-sensitive fingerprint collation, since Postgres's default collation
// is case-sensitive already but the explicit COLLATE "C" must still be
// present and correct for byte-order comparisons.
func TestPostgresStore_SchemaBehaviors(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := NewStore(db, db); err != nil {
		t.Fatalf("schema init: %v", err)
	}

	now := time.Now().UTC()
	if _, err := db.Exec(`
		INSERT INTO alert_sources (id, workspace_id, type, name, created_at, updated_at)
		VALUES ('src-1', 'ws-1', 'datadog', 's1', $1, $1)`, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO alert_watches (id, workspace_id, source_id, workflow_id, workflow_step_id, created_at, updated_at)
		VALUES ('watch-1', 'ws-1', 'src-1', 'wf-1', 'step-1', $1, $1)`, now); err != nil {
		t.Fatalf("insert watch: %v", err)
	}

	t.Run("RestrictBlocksSourceDeleteWithWatch", func(t *testing.T) {
		if _, err := db.Exec(`DELETE FROM alert_sources WHERE id = 'src-1'`); err == nil {
			t.Fatal("expected ON DELETE RESTRICT to reject the delete")
		}
	})

	t.Run("CascadeRemovesReservationsOnWatchDelete", func(t *testing.T) {
		fp := Fingerprint(map[string]string{"seed": "pg-cascade"}, []string{"seed"})
		if _, err := db.Exec(`
			INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
			VALUES ('res-cascade', 'watch-1', $1, $2)`, fp, now); err != nil {
			t.Fatalf("insert reservation: %v", err)
		}
		if _, err := db.Exec(`DELETE FROM alert_watches WHERE id = 'watch-1'`); err != nil {
			t.Fatalf("delete watch: %v", err)
		}
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM alert_reservations WHERE watch_id = 'watch-1'`); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Fatal("expected CASCADE to remove the watch's reservations")
		}
		// Restore the watch for subsequent subtests.
		if _, err := db.Exec(`
			INSERT INTO alert_watches (id, workspace_id, source_id, workflow_id, workflow_step_id, created_at, updated_at)
			VALUES ('watch-1', 'ws-1', 'src-1', 'wf-1', 'step-1', $1, $1)`, now); err != nil {
			t.Fatalf("reinsert watch: %v", err)
		}
	})

	t.Run("HealthStatusCheckRejectsBogusValue", func(t *testing.T) {
		if _, err := db.Exec(`
			INSERT INTO alert_sources (id, workspace_id, type, name, health_status, created_at, updated_at)
			VALUES ('src-bogus', 'ws-1', 'datadog', 's-bogus', 'bogus', $1, $1)`, now); err == nil {
			t.Fatal("expected the health_status CHECK to reject 'bogus'")
		}
	})

	t.Run("FingerprintWidthCheckRejectsShortValue", func(t *testing.T) {
		full := Fingerprint(map[string]string{"seed": "pg-width"}, []string{"seed"})
		short := full[:63]
		if _, err := db.Exec(`
			INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
			VALUES ('res-short', 'watch-1', $1, $2)`, short, now); err == nil {
			t.Fatal("expected the fingerprint width CHECK to reject a 63-character value")
		}
	})

	t.Run("UniqueWorkspaceAndName", func(t *testing.T) {
		if _, err := db.Exec(`
			INSERT INTO alert_sources (id, workspace_id, type, name, created_at, updated_at)
			VALUES ('src-dup', 'ws-1', 'datadog', 's1', $1, $1)`, now); err == nil {
			t.Fatal("expected UNIQUE(workspace_id, name) to reject a duplicate")
		}
	})

	t.Run("CaseSensitiveCollation", func(t *testing.T) {
		lower := Fingerprint(map[string]string{"seed": "pg-collation"}, []string{"seed"})
		upper := make([]byte, len(lower))
		for i := 0; i < len(lower); i++ {
			c := lower[i]
			if c >= 'a' && c <= 'f' {
				c -= 'a' - 'A'
			}
			upper[i] = c
		}
		if _, err := db.Exec(`
			INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
			VALUES ('res-lower', 'watch-1', $1, $2)`, lower, now); err != nil {
			t.Fatalf("insert lowercase: %v", err)
		}
		if _, err := db.Exec(`
			INSERT INTO alert_reservations (id, watch_id, fingerprint, created_at)
			VALUES ('res-upper', 'watch-1', $1, $2)`, string(upper), now); err != nil {
			t.Fatalf("insert uppercase: %v", err)
		}
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM alert_reservations WHERE released_at IS NULL AND watch_id = 'watch-1'`); err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 2 {
			t.Fatalf("expected 2 distinct live rows under case-sensitive collation, got %d", count)
		}
	})
}
