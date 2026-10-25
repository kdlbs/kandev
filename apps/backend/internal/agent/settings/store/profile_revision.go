package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// initProfileRevisions installs database-owned change tracking so every SQL
// writer invalidates snapshots, including writers outside the settings store.
func (r *sqliteRepository) initProfileRevisions() error {
	if _, err := r.db.Exec(`CREATE TABLE IF NOT EXISTS agent_profile_revisions (
		profile_id TEXT PRIMARY KEY,
		revision BIGINT NOT NULL CHECK (revision > 0)
	)`); err != nil {
		return fmt.Errorf("create agent profile revisions: %w", err)
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if err := r.initPostgresProfileRevisionTriggers(); err != nil {
			return err
		}
	} else {
		for _, stmt := range []string{
			`CREATE TRIGGER IF NOT EXISTS agent_profile_revision_insert
			AFTER INSERT ON agent_profiles BEGIN
				INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
				ON CONFLICT(profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
			END`,
			`CREATE TRIGGER IF NOT EXISTS agent_profile_revision_update
			AFTER UPDATE ON agent_profiles BEGIN
				INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
				ON CONFLICT(profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
			END`,
		} {
			if _, err := r.db.Exec(stmt); err != nil {
				return fmt.Errorf("create agent profile revision trigger: %w", err)
			}
		}
	}
	// Backfill after trigger installation: an intervening write wins and is
	// never reset to the legacy starting revision.
	if _, err := r.db.Exec(`INSERT INTO agent_profile_revisions (profile_id, revision)
		SELECT id, 1 FROM agent_profiles WHERE 1 = 1
		ON CONFLICT(profile_id) DO NOTHING`); err != nil {
		return fmt.Errorf("backfill agent profile revisions: %w", err)
	}
	return nil
}

func (r *sqliteRepository) initPostgresProfileRevisionTriggers() error {
	if _, err := r.db.Exec(`CREATE OR REPLACE FUNCTION bump_agent_profile_revision() RETURNS trigger AS $$
	BEGIN
		INSERT INTO agent_profile_revisions (profile_id, revision) VALUES (NEW.id, 1)
		ON CONFLICT (profile_id) DO UPDATE SET revision = agent_profile_revisions.revision + 1;
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql`); err != nil {
		return fmt.Errorf("create agent profile revision function: %w", err)
	}
	for _, stmt := range []string{
		`CREATE OR REPLACE TRIGGER agent_profile_revision_insert
		AFTER INSERT ON agent_profiles FOR EACH ROW EXECUTE FUNCTION bump_agent_profile_revision()`,
		`CREATE OR REPLACE TRIGGER agent_profile_revision_update
		AFTER UPDATE ON agent_profiles FOR EACH ROW EXECUTE FUNCTION bump_agent_profile_revision()`,
	} {
		if _, err := r.db.Exec(stmt); err != nil {
			return fmt.Errorf("create agent profile revision trigger: %w", err)
		}
	}
	return nil
}

// GetAgentProfileRevision returns the persisted mutation token for a live
// profile. Missing revision state is an error; callers must fail closed.
func (r *sqliteRepository) GetAgentProfileRevision(ctx context.Context, id string) (int64, error) {
	var revision int64
	err := r.db.QueryRowContext(ctx, r.db.Rebind(`
		SELECT r.revision FROM agent_profile_revisions r
		JOIN agent_profiles p ON p.id = r.profile_id
		WHERE p.id = ? AND p.deleted_at IS NULL`), id).Scan(&revision)
	if err == sql.ErrNoRows {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("read agent profile revision: %w", err)
	}
	return revision, nil
}
