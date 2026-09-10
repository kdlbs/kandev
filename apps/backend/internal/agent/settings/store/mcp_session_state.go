package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

var _ mcpconfig.SessionMCPSelectionStateRepository = (*sqliteRepository)(nil)

func (r *sqliteRepository) GetMCPSelectionState(
	ctx context.Context,
	sessionID string,
) (mcpconfig.SessionMCPSelectionState, error) {
	if strings.TrimSpace(sessionID) == "" {
		return mcpconfig.SessionMCPSelectionState{}, mcpconfig.ErrMCPInvalidSelection
	}
	var state mcpconfig.SessionMCPSelectionState
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT desired_revision, applied_revision, apply_state,
			failure_code, failure_summary, attachment_attempt_id
		FROM mcp_task_session_apply_state WHERE task_session_id = ?
	`), sessionID).Scan(
		&state.DesiredRevision, &state.AppliedRevision, &state.ApplyState,
		&state.FailureCode, &state.FailureSummary, &state.AttachmentAttemptID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpconfig.SessionMCPSelectionState{}, mcpconfig.ErrMCPSelectionStateNotFound
	}
	return state, err
}

func (r *sqliteRepository) SaveMCPSelectionState(
	ctx context.Context,
	sessionID string,
	state mcpconfig.SessionMCPSelectionState,
) error {
	if strings.TrimSpace(sessionID) == "" {
		return mcpconfig.ErrMCPInvalidSelection
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO mcp_task_session_apply_state
		(task_session_id, desired_revision, applied_revision, apply_state,
		 failure_code, failure_summary, attachment_attempt_id, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_session_id) DO UPDATE SET
			desired_revision = excluded.desired_revision,
			applied_revision = excluded.applied_revision,
			apply_state = excluded.apply_state,
			failure_code = excluded.failure_code,
			failure_summary = excluded.failure_summary,
			attachment_attempt_id = excluded.attachment_attempt_id,
			updated_at = excluded.updated_at
	`), sessionID, state.DesiredRevision, state.AppliedRevision, state.ApplyState,
		state.FailureCode, state.FailureSummary, state.AttachmentAttemptID, time.Now().UTC())
	return err
}
