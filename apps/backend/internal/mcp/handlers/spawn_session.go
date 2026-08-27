package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
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

	spawner := h.resolveSpawnerSession(ctx, &req)
	profileID := h.resolveSpawnAgentProfile(ctx, &req, spawner)

	resp, err := h.sessionLauncher.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         req.TaskID,
		Intent:         orchestrator.IntentStart,
		AgentProfileID: profileID,
		Prompt:         req.Prompt,
		SpawnOrigin:    spawnOriginFromSession(spawner),
	})
	if err != nil {
		if errors.Is(err, models.ErrWorkspacePreparing) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"the task workspace is still being prepared; retry after its initial launch completes",
				map[string]interface{}{
					"reason":      "workspace_preparing",
					"recoverable": true,
					"retry":       "retry after the initial workspace launch completes",
				})
		}
		if errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"the task workspace cannot be safely reused; retry after restoring its existing workspace",
				map[string]interface{}{
					"reason":      "workspace_reuse_unsafe",
					"recoverable": true,
					"retry":       "restore the task workspace, then retry",
				})
		}
		if errors.Is(err, worktree.ErrReuseWorktreeUnavailable) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"the task workspace cannot be safely reused; retry after restoring its existing workspace",
				map[string]interface{}{
					"reason":      "workspace_reuse_unsafe",
					"recoverable": true,
					"retry":       "restore the task workspace, then retry",
				})
		}
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
		"agent_profile_id": resp.AgentProfileID,
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

// resolveSpawnerSession loads the calling agent's own session and verifies it
// belongs to the task the caller claims to be spawning from. Everything derived
// from the spawner — the inherited agent profile and the attribution baked into
// the new session's first turn — comes from this record rather than from the
// request, so a sender_session_id that is unknown or owned by a different task
// cannot become launch attribution. Returns nil when there is no verifiable
// spawner session (external MCP callers, or a mismatched claim).
func (h *Handlers) resolveSpawnerSession(ctx context.Context, req *spawnSessionRequest) *models.TaskSession {
	if req.SenderSessionID == "" || h.taskSvc == nil {
		return nil
	}
	sess, err := h.taskSvc.GetTaskSession(ctx, req.SenderSessionID)
	if err != nil || sess == nil || sess.TaskID != req.SenderTaskID {
		return nil
	}
	return sess
}

// resolveSpawnAgentProfile picks the agent profile for a spawned session:
// explicit value > spawner session's profile (same-task spawns) > target task's
// primary session profile. An empty result is passed through to LaunchSession,
// which applies its own task-level defaults or errors out.
func (h *Handlers) resolveSpawnAgentProfile(
	ctx context.Context, req *spawnSessionRequest, spawner *models.TaskSession,
) string {
	if req.AgentProfileID != "" {
		return req.AgentProfileID
	}
	if spawner != nil && spawner.TaskID == req.TaskID && spawner.AgentProfileID != "" {
		return spawner.AgentProfileID
	}
	if primary, err := h.taskSvc.GetPrimarySession(ctx, req.TaskID); err == nil &&
		primary != nil && primary.AgentProfileID != "" {
		return primary.AgentProfileID
	}
	return ""
}

// spawnOriginFromSession describes the spawning session so the launch site can
// tell the new agent who spawned it and how to reply. Identity comes from the
// verified session row (see resolveSpawnerSession), never from the request.
//
// The attribution text itself is deliberately NOT built here: the orchestrator
// strips any <kandev-system> block it cannot attribute to server state when it
// canonicalizes a session's first turn, so a prompt wrapped this early would be
// dropped before the agent ever saw it.
func spawnOriginFromSession(spawner *models.TaskSession) *orchestrator.SpawnOrigin {
	if spawner == nil {
		return nil
	}
	return &orchestrator.SpawnOrigin{
		TaskID:      spawner.TaskID,
		SessionID:   spawner.ID,
		SessionName: spawner.Name,
	}
}
