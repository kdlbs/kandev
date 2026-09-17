package sqlite

import "github.com/kandev/kandev/internal/db/dialect"

func (r *Repository) createProfileDeletionTrigger() error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		_, err := r.db.Exec(`CREATE TRIGGER IF NOT EXISTS orchestration_profile_deleted
			AFTER UPDATE OF deleted_at ON agent_profiles WHEN NEW.deleted_at IS NOT NULL
			BEGIN DELETE FROM workspace_orchestrators WHERE agent_id=NEW.id; END;`)
		return err
	}
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{
		`CREATE OR REPLACE FUNCTION orchestration_delete_profile_registration() RETURNS trigger
			LANGUAGE plpgsql AS $$ BEGIN
			DELETE FROM workspace_orchestrators WHERE agent_id=NEW.id;
			RETURN NEW; END; $$`,
		`DROP TRIGGER IF EXISTS orchestration_profile_deleted ON agent_profiles`,
		`CREATE TRIGGER orchestration_profile_deleted AFTER UPDATE OF deleted_at ON agent_profiles
			FOR EACH ROW WHEN (NEW.deleted_at IS NOT NULL)
			EXECUTE FUNCTION orchestration_delete_profile_registration()`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}
