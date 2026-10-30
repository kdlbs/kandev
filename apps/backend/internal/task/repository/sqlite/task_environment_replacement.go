package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// Retirement is added after the legacy table rebuilds and ownership cutover.
func (r *Repository) ensureTaskEnvironmentRetirementSchema() error {
	tx, err := r.db.BeginTxx(r.migrationContext(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := r.columnExists(tx, "task_environments", "retired_at")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.ExecContext(r.migrationContext(), `ALTER TABLE task_environments ADD COLUMN retired_at TIMESTAMP`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) replacementEnvironmentIdentityTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.TaskEnvironment, error) {
	env := &models.TaskEnvironment{}
	var retired sql.NullTime
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, task_id, ownership_generation, status, COALESCE(materialization_session_id, ''),
		       COALESCE(task_dir_name, ''), retired_at
		FROM task_environments WHERE id = ?
	`), id).Scan(&env.ID, &env.TaskID, &env.OwnershipGeneration, &env.Status,
		&env.MaterializationSessionID, &env.TaskDirName, &retired)
	if retired.Valid {
		env.RetiredAt = &retired.Time
	}
	return env, err
}

// retireFailedEnvironmentTx changes only retirement metadata. All physical
// inventory and historical session bindings remain owned by the retained row.
func (r *Repository) retireFailedEnvironmentTx(ctx context.Context, tx *sqlx.Tx, expected *models.TaskEnvironment) (bool, error) {
	current, err := r.replacementEnvironmentIdentityTx(ctx, tx, expected.ID)
	if err != nil {
		return false, err
	}
	if current.TaskID != expected.TaskID || current.OwnershipGeneration != expected.OwnershipGeneration {
		return false, ErrTaskEnvironmentOwnershipChanged
	}
	if current.IsRetired() || current.Status != models.TaskEnvironmentStatusFailed || current.MaterializationSessionID != "" {
		return false, nil
	}
	if err := recoveryclaim.EnsureAvailableTx(recoveryclaim.WithoutClaim(ctx), r.db, tx, current.ID); err != nil {
		return false, err
	}
	live, err := r.failedEnvironmentHasConsumersTx(ctx, tx, current.ID)
	if err != nil || live {
		return false, err
	}
	now := r.nowUTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_environments
		SET retired_at = ?, updated_at = ? WHERE id = ? AND task_id = ? AND ownership_generation = ?
		AND retired_at IS NULL AND status = ? AND COALESCE(materialization_session_id, '') = ''`),
		now, now, current.ID, current.TaskID, current.OwnershipGeneration, string(models.TaskEnvironmentStatusFailed))
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

func (r *Repository) failedEnvironmentHasConsumersTx(ctx context.Context, tx *sqlx.Tx, id string) (bool, error) {
	var live bool
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (
		SELECT 1 FROM task_sessions WHERE task_environment_id = ? AND state NOT IN ('COMPLETED', 'FAILED', 'CANCELLED')
	) OR EXISTS (
		SELECT 1 FROM executors_running er JOIN task_sessions ts ON ts.id = er.session_id
		WHERE ts.task_environment_id = ? AND (er.status IS NULL OR er.status NOT IN (?, ?, ?))
	)`), id, id, models.ExecutorRunningStatusFailed, models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete).Scan(&live)
	return live, err
}

func (r *Repository) bindFailedWorkspaceEnvironmentTx(ctx context.Context, tx *sqlx.Tx, id, sessionID string, candidate *models.TaskEnvironment) (string, error) {
	err := r.reelectFailedWorkspaceMaterializationTx(ctx, tx, id, sessionID)
	if !errors.Is(err, models.ErrTaskEnvironmentResetRequired) {
		return id, err
	}
	expected, loadErr := r.replacementEnvironmentIdentityTx(ctx, tx, id)
	if loadErr != nil {
		return "", loadErr
	}
	if expected.TaskID != candidate.TaskID {
		return "", models.ErrWorkspaceReuseUnsafe
	}
	retired, retireErr := r.retireFailedEnvironmentTx(ctx, tx, expected)
	if retireErr != nil {
		return "", retireErr
	}
	if !retired {
		return "", err
	}
	if candidate.TaskDirName == "" {
		candidate.TaskDirName = expected.TaskDirName
	}
	if reserveErr := r.reserveFreshEnvironmentTx(ctx, tx, candidate, sessionID); reserveErr != nil {
		return "", reserveErr
	}
	return candidate.ID, nil
}

// Keep ordinary first creation's naming path, but reserve a fresh root when
// retained history exists and no active owner remains.
func (r *Repository) reserveInitialWorkspaceEnvironmentTx(ctx context.Context, tx *sqlx.Tx, candidate *models.TaskEnvironment, sessionID string) error {
	var retained bool
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (
		SELECT 1 FROM task_environments WHERE task_id = ? AND retired_at IS NOT NULL
	)`), candidate.TaskID).Scan(&retained); err != nil {
		return err
	}
	if retained {
		return r.reserveFreshEnvironmentTx(ctx, tx, candidate, sessionID)
	}
	if candidate.ID == "" {
		candidate.ID = uuid.New().String()
	}
	candidate.Status = models.TaskEnvironmentStatusCreating
	candidate.MaterializationSessionID = sessionID
	candidate.CreatedAt = r.nowUTC()
	candidate.UpdatedAt = candidate.CreatedAt
	return r.insertCreatingWorkspaceEnvironment(ctx, tx, candidate)
}

