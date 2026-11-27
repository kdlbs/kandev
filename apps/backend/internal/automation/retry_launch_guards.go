package automation

import (
	"context"

	"github.com/kandev/kandev/internal/db/dialect"
)

const retrySessionGuardSQLite = `
CREATE TRIGGER IF NOT EXISTS automation_retry_session_identity BEFORE INSERT ON task_sessions
WHEN EXISTS (
 SELECT 1 FROM automation_runs ar
 WHERE ar.retry_group_id <> '' AND (ar.task_id = NEW.task_id OR ar.id = (SELECT json_extract(metadata, '$.automation_run_id') FROM tasks WHERE id = NEW.task_id)) AND ar.status IN ('triggered', 'task_created')
 AND NOT EXISTS (SELECT 1 FROM automation_retry_launches l JOIN automation_retry_launch_protocols p ON p.run_id = l.run_id WHERE p.version = 1 AND l.run_id = ar.id
  AND l.task_id = NEW.task_id AND l.session_id = NEW.id AND l.state = 'preparing' AND l.cancel_requested = FALSE)
)
BEGIN SELECT RAISE(ABORT, 'retry session identity is not reserved'); END;`

const retryTurnGuardSQLite = `
CREATE TRIGGER IF NOT EXISTS automation_retry_turn_identity BEFORE INSERT ON task_session_turns
WHEN EXISTS (
 SELECT 1 FROM automation_runs ar
 WHERE ar.retry_group_id <> '' AND (ar.task_id = NEW.task_id OR ar.id = (SELECT json_extract(metadata, '$.automation_run_id') FROM tasks WHERE id = NEW.task_id)) AND ar.status IN ('triggered', 'task_created')
 AND NOT EXISTS (SELECT 1 FROM automation_retry_launches l JOIN automation_retry_launch_protocols p ON p.run_id = l.run_id WHERE p.version = 1 AND l.run_id = ar.id
  AND l.task_id = NEW.task_id AND l.session_id = NEW.task_session_id AND l.turn_id = NEW.id
  AND l.state = 'preparing' AND l.cancel_requested = FALSE)
)
BEGIN SELECT RAISE(ABORT, 'retry turn identity is not reserved'); END;`

const retryRunIdentityGuardSQLite = `
CREATE TRIGGER IF NOT EXISTS automation_retry_run_identity BEFORE UPDATE ON automation_runs
WHEN OLD.retry_group_id <> '' AND (
 (COALESCE(OLD.task_id, '') <> '' AND COALESCE(NEW.task_id, '') <> OLD.task_id)
 OR (COALESCE(OLD.session_id, '') <> '' AND COALESCE(NEW.session_id, '') <> OLD.session_id)
 OR (COALESCE(OLD.turn_id, '') <> '' AND COALESCE(NEW.turn_id, '') <> OLD.turn_id)
 OR (COALESCE(OLD.session_id, '') <> '' AND COALESCE(OLD.turn_id, '') <> ''
  AND (COALESCE(NEW.thread_action, '') <> COALESCE(OLD.thread_action, '') OR COALESCE(NEW.thread_reason, '') <> COALESCE(OLD.thread_reason, '')))
)
BEGIN SELECT RAISE(ABORT, 'retry binding identity is immutable'); END;`

const retryRunRetentionGuardSQLite = `
CREATE TRIGGER IF NOT EXISTS automation_retry_run_retention BEFORE DELETE ON automation_runs
WHEN OLD.retry_group_id <> '' AND (
 EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id = OLD.id AND l.state <> 'stopped')
 OR (OLD.status IN ('triggered', 'task_created') AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id = OLD.id AND p.version IN (1, -1)))
)
BEGIN SELECT RAISE(ABORT, 'retry launch ownership is unresolved'); END;
CREATE TRIGGER IF NOT EXISTS automation_retry_run_terminal BEFORE UPDATE OF status ON automation_runs
WHEN OLD.retry_group_id <> '' AND NEW.status NOT IN ('triggered', 'task_created') AND (
 EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id = OLD.id AND l.state <> 'stopped')
 OR (OLD.status IN ('triggered', 'task_created') AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id = OLD.id AND p.version IN (1, -1)))
)
BEGIN SELECT RAISE(ABORT, 'retry launch ownership is unresolved'); END;`

