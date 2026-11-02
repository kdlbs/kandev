package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

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
	return readMCPSelectionState(ctx, r.ro, sessionID)
}

type mcpStateReader interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
	Rebind(string) string
}

func readMCPSelectionState(ctx context.Context, reader mcpStateReader, sessionID string) (mcpconfig.SessionMCPSelectionState, error) {
	var state mcpconfig.SessionMCPSelectionState
	var setup sql.NullString
	err := reader.QueryRowxContext(ctx, reader.Rebind(`
		SELECT desired_revision, applied_revision, apply_state,
			failure_code, failure_summary, attachment_attempt_id, accepted_setup_json
		FROM mcp_task_session_apply_state WHERE task_session_id = ?
	`), sessionID).Scan(
		&state.DesiredRevision, &state.AppliedRevision, &state.ApplyState,
		&state.FailureCode, &state.FailureSummary, &state.AttachmentAttemptID, &setup,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpconfig.SessionMCPSelectionState{}, mcpconfig.ErrMCPSelectionStateNotFound
	}
	if setup.Valid {
		var decoded mcpconfig.MCPSetupSnapshot
		if json.Unmarshal([]byte(setup.String), &decoded) == nil && mcpconfig.ValidMCPSetup(&decoded) {
			state.AcceptedSetup = &decoded
		}
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
		 failure_code, failure_summary, attachment_attempt_id, updated_at, accepted_setup_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_session_id) DO UPDATE SET
			desired_revision = excluded.desired_revision,
			applied_revision = excluded.applied_revision,
			apply_state = excluded.apply_state,
			failure_code = excluded.failure_code,
			failure_summary = excluded.failure_summary,
			attachment_attempt_id = excluded.attachment_attempt_id,
			accepted_setup_json = excluded.accepted_setup_json,
			updated_at = excluded.updated_at
	`), sessionID, state.DesiredRevision, state.AppliedRevision, state.ApplyState,
		state.FailureCode, state.FailureSummary, state.AttachmentAttemptID, time.Now().UTC(), encodeMCPSetup(state.AcceptedSetup))
	return err
}

func encodeMCPSetup(setup *mcpconfig.MCPSetupSnapshot) any {
	if !mcpconfig.ValidMCPSetup(setup) {
		return nil
	}
	encoded, err := json.Marshal(setup)
	if err != nil {
		return nil
	}
	return string(encoded)
}

// SaveAcceptedMCPSetup records the exact accepted operation without replacing
// a newer saved selection or a newer accepted revision.
func (r *sqliteRepository) SaveAcceptedMCPSetup(ctx context.Context, sessionID string, revision int64, attemptID string, setup *mcpconfig.MCPSetupSnapshot) error {
	if strings.TrimSpace(sessionID) == "" || revision < 0 {
		return mcpconfig.ErrMCPInvalidSelection
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO mcp_task_session_apply_state
		(task_session_id, desired_revision, applied_revision, apply_state, attachment_attempt_id, accepted_setup_json, updated_at)
		VALUES (?, ?, ?, 'applied', ?, ?, ?)
		ON CONFLICT(task_session_id) DO UPDATE SET
			applied_revision = excluded.applied_revision,
			apply_state = CASE WHEN mcp_task_session_apply_state.desired_revision = excluded.applied_revision THEN 'applied' ELSE 'pending_idle' END,
			attachment_attempt_id = excluded.attachment_attempt_id,
			accepted_setup_json = excluded.accepted_setup_json,
			failure_code = '', failure_summary = '', updated_at = excluded.updated_at
		WHERE mcp_task_session_apply_state.applied_revision <= excluded.applied_revision
	`), sessionID, revision, revision, attemptID, encodeMCPSetup(setup), time.Now().UTC())
	return err
}
