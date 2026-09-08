package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
)

// ErrWorkspaceAlreadyPaused is returned by CreateWorkspacePauseWithActivity
// when the partial unique index on office_workspace_pauses(workspace_id)
// rejects the insert because an unreleased row already exists. Callers
// re-read the active record via GetActiveWorkspacePause rather than treating
// this as a generic failure.
var ErrWorkspaceAlreadyPaused = errors.New("workspace already paused")

// releaseNoopDetails mirrors pause.noopDetails' JSON shape (that package
// cannot be imported here — it imports this one). Every workspace_pause_noop
// activity entry, whichever package writes it, must decode the same way
// (system-design-02.md's Observability section).
type releaseNoopDetails struct {
	RequestedOp string `json:"requested_op"`
	Reason      string `json:"reason"`
	Cause       string `json:"cause"`
}

func (r *Repository) createWorkspacePauseTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_workspace_pauses (
		id                TEXT PRIMARY KEY,
		workspace_id      TEXT NOT NULL,
		reason            TEXT NOT NULL,
		created_by        TEXT NOT NULL,
		created_by_kind   TEXT NOT NULL,
		created_at        TIMESTAMP NOT NULL,
		released_at       TIMESTAMP,
		released_by       TEXT NOT NULL DEFAULT '',
		released_reason   TEXT NOT NULL DEFAULT ''
	);

	CREATE UNIQUE INDEX IF NOT EXISTS idx_office_workspace_pause_active
		ON office_workspace_pauses(workspace_id) WHERE released_at IS NULL;

	CREATE INDEX IF NOT EXISTS idx_office_workspace_pause_history
		ON office_workspace_pauses(workspace_id, created_at DESC);
	`)
	return err
}

// GetActiveWorkspacePause returns the unreleased pause record for a
// workspace, or (nil, nil) when the workspace is running. A non-nil error
// means the read failed; callers treat that as "cannot determine pause
// state" (fail closed), never as "not paused".
func (r *Repository) GetActiveWorkspacePause(ctx context.Context, workspaceID string) (*models.WorkspacePause, error) {
	var pause models.WorkspacePause
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM office_workspace_pauses
		WHERE workspace_id = ? AND released_at IS NULL
	`), workspaceID).StructScan(&pause)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pause, nil
}

