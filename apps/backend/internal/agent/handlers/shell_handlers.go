// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// errTaskEnvIDRequired is the canonical error string for missing
// task_environment_id on user-shell RPCs.
const errTaskEnvIDRequired = "task_environment_id is required"

// ShellHandlers provides WebSocket handlers for shell terminal operations.
//
// User-shell ops (user_shell.list/create/stop) are env-keyed: they take
// task_environment_id directly. Sessions in the same task share one env and
// therefore one shell list — there's no per-session shell state.
//
// Agent passthrough ops (shell.status/subscribe/input) stay session-keyed —
// they target the agent's own PTY, which is intrinsically session-scoped.
type ShellHandlers struct {
	lifecycleMgr  *lifecycle.Manager
	scriptService scripts.ScriptService
	logger        *logger.Logger
}

// NewShellHandlers creates a new ShellHandlers instance
func NewShellHandlers(lifecycleMgr *lifecycle.Manager, scriptService scripts.ScriptService, log *logger.Logger) *ShellHandlers {
	return &ShellHandlers{
		lifecycleMgr:  lifecycleMgr,
		scriptService: scriptService,
		logger:        log.WithFields(zap.String("component", "shell_handlers")),
	}
}

// RegisterHandlers registers shell handlers with the WebSocket dispatcher
func (h *ShellHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionShellStatus, h.wsShellStatus)
	d.RegisterFunc(ws.ActionShellSubscribe, h.wsShellSubscribe)
	d.RegisterFunc(ws.ActionShellInput, h.wsShellInput)
	d.RegisterFunc(ws.ActionUserShellList, h.wsUserShellList)
	d.RegisterFunc(ws.ActionUserShellCreate, h.wsUserShellCreate)
	d.RegisterFunc(ws.ActionUserShellStop, h.wsUserShellStop)
}

// ShellStatusRequest for shell.status action
type ShellStatusRequest struct {
	SessionID string `json:"session_id"`
}

// ShellInputRequest for shell.input action
type ShellInputRequest struct {
	SessionID string `json:"session_id"`
	Data      string `json:"data"`
}

// wsShellStatus returns the status of a shell session for a session
func (h *ShellHandlers) wsShellStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellStatusRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get or create execution on-demand (survives backend restart)
	execution, err := h.lifecycleMgr.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "no agent running for this session: " + err.Error(),
		})
	}

	// Get shell status from agentctl
	client := execution.GetAgentCtlClient()
	if client == nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "agent client not available",
		})
	}

	status, err := client.ShellStatus(ctx)
	if err != nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     err.Error(),
		})
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"available":  true,
		"running":    status.Running,
		"pid":        status.Pid,
		"shell":      status.Shell,
		"cwd":        status.Cwd,
		"started_at": status.StartedAt,
	})
}

// ShellSubscribeRequest for shell.subscribe action
type ShellSubscribeRequest struct {
	SessionID string `json:"session_id"`
}

// wsShellSubscribe subscribes to shell output for a session.
// Shell output is streamed via the event bus (lifecycle manager handles this).
// This endpoint returns the buffered shell output for catchup.
func (h *ShellHandlers) wsShellSubscribe(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get or create execution on-demand (survives backend restart)
	execution, err := h.lifecycleMgr.GetOrEnsureExecution(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("no agent running for session %s: %w", req.SessionID, err)
	}

	// Get buffered output to include in response
	// This ensures client gets current shell state without duplicate broadcasts
	// Shell output streaming is handled by the lifecycle manager via event bus
	buffer := ""
	if client := execution.GetAgentCtlClient(); client != nil {
		if b, err := client.ShellBuffer(ctx); err == nil {
			buffer = b
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
		"buffer":     buffer,
	})
}

// wsShellInput sends input to a shell session
func (h *ShellHandlers) wsShellInput(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ShellInputRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get the agent execution for this session.
	// Use GetExecutionBySessionID (not GetOrEnsureExecution): writing input
	// requires the workspace stream to already be live, and the 5s wait below
	// is too short to absorb a cold-start of agentctl.
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
	}

	// Wait for the workspace stream to be ready with a timeout.
	// This handles the race condition where client sends shell.input before
	// the workspace stream is fully connected (e.g., joining a task too fast).
	var workspaceStream = execution.GetWorkspaceStream()
	if workspaceStream == nil {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(100 * time.Millisecond)
			workspaceStream = execution.GetWorkspaceStream()
			if workspaceStream != nil {
				break
			}
		}
	}

	if workspaceStream == nil {
		return nil, fmt.Errorf("workspace stream not ready for session %s", req.SessionID)
	}

	if err := workspaceStream.WriteShellInput(req.Data); err != nil {
		return nil, fmt.Errorf("failed to send shell input: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}

// UserShellListRequest for user_shell.list action.
// User shells are env-scoped — sessions in the same task share one shell list.
type UserShellListRequest struct {
	TaskEnvironmentID string `json:"task_environment_id"`
}

// wsUserShellList returns all running user shells for a task environment
func (h *ShellHandlers) wsUserShellList(_ context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.TaskEnvironmentID == "" {
		return nil, errors.New(errTaskEnvIDRequired)
	}

	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"shells": []interface{}{},
		})
	}

	shells := interactiveRunner.ListUserShells(req.TaskEnvironmentID)

	h.logger.Debug("listing user shells",
		zap.String("task_environment_id", req.TaskEnvironmentID),
		zap.Int("count", len(shells)))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"shells": shells,
	})
}

