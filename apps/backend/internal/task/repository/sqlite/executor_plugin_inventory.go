package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// CheckpointPluginExecutorInventory inserts a provisional plugin execution or
// updates only its metadata under execution and envelope-revision CAS. It never writes
// resume-token or conversation columns on the update path.
func (r *Repository) CheckpointPluginExecutorInventory(ctx context.Context, running *models.ExecutorRunning) error {
	now := time.Now().UTC()
	metadataJSON, observedRevision, err := preparePluginExecutorInventoryCheckpoint(running, now)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validatePluginExecutorCheckpointOwner(ctx, tx, running, metadataJSON); err != nil {
		return err
	}
	if err := checkpointPluginExecutorInventoryRow(ctx, tx, r.db, running, observedRevision, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	running.UpdatedAt = now
	running.ExpectedPluginExecutorRevision, _ = pluginExecutorInventoryRevision(running.Metadata)
	return nil
}

func preparePluginExecutorInventoryCheckpoint(running *models.ExecutorRunning, now time.Time) ([]byte, uint64, error) {
	if running == nil || running.SessionID == "" || running.TaskID == "" || running.AgentExecutionID == "" {
		return nil, 0, errors.New("plugin executor inventory identity is incomplete")
	}
	if running.ID == "" {
		running.ID = running.SessionID
	}
	if running.CreatedAt.IsZero() {
		running.CreatedAt = now
	}
	running.UpdatedAt = now
	metadataJSON, err := json.Marshal(running.Metadata)
	if err != nil {
		return nil, 0, fmt.Errorf("encode plugin executor inventory metadata: %w", err)
	}
	return metadataJSON, running.ExpectedPluginExecutorRevision, nil
}

func (r *Repository) validatePluginExecutorCheckpointOwner(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, metadataJSON []byte) error {
	if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), running.TaskID); err != nil && !errors.Is(err, kandevdb.ErrTaskRowNotFound) {
		return err
	}
	if _, err := lockTaskSessionRow(ctx, tx, running.SessionID); err != nil {
		return err
	}
	var sessionTaskID string
	var environmentID sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, COALESCE(task_environment_id, '') FROM task_sessions WHERE id = ?`), running.SessionID).Scan(&sessionTaskID, &environmentID); err != nil {
		return err
	}
	if sessionTaskID != running.TaskID || !environmentID.Valid || environmentID.String == "" {
		return models.ErrExecutionRotated
	}
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, environmentID.String); err != nil {
		return err
	}
	expectedGeneration, err := pluginExecutorOwnershipGeneration(metadataJSON)
	if err != nil {
		return err
	}
	environmentQuery := `SELECT task_id, ownership_generation FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		environmentQuery += forUpdateClause
	}
	var environmentTaskID string
	var ownershipGeneration int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(environmentQuery), environmentID.String).Scan(&environmentTaskID, &ownershipGeneration); err != nil {
		return err
	}
	if environmentTaskID != running.TaskID || ownershipGeneration != expectedGeneration {
		return models.ErrExecutionRotated
	}
	return nil
}

func checkpointPluginExecutorInventoryRow(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, running *models.ExecutorRunning, observedRevision uint64, now time.Time) error {
	currentExecution, currentMetadata, currentUpdatedAt, readErr := readPluginExecutorInventoryForUpdate(ctx, tx, running.SessionID)
	switch {
	case errors.Is(readErr, sql.ErrNoRows):
		if observedRevision != 0 || pluginExecutorInventoryPhase(running.Metadata) != "allocating" {
			return models.ErrExecutionRotated
		}
		versioned, err := mergePluginExecutorInventoryMetadata(running.Metadata, running.Metadata)
		if err != nil {
			return err
		}
		versioned, err = setPluginExecutorInventoryRevision(versioned, 1)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(versioned)
		if err != nil {
			return fmt.Errorf("encode plugin executor inventory metadata: %w", err)
		}
		if err := insertPluginExecutorInventory(ctx, tx, running, string(encoded)); err != nil {
			return err
		}
		running.Metadata = versioned
		return nil
	case readErr != nil:
		return readErr
	default:
		return updatePluginExecutorInventoryRow(ctx, tx, db, running, currentExecution, currentMetadata, currentUpdatedAt, observedRevision, now)
	}
}

