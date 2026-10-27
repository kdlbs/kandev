package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

const agentDeliveryRetentionFormatVersion = 1

const agentDeliveryEligibleIndexName = "idx_agent_delivery_inbox_retention_eligible"
const agentDeliveryEligibleIndexQuery = `CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_retention_eligible
	ON agent_delivery_inbox(projected_at, stream_id, sequence)
	WHERE payload_pruned_at IS NULL AND projected_at IS NOT NULL`

const agentDeliveryOwnershipIndexName = "idx_agent_delivery_effects_retirement"
const agentDeliveryOwnershipIndexQuery = `CREATE INDEX IF NOT EXISTS idx_agent_delivery_effects_retirement
	ON agent_delivery_effects(session_id, incarnation_id) WHERE session_id <> ''`
const agentDeliveryOwnedOrderedIndexQuery = `CREATE INDEX IF NOT EXISTS idx_agent_delivery_effects_owned_retirement
	ON agent_delivery_effects(session_id, effect_key) WHERE session_id <> ''`

const agentDeliveryUnprojectedIndexName = "idx_agent_delivery_inbox_unprojected"
const agentDeliveryUnprojectedIndexQuery = `CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_unprojected
	ON agent_delivery_inbox(stream_id, projected_at, sequence) WHERE projected_at IS NULL`

type agentDeliveryRetentionColumn struct {
	table      string
	column     string
	definition string
}

var agentDeliveryRetentionColumns = []agentDeliveryRetentionColumn{
	{table: "agent_delivery_retention_state", column: "event_sweep_phase", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "agent_delivery_retention_state", column: "last_compaction_at", definition: "TIMESTAMP"},
	{table: "agent_delivery_retention_state", column: "last_compaction_stream_id", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "agent_delivery_retention_state", column: "last_compaction_sequence", definition: "BIGINT NOT NULL DEFAULT 0"},
	{table: "agent_delivery_inbox", column: "content_hash", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "agent_delivery_inbox", column: "hash_version", definition: "INTEGER NOT NULL DEFAULT 0"},
	{table: "agent_delivery_inbox", column: "payload_pruned_at", definition: "TIMESTAMP"},
	{table: "agent_delivery_inbox", column: "payload_bytes", definition: "BIGINT NOT NULL DEFAULT 0"},
	{table: "agent_delivery_submissions", column: "payload_pruned_at", definition: "TIMESTAMP"},
	{table: "agent_delivery_submissions", column: "payload_bytes", definition: "BIGINT NOT NULL DEFAULT 0"},
	{table: "agent_delivery_submissions", column: "fully_settled_at", definition: "TIMESTAMP"},
	{table: "agent_delivery_submissions", column: "terminal_stream_id", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "agent_delivery_submissions", column: "terminal_sequence", definition: "BIGINT NOT NULL DEFAULT 0"},
	{table: "agent_delivery_submissions", column: "terminal_turn_id", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "agent_delivery_cursors", column: "retired_sequence", definition: "BIGINT NOT NULL DEFAULT 0 CHECK (retired_sequence >= 0)"},
}

var agentDeliverySegmentationColumns = []agentDeliveryRetentionColumn{
	{table: "agent_delivery_inbox", column: "canonical_message_id", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "agent_delivery_cursors", column: "message_segment_boundary_sequence", definition: "BIGINT CHECK (message_segment_boundary_sequence IS NULL OR message_segment_boundary_sequence >= 0)"},
}

