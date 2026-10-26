package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// PublishRecoveredNomadEnvironmentReadyParams identifies the recovered Nomad
// operation whose environment is published ready. The operation identity is
// the execution ID; the published path is the retained inventory's workspace
// path, never a caller input.
type PublishRecoveredNomadEnvironmentReadyParams struct {
	SessionID           string
	EnvironmentID       string
	ExecutionID         string
	OwnershipGeneration int64
	// ExpectedRevision is the inventory revision the caller read immediately
	// before publishing.
	ExpectedRevision uint64
	// Repos is the canonical repository inventory projected from the prepared
	// continuation. It replaces the environment's existing repository rows.
	Repos []*models.TaskEnvironmentRepo
}

// nomadPublishSession is the locked session row a publish validates.
type nomadPublishSession struct {
	taskID        string
	environmentID string
	state         models.TaskSessionState
}

// nomadPublishEnvironment is the locked environment row a publish validates
// and then compare-and-sets.
type nomadPublishEnvironment struct {
	taskID       string
	generation   int64
	executorType string
	status       models.TaskEnvironmentStatus
	materializer string
	path         string
	updatedAt    time.Time
}

// PublishRecoveredNomadEnvironmentReady publishes the creating environment of
// a recovered Created session as ready, with the retained inventory's
// workspace path and the supplied repository inventory, in one transaction. It
// reports false without writing when the environment is already published for
// the same operation.
func (r *Repository) PublishRecoveredNomadEnvironmentReady(
	ctx context.Context,
	params PublishRecoveredNomadEnvironmentReadyParams,
) (bool, error) {
	if params.SessionID == "" || params.EnvironmentID == "" || params.ExecutionID == "" {
		return false, fmt.Errorf("publish recovered nomad environment: session, environment and execution are required: %w", models.ErrExecutionRotated)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	published, err := r.publishRecoveredNomadEnvironmentReadyTx(ctx, tx, params)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return published, nil
}

func (r *Repository) publishRecoveredNomadEnvironmentReadyTx(
	ctx context.Context,
	tx *sqlx.Tx,
	params PublishRecoveredNomadEnvironmentReadyParams,
) (bool, error) {
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, params.EnvironmentID); err != nil {
		return false, err
	}
	var taskID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_environments WHERE id = ?`), params.EnvironmentID).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, models.ErrExecutionRotated
	}
	if err != nil {
		return false, fmt.Errorf("resolve recovered nomad environment owner: %w", err)
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return false, err
	}
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, params.EnvironmentID); err != nil {
		return false, err
	}
	session, err := r.lockNomadPublishSession(ctx, tx, params.SessionID)
	if err != nil {
		return false, err
	}
	env, err := r.lockNomadPublishEnvironment(ctx, tx, params.EnvironmentID)
	if err != nil {
		return false, err
	}
	if session.taskID != env.taskID || session.environmentID != params.EnvironmentID {
		return false, models.ErrExecutionRotated
	}
	if session.state != models.TaskSessionStateCreated {
		return false, fmt.Errorf("%w: recovered session is not Created", models.ErrWorkspaceReuseUnsafe)
	}
	state, err := r.lockNomadPublishInventory(ctx, tx, params)
	if err != nil {
		return false, err
	}
	return r.publishNomadEnvironment(ctx, tx, params, env, state)
}

func (r *Repository) lockNomadPublishSession(ctx context.Context, tx *sqlx.Tx, sessionID string) (nomadPublishSession, error) {
	found, err := lockTaskSessionRow(ctx, tx, sessionID)
	if err != nil {
		return nomadPublishSession{}, fmt.Errorf("lock recovered session: %w", err)
	}
	if !found {
		return nomadPublishSession{}, models.ErrExecutionRotated
	}
	var session nomadPublishSession
	var state string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, COALESCE(task_environment_id, ''), state FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&session.taskID, &session.environmentID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return nomadPublishSession{}, models.ErrExecutionRotated
	}
	if err != nil {
		return nomadPublishSession{}, fmt.Errorf("load recovered session: %w", err)
	}
	session.state = models.TaskSessionState(state)
	return session, nil
}

func (r *Repository) lockNomadPublishEnvironment(ctx context.Context, tx *sqlx.Tx, environmentID string) (nomadPublishEnvironment, error) {
	query := `
		SELECT task_id, ownership_generation, COALESCE(executor_type, ''), COALESCE(status, ''),
		       COALESCE(materialization_session_id, ''), COALESCE(workspace_path, ''), updated_at
		FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var env nomadPublishEnvironment
	var status string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), environmentID).Scan(
		&env.taskID, &env.generation, &env.executorType, &status, &env.materializer, &env.path, &env.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nomadPublishEnvironment{}, models.ErrExecutionRotated
	}
	if err != nil {
		return nomadPublishEnvironment{}, fmt.Errorf("load recovered nomad environment: %w", err)
	}
	env.status = models.TaskEnvironmentStatus(status)
	return env, nil
}

