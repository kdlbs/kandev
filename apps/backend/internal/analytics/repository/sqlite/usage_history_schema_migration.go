package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

const usageSubjectTaskIncarnationSchemaVersion = int64(1)

// ensureUsageSubjectTaskIncarnationKey upgrades the first retained-schema
// version once. A task ID can be reused after deletion, so the uniqueness key
// must include its captured task incarnation as well as the session identity.
func (r *Repository) ensureUsageSubjectTaskIncarnationKey(ctx context.Context) error {
	var version int64
	err := r.db.GetContext(ctx, &version, r.db.Rebind(`SELECT version FROM usage_history_schema_version WHERE version_key = ?`), "subject_task_incarnation")
	if err == nil && version >= usageSubjectTaskIncarnationSchemaVersion {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read retained usage schema version: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO usage_history_schema_version (version_key, version)
		VALUES (?, 0) ON CONFLICT(version_key) DO NOTHING`), "subject_task_incarnation"); err != nil {
		return fmt.Errorf("reserve retained usage schema migration: %w", err)
	}
	var stored int64
	versionQuery := `SELECT version FROM usage_history_schema_version WHERE version_key = ?`
	if r.db.DriverName() == usageDriverPostgres {
		versionQuery += usageForUpdate
	}
	err = tx.GetContext(ctx, &stored, tx.Rebind(versionQuery), "subject_task_incarnation")
	if err == nil && stored >= usageSubjectTaskIncarnationSchemaVersion {
		return tx.Commit()
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err := upgradeUsageSubjectIncarnationTx(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO usage_history_schema_version (version_key, version)
		VALUES (?, ?) ON CONFLICT(version_key) DO UPDATE SET version = excluded.version`), "subject_task_incarnation", usageSubjectTaskIncarnationSchemaVersion); err != nil {
		return fmt.Errorf("record retained usage schema version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit retained usage subject incarnation migration: %w", err)
	}
	return nil
}

func upgradeUsageSubjectIncarnationTx(ctx context.Context, tx *sqlx.Tx) error {
	for _, statement := range []string{
		`CREATE TABLE usage_subjects_incarnation_upgrade (
			subject_id TEXT PRIMARY KEY,
			scope_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			task_incarnation_id TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			session_incarnation_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			UNIQUE(scope_id, task_id, task_incarnation_id, session_id, session_incarnation_id)
		)`,
		`INSERT INTO usage_subjects_incarnation_upgrade
			(subject_id, scope_id, task_id, task_incarnation_id, session_id, session_incarnation_id, created_at)
			SELECT subject_id, scope_id, task_id, task_incarnation_id, session_id, session_incarnation_id, created_at
			FROM usage_subjects`,
		`DROP TABLE usage_subjects`,
		`ALTER TABLE usage_subjects_incarnation_upgrade RENAME TO usage_subjects`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("upgrade retained usage subject incarnation key: %w", err)
		}
	}
	return nil
}