var agentDeliveryRetentionIndexes = []struct {
	name  string
	query string
}{
	{
		name:  "idx_agent_delivery_cursors_session_stream",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_cursors_session_stream ON agent_delivery_cursors(session_id, stream_id)`,
	},
	{
		name:  "idx_agent_delivery_cursors_retirement",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_cursors_retirement ON agent_delivery_cursors(session_id, incarnation_id, stream_id)`,
	},
	{
		name: "idx_agent_delivery_effects_unbound_workflow",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_effects_unbound_workflow ON agent_delivery_effects(effect_key)
            WHERE effect_type = 'workflow.on_turn_complete' AND (session_id = '' OR incarnation_id = '')`,
	},
	{
		name: "idx_agent_delivery_inbox_terminal_sequence",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_terminal_sequence
			ON agent_delivery_inbox(stream_id, sequence) WHERE terminal <> 0`,
	},
	{
		name: "idx_agent_delivery_inbox_retirement",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_retirement
			ON agent_delivery_inbox(session_id, incarnation_id, stream_id, sequence)`,
	},
	{
		name:  agentDeliveryEligibleIndexName,
		query: agentDeliveryEligibleIndexQuery,
	},
	{
		name: "idx_agent_delivery_inbox_retention_receipts",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_inbox_retention_receipts
			ON agent_delivery_inbox(stream_id, sequence) WHERE payload_pruned_at IS NOT NULL`,
	},
	{
		name: "idx_agent_delivery_submissions_retention_eligible",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_submissions_retention_eligible
			ON agent_delivery_submissions(fully_settled_at, id)
			WHERE payload_pruned_at IS NULL`,
	},
	{
		name: "idx_agent_delivery_submissions_retirement",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_submissions_retirement
			ON agent_delivery_submissions(session_id, incarnation_id, id)`,
	},
	{
		name: "idx_agent_delivery_effects_stream_state",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_effects_stream_state
			ON agent_delivery_effects(stream_id, sequence, state)`,
	},
	{
		name:  agentDeliveryOwnershipIndexName,
		query: agentDeliveryOwnershipIndexQuery,
	},
	{
		name:  "idx_agent_delivery_effects_owned_retirement",
		query: agentDeliveryOwnedOrderedIndexQuery,
	},
	{
		name: "idx_agent_delivery_terminal_lookup",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_terminal_lookup
			ON agent_delivery_effects(effect_type, submission_id, stream_id, sequence)`,
	},
	{
		name: "idx_agent_delivery_cleanup_jobs_phase",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_cleanup_jobs_phase
			ON agent_delivery_cleanup_jobs(phase, session_id, incarnation_id)`,
	},
	{
		name: "idx_agent_delivery_cleanup_jobs_fair",
		query: `CREATE INDEX IF NOT EXISTS idx_agent_delivery_cleanup_jobs_fair
			ON agent_delivery_cleanup_jobs(updated_at, session_id, incarnation_id)`,
	},
}

func (r *Repository) initAgentDeliveryRetentionSchema() error {
	if err := r.ensureAgentDeliveryRetentionFormat(); err != nil {
		return err
	}
	for _, column := range agentDeliveryRetentionColumns {
		exists, err := db.ColumnExistsContext(r.migrationContext(), r.db, column.table, column.column)
		if err != nil {
			return fmt.Errorf("check agent delivery retention column %s.%s: %w", column.table, column.column, err)
		}
		if exists {
			continue
		}
		query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", column.table, column.column, column.definition)
		if dialect.IsPostgres(r.db.DriverName()) {
			query = fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s %s", column.table, column.column, column.definition)
		}
		if err := r.migrate.Apply("agent_delivery_retention.column."+column.table+"."+column.column, query); err != nil {
			return err
		}
	}
	if err := r.migrate.Apply("agent_delivery_cleanup_jobs.table", `
		CREATE TABLE IF NOT EXISTS agent_delivery_cleanup_jobs (
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			retired_at TIMESTAMP NOT NULL,
			phase TEXT NOT NULL,
			last_stream_id TEXT NOT NULL DEFAULT '',
			last_sequence BIGINT NOT NULL DEFAULT 0,
			last_submission_id TEXT NOT NULL DEFAULT '',
			last_effect_key TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			PRIMARY KEY (session_id, incarnation_id)
		)`); err != nil {
		return err
	}

	if err := r.upgradeAgentDeliveryEligibleIndex(); err != nil {
		return err
	}
	if err := r.upgradeAgentDeliveryOwnershipIndex(); err != nil {
		return err
	}
	if err := r.upgradeAgentDeliveryUnprojectedIndex(); err != nil {
		return err
	}
	for _, index := range agentDeliveryRetentionIndexes {
		if err := r.migrate.Apply("agent_delivery_retention.index."+index.name, index.query); err != nil {
			return err
		}
	}
	return r.assertAgentDeliveryRetentionSchemaContext(r.migrationContext())
}

func (r *Repository) ensureAgentDeliveryRetentionFormat() error {
	ctx := r.migrationContext()
	exists, err := db.TableExistsContext(ctx, r.db, "agent_delivery_retention_state")
	if err != nil {
		return fmt.Errorf("check agent delivery retention state table: %w", err)
	}
	if exists {
		return r.validateAgentDeliveryRetentionFormatMarker(ctx)
	}
	if err := assertNoPartialAgentDeliveryRetentionSchema(ctx, r.db); err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin agent delivery retention format migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.failAgentDeliveryRetention("before_format_marker"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS agent_delivery_retention_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			format_version INTEGER NOT NULL,
			pruning_started_at TIMESTAMP,
			sweep_phase TEXT NOT NULL DEFAULT '',
			last_session_id TEXT NOT NULL DEFAULT '',
			last_stream_id TEXT NOT NULL DEFAULT '',
			last_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_sequence >= 0),
			last_submission_id TEXT NOT NULL DEFAULT '',
			last_effect_key TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMP NOT NULL
		)`); err != nil {
		return fmt.Errorf("create agent delivery retention state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO agent_delivery_retention_state (id, format_version, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT (id) DO NOTHING`), 1, agentDeliveryRetentionFormatVersion, r.nowUTC()); err != nil {
		return fmt.Errorf("write agent delivery retention format marker: %w", err)
	}
	if err := r.failAgentDeliveryRetention("after_format_marker"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent delivery retention format migration: %w", err)
	}
	return nil
}

func (r *Repository) validateAgentDeliveryRetentionFormatMarker(ctx context.Context) error {
	var formatVersion int
	err := r.db.QueryRowxContext(ctx, `
		SELECT format_version FROM agent_delivery_retention_state WHERE id = 1`).Scan(&formatVersion)
	if err == sql.ErrNoRows {
		return fmt.Errorf("agent delivery retention format marker is missing")
	}
	if err != nil {
		return fmt.Errorf("read agent delivery retention format marker: %w", err)
	}
	if formatVersion != agentDeliveryRetentionFormatVersion {
		return fmt.Errorf("unsupported agent delivery retention format %d (supported: %d)", formatVersion, agentDeliveryRetentionFormatVersion)
	}
	return nil
}

func assertNoPartialAgentDeliveryRetentionSchema(ctx context.Context, database *sqlx.DB) error {
	for _, column := range agentDeliveryRetentionColumns {
		hasColumn, err := db.ColumnExistsContext(ctx, database, column.table, column.column)
		if err != nil {
			return fmt.Errorf("check legacy agent delivery column %s.%s: %w", column.table, column.column, err)
		}
		if hasColumn {
			return fmt.Errorf("agent delivery retention format marker is missing from a retention schema")
		}
	}
	jobsExist, err := db.TableExistsContext(ctx, database, "agent_delivery_cleanup_jobs")
	if err != nil {
		return fmt.Errorf("check agent delivery cleanup jobs table: %w", err)
	}
	if jobsExist {
		return fmt.Errorf("agent delivery retention format marker is missing from a retention schema")
	}
	return nil
}

func (r *Repository) CheckAgentDeliveryRetentionReady(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.assertAgentDeliveryRetentionSchemaContext(ctx)
}

func (r *Repository) assertAgentDeliveryRetentionSchemaContext(ctx context.Context) error {
	columns := map[string][]string{
		"agent_delivery_inbox": {
			"content_hash", "hash_version", "payload_pruned_at", "payload_bytes",
		},
		"agent_delivery_submissions": {
			"payload_pruned_at", "payload_bytes", "fully_settled_at",
			"terminal_stream_id", "terminal_sequence", "terminal_turn_id",
		},
		"agent_delivery_cursors": {"retired_sequence"},
		"agent_delivery_retention_state": {
			"format_version", "pruning_started_at", "sweep_phase", "last_session_id",
			"last_stream_id", "last_sequence", "last_submission_id", "last_effect_key", "updated_at",
			"event_sweep_phase", "last_compaction_at", "last_compaction_stream_id", "last_compaction_sequence",
		},
		"agent_delivery_cleanup_jobs": {
			"session_id", "incarnation_id", "retired_at", "phase", "last_stream_id",
			"last_sequence", "last_submission_id", "last_effect_key", "error_code", "created_at", "updated_at",
		},
	}
	for table, required := range columns {
		for _, column := range required {
			exists, err := db.ColumnExistsContext(ctx, r.db, table, column)
			if err != nil {
				return fmt.Errorf("check agent delivery retention column %s.%s: %w", table, column, err)
			}
			if !exists {
				return fmt.Errorf("agent delivery retention schema is missing %s.%s", table, column)
			}
		}
	}

	for _, table := range []string{"agent_delivery_cleanup_jobs", "agent_delivery_retention_state"} {
		exists, err := db.TableExistsContext(ctx, r.db, table)
		if err != nil {
			return fmt.Errorf("check agent delivery retention table %s: %w", table, err)
		}
		if !exists {
			return fmt.Errorf("agent delivery retention schema is missing table %s", table)
		}
	}

	var formatVersion int
	if err := r.db.QueryRowxContext(ctx, `
		SELECT format_version FROM agent_delivery_retention_state WHERE id = 1`).Scan(&formatVersion); err != nil {
		return fmt.Errorf("read agent delivery retention format marker: %w", err)
	}
	if formatVersion != agentDeliveryRetentionFormatVersion {
		return fmt.Errorf("unsupported agent delivery retention format %d (supported: %d)", formatVersion, agentDeliveryRetentionFormatVersion)
	}

	actualIndexes, err := r.agentDeliveryRetentionIndexNames(ctx)
	if err != nil {
		return fmt.Errorf("read agent delivery retention indexes: %w", err)
	}
	for _, index := range agentDeliveryRetentionIndexes {
		if !actualIndexes[index.name] {
			return fmt.Errorf("agent delivery retention schema is missing index %s", index.name)
		}
	}
	return nil
}

func (r *Repository) agentDeliveryRetentionIndexNames(ctx context.Context) (map[string]bool, error) {
	indexes := make(map[string]bool)
	for _, index := range agentDeliveryRetentionIndexes {
		exists, err := db.IndexExistsContext(ctx, r.db, index.name)
		if err != nil {
			return nil, fmt.Errorf("check agent delivery retention index %s: %w", index.name, err)
		}
		indexes[index.name] = exists
	}
	return indexes, nil
}
