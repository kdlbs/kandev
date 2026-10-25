package sqlite

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

func (r *Repository) rebindWorkspaceAdoptionEnvironmentForeignKey(tx *sqlx.Tx) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	exists, err := r.columnExists(tx, "task_environment_checkout_adoptions", "task_environment_id")
	if err != nil {
		return fmt.Errorf("cutover: inspect workspace adoption ownership: %w", err)
	}
	if !exists {
		return nil
	}
	if _, err := tx.ExecContext(r.migrationContext(), postgresWorkspaceAdoptionShadowForeignKeyDDL); err != nil {
		return fmt.Errorf("cutover: rebind workspace adoption environment foreign key: %w", err)
	}
	return nil
}

const postgresWorkspaceAdoptionShadowForeignKeyDDL = `
DO $$
DECLARE
	old_constraint_name text;
BEGIN
	FOR old_constraint_name IN
		SELECT DISTINCT tc.constraint_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON kcu.constraint_name = tc.constraint_name
			AND kcu.constraint_schema = tc.constraint_schema
			AND kcu.table_schema = tc.table_schema
			AND kcu.table_name = tc.table_name
		WHERE tc.table_schema = current_schema()
			AND tc.table_name = 'task_environment_checkout_adoptions'
			AND tc.constraint_type = 'FOREIGN KEY'
			AND kcu.column_name = 'task_environment_id'
	LOOP
		EXECUTE format('ALTER TABLE task_environment_checkout_adoptions DROP CONSTRAINT %I', old_constraint_name);
	END LOOP;

	ALTER TABLE task_environment_checkout_adoptions
		ADD CONSTRAINT task_environment_checkout_adoptions_task_environment_id_fkey
		FOREIGN KEY (task_environment_id) REFERENCES task_environments_shadow(id) ON DELETE CASCADE;
END $$;
`
