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
	"github.com/kandev/kandev/internal/agentruntime"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func (r *Repository) CheckpointNomadExecutorInventory(ctx context.Context, running *models.ExecutorRunning) error {
	if running == nil || running.SessionID == "" || running.TaskID == "" || running.AgentExecutionID == "" {
		return errors.New("nomad executor inventory identity is incomplete")
	}
	if running.Runtime != agentruntime.RuntimeNomad {
		return errors.New("nomad executor inventory runtime mismatch")
	}
	state, err := nomad.EnvironmentStateFromMetadata(running.Metadata)
	if err != nil {
		return fmt.Errorf("read Nomad executor state: %w", err)
	}
	if state.ExecutionID != running.AgentExecutionID {
		return models.ErrExecutionRotated
	}
	now := time.Now().UTC()
	if running.ID == "" {
		running.ID = running.SessionID
	}
	if running.CreatedAt.IsZero() {
		running.CreatedAt = now
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateNomadExecutorCheckpointOwner(ctx, tx, running, state); err != nil {
		return err
	}
	if err := r.checkpointNomadExecutorRow(ctx, tx, running, state, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	running.UpdatedAt = now
	if updated, err := nomad.EnvironmentStateFromMetadata(running.Metadata); err == nil {
		running.ExpectedNomadExecutorRevision = updated.Revision
	}
	return nil
}

func (r *Repository) validateNomadExecutorCheckpointOwner(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, state nomad.EnvironmentState) error {
	if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), running.TaskID); err != nil && !errors.Is(err, kandevdb.ErrTaskRowNotFound) {
		return err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), running.SessionID); err != nil {
		return err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{running.SessionID}, nil); err != nil {
		return err
	}
	if _, err := lockTaskSessionRow(ctx, tx, running.SessionID); err != nil {
		return err
	}
	var taskID, environmentID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, COALESCE(task_environment_id, '') FROM task_sessions WHERE id = ?`), running.SessionID).Scan(&taskID, &environmentID); err != nil {
		return err
	}
	if taskID != running.TaskID || environmentID == "" || environmentID != state.EnvironmentID {
		return models.ErrExecutionRotated
	}
	environmentQuery := `SELECT task_id, ownership_generation, executor_type FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		environmentQuery += forUpdateClause
	}
	var environmentTaskID, executorType string
	var generation int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(environmentQuery), environmentID).Scan(&environmentTaskID, &generation, &executorType); err != nil {
		return err
	}
	if environmentTaskID != taskID || generation != state.Generation || executorType != string(models.ExecutorTypeNomad) {
		return models.ErrExecutionRotated
	}
	return recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, environmentID)
}

const (
	nomadPhaseIntent           = "intent"
	nomadPhaseRegistered       = "registered"
	nomadPhaseAllocated        = "allocated"
	nomadPhaseReady            = "ready"
	nomadPhaseCleanupPending   = "cleanup_pending"
	nomadPhaseQuarantined      = "quarantined"
	nomadPhaseAbsent           = "absent"
	nomadPhaseOperatorReleased = nomad.PhaseOperatorReleased
)

func (r *Repository) checkpointNomadExecutorRow(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, incoming nomad.EnvironmentState, now time.Time) error {
	query := `SELECT agent_execution_id, metadata, updated_at FROM executors_running WHERE session_id = ?`
	if dialect.IsPostgres(tx.DriverName()) {
		query += forUpdateClause
	}
	var currentExecution, metadataJSON string
	var updatedAt time.Time
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), running.SessionID).Scan(&currentExecution, &metadataJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return r.insertNomadExecutorRow(ctx, tx, running, incoming, now)
	}
	if err != nil {
		return err
	}
	return r.updateNomadExecutorRow(ctx, tx, running, incoming, now, currentExecution, metadataJSON, updatedAt)
}

func (r *Repository) insertNomadExecutorRow(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, incoming nomad.EnvironmentState, now time.Time) error {
	if running.ExpectedNomadExecutorRevision != 0 || incoming.Phase != nomadPhaseIntent {
		return models.ErrExecutionRotated
	}
	incoming.Revision = 1
	metadata, err := mergeNomadMetadata(running.Metadata, running.Metadata, incoming)
	if err != nil {
		return err
	}
	running.Metadata = metadata
	running.UpdatedAt = now
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode Nomad executor inventory metadata: %w", err)
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO executors_running (
			id, session_id, task_id, execution_profile_id, executor_id, runtime, status, resumable, resume_token,
			last_message_uuid, agent_execution_id, container_id, agentctl_url, agentctl_port, pid, local_pid,
			worktree_id, worktree_path, worktree_branch, last_seen_at, error_message, metadata, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), running.ID, running.SessionID, running.TaskID, running.ExecutionProfileID, running.ExecutorID,
		running.Runtime, running.Status, dialect.BoolToInt(running.Resumable), running.ResumeToken,
		running.LastMessageUUID, running.AgentExecutionID, running.ContainerID, running.AgentctlURL,
		running.AgentctlPort, running.PID, running.LocalPID, running.WorktreeID, running.WorktreePath,
		running.WorktreeBranch, running.LastSeenAt, running.ErrorMessage, string(encoded), running.CreatedAt, now)
	return err
}

