package sqlite

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/steptelemetry"
)

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
		query += ` FOR UPDATE`
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
	return id, nil
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
