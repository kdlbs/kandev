package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/routing"
)

// ListTaskStepTransitions reads newest-first ledger rows using immutable IDs.
func (r *Repository) ListTaskStepTransitions(ctx context.Context, taskID string, beforeID int64, limit int) ([]models.StepTransition, error) {
	query := `SELECT id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, occurred_at
		FROM task_step_transitions WHERE task_id = ?`
	args := []any{taskID}
	if beforeID > 0 {
		query += ` AND id < ?`
		args = append(args, beforeID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list task step transitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]models.StepTransition, 0)
	for rows.Next() {
		var item models.StepTransition
		var fromWorkflow, fromStep, toWorkflow, toStep sql.NullString
		if err := rows.Scan(&item.ID, &fromWorkflow, &fromStep, &toWorkflow, &toStep, &item.Trigger, &item.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan task step transition: %w", err)
		}
		item.FromWorkflowID = nullableValue(fromWorkflow)
		item.FromWorkflowStepID = nullableValue(fromStep)
		item.ToWorkflowID = nullableValue(toWorkflow)
		item.ToWorkflowStepID = nullableValue(toStep)
		items = append(items, item)
	}
	return items, rows.Err()
}

func nullableValue(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

// ListWorkflowTransitionGroups counts retained routes involving one workflow.
// Rows remain present when a step is removed because the ledger stores IDs.
func (r *Repository) ListWorkflowTransitionGroups(ctx context.Context, workspaceID, workflowID, afterKey string, limit int) ([]models.TransitionGroup, error) {
	const query = `WITH scoped AS (
		SELECT CASE WHEN h.from_workflow_id = ? AND h.to_workflow_id = ? THEN 'within'
			WHEN h.to_workflow_id = ? THEN 'entry' ELSE 'exit' END AS kind,
			CASE WHEN h.from_workflow_id = ? THEN h.from_workflow_step_id END AS from_step_id,
			CASE WHEN h.to_workflow_id = ? THEN h.to_workflow_step_id END AS to_step_id
		FROM task_step_transitions h JOIN tasks t ON t.id = h.task_id
		WHERE t.workspace_id = ? AND (h.from_workflow_id = ? OR h.to_workflow_id = ?)
	), grouped AS (
		SELECT kind, from_step_id, to_step_id, COUNT(*) AS route_count
		FROM scoped GROUP BY kind, from_step_id, to_step_id
	), ordered AS (
		SELECT kind, from_step_id, to_step_id, route_count,
			kind || '|' || COALESCE(from_step_id, '') || '|' || COALESCE(to_step_id, '') AS route_key
		FROM grouped
	)
	SELECT kind, from_step_id, to_step_id, route_count FROM ordered
	WHERE route_key > ? ORDER BY route_key LIMIT ?`
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query), workflowID, workflowID, workflowID,
		workflowID, workflowID, workspaceID, workflowID, workflowID, afterKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list workflow transition groups: %w", err)
	}
	defer func() { _ = rows.Close() }()
	items := make([]models.TransitionGroup, 0)
	for rows.Next() {
		var item models.TransitionGroup
		var fromStep, toStep sql.NullString
		if err := rows.Scan(&item.Kind, &fromStep, &toStep, &item.Count); err != nil {
			return nil, fmt.Errorf("scan workflow transition group: %w", err)
		}
		item.FromStepID, item.ToStepID = nullableValue(fromStep), nullableValue(toStep)
		items = append(items, item)
	}
	return items, rows.Err()
}

