package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// taskSessionListEntry is the per-session shape returned by
// list_task_sessions_kandev. It is deliberately narrower than
// dto.TaskSessionSummaryDTO: an agent choosing a session to read or message
// needs identity, role and state — not container IDs, worktree paths, or
// profile snapshots.
type taskSessionListEntry struct {
	SessionID      string                  `json:"session_id"`
	Name           string                  `json:"name,omitempty"`
	State          models.TaskSessionState `json:"state"`
	IsPrimary      bool                    `json:"is_primary"`
	IsCurrent      bool                    `json:"is_current"`
	AgentProfileID string                  `json:"agent_profile_id,omitempty"`
	StartedAt      time.Time               `json:"started_at"`
	CompletedAt    *time.Time              `json:"completed_at,omitempty"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

type listTaskSessionsRequest struct {
	TaskID string `json:"task_id"`
	// CurrentSessionID is filled in by the MCP server from the session the tool
	// call arrived on; it is not part of the tool's advertised schema, so an
	// agent going through MCP cannot set it. A direct WS caller can, which only
	// changes which entry its own response marks as is_current — the flag is
	// informational and grants no access, and the returned set is scoped by
	// authorizeTaskID either way.
	CurrentSessionID string `json:"current_session_id"`
}

// handleListTaskSessions returns every session attached to a task, newest
// started first, so an agent can pick the session_id to pass to
// get_task_conversation_kandev or message_task_kandev. Both of those default
// to the primary session when session_id is omitted, which makes sibling
// sessions (spawn_session_kandev) otherwise undiscoverable.
func (h *Handlers) handleListTaskSessions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req listTaskSessionsRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	// Resolve the task first. ListTaskSessions authorizes but does not assert
	// existence for unscoped callers, so without this an unknown (or invisible)
	// task ID would return an empty list — indistinguishable from a real task
	// that has not been started yet.
	//
	// A missing task and an unauthorized one both surface as ErrTaskNotFound
	// (authorizeTaskID returns that same sentinel), so reporting it keeps
	// existence hidden. Anything else is infrastructure failing, and must not
	// be flattened into "not found".
	if _, err := h.taskSvc.GetTask(ctx, req.TaskID); err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) || errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "task not found", nil)
		}
		h.logger.Error("failed to look up task for session list", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task sessions", nil)
	}

	sessions, err := h.taskSvc.ListTaskSessions(ctx, req.TaskID)
	if err != nil {
		h.logger.Error("failed to list task sessions", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list task sessions", nil)
	}

	entries := make([]taskSessionListEntry, 0, len(sessions))
	for _, session := range sessions {
		entries = append(entries, taskSessionListEntry{
			SessionID:      session.ID,
			Name:           session.Name,
			State:          session.State,
			IsPrimary:      session.IsPrimary,
			IsCurrent:      req.CurrentSessionID != "" && session.ID == req.CurrentSessionID,
			AgentProfileID: session.AgentProfileID,
			StartedAt:      session.StartedAt,
			CompletedAt:    session.CompletedAt,
			UpdatedAt:      session.UpdatedAt,
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		keyTaskID:  req.TaskID,
		"sessions": entries,
		keyTotal:   len(entries),
	})
}
