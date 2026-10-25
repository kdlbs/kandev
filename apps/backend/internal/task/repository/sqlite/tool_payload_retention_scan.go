package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// ErrPayloadEligibilityBudget means no eligibility decision was made. The caller
// must retain the task and report a budget skip, never reuse an earlier decision.
var ErrPayloadEligibilityBudget = errors.New("payload eligibility budget exceeded")

// PayloadTaskEligible checks persisted admission and activity before payload maintenance.
func PayloadTaskEligible(ctx context.Context, q sqlx.QueryerContext, taskID string, cutoff time.Time) (bool, error) {
	driver, ok := q.(interface {
		DriverName() string
		Rebind(string) string
	})
	if !ok || driver.DriverName() != "sqlite3" {
		return false, errors.New("payload retention requires SQLite")
	}
	stmt, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	var eligible bool
	err := sqlx.GetContext(stmt, q, &eligible, driver.Rebind(payloadTaskEligibilitySQL), cutoff.UTC().Format(time.RFC3339Nano), taskID)
	if errors.Is(stmt.Err(), context.DeadlineExceeded) {
		return false, fmt.Errorf("%w: %w", ErrPayloadEligibilityBudget, context.DeadlineExceeded)
	}
	if err != nil {
		return false, err
	}
	if stmt.Err() != nil {
		return false, stmt.Err()
	}
	return eligible, nil
}

var payloadTaskEligibilitySQL = buildPayloadTaskEligibilitySQL()

func buildPayloadTaskEligibilitySQL() string {
	query := payloadTaskEligibilityTemplate
	for _, column := range []string{"t.created_at", "t.updated_at", "s.started_at", "s.completed_at", "s.updated_at", "m.created_at", "m.updated_at", "tr.started_at", "tr.created_at", "tr.completed_at", "tr.updated_at"} {
		query = strings.ReplaceAll(query, "{{"+column+"}}", payloadTimestampBeforeCutoff(column))
	}
	return query
}

// SQLite accepts numeric Julian dates and normalizes impossible calendar days.
// Only recognized calendar dates qualify; offsets are compared chronologically.
func payloadTimestampBeforeCutoff(column string) string {
	return fmt.Sprintf(`COALESCE(typeof(%[1]s)='text'
 AND substr(%[1]s,1,10) GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'
 AND (length(%[1]s)=10 OR (length(%[1]s)>=19 AND substr(%[1]s,11,1) IN ('T',' ')
   AND substr(%[1]s,12,2) BETWEEN '00' AND '23' AND substr(%[1]s,15,2) BETWEEN '00' AND '59'
   AND substr(%[1]s,18,2) BETWEEN '00' AND '59'))
 AND date(substr(%[1]s,1,10),'+0 days')=substr(%[1]s,1,10)
 AND julianday(%[1]s)>julianday('0001-01-01') AND julianday(%[1]s)<t.cutoff,0)`, column)
}