const retryIdentityGuardsPostgres = `
CREATE OR REPLACE FUNCTION automation_retry_session_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM a.id FROM automations a JOIN automation_runs ar ON ar.automation_id = a.id
 WHERE ar.retry_group_id <> '' AND (ar.task_id = NEW.task_id OR ar.id = (SELECT metadata::jsonb->>'automation_run_id' FROM tasks WHERE id = NEW.task_id)) AND ar.status IN ('triggered', 'task_created') FOR UPDATE OF a;
 IF EXISTS (SELECT 1 FROM automation_runs ar
  WHERE ar.retry_group_id <> '' AND (ar.task_id = NEW.task_id OR ar.id = (SELECT metadata::jsonb->>'automation_run_id' FROM tasks WHERE id = NEW.task_id)) AND ar.status IN ('triggered', 'task_created')
  AND NOT EXISTS (SELECT 1 FROM automation_retry_launches l JOIN automation_retry_launch_protocols p ON p.run_id = l.run_id WHERE p.version = 1 AND l.run_id = ar.id
   AND l.task_id = NEW.task_id AND l.session_id = NEW.id AND l.state = 'preparing' AND l.cancel_requested = FALSE))
 THEN RAISE EXCEPTION 'retry session identity is not reserved'; END IF;
 RETURN NEW;
END; $$;
CREATE OR REPLACE FUNCTION automation_retry_turn_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM a.id FROM automations a JOIN automation_runs ar ON ar.automation_id = a.id
 WHERE ar.retry_group_id <> '' AND (ar.task_id = NEW.task_id OR ar.id = (SELECT metadata::jsonb->>'automation_run_id' FROM tasks WHERE id = NEW.task_id)) AND ar.status IN ('triggered', 'task_created') FOR UPDATE OF a;
 IF EXISTS (SELECT 1 FROM automation_runs ar
  WHERE ar.retry_group_id <> '' AND (ar.task_id = NEW.task_id OR ar.id = (SELECT metadata::jsonb->>'automation_run_id' FROM tasks WHERE id = NEW.task_id)) AND ar.status IN ('triggered', 'task_created')
  AND NOT EXISTS (SELECT 1 FROM automation_retry_launches l JOIN automation_retry_launch_protocols p ON p.run_id = l.run_id WHERE p.version = 1 AND l.run_id = ar.id
   AND l.task_id = NEW.task_id AND l.session_id = NEW.task_session_id AND l.turn_id = NEW.id
   AND l.state = 'preparing' AND l.cancel_requested = FALSE))
 THEN RAISE EXCEPTION 'retry turn identity is not reserved'; END IF;
 RETURN NEW;
END; $$;
CREATE OR REPLACE FUNCTION automation_retry_run_identity_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.retry_group_id <> '' AND (
  (COALESCE(OLD.task_id, '') <> '' AND COALESCE(NEW.task_id, '') <> OLD.task_id)
  OR (COALESCE(OLD.session_id, '') <> '' AND COALESCE(NEW.session_id, '') <> OLD.session_id)
  OR (COALESCE(OLD.turn_id, '') <> '' AND COALESCE(NEW.turn_id, '') <> OLD.turn_id)
  OR (COALESCE(OLD.session_id, '') <> '' AND COALESCE(OLD.turn_id, '') <> ''
   AND (COALESCE(NEW.thread_action, '') <> COALESCE(OLD.thread_action, '') OR COALESCE(NEW.thread_reason, '') <> COALESCE(OLD.thread_reason, '')))
 ) THEN RAISE EXCEPTION 'retry binding identity is immutable'; END IF;
 RETURN NEW;
END; $$;
CREATE OR REPLACE FUNCTION automation_retry_run_retention_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.retry_group_id <> '' AND (
  EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id = OLD.id AND l.state <> 'stopped')
  OR (OLD.status IN ('triggered', 'task_created') AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id = OLD.id AND p.version IN (1, -1)))
 ) THEN RAISE EXCEPTION 'retry launch ownership is unresolved'; END IF;
 RETURN OLD;
END; $$;
CREATE OR REPLACE FUNCTION automation_retry_run_terminal_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.retry_group_id <> '' AND NEW.status NOT IN ('triggered', 'task_created') AND (
  EXISTS (SELECT 1 FROM automation_retry_launches l WHERE l.run_id = OLD.id AND l.state <> 'stopped')
  OR (OLD.status IN ('triggered', 'task_created') AND NOT EXISTS (SELECT 1 FROM automation_retry_launch_protocols p WHERE p.run_id = OLD.id AND p.version IN (1, -1)))
 ) THEN RAISE EXCEPTION 'retry launch ownership is unresolved'; END IF;
 RETURN NEW;
END; $$;
DO $$ BEGIN
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 'automation_runs'::regclass AND tgname = 'automation_retry_run_retention') THEN
  CREATE TRIGGER automation_retry_run_retention BEFORE DELETE ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_run_retention_guard();
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 'automation_runs'::regclass AND tgname = 'automation_retry_run_terminal') THEN
  CREATE TRIGGER automation_retry_run_terminal BEFORE UPDATE OF status ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_run_terminal_guard();
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 'automation_runs'::regclass AND tgname = 'automation_retry_run_identity') THEN
  CREATE TRIGGER automation_retry_run_identity BEFORE UPDATE ON automation_runs FOR EACH ROW EXECUTE FUNCTION automation_retry_run_identity_guard();
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 'task_sessions'::regclass AND tgname = 'automation_retry_session_identity') THEN
  CREATE TRIGGER automation_retry_session_identity BEFORE INSERT ON task_sessions FOR EACH ROW EXECUTE FUNCTION automation_retry_session_identity_guard();
 END IF;
 IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid = 'task_session_turns'::regclass AND tgname = 'automation_retry_turn_identity') THEN
  CREATE TRIGGER automation_retry_turn_identity BEFORE INSERT ON task_session_turns FOR EACH ROW EXECUTE FUNCTION automation_retry_turn_identity_guard();
 END IF;
END; $$;`

func (s *Store) installRetryIdentityGuards(ctx context.Context) error {
	tables, err := retryRequiredTablesPresent(ctx, s.db, "tasks", "task_sessions", "task_session_turns", "task_creation_authorizations")
	if err != nil {
		return err
	}
	if !tables {
		return nil
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	guards := retryIdentityGuardsPostgres
	if !dialect.IsPostgres(s.db.DriverName()) {
		guards = `DROP TRIGGER IF EXISTS automation_retry_session_identity;DROP TRIGGER IF EXISTS automation_retry_turn_identity;DROP TRIGGER IF EXISTS automation_retry_run_identity;DROP TRIGGER IF EXISTS automation_retry_run_retention;DROP TRIGGER IF EXISTS automation_retry_run_terminal;` + retrySessionGuardSQLite + retryTurnGuardSQLite + retryRunIdentityGuardSQLite + retryRunRetentionGuardSQLite
	}
	if _, err := tx.ExecContext(ctx, guards); err != nil {
		return err
	}
	if err := installRetryRetainedAdmissionGuards(ctx, tx); err != nil {
		return err
	}
	if err := installRetryLegacyQuarantine(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.launchIdentityGuards = true
	return nil
}
