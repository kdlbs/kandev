package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
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
	if errResp := h.validateSpawnRequest(&req, msg); errResp != nil {
		return errResp, nil
	}
	if errResp := h.authorizeSpawnTarget(ctx, &req, msg); errResp != nil {
		return errResp, nil
	}

	profileID := h.resolveSpawnAgentProfile(ctx, &req)

	resp, err := h.sessionLauncher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         req.TaskID,
		Intent:         orchestrator.IntentStart,
		AgentProfileID: profileID,
		Prompt:         req.Prompt,
		SpawnOrigin:    h.spawnOrigin(ctx, &req),
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

// validateSpawnRequest checks the request's required fields.
// Returns a ready-to-send WS error message, or nil when valid.
func (h *Handlers) validateSpawnRequest(req *spawnSessionRequest, msg *ws.Message) *ws.Message {
	if req.TaskID == "" {
		return wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required")
	}
	if req.Prompt == "" {
		return wsError(msg.ID, msg.Action, ws.ErrorCodeValidation, "prompt is required")
	}
	if req.SenderTaskID == "" {
		return wsError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"sender_task_id is required (the calling agent's MCP server must supply this)")
	}
	return nil
}

// authorizeSpawnTarget verifies the target task exists and shares a workspace
// with the sender. Unlike message_task (where cross-workspace peer messaging
// is an intentional product decision), spawning consumes executor resources
// and starts an agent on the target — scope it to the sender's own workspace.
func (h *Handlers) authorizeSpawnTarget(ctx context.Context, req *spawnSessionRequest, msg *ws.Message) *ws.Message {
	target, err := h.taskSvc.GetTask(ctx, req.TaskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound,
				"task not found: "+req.TaskID+" (pass the full task UUID, not a truncated prefix)")
		}
		return wsError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "failed to look up task: "+err.Error())
	}
	if req.SenderTaskID == req.TaskID {
		return nil
	}
	sender, err := h.taskSvc.GetTask(ctx, req.SenderTaskID)
	if err != nil || sender == nil {
		return wsError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "sender task not found")
	}
	if sender.WorkspaceID != target.WorkspaceID {
		return wsError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"cannot spawn a session on a task in another workspace")
	}
	return nil
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

// spawnOrigin describes the calling agent session so the launch site can build
// the spawner-attribution system block that tells the new agent who spawned it
// and how to reply. The identifiers come from the caller's MCP server (which
// injects its own task/session), never from the tool arguments.
//
// The block itself is deliberately NOT built here: the orchestrator strips any
// <kandev-system> block it cannot attribute to server state when it
// canonicalizes a session's first turn, so a prompt wrapped this early would be
// dropped before the agent ever saw it. Returns nil when there is no spawner
// session to attribute (e.g. external MCP callers).
func (h *Handlers) spawnOrigin(ctx context.Context, req *spawnSessionRequest) *orchestrator.SpawnOrigin {
	if req.SenderSessionID == "" {
		return nil
	}
	return &orchestrator.SpawnOrigin{
		TaskID:      req.SenderTaskID,
		SessionID:   req.SenderSessionID,
		SessionName: h.lookupSenderSessionName(ctx, req.SenderTaskID, req.SenderSessionID),
	}
}
