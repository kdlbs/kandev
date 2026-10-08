package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// RecordProviderRecovery records the actual provider load/create outcome only
// while the execution and its provider token still own this session.
func (r *Repository) RecordProviderRecovery(ctx context.Context, taskID, sessionID, executionID, token, outcome string) (*models.Message, error) {
	if outcome != "restored" && outcome != "fresh" && outcome != models.ExecutorOutcomeUnknown {
		return nil, fmt.Errorf("invalid provider recovery outcome")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err = r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return nil, err
	}
	if _, err = lockTaskSessionRow(ctx, tx, sessionID); err != nil {
		return nil, err
	}
	var current, currentToken, owner string
	if err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT r.agent_execution_id,r.resume_token,s.task_id FROM executors_running r JOIN task_sessions s ON s.id=r.session_id WHERE r.session_id=? AND r.status!='stopped'`), sessionID).Scan(&current, &currentToken, &owner); err != nil {
		return nil, err
	}
	if !providerRecoveryStillOwned(taskID, sessionID, executionID, token, owner, current, currentToken) {
		return nil, models.ErrExecutionRotated
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("provider-recovery:"+sessionID+":"+executionID+":"+token+":"+outcome)).String()
	now := time.Now().UTC()
	metadata := map[string]interface{}{"executor_recovery": true, "provider_conversation": outcome, "workspace": models.ExecutorOutcomeUnknown}
	if err = r.attachRecordedWorkspaceEvidence(ctx, tx, taskID, sessionID, metadata); err != nil {
		return nil, err
	}
	if err = r.insertExecutorStatusHistoryTx(ctx, tx, id, taskID, sessionID, "", metadata, now); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetMessage(ctx, id)
}

// Retention remains an as-of resource observation, separate from the provider
// initialization that proves conversation recovery and current availability.
func (r *Repository) attachRecordedWorkspaceEvidence(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string, metadata map[string]interface{}) error {
	episode, err := scanExecutorFailure(tx.QueryRowContext(ctx, r.db.Rebind(executorFailureSelect+` WHERE task_id=? AND ((environment_id!='' AND environment_id=(SELECT task_environment_id FROM task_sessions WHERE id=?)) OR (environment_id='' AND session_id=?)) ORDER BY last_observed_at DESC,id DESC LIMIT 1`), taskID, sessionID, sessionID))
	if err != nil {
		return err
	}
	if episode != nil && episode.Observation != nil && episode.Observation.Workspace == models.WorkspaceRetained {
		metadata["workspace"] = models.WorkspaceRetained
		metadata["workspace_observed_at"] = episode.Observation.ObservedAt.Format(time.RFC3339Nano)
	}
	return nil
}

func providerRecoveryStillOwned(taskID, sessionID, executionID, token, owner, current, currentToken string) bool {
	return sessionID != "" && owner == taskID && executionID != "" && current == executionID && token != "" && currentToken == token
}
