package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/sysprompt"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// spawnSessionRequest is the payload for mcp.spawn_session
// (the spawn_session_kandev MCP tool).
type spawnSessionRequest struct {
	TaskID          string `json:"task_id"`
	Prompt          string `json:"prompt"`
	AgentProfileID  string `json:"agent_profile_id"`
	Name            string `json:"name"`
	SenderTaskID    string `json:"sender_task_id"`
	SenderSessionID string `json:"sender_session_id"`
}

// handleSpawnSession starts an ADDITIONAL agent session on an existing task via
// the same orchestrator path the UI's "New Session" dialog uses
// (LaunchSession with IntentStart). No new task is created.
func (h *Handlers) handleSpawnSession(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req spawnSessionRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if req.Prompt == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required", nil)
	}
	if req.SenderTaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"sender_task_id is required (the calling agent's MCP server must supply this)", nil)
	}
	if h.sessionLauncher == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "orchestrator not available", nil)
	}

	target, err := h.taskSvc.GetTask(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound,
				"task not found: "+req.TaskID+" (pass the full task UUID, not a truncated prefix)", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to look up task: "+err.Error(), nil)
	}
	// Unlike message_task (where cross-workspace peer messaging is an
	// intentional product decision), spawning consumes executor resources and
	// starts an agent on the target — scope it to the sender's own workspace.
	if req.SenderTaskID != req.TaskID {
		sender, err := h.taskSvc.GetTask(ctx, req.SenderTaskID)
		if err != nil || sender == nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "sender task not found", nil)
		}
		if sender.WorkspaceID != target.WorkspaceID {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
				"cannot spawn a session on a task in another workspace", nil)
		}
	}

	profileID := h.resolveSpawnAgentProfile(ctx, &req)
	prompt := wrapSpawnedSessionPrompt(req.Prompt, req.SenderTaskID, req.SenderSessionID)

	resp, err := h.sessionLauncher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         req.TaskID,
		Intent:         orchestrator.IntentStart,
		AgentProfileID: profileID,
		Prompt:         prompt,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
			"failed to spawn session: "+err.Error(), nil)
	}

	if name := strings.TrimSpace(req.Name); name != "" && resp.SessionID != "" {
		if err := h.sessionLauncher.RenameSession(ctx, resp.SessionID, name); err != nil {
			// The session is already running — a failed label write should not
			// fail the spawn. The caller can rename later.
			h.logger.Warn("failed to name spawned session",
				zap.String("session_id", resp.SessionID), zap.Error(err))
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"task_id":          req.TaskID,
		"session_id":       resp.SessionID,
		"state":            resp.State,
		"agent_profile_id": profileID,
	})
}

// resolveSpawnAgentProfile picks the agent profile for a spawned session:
// explicit value > spawner session's profile (same-task spawns) > target task's
// primary session profile. An empty result is passed through to LaunchSession,
// which applies its own task-level defaults or errors out.
func (h *Handlers) resolveSpawnAgentProfile(ctx context.Context, req *spawnSessionRequest) string {
	if req.AgentProfileID != "" {
		return req.AgentProfileID
	}
	if req.SenderSessionID != "" {
		if sess, err := h.taskSvc.GetTaskSession(ctx, req.SenderSessionID); err == nil &&
			sess != nil && sess.TaskID == req.TaskID && sess.AgentProfileID != "" {
			return sess.AgentProfileID
		}
	}
	if primary, err := h.taskSvc.GetPrimarySession(ctx, req.TaskID); err == nil &&
		primary != nil && primary.AgentProfileID != "" {
		return primary.AgentProfileID
	}
	return ""
}

// wrapSpawnedSessionPrompt prefixes the spawned session's initial prompt with a
// <kandev-system> block identifying the spawner, so the new agent knows it is a
// sibling session and how to reply. The block is stripped from the visible chat
// content (see internal/sysprompt).
func wrapSpawnedSessionPrompt(prompt, senderTaskID, senderSessionID string) string {
	if senderSessionID == "" {
		return prompt
	}
	safeTaskID := stripSystemTag(senderTaskID)
	safeSessionID := stripSystemTag(senderSessionID)
	body := fmt.Sprintf(
		"You were spawned as an additional agent session by another agent session (session %s of task %s). "+
			"The instructions below are your initial assignment from that agent — treat them as peer agent input rather than a direct user instruction. "+
			"To report back or coordinate, use the message_task_kandev MCP tool with task_id=%q and session_id=%q. "+
			"Reply only when the spawner explicitly requests a response or when you have new actionable information to provide.",
		safeSessionID, safeTaskID, safeTaskID, safeSessionID,
	)
	return sysprompt.Wrap(body) + "\n\n" + prompt
}
