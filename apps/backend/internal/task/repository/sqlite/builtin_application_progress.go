package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// AdvanceBuiltinMCPApplication persists progress before provider control. The
// saved target is checked again at retirement and at binding acceptance.
func (r *Repository) AdvanceBuiltinMCPApplication(ctx context.Context, request *models.BuiltinMCPApplication, stage, reason string) (bool, error) {
	if !validBuiltinProgressRequest(request, stage, reason) {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), request.SessionID); err != nil {
		return false, err
	}
	advanced, err := r.advanceBuiltinMCPApplicationTx(ctx, tx, request, stage, reason)
	if err != nil || !advanced {
		return false, err
	}
	return true, tx.Commit()
}

func (r *Repository) advanceBuiltinMCPApplicationTx(ctx context.Context, tx *sqlx.Tx, request *models.BuiltinMCPApplication, stage, reason string) (bool, error) {
	app, metadata, err := r.builtinApplicationOwnerTx(ctx, tx, request.SessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if app.OperationID != request.OperationID || app.RowVersion != request.RowVersion || app.Stage != request.Stage {
		return false, nil
	}
	valid, err := r.applyBuiltinProgressStageTx(ctx, tx, app, request, metadata, stage)
	if err != nil || !valid {
		return false, err
	}
	stage, reason = coalesceBuiltinApplicationTarget(app, stage, reason)

	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE task_builtin_mcp_applications SET stage=?,reason=?,retry_of_operation_id=?,attempt_source=?,target_binding_id=?,target_binding_json=?,row_version=row_version+1,updated_at=? WHERE session_id=? AND operation_id=? AND row_version=?`), stage, reason, app.RetryOfOperationID, app.AttemptSource, app.TargetBindingID, app.TargetBindingJSON, time.Now().UTC(), app.SessionID, app.OperationID, app.RowVersion)
	if err != nil {
		return false, err
	}
	return true, nil
}

func builtinProgressAllowed(from, to string) bool {
	if to == models.BuiltinMCPApplicationFailed {
		return from == models.BuiltinMCPApplicationPreparing || from == models.BuiltinMCPApplicationRetiring || from == models.BuiltinMCPApplicationStarting || from == models.BuiltinMCPApplicationVerifying || from == models.BuiltinMCPApplicationAccepting
	}
	switch from {
	case models.BuiltinMCPApplicationPreparing:
		return to == models.BuiltinMCPApplicationPending || to == models.BuiltinMCPApplicationRetiring
	case models.BuiltinMCPApplicationRetiring:
		return to == models.BuiltinMCPApplicationStarting
	case models.BuiltinMCPApplicationStarting:
		return to == models.BuiltinMCPApplicationVerifying
	case models.BuiltinMCPApplicationAccepting:
		return to == models.BuiltinMCPApplicationApplied || to == models.BuiltinMCPApplicationAccepting
	}
	return false
}

// ListBuiltinMCPApplications includes unfinished work after a backend restart.
func (r *Repository) ListBuiltinMCPApplications(ctx context.Context) ([]*models.BuiltinMCPApplication, error) {
	rows := []*models.BuiltinMCPApplication{}
	err := r.ro.SelectContext(ctx, &rows, `SELECT a.* FROM task_builtin_mcp_applications a JOIN tasks t ON t.id=a.task_id AND t.workspace_id=a.workspace_id WHERE t.archived_at IS NULL AND a.stage NOT IN ('applied','failed','superseded') ORDER BY a.updated_at, a.session_id`)
	return rows, err
}

func validBuiltinProgressRequest(request *models.BuiltinMCPApplication, stage, reason string) bool {
	if request == nil || !builtinProgressAllowed(request.Stage, stage) {
		return false
	}
	return request.Stage != stage || stage == models.BuiltinMCPApplicationAccepting && reason == "acceptance_unconfirmed"
}
func (r *Repository) applyBuiltinProgressStageTx(ctx context.Context, tx *sqlx.Tx, app, request *models.BuiltinMCPApplication, metadata map[string]any, stage string) (bool, error) {
	switch stage {
	case models.BuiltinMCPApplicationRetiring:
		return r.stageBuiltinRetirementTx(ctx, tx, app, request, metadata)
	case models.BuiltinMCPApplicationStarting:
		return r.stageBuiltinProviderTarget(app, request)
	case models.BuiltinMCPApplicationApplied:
		return r.finalizeBuiltinProviderTx(ctx, tx, app, request, metadata)
	default:
		return true, nil
	}
}
func coalesceBuiltinApplicationTarget(app *models.BuiltinMCPApplication, stage, reason string) (string, string) {
	if stage == models.BuiltinMCPApplicationFailed && reason == "settings_changed" && (app.AttemptRevision != app.TargetRevision || app.AttemptSelectionHash != app.SelectionHash) {
		stage = models.BuiltinMCPApplicationPending
		reason = ""
		app.RetryOfOperationID = app.OperationID
	}
	if stage == models.BuiltinMCPApplicationApplied && (app.AttemptRevision != app.TargetRevision || app.AttemptSelectionHash != app.SelectionHash) {
		stage = models.BuiltinMCPApplicationPending
	}
	return stage, reason
}
