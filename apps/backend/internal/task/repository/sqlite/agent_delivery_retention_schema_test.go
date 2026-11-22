package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/testutil"
)

func TestDeliveryRetentionSchema(t *testing.T) {
	t.Run("SQLite", func(t *testing.T) {
		repo := newRepoForSessionTests(t)
		assertDeliveryRetentionSchema(t, repo)
	})
	t.Run("Postgres", func(t *testing.T) {
		assertDeliveryRetentionSchema(t, newPostgresRetentionTestRepo(t))
	})
}

func TestDeliveryRetentionFormat(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo := newRetentionTestRepo(t, engine)
			ensureRetentionStateTableForTest(t, repo)
			if _, err := repo.db.Exec(`UPDATE agent_delivery_retention_state SET format_version = 99 WHERE id = 1`); err != nil {
				t.Fatalf("set unsupported retention format: %v", err)
			}

			if err := repo.initAgentDeliverySchema(); err == nil {
				t.Fatal("schema initialization accepted unknown retention format 99")
			}
		})
	}
}

func TestDeliveryRetentionFormatRejectsMissingMarker(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ensureRetentionStateTableForTest(t, repo)
	if _, err := repo.db.Exec(`DELETE FROM agent_delivery_retention_state`); err != nil {
		t.Fatalf("delete retention format marker: %v", err)
	}
	if err := repo.initAgentDeliverySchema(); err == nil {
		t.Fatal("schema initialization recreated a missing retention format marker")
	}
}

func assertDeliveryRetentionSchema(t *testing.T, repo *Repository) {
	t.Helper()
	columns := map[string][]string{
		"agent_delivery_inbox": {
			"content_hash", "hash_version", "payload_pruned_at", "payload_bytes", "canonical_message_id",
		},
		"agent_delivery_submissions": {
			"payload_pruned_at", "payload_bytes", "fully_settled_at",
			"terminal_stream_id", "terminal_sequence", "terminal_turn_id",
		},
		"agent_delivery_cursors": {"retired_sequence"},
		"agent_delivery_retention_state": {
			"format_version", "pruning_started_at", "sweep_phase", "last_session_id",
			"last_stream_id", "last_sequence", "last_submission_id", "last_effect_key", "updated_at",
		},
		"agent_delivery_cleanup_jobs": {
			"session_id", "incarnation_id", "retired_at", "phase", "last_stream_id",
			"last_sequence", "last_submission_id", "last_effect_key", "error_code", "created_at", "updated_at",
		},
	}
	for table, expected := range columns {
		got := deliveryRetentionColumnNames(t, repo.db, table)
		for _, column := range expected {
			if !got[column] {
				t.Errorf("%s.%s is missing", table, column)
			}
		}
	}

	for _, table := range []string{"agent_delivery_cleanup_jobs", "agent_delivery_retention_state"} {
		var exists bool
		var err error
		if dialect.IsPostgres(repo.db.DriverName()) {
			err = repo.db.Get(&exists, `
				SELECT EXISTS (
					SELECT 1 FROM information_schema.tables
					WHERE table_schema = current_schema() AND table_name = $1
				)`, table)
			if err != nil {
				t.Fatalf("check table %s: %v", table, err)
			}
		} else {
			err = repo.db.Get(&exists, `
				SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, table)
			if err != nil {
				t.Fatalf("check table %s: %v", table, err)
			}
		}
		if !exists {
			t.Errorf("required table %s is missing", table)
		}
	}

	var formatVersion int
	var pruningStartedAt *string
	if err := repo.db.QueryRowxContext(context.Background(), `
		SELECT format_version, pruning_started_at
		FROM agent_delivery_retention_state WHERE id = 1`).Scan(&formatVersion, &pruningStartedAt); err != nil {
		t.Fatalf("read retention format marker: %v", err)
	}
	if formatVersion != 1 {
		t.Errorf("retention format = %d, want 1", formatVersion)
	}
	if pruningStartedAt != nil {
		t.Errorf("fresh schema pruning marker = %q, want NULL", *pruningStartedAt)
	}

	indexes, err := repo.agentDeliveryRetentionIndexNames(context.Background())
	if err != nil {
		t.Fatalf("read retention indexes: %v", err)
	}
	for _, index := range agentDeliveryRetentionIndexes {
		if !indexes[index.name] {
			t.Errorf("required retention index %s is missing", index.name)
		}
	}

	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, received_sequence,
		 projected_sequence, remote_high_water, updated_at)
		VALUES (?, ?, ?, ?, 0, 0, 0, ?)`), "retention-default", "retention-session", "retention-incarnation", 1, now); err != nil {
		t.Fatalf("insert cursor with default retirement checkpoint: %v", err)
	}
	var retiredSequence int64
	if err := repo.db.QueryRowx(`SELECT retired_sequence FROM agent_delivery_cursors WHERE stream_id = 'retention-default'`).Scan(&retiredSequence); err != nil {
		t.Fatalf("read default retirement checkpoint: %v", err)
	}
	if retiredSequence != 0 {
		t.Errorf("new cursor retired_sequence = %d, want 0", retiredSequence)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO agent_delivery_cursors
		(stream_id, session_id, incarnation_id, harness_generation, received_sequence,
		 projected_sequence, remote_high_water, retired_sequence, updated_at)
		VALUES (?, ?, ?, ?, 0, 0, 0, -1, ?)`), "retention-negative", "retention-session", "retention-incarnation", 1, now); err == nil {
		t.Fatal("cursor accepted a negative retired_sequence")
	}
	if err := repo.initAgentDeliverySchema(); err != nil {
		t.Fatalf("replay retention schema initialization: %v", err)
	}
}

func deliveryRetentionColumnNames(t *testing.T, db *sqlx.DB, table string) map[string]bool {
	t.Helper()
	query := `SELECT name FROM pragma_table_info(?)`
	args := []any{table}
	if dialect.IsPostgres(db.DriverName()) {
		query = `SELECT column_name FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1`
	}
	rows, err := db.Queryx(db.Rebind(query), args...)
	if err != nil {
		t.Fatalf("read columns for %s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	columns := make(map[string]bool)
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan column for %s: %v", table, err)
		}
		columns[column] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate columns for %s: %v", table, err)
	}
	return columns
}

func newRetentionTestRepo(t *testing.T, engine string) *Repository {
	t.Helper()
	if engine == "Postgres" {
		return newPostgresRetentionTestRepo(t)
	}
	return newRepoForSessionTests(t)
}

func newPostgresRetentionTestRepo(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize task schema: %v", err)
	}
	return repo
}

func ensureRetentionStateTableForTest(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_delivery_retention_state (
			id INTEGER PRIMARY KEY,
			format_version INTEGER NOT NULL,
			pruning_started_at TIMESTAMP,
			sweep_phase TEXT NOT NULL DEFAULT '',
			last_session_id TEXT NOT NULL DEFAULT '',
			last_stream_id TEXT NOT NULL DEFAULT '',
			last_sequence BIGINT NOT NULL DEFAULT 0,
			last_submission_id TEXT NOT NULL DEFAULT '',
			last_effect_key TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL
		)`); err != nil {
		t.Fatalf("create retention state fixture: %v", err)
	}
	if _, err := repo.db.Exec(`
		INSERT INTO agent_delivery_retention_state (id, format_version, updated_at)
		VALUES (1, 1, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO NOTHING`); err != nil {
		t.Fatalf("seed retention state fixture: %v", err)
	}
}