func nomadCheckpointOwnerMatches(current, incoming nomad.EnvironmentState, running *models.ExecutorRunning, currentExecution string) bool {
	return currentExecution == running.AgentExecutionID && current.Revision == running.ExpectedNomadExecutorRevision &&
		current.Generation == incoming.Generation && current.OperationID == incoming.OperationID &&
		current.InputDigest == incoming.InputDigest && current.JobID == incoming.JobID
}

func (r *Repository) updateNomadExecutorRow(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, incoming nomad.EnvironmentState, now time.Time, currentExecution, metadataJSON string, updatedAt time.Time) error {
	var currentMetadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataJSON), &currentMetadata); err != nil {
		return fmt.Errorf("decode current Nomad inventory metadata: %w", err)
	}
	current, err := nomad.EnvironmentStateFromMetadata(currentMetadata)
	if err != nil {
		return err
	}
	if !nomadCheckpointOwnerMatches(current, incoming, running, currentExecution) {
		return models.ErrExecutionRotated
	}
	if !validNomadStateTransition(current.Phase, incoming.Phase) {
		return fmt.Errorf("nomad inventory phase transition is invalid: %w", models.ErrExecutionRotated)
	}
	incoming.Revision = current.Revision + 1
	merged, err := mergeNomadMetadata(currentMetadata, running.Metadata, incoming)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("encode Nomad executor inventory metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE executors_running SET metadata = ?, updated_at = ? WHERE session_id = ? AND agent_execution_id = ? AND updated_at = ?`), string(encoded), now, running.SessionID, running.AgentExecutionID, updatedAt)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return models.ErrExecutionRotated
	}
	running.Metadata = merged
	running.UpdatedAt = now
	return nil
}

var nomadImmutableCheckpointMetadataKeys = []string{
	"env_secret_id_AGENTCTL_BOOTSTRAP_NONCE", "env_secret_id_AGENTCTL_AUTH_TOKEN",
	"ssh_runtime_api_local_url", "ssh_runtime_api_remote_port",
}

func mergeNomadMetadata(current, incomingMetadata map[string]interface{}, state nomad.EnvironmentState) (map[string]interface{}, error) {
	merged := make(map[string]interface{}, len(current)+3)
	for key, value := range current {
		merged[key] = value
	}
	// Credential values never belong in executor inventory. The two secret
	// references and the reverse Kandev API tunnel identity (original URL and
	// remote port, needed to reopen the same listener on recovery) are the only
	// metadata accepted from a Nomad checkpoint update; an operation cannot
	// silently rotate any of them.
	for _, key := range nomadImmutableCheckpointMetadataKeys {
		next, _ := incomingMetadata[key].(string)
		if next == "" {
			continue
		}
		prior, _ := merged[key].(string)
		if prior != "" && prior != next {
			return nil, models.ErrExecutionRotated
		}
		merged[key] = next
	}
	if err := nomad.PutEnvironmentState(merged, state); err != nil {
		return nil, err
	}
	return merged, nil
}

func validNomadStateTransition(from, to string) bool {
	allowed := map[string]map[string]bool{
		nomadPhaseIntent:           {nomadPhaseIntent: true, nomadPhaseRegistered: true, nomadPhaseCleanupPending: true, nomadPhaseQuarantined: true},
		nomadPhaseRegistered:       {nomadPhaseRegistered: true, nomadPhaseAllocated: true, nomadPhaseCleanupPending: true, nomadPhaseQuarantined: true},
		nomadPhaseAllocated:        {nomadPhaseAllocated: true, nomadPhaseReady: true, nomadPhaseCleanupPending: true, nomadPhaseQuarantined: true},
		nomadPhaseReady:            {nomadPhaseReady: true, nomadPhaseCleanupPending: true, nomadPhaseQuarantined: true},
		nomadPhaseCleanupPending:   {nomadPhaseCleanupPending: true, nomadPhaseQuarantined: true, nomadPhaseAbsent: true, nomadPhaseOperatorReleased: true},
		nomadPhaseQuarantined:      {nomadPhaseQuarantined: true, nomadPhaseAbsent: true, nomadPhaseOperatorReleased: true},
		nomadPhaseAbsent:           {nomadPhaseAbsent: true},
		nomadPhaseOperatorReleased: {nomadPhaseOperatorReleased: true},
	}
	return allowed[from][to]
}
