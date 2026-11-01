package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// pluginPublishEnvelope is the part of the stored envelope a publication
// verifies.
type pluginPublishEnvelope struct {
	OperationID           string `json:"operation_id"`
	EnvironmentID         string `json:"environment_id"`
	EnvironmentGeneration int64  `json:"environment_generation"`
	RuntimeIdentity       string `json:"runtime_identity"`
	Phase                 string `json:"phase"`
	Revision              uint64 `json:"revision"`
	Descriptor            *struct {
		Path string `json:"path"`
	} `json:"workspace_descriptor"`
	Repositories []struct {
		ID string `json:"id"`
	} `json:"repositories"`
	// WorkspacePreparation is core or provider; a missing value is core.
	WorkspacePreparation string `json:"workspace_preparation"`
}

// pluginPublishCoreWorkspacePath is the fixed workspace path of a provider
// that records no workspace descriptor.
const pluginPublishCoreWorkspacePath = "/workspace"

func publishMismatch(reason string) error { return &models.PluginPublishMismatchError{Reason: reason} }

// PublishRecoveredTaskEnvironment flips a creating plugin environment to ready
// in one transaction under the recovery claim. It verifies the operation, task
// and session, environment owner and generation, runtime identity, workspace
// descriptor and repository inventory against the stored envelope, then
// compare-and-sets the environment row and replaces its repository rows. It
// reports false without writing when the environment is already ready for the
// same evidence; any mismatch publishes nothing and names its reason.
func (r *Repository) PublishRecoveredTaskEnvironment(ctx context.Context, params models.PublishRecoveredTaskEnvironmentParams) (bool, error) {
	if params.Claim == nil || params.SessionID == "" || params.EnvironmentID == "" || params.OperationID == "" || params.RuntimeIdentity == "" {
		return false, publishMismatch(models.PluginPublishReasonOperation)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, params.EnvironmentID); err != nil {
		return false, err
	}
	if err := lockPluginInventory(ctx, tx, r.db.DriverName(), params.EnvironmentID); err != nil {
		return false, err
	}
	if err := recoveryclaim.ValidateTx(ctx, r.db, tx, params.Claim); err != nil {
		return false, mapPluginClaimError(err)
	}
	published, err := r.publishRecoveredTx(ctx, tx, params)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return published, nil
}

func (r *Repository) publishRecoveredTx(ctx context.Context, tx *sqlx.Tx, params models.PublishRecoveredTaskEnvironmentParams) (bool, error) {
	env, err := r.lockPublishEnvironment(ctx, tx, params.EnvironmentID)
	if err != nil {
		return false, err
	}
	if env.generation != params.OwnershipGeneration {
		return false, publishMismatch(models.PluginPublishReasonGeneration)
	}
	if err := r.verifyPublishSession(ctx, tx, params, env.taskID); err != nil {
		return false, err
	}
	envelope, err := r.lockPublishEnvelope(ctx, tx, params)
	if err != nil {
		return false, err
	}
	path := publishWorkspacePath(envelope, params)
	already, err := classifyPublishTarget(env, path, params.SessionID)
	if err != nil || already {
		return false, err
	}
	if params.CorePreparedWorkspace {
		err = r.requireCorePublishRepos(ctx, tx, env.taskID, params.EnvironmentID)
	} else {
		err = r.replacePublishRepos(ctx, tx, env.taskID, params)
	}
	if err != nil {
		return false, err
	}
	return true, r.casPublishReady(ctx, tx, params, env, path)
}

// publishWorkspacePath is the recorded descriptor path, or for a core-prepared
// workspace without one the host's own path.
func publishWorkspacePath(envelope pluginPublishEnvelope, params models.PublishRecoveredTaskEnvironmentParams) string {
	if envelope.Descriptor != nil && envelope.Descriptor.Path != "" {
		return envelope.Descriptor.Path
	}
	if params.CorePreparedWorkspace && params.WorkspacePath == pluginPublishCoreWorkspacePath {
		return params.WorkspacePath
	}
	return ""
}