// lockNomadPublishInventory locks the session's executors_running row and
// returns its decoded state once it is the ready inventory of exactly the
// operation in params. Checks run in the order identity, phase, ready
// evidence, revision.
func (r *Repository) lockNomadPublishInventory(
	ctx context.Context,
	tx *sqlx.Tx,
	params PublishRecoveredNomadEnvironmentReadyParams,
) (nomad.EnvironmentState, error) {
	query := `SELECT agent_execution_id, metadata FROM executors_running WHERE session_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var executionID, metadataJSON string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), params.SessionID).Scan(&executionID, &metadataJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nomad.EnvironmentState{}, models.ErrExecutionRotated
	}
	if err != nil {
		return nomad.EnvironmentState{}, fmt.Errorf("load recovered nomad inventory: %w", err)
	}
	if executionID != params.ExecutionID {
		return nomad.EnvironmentState{}, models.ErrExecutionRotated
	}
	metadata := map[string]interface{}{}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return nomad.EnvironmentState{}, fmt.Errorf("decode recovered nomad inventory: %w", models.ErrExecutionRotated)
	}
	state, err := nomad.EnvironmentStateFromMetadata(metadata)
	if err != nil {
		return nomad.EnvironmentState{}, fmt.Errorf("read recovered nomad inventory: %w", models.ErrExecutionRotated)
	}
	return state, validateNomadPublishState(state, params)
}

func validateNomadPublishState(state nomad.EnvironmentState, params PublishRecoveredNomadEnvironmentReadyParams) error {
	if state.OperationID != params.ExecutionID || state.ExecutionID != params.ExecutionID ||
		state.EnvironmentID != params.EnvironmentID || state.Generation != params.OwnershipGeneration {
		return models.ErrExecutionRotated
	}
	switch state.Phase {
	case nomadPhaseReady:
	case nomadPhaseIntent, nomadPhaseRegistered, nomadPhaseAllocated:
		return models.ErrNomadRecoveryNotReady
	default:
		return models.ErrExecutionRotated
	}
	if state.WorkspacePath == "" || state.AllocationID == "" || state.ServerIdentity == "" {
		return models.ErrExecutionRotated
	}
	if state.Revision != params.ExpectedRevision {
		return models.ErrExecutionRotated
	}
	return nil
}

// publishNomadEnvironment applies the environment-state branch of a publish
// under the locks and checks already taken.
func (r *Repository) publishNomadEnvironment(
	ctx context.Context,
	tx *sqlx.Tx,
	params PublishRecoveredNomadEnvironmentReadyParams,
	env nomadPublishEnvironment,
	state nomad.EnvironmentState,
) (bool, error) {
	if env.generation != state.Generation {
		return false, models.ErrExecutionRotated
	}
	if env.executorType != string(models.ExecutorTypeNomad) {
		return false, fmt.Errorf("%w: environment is not a Nomad environment", models.ErrWorkspaceReuseUnsafe)
	}
	switch env.status {
	case models.TaskEnvironmentStatusReady:
		if env.materializer != "" || env.path != state.WorkspacePath {
			return false, fmt.Errorf("%w: ready environment does not match the recovered operation", models.ErrWorkspaceReuseUnsafe)
		}
		return false, nil
	case models.TaskEnvironmentStatusCreating:
		if env.materializer != params.SessionID || (env.path != "" && env.path != state.WorkspacePath) {
			return false, fmt.Errorf("%w: creating environment does not belong to the recovered operation", models.ErrWorkspaceReuseUnsafe)
		}
	default:
		return false, fmt.Errorf("%w: environment status %q is not publishable", models.ErrWorkspaceReuseUnsafe, env.status)
	}
	if err := r.requireNomadPublishRepos(ctx, tx, env.taskID, params.Repos); err != nil {
		return false, err
	}
	if err := r.replaceTaskEnvironmentReposTx(ctx, tx, params.EnvironmentID, params.Repos); err != nil {
		return false, err
	}
	return true, r.casNomadEnvironmentReady(ctx, tx, params, env, state.WorkspacePath)
}

func (r *Repository) requireNomadPublishRepos(ctx context.Context, tx *sqlx.Tx, taskID string, repos []*models.TaskEnvironmentRepo) error {
	if len(repos) > 0 {
		return nil
	}
	hasRepos, err := r.taskHasRepositoriesTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if hasRepos {
		return fmt.Errorf("%w: repository-backed task has no repository inventory to publish", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func (r *Repository) casNomadEnvironmentReady(
	ctx context.Context,
	tx *sqlx.Tx,
	params PublishRecoveredNomadEnvironmentReadyParams,
	env nomadPublishEnvironment,
	workspacePath string,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments
		SET status = ?, materialization_session_id = '', workspace_path = ?, updated_at = ?
		WHERE id = ? AND status = ? AND COALESCE(materialization_session_id, '') = ?
		  AND COALESCE(workspace_path, '') = ? AND ownership_generation = ? AND updated_at = ?
	`), string(models.TaskEnvironmentStatusReady), workspacePath, time.Now().UTC(),
		params.EnvironmentID, string(models.TaskEnvironmentStatusCreating), params.SessionID,
		env.path, env.generation, env.updatedAt)
	if err != nil {
		return fmt.Errorf("publish recovered nomad environment: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return models.ErrExecutionRotated
	}
	return nil
}