// A reservation chooses its root while holding the task writer lock, against
// every recorded root, including retired rows and prior retries of this session.
func (r *Repository) reserveFreshEnvironmentTx(ctx context.Context, tx *sqlx.Tx, candidate *models.TaskEnvironment, sessionID string) error {
	if err := recoveryclaim.EnsureTaskAvailableTx(ctx, r.db, tx, candidate.TaskID); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, r.db.Rebind(`SELECT COALESCE(task_dir_name, '') FROM task_environments`))
	if err != nil {
		return err
	}
	names := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		names[strings.ToLower(name)] = true
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return err
	}
	base := candidate.TaskDirName
	if base == "" {
		base = "task-" + candidate.TaskID
	}
	name := base
	for names[strings.ToLower(name)] {
		name = base + "_" + uuid.NewString()[:8]
	}
	candidate.ID = uuid.NewString()
	candidate.OwnershipGeneration = 1
	candidate.Status = models.TaskEnvironmentStatusCreating
	candidate.MaterializationSessionID = sessionID
	candidate.TaskDirName = name
	candidate.WorkspacePath = ""
	candidate.CreatedAt = r.nowUTC()
	candidate.UpdatedAt = candidate.CreatedAt
	return r.insertCreatingWorkspaceEnvironment(ctx, tx, candidate)
}

// ReserveTaskEnvironmentReplacement atomically retires the selected owner,
// reserves a fresh materializer, and binds the persisted requesting session.
// An already-retired binding joins the active reservation instead of inserting
// another canonical owner. Only terminal sessions may replace their binding.
func (r *Repository) ReserveTaskEnvironmentReplacement(
	ctx context.Context, expected *models.TaskEnvironment, session *models.TaskSession, candidate *models.TaskEnvironment,
) (*models.TaskEnvironment, error) {
	if expected == nil || session == nil || candidate == nil || expected.TaskID != session.TaskID || candidate.TaskID != session.TaskID {
		return nil, models.ErrWorkspaceReuseUnsafe
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.taskCleanupBarrierLocked(ctx, tx, session.TaskID); err != nil {
		return nil, err
	}
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, expected.ID); err != nil {
		return nil, err
	}
	if err := r.validateReplacementSessionTx(ctx, tx, expected, session); err != nil {
		return nil, err
	}
	selected, err := r.electReplacementEnvironmentTx(ctx, tx, expected, candidate, session.ID)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET task_environment_id = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND COALESCE(task_environment_id, '') = ? AND state = ?`),
		selected.ID, r.nowUTC(), session.ID, session.TaskID, session.TaskEnvironmentID, string(session.State))
	if err != nil {
		return nil, err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return nil, fmt.Errorf("replacement session binding changed: %w", models.ErrWorkspaceReuseUnsafe)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetTaskEnvironment(ctx, selected.ID)
}

func (r *Repository) validateReplacementSessionTx(ctx context.Context, tx *sqlx.Tx, expected *models.TaskEnvironment, session *models.TaskSession) error {
	current, err := r.replacementEnvironmentIdentityTx(ctx, tx, expected.ID)
	if err != nil {
		return err
	}
	if current.TaskID != expected.TaskID || current.OwnershipGeneration != expected.OwnershipGeneration {
		return ErrTaskEnvironmentOwnershipChanged
	}
	var state, binding string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT state, COALESCE(task_environment_id, '') FROM task_sessions WHERE id = ? AND task_id = ?`),
		session.ID, session.TaskID).Scan(&state, &binding)
	if err != nil {
		return err
	}
	if state != string(session.State) || binding != session.TaskEnvironmentID {
		return models.ErrWorkspaceReuseUnsafe
	}
	switch models.TaskSessionState(state) {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return nil
	default:
		return models.ErrWorkspaceReuseUnsafe
	}
}

func (r *Repository) electReplacementEnvironmentTx(ctx context.Context, tx *sqlx.Tx, expected, candidate *models.TaskEnvironment, sessionID string) (*models.TaskEnvironment, error) {
	current, err := r.replacementEnvironmentIdentityTx(ctx, tx, expected.ID)
	if err != nil {
		return nil, err
	}
	if !current.IsRetired() {
		retired, err := r.retireFailedEnvironmentTx(ctx, tx, expected)
		if err != nil {
			return nil, err
		}
		if !retired {
			return nil, models.ErrTaskEnvironmentResetRequired
		}
	}
	var activeID string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM task_environments WHERE task_id = ? AND retired_at IS NULL`), expected.TaskID).Scan(&activeID)
	if errors.Is(err, sql.ErrNoRows) {
		if candidate.TaskDirName == "" {
			candidate.TaskDirName = current.TaskDirName
		}
		if err := r.reserveFreshEnvironmentTx(ctx, tx, candidate, sessionID); err != nil {
			return nil, err
		}
		return candidate, nil
	}
	if err != nil {
		return nil, err
	}
	active, err := r.replacementEnvironmentIdentityTx(ctx, tx, activeID)
	if err != nil {
		return nil, err
	}
	return r.adoptReplacementReservationTx(ctx, tx, active, candidate, sessionID)
}

func (r *Repository) adoptReplacementReservationTx(ctx context.Context, tx *sqlx.Tx, active, candidate *models.TaskEnvironment, sessionID string) (*models.TaskEnvironment, error) {
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, active.ID); err != nil {
		return nil, err
	}
	switch active.Status {
	case models.TaskEnvironmentStatusReady, models.TaskEnvironmentStatusStopped:
		return active, nil
	case models.TaskEnvironmentStatusCreating:
		if active.MaterializationSessionID == sessionID {
			return active, nil
		}
	case models.TaskEnvironmentStatusFailed:
		selectedID, err := r.bindFailedWorkspaceEnvironmentTx(ctx, tx, active.ID, sessionID, candidate)
		if err != nil {
			return nil, err
		}
		return r.replacementEnvironmentIdentityTx(ctx, tx, selectedID)
	}
	return nil, &models.WorkspacePreparingError{EnvironmentID: active.ID, Reason: "retry after replacement materialization completes"}
}