const payloadTaskEligibilityTemplate = `WITH candidate AS (SELECT *, julianday(?) AS cutoff FROM tasks WHERE id = ?)
SELECT EXISTS(SELECT 1 FROM candidate t
 WHERE t.state IN ('TODO','CREATED','REVIEW','BLOCKED','COMPLETED','FAILED','CANCELLED')
 AND COALESCE(t.queued_for_step_id,'')='' AND t.queued_at IS NULL
 AND {{t.created_at}} AND {{t.updated_at}}
 AND EXISTS(SELECT 1 FROM task_sessions s WHERE s.task_id=t.id)
 AND NOT EXISTS(SELECT 1 FROM task_sessions s WHERE s.task_id=t.id AND (
   s.state NOT IN ('COMPLETED','FAILED','CANCELLED') OR s.state IS NULL
   OR NOT {{s.started_at}} OR NOT {{s.completed_at}} OR NOT {{s.updated_at}}
   OR EXISTS(SELECT 1 FROM task_session_messages m INDEXED BY idx_messages_session_updated
     WHERE m.task_session_id=s.id AND NOT {{m.updated_at}})
   OR EXISTS(SELECT 1 FROM task_session_messages m INDEXED BY idx_messages_session_created
     WHERE m.task_session_id=s.id AND NOT {{m.created_at}})
   OR EXISTS(SELECT 1 FROM task_session_messages m WHERE m.task_session_id=s.id AND (
     m.task_id IS NULL OR m.task_id!=t.id OR m.author_type IS NULL OR m.author_type NOT IN ('user','agent')
     OR (m.type IN ('permission_request','clarification_request') AND m.author_type!='agent')
     OR NOT EXISTS(SELECT 1 FROM task_session_turns tr WHERE tr.id=m.turn_id AND tr.task_id=t.id AND tr.task_session_id=s.id)))
 ))
 AND NOT EXISTS(SELECT 1 FROM task_session_messages m INDEXED BY idx_messages_task_author_created
   WHERE m.task_id=t.id AND NOT EXISTS(SELECT 1 FROM task_sessions s WHERE s.id=m.task_session_id AND s.task_id=t.id))
 AND NOT EXISTS(SELECT 1 FROM task_session_turns tr
   WHERE (tr.task_id=t.id OR tr.task_session_id IN (SELECT id FROM task_sessions WHERE task_id=t.id)) AND (
     tr.task_id IS NULL OR tr.task_id!=t.id
     OR NOT EXISTS(SELECT 1 FROM task_sessions s WHERE s.id=tr.task_session_id AND s.task_id=t.id)
     OR NOT {{tr.started_at}} OR NOT {{tr.created_at}} OR NOT {{tr.completed_at}} OR NOT {{tr.updated_at}}
     OR CASE WHEN json_valid(tr.metadata) THEN
       json_type(tr.metadata)!='object'
       OR (json_type(tr.metadata,'$.prompt_dispatch_pending') IS NOT NULL AND COALESCE(CAST(json_extract(tr.metadata,'$.prompt_dispatch_pending') AS TEXT) IN ('false','0'),0)=0)
       OR (json_type(tr.metadata,'$.prompt_dispatch_start_event_pending') IS NOT NULL AND COALESCE(CAST(json_extract(tr.metadata,'$.prompt_dispatch_start_event_pending') AS TEXT) IN ('false','0'),0)=0)
       ELSE 1 END))
 AND NOT EXISTS(SELECT 1 FROM task_session_messages m INDEXED BY idx_messages_task_author_created
   WHERE m.task_id=t.id AND m.author_type='agent' AND m.type IN ('permission_request','clarification_request')
   AND CASE WHEN json_valid(m.metadata) THEN
     json_type(m.metadata)!='object'
     OR (m.type='permission_request' AND COALESCE(json_extract(m.metadata,'$.status') IN ('approved','rejected','expired'),0)=0)
     OR (m.type='clarification_request' AND COALESCE(json_extract(m.metadata,'$.status') IN ('answered','rejected','expired'),0)=0)
     OR (json_type(m.metadata,'$.response_delivery_pending') IS NOT NULL AND COALESCE(CAST(json_extract(m.metadata,'$.response_delivery_pending') AS TEXT) IN ('false','0'),0)=0)
     OR (json_type(m.metadata,'$.permission_resolution') IS NOT NULL AND COALESCE(json_type(m.metadata,'$.permission_resolution')='object' AND json_extract(m.metadata,'$.permission_resolution.result') IN ('accepted','stale','expired','failed','indeterminate'),0)=0)
     ELSE 1 END)
 AND NOT EXISTS(SELECT 1 FROM queued_messages q WHERE q.task_id=t.id OR q.session_id IN (SELECT id FROM task_sessions WHERE task_id=t.id))
 AND NOT EXISTS(SELECT 1 FROM pending_moves p WHERE p.task_id=t.id OR p.session_id IN (SELECT id FROM task_sessions WHERE task_id=t.id))
 AND NOT EXISTS(SELECT 1 FROM runs r WHERE r.rowid IN (
   SELECT rowid FROM runs INDEXED BY idx_run_status_requested WHERE status NOT IN ('finished','failed','cancelled') OR status IS NULL)
   AND (CASE WHEN json_valid(r.payload) AND json_type(r.payload)='object' THEN
     COALESCE(json_extract(r.payload,'$.task_id')=t.id,0)
     OR (COALESCE(json_type(r.payload,'$.task_id')='text' AND json_extract(r.payload,'$.task_id')!='',0)=0 AND COALESCE(r.session_id,'')='')
     ELSE 1 END
     OR r.session_id IN (SELECT id FROM task_sessions WHERE task_id=t.id)))
)`
