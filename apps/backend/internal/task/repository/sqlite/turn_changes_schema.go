package sqlite

import (
	"fmt"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) initTurnChangesSchema() error {
	payloadType := "BLOB"
	if dialect.IsPostgres(r.db.DriverName()) {
		payloadType = "BYTEA"
	}
	for _, statement := range turnChangeSchemaStatements(payloadType) {
		if err := r.migrate.Apply(statement.name, statement.sql); err != nil {
			return fmt.Errorf("initialize turn changes schema: %w", err)
		}
	}
	for _, column := range []struct {
		name string
		sql  string
	}{
		{"terminal_capture_started_at", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_started_at TIMESTAMP`},
		{"terminal_capture_execution_id", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_execution_id TEXT NOT NULL DEFAULT ''`},
		{"terminal_capture_startup_attempt_id", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_startup_attempt_id TEXT NOT NULL DEFAULT ''`},
		{"terminal_capture_prompt_generation", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_prompt_generation BIGINT NOT NULL DEFAULT 0`},
		{"terminal_capture_environment_id", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_environment_id TEXT NOT NULL DEFAULT ''`},
		{"terminal_capture_outcome", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_outcome TEXT NOT NULL DEFAULT ''`},
		{"terminal_capture_final_message_id", `ALTER TABLE turn_change_sets ADD COLUMN terminal_capture_final_message_id TEXT NOT NULL DEFAULT ''`},
	} {
		exists, err := db.ColumnExistsContext(r.migrationContext(), r.db, "turn_change_sets", column.name)
		if err != nil {
			return fmt.Errorf("inspect turn change column %s: %w", column.name, err)
		}
		if !exists {
			if err := r.migrate.Apply("turn_change_sets."+column.name, column.sql); err != nil {
				return fmt.Errorf("add turn change column %s: %w", column.name, err)
			}
		}
	}
	repositoryCleanupColumn, err := db.ColumnExistsContext(r.migrationContext(), r.db, "turn_repository_changes", "cleanup_pending")
	if err != nil {
		return fmt.Errorf("inspect turn repository cleanup receipt: %w", err)
	}
	if !repositoryCleanupColumn {
		if err := r.migrate.Apply("turn_repository_changes.cleanup_pending", `ALTER TABLE turn_repository_changes ADD COLUMN cleanup_pending BOOLEAN NOT NULL DEFAULT FALSE`); err != nil {
			return fmt.Errorf("add turn repository cleanup receipt: %w", err)
		}
	}
	for _, column := range []struct {
		name string
		sql  string
	}{
		{"content_source", `ALTER TABLE turn_repository_changes ADD COLUMN content_source TEXT NOT NULL DEFAULT ''`},
		{"source_git_dir", `ALTER TABLE turn_repository_changes ADD COLUMN source_git_dir TEXT NOT NULL DEFAULT ''`},
		{"preservation_state", `ALTER TABLE turn_repository_changes ADD COLUMN preservation_state TEXT NOT NULL DEFAULT ''`},
		{"preservation_reason", `ALTER TABLE turn_repository_changes ADD COLUMN preservation_reason TEXT NOT NULL DEFAULT ''`},
		{"preservation_attempts", `ALTER TABLE turn_repository_changes ADD COLUMN preservation_attempts BIGINT NOT NULL DEFAULT 0`},
		{"preservation_bytes", `ALTER TABLE turn_repository_changes ADD COLUMN preservation_bytes BIGINT NOT NULL DEFAULT 0`},
		{"preservation_receipt", `ALTER TABLE turn_repository_changes ADD COLUMN preservation_receipt TEXT NOT NULL DEFAULT ''`},
		{"preservation_deadline", `ALTER TABLE turn_repository_changes ADD COLUMN preservation_deadline TIMESTAMP`},
	} {
		exists, columnErr := db.ColumnExistsContext(r.migrationContext(), r.db, "turn_repository_changes", column.name)
		if columnErr != nil {
			return fmt.Errorf("inspect turn repository column %s: %w", column.name, columnErr)
		}
		if !exists {
			if err := r.migrate.Apply("turn_repository_changes."+column.name, column.sql); err != nil {
				return fmt.Errorf("add turn repository column %s: %w", column.name, err)
			}
		}
	}
	if err := r.migrate.Apply("turn_history_cleanup_intents.table", `
		CREATE TABLE IF NOT EXISTS turn_history_cleanup_intents (
			id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL DEFAULT '',
			start_ref TEXT NOT NULL DEFAULT '',
			end_ref TEXT NOT NULL DEFAULT '',
			start_commit_oid TEXT NOT NULL DEFAULT '',
			end_commit_oid TEXT NOT NULL DEFAULT '',
			change_set_id TEXT NOT NULL DEFAULT '',
			checkout_id TEXT NOT NULL DEFAULT '',
			repository_id TEXT NOT NULL DEFAULT '',
			source_git_dir TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("initialize turn history cleanup intents: %w", err)
	}
	if err := r.migrate.Err(); err != nil {
		return fmt.Errorf("initialize turn changes schema: %w", err)
	}
	return nil
}

// migrateTurnChangeHistorySchema upgrades existing tables with the history
// provenance and export progress columns. It lives in runMigrations because
// the dependent index and backfill must follow the ADD COLUMN steps on an
// existing database.
func (r *Repository) migrateTurnChangeHistorySchema() error {
	if err := r.migrateTurnRepositoryHistoryColumns(); err != nil {
		return err
	}
	if err := r.migrate.Apply("turn_repository_changes.export_index",
		`CREATE INDEX IF NOT EXISTS idx_turn_repository_changes_export ON turn_repository_changes(export_state, export_next_attempt_at)`); err != nil {
		return fmt.Errorf("record turn repository export index: %w", err)
	}
	if err := r.backfillLegacyTurnRepositoryComparisons(); err != nil {
		return err
	}
	return nil
}
func (r *Repository) migrateTurnRepositoryHistoryColumns() error {
	for _, column := range []struct {
		name string
		sql  string
	}{
		{"start_provenance_json", `ALTER TABLE turn_repository_changes ADD COLUMN start_provenance_json TEXT NOT NULL DEFAULT ''`},
		{"end_provenance_json", `ALTER TABLE turn_repository_changes ADD COLUMN end_provenance_json TEXT NOT NULL DEFAULT ''`},
		{"comparison_version", `ALTER TABLE turn_repository_changes ADD COLUMN comparison_version BIGINT NOT NULL DEFAULT 0`},
		{"comparison_basis", `ALTER TABLE turn_repository_changes ADD COLUMN comparison_basis TEXT NOT NULL DEFAULT ''`},
		{"comparison_reason", `ALTER TABLE turn_repository_changes ADD COLUMN comparison_reason TEXT NOT NULL DEFAULT ''`},
		{"effective_start_commit_oid", `ALTER TABLE turn_repository_changes ADD COLUMN effective_start_commit_oid TEXT NOT NULL DEFAULT ''`},
		{"effective_start_tree_oid", `ALTER TABLE turn_repository_changes ADD COLUMN effective_start_tree_oid TEXT NOT NULL DEFAULT ''`},
		{"effective_end_commit_oid", `ALTER TABLE turn_repository_changes ADD COLUMN effective_end_commit_oid TEXT NOT NULL DEFAULT ''`},
		{"effective_end_tree_oid", `ALTER TABLE turn_repository_changes ADD COLUMN effective_end_tree_oid TEXT NOT NULL DEFAULT ''`},
		{"comparison_ref", `ALTER TABLE turn_repository_changes ADD COLUMN comparison_ref TEXT NOT NULL DEFAULT ''`},
		{"export_state", `ALTER TABLE turn_repository_changes ADD COLUMN export_state TEXT NOT NULL DEFAULT ''`},
		{"export_cursor", `ALTER TABLE turn_repository_changes ADD COLUMN export_cursor BIGINT NOT NULL DEFAULT 0`},
		{"export_attempts", `ALTER TABLE turn_repository_changes ADD COLUMN export_attempts BIGINT NOT NULL DEFAULT 0`},
		{"export_next_attempt_at", `ALTER TABLE turn_repository_changes ADD COLUMN export_next_attempt_at TIMESTAMP`},
		{"export_bytes", `ALTER TABLE turn_repository_changes ADD COLUMN export_bytes BIGINT NOT NULL DEFAULT 0`},
		{"export_reason", `ALTER TABLE turn_repository_changes ADD COLUMN export_reason TEXT NOT NULL DEFAULT ''`},
		{"export_claim_owner", `ALTER TABLE turn_repository_changes ADD COLUMN export_claim_owner TEXT NOT NULL DEFAULT ''`},
		{"export_claim_token", `ALTER TABLE turn_repository_changes ADD COLUMN export_claim_token BIGINT NOT NULL DEFAULT 0`},
		{"export_claim_expires_at", `ALTER TABLE turn_repository_changes ADD COLUMN export_claim_expires_at TIMESTAMP`},
		{"export_fingerprint", `ALTER TABLE turn_repository_changes ADD COLUMN export_fingerprint TEXT NOT NULL DEFAULT ''`},
	} {
		exists, err := db.ColumnExistsContext(r.migrationContext(), r.db, "turn_repository_changes", column.name)
		if err != nil {
			return fmt.Errorf("inspect turn repository history column %s: %w", column.name, err)
		}
		if exists {
			continue
		}
		if err := r.migrate.Apply("turn_repository_changes."+column.name, column.sql); err != nil {
			return fmt.Errorf("add turn repository history column %s: %w", column.name, err)
		}
	}
	return nil
}

// backfillLegacyTurnRepositoryComparisons labels only terminal captures that
// never received an accepted comparison. Pending comparisons stay unlabelled so
// a retry can still accept them exactly once.
func (r *Repository) backfillLegacyTurnRepositoryComparisons() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
		UPDATE turn_repository_changes
		   SET comparison_basis = 'legacy_raw',
		       effective_start_commit_oid = start_commit_oid,
		       effective_start_tree_oid = start_tree_oid,
		       effective_end_commit_oid = end_commit_oid,
		       effective_end_tree_oid = end_tree_oid
		 WHERE comparison_basis = ''
		   AND start_commit_oid <> '' AND end_commit_oid <> ''
		   AND change_set_id IN (SELECT id FROM turn_change_sets WHERE terminal_at IS NOT NULL)
	`)
	if err != nil {
		return fmt.Errorf("backfill legacy turn repository comparisons: %w", err)
	}
	return nil
}