// CreateWorkspacePauseWithActivity inserts the pause record and its paired
// activity log entry in one transaction: either both are durable or
// neither is. When the partial unique index rejects the insert (a pause is
// already active), the transaction is rolled back and
// ErrWorkspaceAlreadyPaused is returned with no side effects — the caller
// re-reads the active record and, on the ordinary repeat-pause path, logs
// its own separate workspace_pause_noop entry rather than retrying here.
func (r *Repository) CreateWorkspacePauseWithActivity(
	ctx context.Context, pause *models.WorkspacePause, activity *models.ActivityEntry,
) error {
	if pause.ID == "" {
		pause.ID = uuid.New().String()
	}
	if pause.CreatedAt.IsZero() {
		pause.CreatedAt = time.Now().UTC()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_workspace_pauses (
			id, workspace_id, reason, created_by, created_by_kind, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`), pause.ID, pause.WorkspaceID, pause.Reason, pause.CreatedBy, pause.CreatedByKind, pause.CreatedAt)
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrWorkspaceAlreadyPaused
		}
		return fmt.Errorf("insert workspace pause: %w", err)
	}

	if err := insertActivityEntryTx(ctx, tx, r.db, activity); err != nil {
		return fmt.Errorf("insert pause activity: %w", err)
	}
	return tx.Commit()
}

// ReleaseWorkspacePauseWithActivity CAS-releases the pause record `id`
// (`WHERE id = ? AND released_at IS NULL`) and writes an activity log entry
// in the same transaction, whether or not the CAS actually changed a row —
// a losing resume in a concurrent release race is still an auditable
// request (AC-005.8). Which action/details to write depends on the CAS
// outcome, and that outcome (RowsAffected) is only known inside this
// transaction, so this method — not its caller — decides the entry's
// content: workspace_resumed on a win, workspace_pause_noop on a loss.
// Returns whether this call released the record.
func (r *Repository) ReleaseWorkspacePauseWithActivity(
	ctx context.Context, id, workspaceID, releasedBy, releasedByKind, releasedReason string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_workspace_pauses
		SET released_at = ?, released_by = ?, released_reason = ?
		WHERE id = ? AND released_at IS NULL
	`), time.Now().UTC(), releasedBy, releasedReason, id)
	if err != nil {
		return false, fmt.Errorf("release workspace pause: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	released := rows > 0

	action := models.ActivityActionWorkspaceResumed
	details := releasedReason
	if !released {
		action = models.ActivityActionWorkspacePauseNoop
		// A losing CAS is the lost-race case (a concurrent resume already
		// won): the noop entry gets the same structured {requested_op,
		// reason, cause} shape pause/service.go's logNoop writes for every
		// other workspace_pause_noop entry, not a bare reason string.
		encoded, err := json.Marshal(releaseNoopDetails{
			RequestedOp: "resume",
			Reason:      releasedReason,
			Cause:       "lost_race",
		})
		if err != nil {
			return false, fmt.Errorf("encode release noop details: %w", err)
		}
		details = string(encoded)
	}
	activity := &models.ActivityEntry{
		WorkspaceID: workspaceID,
		ActorType:   models.ActivityActorType(releasedByKind),
		ActorID:     releasedBy,
		Action:      action,
		TargetType:  models.ActivityTargetWorkspace,
		TargetID:    workspaceID,
		Details:     details,
	}
	if err := insertActivityEntryTx(ctx, tx, r.db, activity); err != nil {
		return false, fmt.Errorf("insert release activity: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return released, nil
}

// insertActivityEntryTx writes an activity log entry inside an
// already-open transaction. CreateActivityEntry (activity.go) is the
// non-transactional counterpart used for best-effort, standalone writes
// (e.g. the no-op entries this package's callers log outside a
// pause/resume transaction).
func insertActivityEntryTx(ctx context.Context, tx dbExecer, rebinder rebinder, entry *models.ActivityEntry) error {
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	_, err := tx.ExecContext(ctx, rebinder.Rebind(`
		INSERT INTO office_activity_log (
			id, workspace_id, actor_type, actor_id, action,
			target_type, target_id, details, run_id, session_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), entry.ID, entry.WorkspaceID, entry.ActorType, entry.ActorID,
		entry.Action, entry.TargetType, entry.TargetID, entry.Details,
		entry.RunID, entry.SessionID, entry.CreatedAt)
	return err
}

// dbExecer is the subset of *sqlx.Tx this file needs; declared locally so
// insertActivityEntryTx doesn't have to import sqlx just for the type name.
type dbExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// rebinder is the subset of *sqlx.DB this file needs for placeholder
// rebinding.
type rebinder interface {
	Rebind(query string) string
}

// ListInflightRunsForWorkspace returns every queued or claimed run
// belonging to an agent in the workspace. Selecting on the agent (not the
// payload task id) is what covers taskless runs, which lightweight
// routines produce. An empty result is not an error.
func (r *Repository) ListInflightRunsForWorkspace(ctx context.Context, workspaceID string) ([]models.InflightRun, error) {
	taskIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")
	query := fmt.Sprintf(`
		SELECT id, COALESCE(%s, '') AS task_id
		FROM runs
		WHERE status IN ('queued', 'claimed')
		  AND agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)
	`, taskIDExpr)
	var runs []models.InflightRun
	if err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(query), workspaceID); err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []models.InflightRun{}
	}
	return runs, nil
}

// ListLiveRoutineTaskIDsForWorkspace returns the distinct linked task ids
// of the workspace's routine runs whose task has not reached a terminal
// state (completed, failed, cancelled). Routine-run status is deliberately
// not the filter: SyncRunStatus is a no-op stub, so a heavy routine run
// never leaves task_created and filtering on it would select every heavy
// task the workspace has ever produced. An empty result is not an error.
func (r *Repository) ListLiveRoutineTaskIDsForWorkspace(ctx context.Context, workspaceID string) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(`
		SELECT DISTINCT rr.linked_task_id
		FROM office_routine_runs rr
		JOIN office_routines ro ON ro.id = rr.routine_id
		JOIN tasks t ON t.id = rr.linked_task_id
		WHERE ro.workspace_id = ?
		  AND rr.linked_task_id != ''
		  AND t.state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// CancelRunsForWorkspace cancels the given runs (queued or claimed only,
// per CancelRunsWhere's guard) with the given reason and returns how many
// rows were actually cancelled.
func (r *Repository) CancelRunsForWorkspace(ctx context.Context, runIDs []string, reason string) (int64, error) {
	if len(runIDs) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(runIDs))
	args := make([]interface{}, len(runIDs))
	for i, id := range runIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	selector := fmt.Sprintf(`id IN (%s)`, strings.Join(placeholders, ","))
	return r.CancelRunsWhere(ctx, reason, selector, args...)
}

// ReleaseCheckoutsForWorkspace clears the task checkout held by each given
// run id. Safe to call with the sweep's whole run-id snapshot rather than
// only the subset CancelRunsForWorkspace actually transitioned: the
// predicate is keyed on checkout_run_id, so a run that reached a terminal
// state independently either already cleared its checkout (no row
// matches) or still holds one (releasing it is correct).
func (r *Repository) ReleaseCheckoutsForWorkspace(ctx context.Context, runIDs []string) error {
	if len(runIDs) == 0 {
		return nil
	}
	placeholders := make([]string, len(runIDs))
	args := make([]interface{}, len(runIDs))
	for i, id := range runIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := fmt.Sprintf(`
		UPDATE tasks SET checkout_agent_id = '', checkout_at = NULL, checkout_run_id = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE checkout_run_id IN (%s)
	`, strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}

// CreatePauseSkippedRoutineRun records a routine fire blocked by a
// workspace pause as a skipped run, keyed on (routine_id, pause_id) by the
// partial unique index idx_office_routine_run_pause_once. Reports false
// without error when that index rejects the insert — the expected outcome
// for every fire after the first under the same pause (-002.13). routine_id
// and source are NOT NULL with no default; every other column not named
// here is defaulted by the table.
func (r *Repository) CreatePauseSkippedRoutineRun(
	ctx context.Context, routineID, triggerID, source, pauseID string,
) (bool, error) {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_routine_runs (
			id, routine_id, trigger_id, source, status, skip_reason, pause_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.New().String(), routineID, triggerID, source,
		models.RoutineRunStatusSkipped, workspacePausedSkipReason, pauseID, time.Now().UTC())
	if err != nil {
		if isUniqueConstraintErr(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// workspacePausedSkipReason is the sole skip_reason value a workspace
// pause writes onto a blocked office_routine_runs row.
const workspacePausedSkipReason = "workspace_paused"
