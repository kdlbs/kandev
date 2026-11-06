package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/testutil"
)

func TestAgentDeliverySegmentationColumnsAreAddedByRunMigrations(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			for _, column := range agentDeliverySegmentationColumns {
				query := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", column.table, column.column)
				if dialect.IsPostgres(repo.db.DriverName()) {
					query = fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", column.table, column.column)
				}
				if _, err := repo.db.Exec(query); err != nil {
					t.Fatalf("remove %s.%s to simulate pre-migration schema: %v", column.table, column.column, err)
				}
			}

			if err := repo.initAgentDeliverySchema(); err != nil {
				t.Fatalf("run schema initialization before migrations: %v", err)
			}
			for _, column := range agentDeliverySegmentationColumns {
				if deliveryRetentionColumnNames(t, repo.db, column.table)[column.column] {
					t.Fatalf("schema initialization added %s.%s before runMigrations", column.table, column.column)
				}
			}

			if err := repo.runMigrations(context.Background()); err != nil {
				t.Fatalf("run additive delivery segmentation migrations: %v", err)
			}
			for _, column := range agentDeliverySegmentationColumns {
				if !deliveryRetentionColumnNames(t, repo.db, column.table)[column.column] {
					t.Errorf("runMigrations did not add %s.%s", column.table, column.column)
				}
			}
		})
	}
}

func TestDeliveryRetentionSchemaUpgradePreservesPreRetentionEvidence(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			db := openDeliveryRetentionTestDB(t, engine)
			seedPreRetentionDeliverySchema(t, db)

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("upgrade pre-retention schema: %v", err)
			}
			assertPreRetentionDeliveryEvidence(t, db)
			assertDeliveryRetentionSchema(t, repo)
			var canonicalMessageID string
			if err := db.Get(&canonicalMessageID, db.Rebind(`
				SELECT canonical_message_id FROM agent_delivery_inbox
				WHERE stream_id = ? AND sequence = 1`), "stream-settling"); err != nil {
				t.Fatalf("read additive canonical message ID: %v", err)
			}
			if canonicalMessageID != "" {
				t.Fatalf("pre-migration receipt ID = %q, want empty default", canonicalMessageID)
			}
			if err := assertNullSegmentBoundary(db, "stream-settling"); err != nil {
				t.Fatalf("pre-boundary cursor must remain uninitialized for lazy projection migration: %v", err)
			}

			if _, err := NewWithDB(db, db, nil); err != nil {
				t.Fatalf("reopen upgraded schema: %v", err)
			}
			assertPreRetentionDeliveryEvidence(t, db)
			if err := assertNullSegmentBoundary(db, "stream-settling"); err != nil {
				t.Fatalf("reopen must not reset or prematurely initialize the boundary: %v", err)
			}
		})
	}
}

func TestDeliveryRetentionInterruptedMigrationResumes(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			db := openDeliveryRetentionTestDB(t, engine)
			seedPreRetentionDeliverySchema(t, db)
			partial := &Repository{db: db, ro: db}
			ensureRetentionStateTableForTest(t, partial)
			if _, err := db.Exec(`ALTER TABLE agent_delivery_inbox ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`); err != nil {
				t.Fatalf("simulate committed partial column migration: %v", err)
			}

			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("resume interrupted schema migration: %v", err)
			}
			assertDeliveryRetentionSchema(t, repo)
			assertPreRetentionDeliveryEvidence(t, db)
			if _, err := NewWithDB(db, db, nil); err != nil {
				t.Fatalf("reopen after resumed schema migration: %v", err)
			}
		})
	}
}