// stepTransitionTx is satisfied by both *sql.Tx and *sqlx.Tx, the two
// transaction types the mutation paths in this package use.
type stepTransitionTx interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// readTaskStepInTx reads the task's current (workflow_id, workflow_step_id)
// inside the write transaction, taking a row lock on Postgres. Reading the
// old step outside the write transaction — or from an in-memory *models.Task
// the caller handed in — would break the chain invariant under concurrent
// moves: the read of the old step and the write of the new one must be
// serialized against other writers of the same task row. On SQLite the
// writer pool already serializes writers, so the plain read is sufficient.
func (r *Repository) readTaskStepInTx(ctx context.Context, tx stepTransitionTx, taskID string) (workflowID, stepID string, found bool, err error) {
	query := `SELECT workflow_id, workflow_step_id FROM tasks WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += postgresForUpdateClause
	}
	var wf, step sql.NullString
	err = tx.QueryRowContext(ctx, r.db.Rebind(query), taskID).Scan(&wf, &step)
	if err == sql.ErrNoRows {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return wf.String, step.String, true, nil
}

// stepTransitionInput is the fully-resolved shape of one ledger row, with
// empty strings still present — recordStepTransition normalizes them to NULL.
type stepTransitionInput struct {
	taskID             string
	fromWorkflowID     string
	fromWorkflowStepID string
	toWorkflowID       string
	toWorkflowStepID   string
	// occurredAt is the caller's own timestamp for this transition — the same
	// value the caller already stamped onto tasks.updated_at (or created_at
	// for the genesis row), computed once after the transactional old-state
	// read/lock. Callers MUST NOT leave this zero: recordStepTransition does
	// not call time.Now() itself, because a second, independent clock read
	// here (after the caller's) can drift from tasks.updated_at under lock
	// contention, breaking the spec's "occurred_at and updated_at agree at
	// write time" guarantee.
	occurredAt time.Time
}

// recordStepTransition writes exactly one ledger row when the step actually
// changed and returns that row's own immutable ledger identifier. Callers
// that need the step-entry requirement's entry identity (AC-OFFICE-STEP-
// ENTRY-001.2, .7) derive it from this id via formatEntryID; callers that
// need the identifier for models.Task.WorkflowStepTransitionID use it
// directly. It is a no-op (id 0, nil error) when fromStepID == toStepID
// (position-only reorder, re-issued move to the current step) and when both
// sides are empty (a task with no workflow at all — a row with both sides
// NULL is forbidden). Retrieval of the identifier is dialect-appropriate
// (ADR-0027): Postgres uses RETURNING id, SQLite uses the exec result's
// LastInsertId, both inside the same statement that commits the row so no
// separate count-then-use read can race it.
//
// The missing-table case needs no detection code and must not get any: if
// the CREATE TABLE was silently swallowed by the migration runner, the
// INSERT fails with "no such table" / "relation does not exist", that error
// returns unchanged, and the enclosing transaction rolls back — which is
// precisely the spec's required behaviour (the step change does not commit
// with no row). Never log-and-continue on this error.
func (r *Repository) recordStepTransition(ctx context.Context, tx stepTransitionTx, in stepTransitionInput) (id int64, err error) {
	if in.fromWorkflowStepID == in.toWorkflowStepID {
		if operation, ok := routing.FromContext(ctx); ok {
			operation.TaskID = in.taskID
			operation.ObservedStepID = in.fromWorkflowStepID
			operation.TargetStepID = in.toWorkflowStepID
			operation.Outcome = routing.OutcomeAlreadySatisfied
			if err := r.recordWorkflowRouteOperationTx(ctx, tx, operation); err != nil {
				return 0, err
			}
		}
		return 0, nil
	}
	if in.fromWorkflowStepID == "" && in.toWorkflowStepID == "" {
		return 0, nil
	}

	occurredAt := in.occurredAt
	if occurredAt.IsZero() {
		// Safety net for a caller that forgot to stamp its own shared
		// timestamp, not the expected path — every registered chokepoint
		// passes its own tasks.updated_at/created_at value.
		occurredAt = time.Now().UTC()
	}

	attribution := steptelemetry.FromContext(ctx)
	insertSQL := `
		INSERT INTO task_step_transitions
			(task_id, session_id, from_workflow_id, from_workflow_step_id, to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id, contract_version, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	args := []any{
		in.taskID,
		nullableString(attribution.SessionID),
		nullableString(in.fromWorkflowID),
		nullableString(in.fromWorkflowStepID),
		nullableString(in.toWorkflowID),
		nullableString(in.toWorkflowStepID),
		string(attribution.Trigger),
		string(attribution.ActorKind),
		nullableString(attribution.ActorID),
		steptelemetry.ContractVersion,
		occurredAt,
	}

	if dialect.IsPostgres(r.db.DriverName()) {
		if err := tx.QueryRowContext(ctx, r.db.Rebind(insertSQL+" RETURNING id"), args...).Scan(&id); err != nil {
			return 0, err
		}
	} else {
		result, execErr := tx.ExecContext(ctx, r.db.Rebind(insertSQL), args...)
		if execErr != nil {
			return 0, execErr
		}
		id, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	}

	// Counted at INSERT time, not at commit: a later statement in the same
	// caller-owned transaction failing after this point (rare — e.g. the
	// runner sync in UpdateTask) rolls the row back but the counter still
	// bumped. The counter (and its "_inserted_" name) is a health signal
	// ("is the writer alive"), not a commit-confirmed row-for-row audit
	// trail; the ledger table itself is that audit trail.
	steptelemetry.RecordLedgerRow(r.log, attribution.Trigger)
	if operation, ok := routing.FromContext(ctx); ok {
		operation.TaskID = in.taskID
		operation.ObservedStepID = in.fromWorkflowStepID
		operation.TargetStepID = in.toWorkflowStepID
		operation.Outcome = routing.OutcomeCommitted
		operation.TransitionID = id
		operation.EffectID = operation.ID + ":destination-entry"
		if err := r.recordWorkflowRouteOperationTx(ctx, tx, operation); err != nil {
			return 0, err
		}
		if result, ok := routing.ResultHolderFromContext(ctx); ok {
			result.TransitionID = id
			result.EffectID = operation.EffectID
		}
	}
	return id, nil
}

// GetLatestTaskStepTransitionID returns the immutable ledger identity of the
// task's latest workflow-step entry. Loaded task projections intentionally do
// not carry the transient transition field, so retryable workflow routing uses
// this read when it needs to reconstruct the current entry after a restart.
func (r *Repository) GetLatestTaskStepTransitionID(ctx context.Context, taskID string) (int64, error) {
	var id int64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id
		FROM task_step_transitions
		WHERE task_id = ?
		ORDER BY id DESC
		LIMIT 1
	`), taskID).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// EnsureCurrentTaskStepTransition returns the latest workflow-entry identity,
// creating a durable current-entry row when an older database predates the
// transition ledger. The task-row write guard serializes this backfill with
// workflow moves, so a launch never captures a synthetic identity for an
// obsolete step.
func (r *Repository) EnsureCurrentTaskStepTransition(ctx context.Context, taskID string) (int64, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin workflow entry backfill: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	entryID, err := r.ensureCurrentTaskStepTransitionTx(ctx, tx, taskID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return entryID, nil
}

func (r *Repository) ensureCurrentTaskStepTransitionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (int64, error) {
	// Acquire the same task-row write boundary used by queue admission. SQLite
	// otherwise allows a read transaction to observe the task before a
	// concurrent writer commits its move, while PostgreSQL obtains the row lock
	// through the same UPDATE and its FOR UPDATE read below.
	guard, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE tasks SET updated_at = updated_at WHERE id = ?
	`), taskID)
	if err != nil {
		return 0, fmt.Errorf("guard task for workflow entry backfill: %w", err)
	}
	rows, err := guard.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("workflow entry backfill task rows affected: %w", err)
	}
	if rows == 0 {
		return 0, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}

	var latestID int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id
		FROM task_step_transitions
		WHERE task_id = ?
		ORDER BY id DESC
		LIMIT 1
	`), taskID).Scan(&latestID)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("read workflow entry during backfill: %w", err)
	}
	if err == nil && latestID > 0 {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return latestID, nil
	}

	workflowID, workflowStepID, found, err := r.readTaskStepInTx(ctx, tx, taskID)
	if err != nil {
		return 0, err
	}
	if !found || workflowID == "" || workflowStepID == "" {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return 0, nil
	}

	backfilledID, err := r.recordStepTransition(ctx, tx, stepTransitionInput{
		taskID:           taskID,
		toWorkflowID:     workflowID,
		toWorkflowStepID: workflowStepID,
		occurredAt:       time.Now().UTC(),
	})
	if err != nil {
		return 0, fmt.Errorf("write workflow entry backfill: %w", err)
	}
	return backfilledID, nil
}

// CountStepEntries returns the number of committed task_step_transitions
// rows whose to_workflow_step_id is stepID for taskID — the recorded entry
// count REQ-TWS-001 floors at 1 to derive the step-entry number. Recorded
// entries before the ledger's first row (2026-08-16) do not exist and cannot
// be counted, so the result is a lower bound on the true entry count for a
// task whose history predates the ledger. An empty taskID or stepID returns
// (0, nil) without issuing a query: the ledger normalizes "" to NULL, so a
// query would only ever be able to return 0.
func (r *Repository) CountStepEntries(ctx context.Context, taskID, stepID string) (int, error) {
	if taskID == "" || stepID == "" {
		return 0, nil
	}
	const query = `SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ? AND to_workflow_step_id = ?`
	var count int
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(query), taskID, stepID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count step entries: %w", err)
	}
	return count, nil
}

// formatEntryID converts recordStepTransition's ledger identifier into the
// step-entry requirement's entry identity string (AC-OFFICE-STEP-ENTRY-
// 001.2, .7). id 0 means recordStepTransition was a no-op — dispatchStepEntry
// relies on "" (not "0") to detect that case, so it must not be formatted.
func formatEntryID(id int64) string {
	if id == 0 {
		return ""
	}
	return strconv.FormatInt(id, 10)
}

// genesisAttribution is the task-creation ledger row's attribution: the
// writer hard-codes TriggerTaskCreated (there is exactly one INSERT path, so
// no caller disambiguation is needed the way the other six chokepoints need
// it). See hardcodedTriggerAttribution for the actor-resolution rule shared
// with detachAttribution.
func genesisAttribution(ctx context.Context) steptelemetry.Attribution {
	return hardcodedTriggerAttribution(ctx, steptelemetry.TriggerTaskCreated)
}

// detachAttribution is RemoveTaskFromWorkflow's ledger row attribution: the
// writer hard-codes TriggerWorkflowDetached rather than relying on a caller
// to supply it. Unlike AddTaskToWorkflow — whose one production caller
// (office/engine_adapters/workflow_switcher_adapter.go) explicitly sets
// TriggerWorkflowAttached before calling — RemoveTaskFromWorkflow has no
// production caller today, so there is no wrapper anywhere to get this
// right; hardcoding it here means a future caller cannot get it wrong.
func detachAttribution(ctx context.Context) steptelemetry.Attribution {
	return hardcodedTriggerAttribution(ctx, steptelemetry.TriggerWorkflowDetached)
}

// hardcodedTriggerAttribution is the actor-resolution rule shared by ledger
// chokepoints whose trigger has exactly one semantic meaning regardless of
// caller. The actor prefers an explicit caller-supplied attribution when one
// is already on ctx (e.g. the watcher dispatch coordinator setting
// ActorIntegration for a Jira/Linear-originated create) and otherwise falls
// back to the identity already on the context — the existing authn seam.
func hardcodedTriggerAttribution(ctx context.Context, trigger steptelemetry.Trigger) steptelemetry.Attribution {
	if preset := steptelemetry.FromContext(ctx); preset.ActorKind != steptelemetry.ActorUnknown {
		return steptelemetry.Attribution{
			Trigger:   trigger,
			ActorKind: preset.ActorKind,
			ActorID:   preset.ActorID,
			SessionID: preset.SessionID,
		}
	}
	actorKind, actorID := steptelemetry.HumanOrSystemActor(ctx)
	return steptelemetry.Attribution{
		Trigger:   trigger,
		ActorKind: actorKind,
		ActorID:   actorID,
	}
}

// nullableString normalizes "" to SQL NULL: tasks.workflow_id and
// tasks.workflow_step_id use "" for "no step", but the ledger stores NULL for
// the same condition in every from_*/to_* column.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