// classifyPublishTarget reports whether the environment is already published
// with this evidence; a status or owner that fits neither case is refused.
func classifyPublishTarget(env pluginPublishEnvironment, descriptorPath, sessionID string) (bool, error) {
	switch env.status {
	case models.TaskEnvironmentStatusReady:
		if env.materializer != "" || env.path != descriptorPath {
			return false, publishMismatch(models.PluginPublishReasonStatus)
		}
		return true, nil
	case models.TaskEnvironmentStatusCreating:
		if env.materializer != sessionID || (env.path != "" && env.path != descriptorPath) {
			return false, publishMismatch(models.PluginPublishReasonSession)
		}
		return false, nil
	default:
		return false, publishMismatch(models.PluginPublishReasonStatus)
	}
}

// requireCorePublishRepos keeps the repository rows the environment already
// has. Core writes them only when it materializes the workspace, which a
// recovery never does, so a repo-backed task whose environment has none is
// refused and left to cleanup and relaunch rather than published empty.
func (r *Repository) requireCorePublishRepos(ctx context.Context, tx *sqlx.Tx, taskID, environmentID string) error {
	var existing int
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM task_environment_repos WHERE task_environment_id = ?`), environmentID).Scan(&existing); err != nil {
		return fmt.Errorf("count environment repos to publish: %w", err)
	}
	if existing > 0 {
		return nil
	}
	return r.requirePublishRepos(ctx, tx, taskID, nil)
}

func (r *Repository) replacePublishRepos(ctx context.Context, tx *sqlx.Tx, taskID string, params models.PublishRecoveredTaskEnvironmentParams) error {
	if err := r.requirePublishRepos(ctx, tx, taskID, params.Repos); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_environment_repos WHERE task_environment_id = ?`), params.EnvironmentID); err != nil {
		return err
	}
	for _, repo := range params.Repos {
		repo.TaskEnvironmentID = params.EnvironmentID
		if err := r.insertTaskEnvironmentRepoTx(ctx, tx, repo); err != nil {
			return err
		}
	}
	return nil
}

type pluginPublishEnvironment struct {
	taskID       string
	generation   int64
	status       models.TaskEnvironmentStatus
	materializer string
	path         string
	updatedAt    time.Time
}

func (r *Repository) lockPublishEnvironment(ctx context.Context, tx *sqlx.Tx, environmentID string) (pluginPublishEnvironment, error) {
	query := `
		SELECT task_id, ownership_generation, COALESCE(status, ''),
		       COALESCE(materialization_session_id, ''), COALESCE(workspace_path, ''), updated_at
		FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var env pluginPublishEnvironment
	var status string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), environmentID).Scan(&env.taskID, &env.generation, &status, &env.materializer, &env.path, &env.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return env, publishMismatch(models.PluginPublishReasonOwner)
	}
	if err != nil {
		return env, fmt.Errorf("load environment to publish: %w", err)
	}
	env.status = models.TaskEnvironmentStatus(status)
	return env, nil
}

func (r *Repository) verifyPublishSession(ctx context.Context, tx *sqlx.Tx, params models.PublishRecoveredTaskEnvironmentParams, ownerTaskID string) error {
	found, err := lockTaskSessionRow(ctx, tx, params.SessionID)
	if err != nil {
		return fmt.Errorf("lock session to publish: %w", err)
	}
	if !found {
		return publishMismatch(models.PluginPublishReasonSession)
	}
	var taskID, environmentID, state string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, COALESCE(task_environment_id, ''), state FROM task_sessions WHERE id = ?`), params.SessionID).
		Scan(&taskID, &environmentID, &state)
	if err != nil {
		return fmt.Errorf("load session to publish: %w", err)
	}
	if taskID != ownerTaskID || params.Claim.OwnerTaskID != ownerTaskID {
		return publishMismatch(models.PluginPublishReasonOwner)
	}
	if environmentID != params.EnvironmentID {
		return publishMismatch(models.PluginPublishReasonSession)
	}
	switch models.TaskSessionState(state) {
	case models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled:
		return publishMismatch(models.PluginPublishReasonSession)
	}
	return nil
}

