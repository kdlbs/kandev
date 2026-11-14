package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

const replacementPreparingReason = "retry after replacement materialization completes"

func (r *Repository) replacementEnvironmentIdentityTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.TaskEnvironment, error) {
	env := &models.TaskEnvironment{}
	var retired sql.NullTime
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, task_id, ownership_generation, status, COALESCE(materialization_session_id, ''),
		       COALESCE(task_dir_name, ''), COALESCE(executor_type, ''), updated_at, retired_at
		FROM task_environments WHERE id = ?
	`), id).Scan(&env.ID, &env.TaskID, &env.OwnershipGeneration, &env.Status,
		&env.MaterializationSessionID, &env.TaskDirName, &env.ExecutorType, &env.UpdatedAt, &retired)
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
	if expected.ExecutorType != string(models.ExecutorTypeWorktree) || candidate.ExecutorType != string(models.ExecutorTypeWorktree) {
		return "", models.ErrTaskEnvironmentResetRequired
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
	if err := r.bindReplacementSessionTx(ctx, tx, session, selected.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	reloaded, err := r.GetTaskEnvironment(ctx, selected.ID)
	if err != nil {
		return nil, errors.Join(err, r.releaseUnpublishedReplacement(ctx, selected, session.ID))
	}
	return reloaded, nil
}

func (r *Repository) bindReplacementSessionTx(ctx context.Context, tx *sqlx.Tx, session *models.TaskSession, environmentID string) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET task_environment_id = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND COALESCE(task_environment_id, '') = ? AND state = ?`),
		environmentID, r.nowUTC(), session.ID, session.TaskID, session.TaskEnvironmentID, string(session.State))
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("replacement session binding changed: %w", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

// The committed reservation is invisible to a caller whose reload failed, so
// the caller cannot arm its own failure release. Release the reservation this
// session owns through the ordinary owner-checked failure path; a joined ready
// environment is left unchanged.
func (r *Repository) releaseUnpublishedReplacement(ctx context.Context, selected *models.TaskEnvironment, sessionID string) error {
	if selected.Status != models.TaskEnvironmentStatusCreating || selected.MaterializationSessionID != sessionID {
		return nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if _, err := r.ReleaseTaskEnvironmentReplacement(cleanupCtx, selected, sessionID); err != nil {
		return fmt.Errorf("release unpublished replacement %s: %w", selected.ID, err)
	}
	return nil
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
		return models.ErrTaskEnvironmentResetRequired
	}
}

func (r *Repository) electReplacementEnvironmentTx(ctx context.Context, tx *sqlx.Tx, expected, candidate *models.TaskEnvironment, sessionID string) (*models.TaskEnvironment, error) {
	current, err := r.replacementEnvironmentIdentityTx(ctx, tx, expected.ID)
	if err != nil {
		return nil, err
	}
	if current.ExecutorType != string(models.ExecutorTypeWorktree) || candidate.ExecutorType != string(models.ExecutorTypeWorktree) {
		return nil, models.ErrTaskEnvironmentResetRequired
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
	active, err := r.lockedActiveReplacementEnvironmentTx(ctx, tx, activeID, expected.TaskID, candidate.ExecutorType)
	if err != nil {
		return nil, err
	}
	return r.adoptReplacementReservationTx(ctx, tx, active, candidate, sessionID)
}

func (r *Repository) lockedActiveReplacementEnvironmentTx(ctx context.Context, tx *sqlx.Tx, activeID, taskID, executorType string) (*models.TaskEnvironment, error) {
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, activeID); err != nil {
		return nil, err
	}
	var currentActiveID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT id FROM task_environments WHERE task_id = ? AND retired_at IS NULL`),
		taskID).Scan(&currentActiveID); err != nil {
		return nil, err
	}
	if currentActiveID != activeID {
		return nil, ErrTaskEnvironmentOwnershipChanged
	}
	active, err := r.replacementEnvironmentIdentityTx(ctx, tx, activeID)
	if err != nil {
		return nil, err
	}
	if active.IsRetired() || active.TaskID != taskID {
		return nil, ErrTaskEnvironmentOwnershipChanged
	}
	if active.ExecutorType != executorType {
		return nil, models.ErrTaskEnvironmentResetRequired
	}
	return active, nil
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
		if err := r.guardReplacementMaterializerStartTx(ctx, tx, active); err != nil {
			return nil, err
		}
		outcome, err := r.settleWorkspaceMaterializationClaim(
			ctx, tx, active.ID, active.MaterializationSessionID, active.UpdatedAt, sessionID,
		)
		if err != nil {
			return nil, err
		}
		switch outcome {
		case materializationClaimTakenOver:
			return r.replacementEnvironmentIdentityTx(ctx, tx, active.ID)
		case materializationClaimFailed:
			selectedID, err := r.bindFailedWorkspaceEnvironmentTx(ctx, tx, active.ID, sessionID, candidate)
			if err != nil {
				return nil, err
			}
			return r.replacementEnvironmentIdentityTx(ctx, tx, selectedID)
		default:
			return nil, &models.WorkspacePreparingError{EnvironmentID: active.ID, Reason: replacementPreparingReason}
		}
	case models.TaskEnvironmentStatusFailed:
		selectedID, err := r.bindFailedWorkspaceEnvironmentTx(ctx, tx, active.ID, sessionID, candidate)
		if err != nil {
			return nil, err
		}
		return r.replacementEnvironmentIdentityTx(ctx, tx, selectedID)
	}
	return nil, &models.WorkspacePreparingError{EnvironmentID: active.ID, Reason: replacementPreparingReason}
}

// A resumed materializer can still have its previous terminal state until launch
// begins. Only a later completion or an expired launch budget abandons that claim.
func (r *Repository) guardReplacementMaterializerStartTx(ctx context.Context, tx *sqlx.Tx, active *models.TaskEnvironment) error {
	if !active.UpdatedAt.After(r.nowUTC().Add(-workspaceMaterializationAbandonAfter())) {
		return nil
	}
	var state models.TaskSessionState
	var completedAt sql.NullTime
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT state, completed_at FROM task_sessions WHERE id = ?`),
		active.MaterializationSessionID).Scan(&state, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load replacement materializer: %w", err)
	}
	if isTerminalWorkspaceMaterializerState(state) && (!completedAt.Valid || !completedAt.Time.After(active.UpdatedAt)) {
		return &models.WorkspacePreparingError{EnvironmentID: active.ID, Reason: replacementPreparingReason}
	}
	return nil
}
