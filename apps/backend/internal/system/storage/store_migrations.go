package storage

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

var storageSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS storage_maintenance_runs (
		id TEXT PRIMARY KEY,
		trigger TEXT NOT NULL,
		state TEXT NOT NULL,
		settings_snapshot TEXT NOT NULL,
		result TEXT NOT NULL,
		message TEXT NOT NULL DEFAULT '',
		started_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_storage_maintenance_runs_state_started
		ON storage_maintenance_runs (state, started_at DESC)`,
	`CREATE TABLE IF NOT EXISTS storage_quarantine_entries (
		id TEXT PRIMARY KEY,
		resource_type TEXT NOT NULL,
		task_id TEXT NULL,
		workspace_id TEXT NULL,
		original_path TEXT NOT NULL,
		quarantine_path TEXT NOT NULL UNIQUE,
		size_bytes BIGINT NOT NULL,
		state TEXT NOT NULL,
		quarantined_at TIMESTAMP NOT NULL,
		delete_after TIMESTAMP NOT NULL,
		restored_at TIMESTAMP NULL,
		deleted_at TIMESTAMP NULL,
		last_error TEXT NOT NULL DEFAULT '',
		metadata TEXT NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_storage_quarantine_state_deadline
		ON storage_quarantine_entries (state, delete_after)`,
	`CREATE INDEX IF NOT EXISTS idx_storage_quarantine_task
		ON storage_quarantine_entries (task_id)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_storage_quarantine_active_original
		ON storage_quarantine_entries (original_path)
		WHERE state IN ('quarantined', 'failed')`,
}

func initStorageSchema(conn *sqlx.DB) error {
	for _, statement := range storageSchemaStatements {
		if _, err := conn.Exec(statement); err != nil {
			return fmt.Errorf("initialize storage maintenance schema: %w", err)
		}
	}
	return nil
}
