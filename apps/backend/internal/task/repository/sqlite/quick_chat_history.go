package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const quickChatHistoryClaudeAgent = "claude-acp"

// retainQuickChatHistoryTx shares the executor write's ownership locks and
// transaction. A rejected token CAS rolls back its history writes as well.
func (r *Repository) retainQuickChatHistoryTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	record, metadata, err := r.readQuickChatHistoryTx(ctx, tx, sessionID)
	if err != nil || record == nil {
		return err
	}
	var records []models.QuickChatAgentHistory
	if raw := metadata[models.SessionMetaKeyQuickChatAgentHistory]; len(raw) != 0 {
		if err := json.Unmarshal(raw, &records); err != nil {
			return fmt.Errorf("decode quick-chat history identities: %w", err)
		}
	}
	for _, existing := range records {
		if existing == *record {
			return nil
		}
	}
	payload, err := json.Marshal(append(records, *record))
	if err != nil {
		return err
	}
	return r.writeQuickChatHistoryTx(ctx, tx, sessionID, string(payload))
}

// Recording retained identities must not renew an idle chat's activity clock.
func (r *Repository) writeQuickChatHistoryTx(ctx context.Context, tx *sqlx.Tx, sessionID, payload string) error {
	key := models.SessionMetaKeyQuickChatAgentHistory
	var query string
	if dialect.IsPostgres(r.db.DriverName()) {
		query = "UPDATE task_sessions SET metadata = jsonb_set(" + postgresMetadataObject + ", ARRAY[?]::text[], ?::jsonb, true)::text WHERE id = ?"
	} else {
		query = "UPDATE task_sessions SET metadata = json_set(" + sqliteMetadataObject + ", ?, json(?)) WHERE id = ?"
		key = jsonPath(key)
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(query), key, payload, sessionID)
	return err
}

func (r *Repository) readQuickChatHistoryTx(
	ctx context.Context, tx *sqlx.Tx, sessionID string,
) (*models.QuickChatAgentHistory, map[string]json.RawMessage, error) {
	var task models.Task
	var record models.QuickChatAgentHistory
	var taskMetadata, snapshot, sessionMetadata, sessionProfile, executorProfile, executorTask string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT s.task_id, t.is_ephemeral, COALESCE(t.workflow_id, ''), COALESCE(t.origin, ''),
		       COALESCE(NULLIF(t.metadata, ''), '{}'), COALESCE(NULLIF(s.agent_profile_snapshot, ''), '{}'),
		       COALESCE(NULLIF(s.metadata, ''), '{}'), COALESCE(NULLIF(s.execution_profile_id, ''), s.agent_profile_id, ''),
		       e.task_id, COALESCE(e.execution_profile_id, ''), COALESCE(e.runtime, ''),
		       COALESCE(e.resume_token, ''), COALESCE(e.worktree_path, '')
		FROM task_sessions s JOIN tasks t ON t.id = s.task_id
		JOIN executors_running e ON e.session_id = s.id
		WHERE s.id = ?
	`), sessionID).Scan(&task.ID, &task.IsEphemeral, &task.WorkflowID, &task.Origin,
		&taskMetadata, &snapshot, &sessionMetadata, &sessionProfile, &executorTask, &executorProfile,
		&record.Runtime, &record.ResumeToken, &record.WorkDir)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if record.Runtime != agentruntime.RuntimeStandalone || record.ResumeToken == "" || executorTask != task.ID ||
		(sessionProfile != "" && executorProfile != "" && sessionProfile != executorProfile) {
		return nil, nil, nil
	}
	return parseQuickChatHistoryRecord(&task, &record, taskMetadata, snapshot, sessionMetadata)
}

func parseQuickChatHistoryRecord(
	task *models.Task, record *models.QuickChatAgentHistory, taskMetadata, snapshot, sessionMetadata string,
) (*models.QuickChatAgentHistory, map[string]json.RawMessage, error) {
	if err := json.Unmarshal([]byte(taskMetadata), &task.Metadata); err != nil {
		return nil, nil, err
	}
	if !models.IsRestorableQuickChatTask(task) {
		return nil, nil, nil
	}
	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(snapshot), &profile); err != nil {
		return nil, nil, err
	}
	record.AgentName = models.StringFromAny(profile["agent_name"])
	switch record.AgentName {
	case quickChatHistoryClaudeAgent, "codex-acp", "codex-app-server":
	default:
		return nil, nil, nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(sessionMetadata), &metadata); err != nil {
		return nil, nil, err
	}
	return record, metadata, nil
}