func updatePluginExecutorInventoryRow(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, running *models.ExecutorRunning, currentExecution string, currentMetadata map[string]interface{}, currentUpdatedAt time.Time, observedRevision uint64, now time.Time) error {
	currentRevision, err := pluginExecutorInventoryRevision(currentMetadata)
	if err != nil {
		return err
	}
	if currentExecution != running.AgentExecutionID || observedRevision != currentRevision {
		return models.ErrExecutionRotated
	}
	if !validPluginExecutorInventoryTransition(currentMetadata, running.Metadata) {
		return fmt.Errorf("plugin executor inventory phase transition is invalid: %w", models.ErrExecutionRotated)
	}
	merged, err := mergePluginExecutorInventoryMetadata(currentMetadata, running.Metadata)
	if err != nil {
		return err
	}
	merged, err = setPluginExecutorInventoryRevision(merged, currentRevision+1)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encode plugin executor inventory metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE executors_running SET metadata = ?, updated_at = ?
		WHERE session_id = ? AND agent_execution_id = ? AND updated_at = ?
	`), string(encoded), now, running.SessionID, running.AgentExecutionID, currentUpdatedAt)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return models.ErrExecutionRotated
	}
	running.Metadata = merged
	return nil
}

// DeletePluginExecutorInventoryIfCurrent removes one plugin runtime row only
// while its environment cleanup claim and both execution and owner generations
// still match. Callers must pass the claim context returned by the repository.
func (r *Repository) DeletePluginExecutorInventoryIfCurrent(
	ctx context.Context,
	sessionID, executionID string,
	ownershipGeneration int64,
) error {
	if sessionID == "" || executionID == "" || ownershipGeneration <= 0 {
		return errors.New("plugin executor inventory cleanup identity is incomplete")
	}
	claim := recoveryclaim.ClaimFromContext(ctx)
	if err := validatePluginExecutorDeleteClaim(claim, sessionID, ownershipGeneration); err != nil {
		return err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := verifyPluginExecutorDeleteTarget(ctx, tx, r.db, claim, sessionID, executionID, ownershipGeneration); err != nil {
		return err
	}
	if err := deletePluginExecutorInventoryRow(ctx, tx, r.db, sessionID, executionID); err != nil {
		return err
	}
	return tx.Commit()
}

func validatePluginExecutorDeleteClaim(claim *models.TaskEnvironmentRecoveryClaim, sessionID string, ownershipGeneration int64) error {
	if claim == nil || claim.SessionID != sessionID || claim.OwnershipGeneration != ownershipGeneration || claim.ExecutorType != string(models.ExecutorTypePluginRemote) {
		return recoveryclaim.ErrClaimMismatch
	}
	return nil
}

func verifyPluginExecutorDeleteTarget(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, claim *models.TaskEnvironmentRecoveryClaim, sessionID, executionID string, ownershipGeneration int64) error {
	if err := verifyPluginExecutorDeleteOwner(ctx, tx, db, claim, sessionID, ownershipGeneration); err != nil {
		return err
	}
	return verifyPluginExecutorDeleteExecution(ctx, tx, sessionID, executionID, ownershipGeneration)
}

func verifyPluginExecutorDeleteOwner(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, claim *models.TaskEnvironmentRecoveryClaim, sessionID string, ownershipGeneration int64) error {
	if err := kandevdb.LockTaskRowInTx(ctx, tx, db.DriverName(), claim.OwnerTaskID); err != nil {
		return err
	}
	if _, err := lockTaskSessionRow(ctx, tx, sessionID); err != nil {
		return err
	}
	var taskID, environmentID string
	if err := tx.QueryRowContext(ctx, db.Rebind(`
		SELECT task_id, COALESCE(task_environment_id, '') FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&taskID, &environmentID); err != nil {
		return err
	}
	if taskID != claim.OwnerTaskID || environmentID == "" || environmentID != claim.TaskEnvironmentID {
		return models.ErrExecutionRotated
	}
	environmentQuery := `SELECT task_id, ownership_generation FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(db.DriverName()) {
		environmentQuery += forUpdateClause
	}
	var environmentTaskID string
	var currentGeneration int64
	if err := tx.QueryRowContext(ctx, db.Rebind(environmentQuery), environmentID).Scan(&environmentTaskID, &currentGeneration); err != nil {
		return err
	}
	if environmentTaskID != taskID || currentGeneration != ownershipGeneration {
		return models.ErrExecutionRotated
	}
	if err := recoveryclaim.EnsureAvailableTx(ctx, db, tx, environmentID); err != nil {
		return err
	}
	return nil
}

func verifyPluginExecutorDeleteExecution(ctx context.Context, tx *sqlx.Tx, sessionID, executionID string, ownershipGeneration int64) error {
	currentExecution, currentMetadata, _, err := readPluginExecutorInventoryForUpdate(ctx, tx, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w for session: %s", models.ErrExecutorRunningNotFound, sessionID)
	}
	if err != nil {
		return err
	}
	if currentExecution != executionID {
		return models.ErrExecutionRotated
	}
	metadataJSON, err := json.Marshal(currentMetadata)
	if err != nil {
		return err
	}
	storedGeneration, err := pluginExecutorOwnershipGeneration(metadataJSON)
	if err != nil {
		return err
	}
	if storedGeneration != ownershipGeneration {
		return models.ErrExecutionRotated
	}
	return nil
}

func deletePluginExecutorInventoryRow(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, sessionID, executionID string) error {
	result, err := tx.ExecContext(ctx, db.Rebind(`
		DELETE FROM executors_running WHERE session_id = ? AND agent_execution_id = ?
	`), sessionID, executionID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return models.ErrExecutionRotated
	}
	return nil
}

func pluginExecutorOwnershipGeneration(metadataJSON []byte) (int64, error) {
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return 0, fmt.Errorf("decode plugin executor ownership generation: %w", err)
	}
	var envelope struct {
		EnvironmentGeneration int64 `json:"environment_generation"`
	}
	if err := json.Unmarshal(metadata["plugin_executor"], &envelope); err != nil || envelope.EnvironmentGeneration <= 0 {
		return 0, errors.New("plugin executor ownership generation is missing")
	}
	return envelope.EnvironmentGeneration, nil
}

func readPluginExecutorInventoryForUpdate(ctx context.Context, tx *sqlx.Tx, sessionID string) (string, map[string]interface{}, time.Time, error) {
	query := `SELECT agent_execution_id, metadata, updated_at FROM executors_running WHERE session_id = ?`
	if dialect.IsPostgres(tx.DriverName()) {
		query += forUpdateClause
	}
	var executionID, metadataJSON string
	var updatedAt time.Time
	if err := tx.QueryRowxContext(ctx, tx.Rebind(query), sessionID).Scan(&executionID, &metadataJSON, &updatedAt); err != nil {
		return "", nil, time.Time{}, err
	}
	metadata := make(map[string]interface{})
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return "", nil, time.Time{}, fmt.Errorf("decode existing executor inventory metadata: %w", err)
		}
	}
	return executionID, metadata, updatedAt, nil
}

func mergePluginExecutorInventoryMetadata(current, incoming map[string]interface{}) (map[string]interface{}, error) {
	merged := make(map[string]interface{}, len(current)+1)
	for key, value := range current {
		merged[key] = value
	}
	envelope, exists := incoming["plugin_executor"]
	if !exists {
		return nil, errors.New("plugin executor checkpoint is missing its inventory envelope")
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode plugin executor inventory envelope: %w", err)
	}
	var normalized map[string]interface{}
	if err := json.Unmarshal(encoded, &normalized); err != nil || normalized == nil {
		return nil, errors.New("plugin executor checkpoint envelope is invalid")
	}
	merged["plugin_executor"] = normalized
	return merged, nil
}

func validPluginExecutorInventoryTransition(current, incoming map[string]interface{}) bool {
	from, to := pluginExecutorInventoryPhase(current), pluginExecutorInventoryPhase(incoming)
	allowed := map[string]map[string]bool{
		"allocating":       {"allocating": true, "artifact_staging": true, "bootstrapping": true, "provisioned": true, "ready": true, "cleanup_pending": true, "absent": true},
		"artifact_staging": {"artifact_staging": true, "bootstrapping": true, "provisioned": true, "cleanup_pending": true, "absent": true},
		"bootstrapping":    {"bootstrapping": true, "provisioned": true, "ready": true, "cleanup_pending": true, "absent": true},
		"provisioned":      {"provisioned": true, "bootstrapping": true, "ready": true, "cleanup_pending": true, "expired": true, "absent": true},
		"ready":            {"ready": true, "cleanup_pending": true, "expired": true, "absent": true},
		"cleanup_pending":  {"cleanup_pending": true, "absent": true},
		"expired":          {"expired": true, "absent": true},
		"absent":           {"absent": true},
	}
	return allowed[from][to]
}

func pluginExecutorInventoryPhase(metadata map[string]interface{}) string {
	encoded, err := json.Marshal(metadata["plugin_executor"])
	if err != nil {
		return ""
	}
	var envelope struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return ""
	}
	return envelope.Phase
}

func pluginExecutorInventoryRevision(metadata map[string]interface{}) (uint64, error) {
	encoded, err := json.Marshal(metadata["plugin_executor"])
	if err != nil {
		return 0, fmt.Errorf("encode plugin executor inventory revision: %w", err)
	}
	var envelope struct {
		Phase    string  `json:"phase"`
		Revision *uint64 `json:"revision"`
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil || envelope.Phase == "" {
		return 0, errors.New("plugin executor inventory revision envelope is missing")
	}
	if envelope.Revision == nil {
		return 0, nil
	}
	return *envelope.Revision, nil
}

func setPluginExecutorInventoryRevision(metadata map[string]interface{}, revision uint64) (map[string]interface{}, error) {
	merged, err := mergePluginExecutorInventoryMetadata(metadata, metadata)
	if err != nil {
		return nil, err
	}
	envelope := merged["plugin_executor"].(map[string]interface{})
	envelope["revision"] = revision
	return merged, nil
}

func insertPluginExecutorInventory(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, metadataJSON string) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO executors_running (
			id, session_id, task_id, execution_profile_id, executor_id, runtime, status, resumable, resume_token,
			last_message_uuid, agent_execution_id, container_id, agentctl_url, agentctl_port, pid, local_pid,
			worktree_id, worktree_path, worktree_branch, last_seen_at, error_message, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		running.ID, running.SessionID, running.TaskID, running.ExecutionProfileID, running.ExecutorID,
		running.Runtime, running.Status, dialect.BoolToInt(running.Resumable), running.ResumeToken,
		running.LastMessageUUID, running.AgentExecutionID, running.ContainerID, running.AgentctlURL,
		running.AgentctlPort, running.PID, running.LocalPID, running.WorktreeID, running.WorktreePath,
		running.WorktreeBranch, running.LastSeenAt, running.ErrorMessage, metadataJSON, running.CreatedAt, running.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert plugin executor provisional inventory: %w", err)
	}
	return nil
}
