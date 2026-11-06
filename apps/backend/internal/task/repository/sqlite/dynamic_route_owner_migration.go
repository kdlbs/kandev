package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// Native routes keep their actual owner key without occupying a task-session FK.
// The original task-session constraint and both immutable journals remain intact.
func (r *Repository) ensureNativeRouteOwners(ctx context.Context) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('dynamic-route-owner-migration', 0))`); err != nil {
			return err
		}
	}
	exists, err := db.ColumnExistsContext(ctx, tx, "dynamic_route_states", "route_id")
	if err != nil {
		return err
	}
	if !exists {
		if dialect.IsPostgres(r.db.DriverName()) {
			err = r.migrateNativeRouteOwnersPostgres(ctx, tx)
		} else {
			err = r.migrateNativeRouteOwnersSQLite(ctx, tx)
		}
		if err != nil {
			return fmt.Errorf("migrate native route owners: %w", err)
		}
	}
	if err := ensureNativeRouteOwnerIndexes(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) migrateNativeRouteOwnersPostgres(ctx context.Context, tx *sqlx.Tx) error {
	_, err := tx.ExecContext(ctx, `
 DO $$ DECLARE key_name TEXT; BEGIN
 SELECT conname INTO key_name FROM pg_constraint
 WHERE conrelid = 'dynamic_route_states'::regclass AND contype = 'p';
 IF key_name IS NOT NULL THEN EXECUTE format('ALTER TABLE dynamic_route_states DROP CONSTRAINT %I', key_name); END IF;
 END $$;
 ALTER TABLE dynamic_route_states ALTER COLUMN session_id DROP NOT NULL;
 ALTER TABLE dynamic_route_states ADD COLUMN route_id TEXT;
 ALTER TABLE dynamic_route_states ADD CONSTRAINT dynamic_route_states_owner CHECK
 ((session_id IS NOT NULL AND route_id IS NULL) OR (session_id IS NULL AND route_id IS NOT NULL AND route_id <> ''));
 ALTER TABLE dynamic_route_attempts ALTER COLUMN session_id DROP NOT NULL;
 ALTER TABLE dynamic_route_attempts ADD COLUMN route_id TEXT;
 ALTER TABLE dynamic_route_attempts ADD CONSTRAINT dynamic_route_attempts_owner CHECK
 ((session_id IS NOT NULL AND route_id IS NULL) OR (session_id IS NULL AND route_id IS NOT NULL AND route_id <> ''));
 `)
	return err
}

func (r *Repository) migrateNativeRouteOwnersSQLite(ctx context.Context, tx *sqlx.Tx) error {
	for _, query := range []string{
		`CREATE TABLE dynamic_route_states_native_shadow (
   session_id TEXT UNIQUE REFERENCES task_sessions(id) ON DELETE CASCADE,
   route_id TEXT,
   logical_profile_id TEXT NOT NULL, execution_profile_id TEXT NOT NULL DEFAULT '',
   route_generation BIGINT NOT NULL DEFAULT 0, profile_version BIGINT NOT NULL DEFAULT 0,
   state TEXT NOT NULL DEFAULT 'selecting', continuation_json TEXT NOT NULL DEFAULT '',
   policy_state_json TEXT NOT NULL DEFAULT '', legacy_active_backfill_applied INTEGER NOT NULL DEFAULT 1,
   updated_at TIMESTAMP NOT NULL,
   CHECK ((session_id IS NOT NULL AND route_id IS NULL) OR (session_id IS NULL AND route_id IS NOT NULL AND route_id <> ''))
  )`,
		`INSERT INTO dynamic_route_states_native_shadow
   (session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, state, continuation_json, policy_state_json, legacy_active_backfill_applied, updated_at)
   SELECT session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, state, continuation_json, policy_state_json, legacy_active_backfill_applied, updated_at FROM dynamic_route_states`,
		`CREATE TABLE dynamic_route_attempts_native_shadow (
   id TEXT PRIMARY KEY, session_id TEXT REFERENCES task_sessions(id) ON DELETE CASCADE, route_id TEXT,
   logical_profile_id TEXT NOT NULL, execution_profile_id TEXT NOT NULL DEFAULT '',
   route_generation BIGINT NOT NULL, profile_version BIGINT NOT NULL, reason TEXT NOT NULL DEFAULT '',
   created_at TIMESTAMP NOT NULL, selection_ordinal BIGINT, UNIQUE (session_id, route_generation),
   CHECK ((session_id IS NOT NULL AND route_id IS NULL) OR (session_id IS NULL AND route_id IS NOT NULL AND route_id <> ''))
  )`,
		`INSERT INTO dynamic_route_attempts_native_shadow
   (id, session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, reason, created_at, selection_ordinal)
   SELECT id, session_id, logical_profile_id, execution_profile_id, route_generation, profile_version, reason, created_at, selection_ordinal FROM dynamic_route_attempts`,
		`DROP TABLE dynamic_route_attempts`,
		`DROP TABLE dynamic_route_states`,
		`ALTER TABLE dynamic_route_states_native_shadow RENAME TO dynamic_route_states`,
		`ALTER TABLE dynamic_route_attempts_native_shadow RENAME TO dynamic_route_attempts`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func ensureNativeRouteOwnerIndexes(ctx context.Context, tx *sqlx.Tx) error {
	for _, query := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_dynamic_route_states_owner ON dynamic_route_states ((COALESCE(route_id, session_id)))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_dynamic_route_attempts_owner_generation ON dynamic_route_attempts ((COALESCE(route_id, session_id)), route_generation)`,
		`CREATE INDEX IF NOT EXISTS idx_dynamic_route_attempts_session ON dynamic_route_attempts(session_id, route_generation)`,
		`CREATE INDEX IF NOT EXISTS idx_dynamic_route_attempts_profile_ordinal ON dynamic_route_attempts(logical_profile_id, selection_ordinal)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}
