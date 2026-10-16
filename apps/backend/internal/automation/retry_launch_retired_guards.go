package automation

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	retrySessionCreationKind = "session"
	retryTurnCreationKind    = "turn"
)

const retryRetainedAdmissionPredicate = `
 EXISTS (SELECT 1 FROM automation_retry_launch_protocols p
  WHERE p.task_id = NEW.task_id OR p.run_id = __TASK_RUN_OWNER__
   OR EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id=p.run_id AND l.task_id=NEW.task_id))
 AND NOT EXISTS (
  SELECT 1 FROM automation_retry_launches l
  JOIN automation_retry_launch_protocols p ON p.run_id = l.run_id AND p.version = 1
  JOIN automation_runs ar ON ar.id = l.run_id AND ar.status IN ('triggered', 'task_created')
  JOIN automation_retry_groups rg ON rg.id = ar.retry_group_id AND rg.generation = l.group_generation AND rg.state = 'live'
  JOIN automations a ON a.id = ar.automation_id AND a.enabled = TRUE
  WHERE l.task_id = NEW.task_id AND __EXACT_CREATION__ AND l.state = 'preparing' AND l.cancel_requested = FALSE)
 AND NOT (
  __EXPLICIT_MANUAL__
  AND NOT EXISTS (SELECT 1 FROM automation_runs ar WHERE ar.retry_group_id <> ''
   AND (ar.task_id = NEW.task_id OR ar.id = __TASK_RUN_OWNER__) AND ar.status IN ('triggered', 'task_created'))
  AND NOT EXISTS (SELECT 1 FROM automation_retry_launches l WHERE
   (l.task_id = NEW.task_id OR l.run_id = __TASK_RUN_OWNER__) AND l.state <> 'stopped'))`

func retryRetainedAdmissionCondition(driver string, turn bool) string {
	owner := "(SELECT " + dialect.JSONExtract(driver, "metadata", "automation_run_id") + " FROM tasks WHERE id = NEW.task_id)"
	exact := "l.session_id = NEW.id"
	if turn {
		exact = "l.session_id = NEW.task_session_id AND l.turn_id = NEW.id"
	}
	kind := retrySessionCreationKind
	if turn {
		kind = retryTurnCreationKind
	}
	manual := "EXISTS (SELECT 1 FROM task_creation_authorizations mc WHERE mc.kind = '" + kind + "' AND mc.creation_id=NEW.id AND mc.task_id=NEW.task_id)"

	return strings.NewReplacer("__TASK_RUN_OWNER__", owner, "__EXACT_CREATION__", exact, "__EXPLICIT_MANUAL__", manual).Replace(retryRetainedAdmissionPredicate)
}