// lockPublishEnvelope reads the session's envelope under the row lock and
// checks it in the order operation, runtime identity, phase, descriptor,
// repository inventory, revision.
func (r *Repository) lockPublishEnvelope(ctx context.Context, tx *sqlx.Tx, params models.PublishRecoveredTaskEnvironmentParams) (pluginPublishEnvelope, error) {
	query := `SELECT metadata FROM executors_running WHERE session_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var raw string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), params.SessionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return pluginPublishEnvelope{}, publishMismatch(models.PluginPublishReasonOperation)
	}
	if err != nil {
		return pluginPublishEnvelope{}, fmt.Errorf("load inventory to publish: %w", err)
	}
	metadata := map[string]json.RawMessage{}
	var envelope pluginPublishEnvelope
	if json.Unmarshal([]byte(raw), &metadata) != nil || json.Unmarshal(metadata[pluginExecutorEnvelopeKey], &envelope) != nil {
		return envelope, publishMismatch(models.PluginPublishReasonOperation)
	}
	return envelope, validatePublishEnvelope(envelope, params)
}

func validatePublishEnvelope(envelope pluginPublishEnvelope, params models.PublishRecoveredTaskEnvironmentParams) error {
	switch {
	case envelope.OperationID != params.OperationID || envelope.EnvironmentID != params.EnvironmentID:
		return publishMismatch(models.PluginPublishReasonOperation)
	case envelope.EnvironmentGeneration != params.OwnershipGeneration:
		return publishMismatch(models.PluginPublishReasonGeneration)
	case envelope.RuntimeIdentity != params.RuntimeIdentity:
		return publishMismatch(models.PluginPublishReasonRuntime)
	case envelope.Phase != pluginPhaseReady:
		return publishMismatch(models.PluginPublishReasonPhase)
	case envelope.WorkspacePreparation != "" && envelope.WorkspacePreparation != "core" && envelope.WorkspacePreparation != "provider",
		(envelope.WorkspacePreparation == "provider") == params.CorePreparedWorkspace:
		return publishMismatch(models.PluginPublishReasonWorkspace)
	case params.CorePreparedWorkspace && len(envelope.Repositories) > 0:
		return publishMismatch(models.PluginPublishReasonRepos)
	case publishWorkspacePath(envelope, params) == "":
		return publishMismatch(models.PluginPublishReasonWorkspace)
	case !params.CorePreparedWorkspace && !publishReposMatch(envelope, params.Repos):
		return publishMismatch(models.PluginPublishReasonRepos)
	case envelope.Revision != params.ExpectedRevision:
		return publishMismatch(models.PluginPublishReasonRevision)
	}
	return nil
}

// publishReposMatch compares the repository set recorded in the envelope with
// the projection. An envelope that records no inventory accepts any projection.
func publishReposMatch(envelope pluginPublishEnvelope, repos []*models.TaskEnvironmentRepo) bool {
	if len(envelope.Repositories) == 0 {
		return true
	}
	want := make([]string, 0, len(envelope.Repositories))
	for _, repo := range envelope.Repositories {
		want = append(want, repo.ID)
	}
	got := make([]string, 0, len(repos))
	for _, repo := range repos {
		if repo != nil {
			got = append(got, repo.RepositoryID)
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] {
			return false
		}
	}
	return true
}

func (r *Repository) requirePublishRepos(ctx context.Context, tx *sqlx.Tx, taskID string, repos []*models.TaskEnvironmentRepo) error {
	if len(repos) > 0 {
		return nil
	}
	hasRepos, err := r.taskHasRepositoriesTx(ctx, tx, taskID)
	if err != nil {
		return err
	}
	if hasRepos {
		return publishMismatch(models.PluginPublishReasonRepos)
	}
	return nil
}

func (r *Repository) casPublishReady(ctx context.Context, tx *sqlx.Tx, params models.PublishRecoveredTaskEnvironmentParams, env pluginPublishEnvironment, workspacePath string) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environments
		SET status = ?, materialization_session_id = '', workspace_path = ?, updated_at = ?
		WHERE id = ? AND status = ? AND COALESCE(materialization_session_id, '') = ?
		  AND COALESCE(workspace_path, '') = ? AND ownership_generation = ? AND updated_at = ?
	`), string(models.TaskEnvironmentStatusReady), workspacePath, time.Now().UTC(),
		params.EnvironmentID, string(models.TaskEnvironmentStatusCreating), params.SessionID,
		env.path, env.generation, env.updatedAt)
	if err != nil {
		return fmt.Errorf("publish recovered environment: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return publishMismatch(models.PluginPublishReasonStatus)
	}
	return nil
}