func openDeliveryRetentionTestDB(t *testing.T, engine string) *sqlx.DB {
	t.Helper()
	if engine == "Postgres" {
		return testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	}
	raw, err := internaldb.OpenSQLite(filepath.Join(t.TempDir(), "delivery-retention.db"))
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedPreRetentionDeliverySchema(t *testing.T, db *sqlx.DB) {
	t.Helper()
	blob := dialect.BlobType(db.DriverName())
	for _, statement := range []string{
		fmt.Sprintf(`CREATE TABLE agent_delivery_submissions (
			id TEXT PRIMARY KEY, session_id TEXT NOT NULL, incarnation_id TEXT NOT NULL,
			harness_generation BIGINT NOT NULL, owner_generation BIGINT NOT NULL,
			dispatch_attempt_id TEXT NOT NULL DEFAULT '', payload_hash TEXT NOT NULL,
			payload %s NOT NULL, state TEXT NOT NULL, outcome TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL)`, blob),
		fmt.Sprintf(`CREATE TABLE agent_delivery_inbox (
			stream_id TEXT NOT NULL, sequence BIGINT NOT NULL, session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL, harness_generation BIGINT NOT NULL,
			submission_id TEXT NOT NULL DEFAULT '', event_type TEXT NOT NULL,
			payload %s NOT NULL, terminal INTEGER NOT NULL DEFAULT 0,
			received_at TIMESTAMP NOT NULL, projected_at TIMESTAMP,
			PRIMARY KEY (stream_id, sequence))`, blob),
		`CREATE TABLE agent_delivery_cursors (
			stream_id TEXT PRIMARY KEY, session_id TEXT NOT NULL, incarnation_id TEXT NOT NULL,
			harness_generation BIGINT NOT NULL, received_sequence BIGINT NOT NULL DEFAULT 0,
			projected_sequence BIGINT NOT NULL DEFAULT 0, remote_high_water BIGINT NOT NULL DEFAULT 0,
			updated_at TIMESTAMP NOT NULL)`,
		`CREATE TABLE agent_delivery_effects (
			effect_key TEXT PRIMARY KEY, stream_id TEXT NOT NULL, sequence BIGINT NOT NULL,
			effect_type TEXT NOT NULL, state TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
			completed_at TIMESTAMP, session_id TEXT NOT NULL DEFAULT '',
			incarnation_id TEXT NOT NULL DEFAULT '', harness_generation BIGINT NOT NULL DEFAULT 0,
			submission_id TEXT NOT NULL DEFAULT '', turn_id TEXT NOT NULL DEFAULT '',
			outcome TEXT NOT NULL DEFAULT '')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("create pre-retention delivery schema: %v", err)
		}
	}

	base := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)
	submissions := []struct {
		id, session, incarnation, state, outcome, payload string
	}{
		{"submission-active", "session-active", "inc-active", "dispatching", "", "active prompt bytes"},
		{"submission-uncertain", "session-uncertain", "inc-uncertain", "interrupted_unknown", "uncertain", "uncertain prompt bytes"},
		{"submission-settling", "session-settling", "inc-settling", "completed", "completed", "settling prompt bytes"},
	}
	for i, row := range submissions {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO agent_delivery_submissions
			(id, session_id, incarnation_id, harness_generation, owner_generation,
			 dispatch_attempt_id, payload_hash, payload, state, outcome, created_at, updated_at)
			VALUES (?, ?, ?, 4, 7, ?, ?, ?, ?, ?, ?, ?)`),
			row.id, row.session, row.incarnation, "attempt-"+row.id, "hash-"+row.id,
			[]byte(row.payload), row.state, row.outcome, base.Add(time.Duration(i)*time.Second), base.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatalf("seed %s submission: %v", row.state, err)
		}
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO agent_delivery_inbox
		(stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id,
		 event_type, payload, terminal, received_at, projected_at)
		VALUES (?, 1, 'session-settling', 'inc-settling', 4, 'submission-settling',
		 'complete', ?, 1, ?, ?)`), "stream-settling", []byte(`{"type":"complete"}`), base, base); err != nil {
		t.Fatalf("seed partially settled terminal event: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, received_sequence,
		 projected_sequence, remote_high_water, updated_at)
		VALUES ('stream-settling', 'session-settling', 'inc-settling', 4, 1, 1, 1, ?)`), base); err != nil {
		t.Fatalf("seed partially settled cursor: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO agent_delivery_effects
		(effect_key, stream_id, sequence, effect_type, state, created_at, completed_at,
		 session_id, incarnation_id, harness_generation, submission_id, turn_id, outcome)
		VALUES ('effect-settling', 'stream-settling', 1, 'agent_delivery.terminal_settlement',
		 'pending', ?, NULL, 'session-settling', 'inc-settling', 4, 'submission-settling',
		 'turn-settling', 'completed')`), base); err != nil {
		t.Fatalf("seed pending terminal effect: %v", err)
	}
}

func assertPreRetentionDeliveryEvidence(t *testing.T, db *sqlx.DB) {
	t.Helper()
	expected := map[string]struct {
		state, outcome, payload string
	}{
		"submission-active":    {"dispatching", "", "active prompt bytes"},
		"submission-uncertain": {"interrupted_unknown", "uncertain", "uncertain prompt bytes"},
		"submission-settling":  {"completed", "completed", "settling prompt bytes"},
	}
	for id, want := range expected {
		var state, outcome string
		var payload []byte
		var payloadBytes int64
		var fullySettledAt sql.NullTime
		if err := db.QueryRowx(db.Rebind(`
			SELECT state, outcome, payload, payload_bytes, fully_settled_at
			FROM agent_delivery_submissions WHERE id = ?`), id).Scan(
			&state, &outcome, &payload, &payloadBytes, &fullySettledAt); err != nil {
			t.Fatalf("read migrated submission %s: %v", id, err)
		}
		if state != want.state || outcome != want.outcome || string(payload) != want.payload {
			t.Errorf("migrated submission %s = (%q, %q, %q), want (%q, %q, %q)",
				id, state, outcome, payload, want.state, want.outcome, want.payload)
		}
		if payloadBytes != 0 || fullySettledAt.Valid {
			t.Errorf("migration inferred retention metadata for %s: bytes=%d settled=%v", id, payloadBytes, fullySettledAt.Valid)
		}
	}

	var eventPayload []byte
	var contentHash string
	var hashVersion int
	var payloadBytes int64
	if err := db.QueryRowx(`
		SELECT payload, content_hash, hash_version, payload_bytes
		FROM agent_delivery_inbox WHERE stream_id = 'stream-settling' AND sequence = 1`).Scan(
		&eventPayload, &contentHash, &hashVersion, &payloadBytes); err != nil {
		t.Fatalf("read migrated terminal event: %v", err)
	}
	if string(eventPayload) != `{"type":"complete"}` || contentHash != "" || hashVersion != 0 || payloadBytes != 0 {
		t.Errorf("migrated event payload/hash metadata = (%q, %q, %d, %d)", eventPayload, contentHash, hashVersion, payloadBytes)
	}
	var retired int64
	if err := db.QueryRowx(`SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = 'stream-settling'`).Scan(&retired); err != nil {
		t.Fatalf("read migrated cursor: %v", err)
	}
	if retired != 0 {
		t.Errorf("migration advanced retired sequence to %d, want 0", retired)
	}
	var effectState, effectOutcome string
	if err := db.QueryRowx(`SELECT state, outcome FROM agent_delivery_effects WHERE effect_key = 'effect-settling'`).Scan(&effectState, &effectOutcome); err != nil {
		t.Fatalf("read migrated terminal effect: %v", err)
	}
	if effectState != "pending" || effectOutcome != "completed" {
		t.Errorf("migration changed unsettled terminal effect to (%q, %q)", effectState, effectOutcome)
	}
}
