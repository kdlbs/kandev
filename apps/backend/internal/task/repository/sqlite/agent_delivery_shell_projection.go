package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) persistCanonicalShellOutputTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, agent streams.AgentEvent) error {
	if agent.Type != streams.EventTypeShellOutputUpdate || agent.Role == authorKindUser || agent.ShellOutput == nil {
		return streams.ErrShellOutputBaseline
	}
	messageID, metadata, err := loadShellOutputMessageTx(ctx, tx, event.SessionID, agent.ToolCallID)
	if err != nil {
		return err
	}
	if models.ToolPayloadRemoved(metadata) {
		return nil
	}
	head, err := loadShellOutputHead(ctx, tx, messageID)
	if err != nil {
		return err
	}
	identityBytes, err := json.Marshal([]any{event.StreamID, event.SubmissionID, agent.ToolCallID, agent.ParentToolCallID, agent.PromptGeneration})
	if err != nil {
		return err
	}
	identity := string(identityBytes)
	head, err = reconcileCanonicalShellIdentityTx(ctx, tx, messageID, head, identity, agent.ShellOutput)
	if err != nil {
		return err
	}
	if head == nil && agent.ShellOutput.BaseRevision != 0 {
		return streams.ErrShellOutputBaseline
	}
	base, err := shellOutputStateTx(ctx, tx, messageID, head)
	if err != nil {
		return err
	}
	return r.persistCanonicalShellChunksTx(ctx, tx, messageID, metadata, event, agent, head, base, identity)
}

func reconcileCanonicalShellIdentityTx(ctx context.Context, tx *sqlx.Tx, messageID string, head *shellOutputHead, identity string, patch *streams.ShellOutputPatch) (*shellOutputHead, error) {
	if head == nil || head.Identity == identity {
		return head, nil
	}
	if !patch.Checkpoint || patch.BaseRevision != 0 {
		return nil, streams.ErrShellOutputBaseline
	}
	if err := RemoveMessageShellOutputTx(ctx, tx, messageID); err != nil {
		return nil, err
	}
	return nil, nil
}

func (r *Repository) persistCanonicalShellChunksTx(ctx context.Context, tx *sqlx.Tx, messageID string, metadata map[string]any, event *models.AgentDeliveryEvent, agent streams.AgentEvent, head *shellOutputHead, base *streams.ShellOutputState, identity string) error {
	next, err := streams.ApplyShellOutputPatch(base, agent.ShellOutput)
	if err != nil {
		return err
	}
	if head == nil {
		head = &shellOutputHead{Identity: identity}
		if err := saveShellOutputHead(ctx, tx, messageID, head); err != nil {
			return err
		}
	}
	head.StdoutStart, head.StdoutEnd, err = applyShellOutputChunksTx(ctx, tx, messageID, "stdout", head.StdoutStart, head.StdoutEnd, agent.ShellOutput.Stdout)
	if err != nil {
		return err
	}
	head.StderrStart, head.StderrEnd, err = applyShellOutputChunksTx(ctx, tx, messageID, "stderr", head.StderrStart, head.StderrEnd, agent.ShellOutput.Stderr)
	if err != nil {
		return err
	}
	head.Header, err = shellOutputHeader(agent.ShellOutput, next.Output)
	if err != nil {
		return err
	}
	head.Sequence = event.Sequence
	if err := saveShellOutputHead(ctx, tx, messageID, head); err != nil {
		return err
	}
	return r.updateShellOutputSummaryTx(ctx, tx, messageID, metadata, agent, next.Output)
}

func loadShellOutputMessageTx(ctx context.Context, tx *sqlx.Tx, sessionID, toolID string) (string, map[string]any, error) {
	if toolID == "" {
		return "", nil, streams.ErrShellOutputBaseline
	}
	query := fmt.Sprintf(`SELECT id,metadata FROM task_session_messages WHERE task_session_id=? AND %s=? AND type!='permission_request' ORDER BY created_at ASC LIMIT 1`, dialect.JSONExtract(tx.DriverName(), "metadata", "tool_call_id"))
	var messageID, raw string
	err := tx.QueryRowContext(ctx, tx.Rebind(query), sessionID, toolID).Scan(&messageID, &raw)
	if err != nil {
		return "", nil, err
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return "", nil, err
	}
	return messageID, metadata, nil
}

func (r *Repository) updateShellOutputSummaryTx(ctx context.Context, tx *sqlx.Tx, messageID string, metadata map[string]any, agent streams.AgentEvent, output streams.ShellExecOutput) error {
	if agent.NormalizedPayload != nil {
		metadata["normalized"] = agent.NormalizedPayload
	}
	full, ok := models.RehydrateShellOutput(metadata, models.ShellExecOutputSnapshot{Stdout: output.Stdout, Stderr: output.Stderr, ExitCode: output.ExitCode, Truncated: output.Truncated})
	if !ok {
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(encoded, &metadata); err != nil {
			return err
		}
		full, ok = models.RehydrateShellOutput(metadata, models.ShellExecOutputSnapshot{Stdout: output.Stdout, Stderr: output.Stderr, ExitCode: output.ExitCode, Truncated: output.Truncated})
	}
	if !ok {
		return streams.ErrShellOutputBaseline
	}
	projected := models.ProjectMessageMetadata(full)
	projected[shellOutputStorageKey] = 1
	if agent.ToolStatus != "" {
		projected["status"] = agent.ToolStatus
	}
	if agent.ToolTitle != "" {
		projected["title"] = agent.ToolTitle
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_messages SET metadata=?,payload_digest='',payload_size=0,content=CASE WHEN ?='' THEN content ELSE ? END,updated_at=? WHERE id=?`), string(encoded), agent.ToolTitle, agent.ToolTitle, r.nowUTC(), messageID)
	return err
}

func (r *Repository) rehydrateShellOutputChunks(ctx context.Context, message *models.Message) error {
	tx, err := r.ro.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var raw string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT metadata,updated_at FROM task_session_messages WHERE id=? AND task_session_id=?`), message.ID, message.TaskSessionID).Scan(&raw, &message.UpdatedAt); err != nil {
		return err
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return err
	}
	if models.ToolPayloadRemoved(metadata) {
		return fmt.Errorf("tool_payload_removed")
	}
	head, err := loadShellOutputHead(ctx, tx, message.ID)
	if err != nil {
		return err
	}
	if head == nil {
		return streams.ErrShellOutputBaseline
	}
	state, err := shellOutputStateTx(ctx, tx, message.ID, head)
	if err != nil {
		return err
	}
	var ok bool
	message.Metadata, ok = models.RehydrateShellOutput(metadata, models.ShellExecOutputSnapshot{Stdout: state.Output.Stdout, Stderr: state.Output.Stderr, ExitCode: state.Output.ExitCode, Truncated: state.Output.Truncated})
	if !ok {
		return streams.ErrShellOutputBaseline
	}
	return tx.Commit()
}