// UserShellCreateRequest for user_shell.create action.
// Exactly one of (ScriptID) or (Command) may be set; if both are empty the
// handler creates a plain shell. When Command is set, Label is used as the
// terminal label (defaults to "Script" when omitted).
//
// User shells are env-scoped — TaskEnvironmentID is required.
type UserShellCreateRequest struct {
	TaskEnvironmentID string `json:"task_environment_id"`
	ScriptID          string `json:"script_id,omitempty"`
	Command           string `json:"command,omitempty"`
	Label             string `json:"label,omitempty"`
}

// wsUserShellCreate creates a new user shell terminal and returns the assigned ID and label.
func (h *ShellHandlers) wsUserShellCreate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellCreateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if req.TaskEnvironmentID == "" {
		return nil, errors.New(errTaskEnvIDRequired)
	}

	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available")
	}

	label, command, err := h.resolveShellScript(ctx, &req)
	if err != nil {
		return nil, err
	}

	scopeID := req.TaskEnvironmentID

	if command != "" {
		terminalID := "script-" + uuid.New().String()
		interactiveRunner.RegisterScriptShell(scopeID, terminalID, label, command)
		h.logger.Info("created script terminal",
			zap.String("task_environment_id", scopeID),
			zap.String("terminal_id", terminalID),
			zap.String("label", label),
			zap.String("initial_command", command))
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			"terminal_id":     terminalID,
			"label":           label,
			"closable":        true,
			"initial_command": command,
		})
	}

	result := interactiveRunner.CreateUserShell(scopeID)
	h.logger.Info("created user shell",
		zap.String("task_environment_id", scopeID),
		zap.String("terminal_id", result.TerminalID),
		zap.String("label", result.Label),
		zap.Bool("closable", result.Closable))
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"terminal_id":     result.TerminalID,
		"label":           result.Label,
		"closable":        result.Closable,
		"initial_command": "",
	})
}

// resolveShellScript returns the (label, command) pair for a script terminal,
// or ("", "") when the request is for a plain shell.
func (h *ShellHandlers) resolveShellScript(
	ctx context.Context, req *UserShellCreateRequest,
) (string, string, error) {
	if req.ScriptID != "" {
		if h.scriptService == nil {
			return "", "", fmt.Errorf("script service not available")
		}
		script, err := h.scriptService.GetRepositoryScript(ctx, req.ScriptID)
		if err != nil {
			h.logger.Error("failed to get repository script",
				zap.String("script_id", req.ScriptID), zap.Error(err))
			return "", "", fmt.Errorf("invalid script ID: %w", err)
		}
		return script.Name, script.Command, nil
	}
	if req.Command != "" {
		label := req.Label
		if label == "" {
			label = "Script"
		}
		return label, req.Command, nil
	}
	return "", "", nil
}

// UserShellStopRequest for user_shell.stop action.
// User shells are env-scoped — TaskEnvironmentID is required.
type UserShellStopRequest struct {
	TaskEnvironmentID string `json:"task_environment_id"`
	TerminalID        string `json:"terminal_id"`
}

// wsUserShellStop stops a user shell terminal process
func (h *ShellHandlers) wsUserShellStop(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellStopRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.TaskEnvironmentID == "" {
		return nil, errors.New(errTaskEnvIDRequired)
	}
	if req.TerminalID == "" {
		return nil, fmt.Errorf("terminal_id is required")
	}

	// Get the interactive runner
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available")
	}

	if err := interactiveRunner.StopUserShell(ctx, req.TaskEnvironmentID, req.TerminalID); err != nil {
		h.logger.Warn("failed to stop user shell",
			zap.String("task_environment_id", req.TaskEnvironmentID),
			zap.String("terminal_id", req.TerminalID),
			zap.Error(err))
		// Don't return error - shell may already be stopped
	}

	h.logger.Info("user shell stopped",
		zap.String("task_environment_id", req.TaskEnvironmentID),
		zap.String("terminal_id", req.TerminalID))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
}

// RegisterShellRoutes registers HTTP routes for shell operations.
// This is separate from RegisterHandlers which registers WebSocket handlers.
func RegisterShellRoutes(router *gin.Engine, lifecycleMgr *lifecycle.Manager, log *logger.Logger) {
	handlers := NewShellHandlers(lifecycleMgr, nil, log)
	api := router.Group("/api/v1")
	api.GET("/environments/:id/terminals", handlers.httpListTerminals)
}

// httpListTerminals returns all terminals for a task environment (for SSR).
func (h *ShellHandlers) httpListTerminals(c *gin.Context) {
	environmentID := c.Param("id")
	if environmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errTaskEnvIDRequired})
		return
	}

	// Get the interactive runner
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		c.JSON(http.StatusOK, gin.H{"terminals": []interface{}{}})
		return
	}

	shells := interactiveRunner.ListUserShells(environmentID)

	h.logger.Debug("listing terminals via HTTP",
		zap.String("task_environment_id", environmentID),
		zap.Int("count", len(shells)))

	// Transform to response format
	terminals := make([]map[string]interface{}, len(shells))
	for i, shell := range shells {
		terminals[i] = map[string]interface{}{
			"terminal_id":     shell.TerminalID,
			"process_id":      shell.ProcessID,
			"running":         shell.Running,
			"label":           shell.Label,
			"closable":        shell.Closable,
			"initial_command": shell.InitialCommand,
		}
	}

	c.JSON(http.StatusOK, gin.H{"terminals": terminals})
}