func installRetryRetainedAdmissionGuards(ctx context.Context, tx *sqlx.Tx) error {
	for _, turn := range []bool{false, true} {
		table, kind := "task_sessions", retrySessionCreationKind
		if turn {
			table, kind = "task_session_turns", retryTurnCreationKind
		}
		name := "automation_retry_retired_" + kind + "_identity"
		condition := retryRetainedAdmissionCondition(tx.DriverName(), turn)
		query := fmt.Sprintf(`DROP TRIGGER IF EXISTS %s; CREATE TRIGGER %s BEFORE INSERT ON %s WHEN %s BEGIN SELECT RAISE(ABORT, 'retry admission is retired or identity is not reserved'); END;`, name, name, table, condition)
		if dialect.IsPostgres(tx.DriverName()) {
			query = fmt.Sprintf(`CREATE OR REPLACE FUNCTION %s_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF %s THEN RAISE EXCEPTION 'retry admission is retired or identity is not reserved'; END IF; RETURN NEW; END; $$;
    DROP TRIGGER IF EXISTS %s ON %s; CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s_guard();`, name, condition, name, table, name, table, name)
		}
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return installRetryAdmissionOwners(ctx, tx)
}

func retryAdmissionOwnerStatement(row string) string {
	return `INSERT INTO automation_retry_launch_protocols(run_id,task_id,version) SELECT ` + row + `.id,COALESCE(` + row + `.task_id,''),0 WHERE ` + row + `.retry_group_id <> ''
 ON CONFLICT(run_id) DO UPDATE SET task_id=excluded.task_id WHERE automation_retry_launch_protocols.task_id='' AND excluded.task_id<>'';`
}

func installRetryAdmissionOwners(ctx context.Context, tx *sqlx.Tx) error {
	newOwner, oldOwner := retryAdmissionOwnerStatement("NEW"), retryAdmissionOwnerStatement("OLD")
	query := `DROP TRIGGER IF EXISTS automation_retry_admission_owner_insert; DROP TRIGGER IF EXISTS automation_retry_admission_owner_update; DROP TRIGGER IF EXISTS automation_retry_admission_owner_delete;
 CREATE TRIGGER automation_retry_admission_owner_insert AFTER INSERT ON automation_runs BEGIN ` + newOwner + ` END;
 CREATE TRIGGER automation_retry_admission_owner_update AFTER UPDATE OF task_id ON automation_runs BEGIN ` + newOwner + ` END;
 CREATE TRIGGER automation_retry_admission_owner_delete BEFORE DELETE ON automation_runs BEGIN ` + oldOwner + ` END;`
	if dialect.IsPostgres(tx.DriverName()) {
		query = `CREATE OR REPLACE FUNCTION automation_retry_admission_owner_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF TG_OP='DELETE' THEN ` + oldOwner + ` RETURN OLD; END IF; ` + newOwner + ` RETURN NEW; END; $$;
   DROP TRIGGER IF EXISTS automation_retry_admission_owner_insert ON automation_runs;
   DROP TRIGGER IF EXISTS automation_retry_admission_owner_update ON automation_runs;
   DROP TRIGGER IF EXISTS automation_retry_admission_owner_delete ON automation_runs;
   CREATE TRIGGER automation_retry_admission_owner_insert AFTER INSERT ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_admission_owner_guard();
   CREATE TRIGGER automation_retry_admission_owner_update AFTER UPDATE OF task_id ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_admission_owner_guard();
   CREATE TRIGGER automation_retry_admission_owner_delete BEFORE DELETE ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_admission_owner_guard();`
	}
	if _, err := tx.ExecContext(ctx, query); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO automation_retry_launch_protocols(run_id,task_id,version) SELECT id,COALESCE(task_id,''),0 FROM automation_runs WHERE retry_group_id<>''
 ON CONFLICT(run_id) DO UPDATE SET task_id=excluded.task_id WHERE automation_retry_launch_protocols.task_id='' AND excluded.task_id<>''`); err != nil {
		return err
	}
	return installRetryOperationOwners(ctx, tx)
}

const retryOperationOwnerSQL = `INSERT INTO automation_retry_launch_protocols(run_id,task_id,version)
 SELECT ar.id,o.external_task_id,0 FROM automation_runs ar JOIN automation_run_operations o
 ON o.run_id=ar.id AND o.group_generation=ar.retry_group_generation
 WHERE ar.retry_group_id<>'' AND o.external_task_id<>'' __OPERATION_FILTER__
 ON CONFLICT(run_id) DO UPDATE SET task_id=excluded.task_id WHERE automation_retry_launch_protocols.task_id='' AND excluded.task_id<>'';`

func installRetryOperationOwners(ctx context.Context, tx *sqlx.Tx) error {
	scoped := strings.ReplaceAll(retryOperationOwnerSQL, "__OPERATION_FILTER__", "AND o.operation_id=NEW.operation_id")
	query := `DROP TRIGGER IF EXISTS automation_retry_operation_owner_insert;DROP TRIGGER IF EXISTS automation_retry_operation_owner_update;
 CREATE TRIGGER automation_retry_operation_owner_insert AFTER INSERT ON automation_run_operations BEGIN ` + scoped + ` END;
 CREATE TRIGGER automation_retry_operation_owner_update AFTER UPDATE OF external_task_id ON automation_run_operations BEGIN ` + scoped + ` END;`
	if dialect.IsPostgres(tx.DriverName()) {
		query = `CREATE OR REPLACE FUNCTION automation_retry_operation_owner_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN ` + scoped + ` RETURN NEW; END; $$;
 DROP TRIGGER IF EXISTS automation_retry_operation_owner_insert ON automation_run_operations;
 DROP TRIGGER IF EXISTS automation_retry_operation_owner_update ON automation_run_operations;
 CREATE TRIGGER automation_retry_operation_owner_insert AFTER INSERT ON automation_run_operations FOR EACH ROW EXECUTE FUNCTION automation_retry_operation_owner_guard();
 CREATE TRIGGER automation_retry_operation_owner_update AFTER UPDATE OF external_task_id ON automation_run_operations FOR EACH ROW EXECUTE FUNCTION automation_retry_operation_owner_guard();`
	}
	if _, err := tx.ExecContext(ctx, query); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, strings.ReplaceAll(retryOperationOwnerSQL, "__OPERATION_FILTER__", ""))
	return err
}
