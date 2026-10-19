package automation

import (
	"context"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

// Quarantine carries only observed session ownership. It does not manufacture
// a submission, turn, execution or provider acceptance for a legacy attempt.
const retryLegacyQuarantineSQL = `
 INSERT INTO session_recovery_blocks
 (id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference, created_at, updated_at)
 SELECT 'automation-retry:' || ar.id || ':' || ts.id || ':' || CAST(COALESCE((SELECT MAX(hg.generation) FROM harness_session_generations hg WHERE hg.session_id = ts.id AND hg.incarnation_id = COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id)), 0) AS TEXT),
 ts.id, COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id),
 COALESCE((SELECT MAX(hg.generation) FROM harness_session_generations hg WHERE hg.session_id = ts.id AND hg.incarnation_id = COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id)), 0),
 'automation_retry_acceptance_unresolved', 'open', 'automation_retry:' || ar.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
 FROM automation_runs ar JOIN tasks t ON (t.id = ar.task_id OR ar.id = __RUN_OWNER__ OR EXISTS (SELECT 1 FROM automation_run_operations o WHERE o.run_id=ar.id AND o.group_generation=ar.retry_group_generation AND o.external_task_id=t.id))
 JOIN task_sessions ts ON ts.task_id = t.id
 WHERE ar.retry_group_id <> '' AND ar.status IN ('triggered', 'task_created')
 AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id = ar.id AND p.version IN (1, -1))
 __RUN_FILTER__
 ON CONFLICT (session_id, incarnation_id, expected_generation, reason) DO NOTHING;
`

func retryLegacyQuarantineQuery(driver, filter string) string {
	query := strings.ReplaceAll(retryLegacyQuarantineSQL, "__RUN_OWNER__", dialect.JSONExtract(driver, "t.metadata", "automation_run_id"))
	return strings.ReplaceAll(query, "__RUN_FILTER__", filter)
}

func installRetryLegacyQuarantine(ctx context.Context, tx *sqlx.Tx) error {
	tables, err := retryRequiredTablesPresent(ctx, tx, "session_recovery_blocks", "harness_session_generations")
	if err != nil {
		return err
	}
	if !tables {
		return nil
	}
	backfill := retryLegacyQuarantineQuery(tx.DriverName(), "")
	scoped := retryLegacyQuarantineQuery(tx.DriverName(), "AND ar.id = NEW.id")
	var guards string
	if dialect.IsPostgres(tx.DriverName()) {
		guards = `CREATE OR REPLACE FUNCTION automation_retry_legacy_quarantine_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN ` + scoped + ` RETURN NEW; END; $$;
  DROP TRIGGER IF EXISTS automation_retry_legacy_quarantine_insert ON automation_runs;
  DROP TRIGGER IF EXISTS automation_retry_legacy_quarantine_update ON automation_runs;
  CREATE TRIGGER automation_retry_legacy_quarantine_insert AFTER INSERT ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_legacy_quarantine_guard();
  CREATE TRIGGER automation_retry_legacy_quarantine_update AFTER UPDATE OF task_id, session_id, status ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_legacy_quarantine_guard();`
	} else {
		guards = `DROP TRIGGER IF EXISTS automation_retry_legacy_quarantine_insert;
  DROP TRIGGER IF EXISTS automation_retry_legacy_quarantine_update;
  CREATE TRIGGER automation_retry_legacy_quarantine_insert AFTER INSERT ON automation_runs BEGIN ` + scoped + ` END;
  CREATE TRIGGER automation_retry_legacy_quarantine_update AFTER UPDATE OF task_id, session_id, status ON automation_runs BEGIN ` + scoped + ` END;`
	}
	if _, err := tx.ExecContext(ctx, guards); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, backfill); err != nil {
		return err
	}
	operationScoped := retryLegacyQuarantineQuery(tx.DriverName(), "AND ar.id = NEW.run_id")
	operationGuards := `DROP TRIGGER IF EXISTS automation_retry_operation_quarantine_insert;DROP TRIGGER IF EXISTS automation_retry_operation_quarantine_update;
 CREATE TRIGGER automation_retry_operation_quarantine_insert AFTER INSERT ON automation_run_operations BEGIN ` + operationScoped + ` END;
 CREATE TRIGGER automation_retry_operation_quarantine_update AFTER UPDATE OF external_task_id ON automation_run_operations BEGIN ` + operationScoped + ` END;`
	if dialect.IsPostgres(tx.DriverName()) {
		operationGuards = `CREATE OR REPLACE FUNCTION automation_retry_operation_quarantine_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN ` + operationScoped + ` RETURN NEW; END; $$;
 DROP TRIGGER IF EXISTS automation_retry_operation_quarantine_insert ON automation_run_operations;
 DROP TRIGGER IF EXISTS automation_retry_operation_quarantine_update ON automation_run_operations;
 CREATE TRIGGER automation_retry_operation_quarantine_insert AFTER INSERT ON automation_run_operations FOR EACH ROW EXECUTE FUNCTION automation_retry_operation_quarantine_guard();
 CREATE TRIGGER automation_retry_operation_quarantine_update AFTER UPDATE OF external_task_id ON automation_run_operations FOR EACH ROW EXECUTE FUNCTION automation_retry_operation_quarantine_guard();`
	}
	_, err = tx.ExecContext(ctx, operationGuards)
	return err
}
