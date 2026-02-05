// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
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

// ShellHandlers provides WebSocket handlers for shell terminal operations.
// Shell output is streamed via the lifecycle manager and event bus.
// This handler provides shell.status, shell.subscribe (for buffer), and shell.input.
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

	// Get the agent execution for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"available": false,
			"error":     "no agent running for this session",
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

	// Verify the agent execution exists for this session
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", req.SessionID)
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

	// Get the agent execution for this session
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

// UserShellListRequest for user_shell.list action
type UserShellListRequest struct {
	SessionID string `json:"session_id"`
}

// wsUserShellList returns all running user shells for a session
func (h *ShellHandlers) wsUserShellList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get the interactive runner
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
			"shells": []interface{}{},
		})
	}

	// Get list of user shells
	shells := interactiveRunner.ListUserShells(req.SessionID)

	h.logger.Debug("listing user shells",
		zap.String("session_id", req.SessionID),
		zap.Int("count", len(shells)))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"shells": shells,
	})
}

// UserShellCreateRequest for user_shell.create action
type UserShellCreateRequest struct {
	SessionID string `json:"session_id"`
	ScriptID  string `json:"script_id,omitempty"` // Optional: create a script terminal instead of plain shell
}

// wsUserShellCreate creates a new user shell terminal and returns the assigned ID and label
func (h *ShellHandlers) wsUserShellCreate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellCreateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	// Get the interactive runner
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available")
	}

	var terminalID, label, initialCommand string
	var closable bool

	if req.ScriptID != "" {
		// Creating a script terminal - look up the script
		if h.scriptService == nil {
			return nil, fmt.Errorf("script service not available")
		}

		script, err := h.scriptService.GetRepositoryScript(ctx, req.ScriptID)
		if err != nil {
			h.logger.Error("failed to get repository script",
				zap.String("script_id", req.ScriptID),
				zap.Error(err))
			return nil, fmt.Errorf("invalid script ID: %w", err)
		}

		terminalID = "script-" + uuid.New().String()
		label = script.Name
		initialCommand = script.Command
		closable = true // Script terminals are always closable

		// Register the script terminal with the interactive runner
		interactiveRunner.RegisterScriptShell(req.SessionID, terminalID, label, initialCommand)

		h.logger.Info("created script terminal",
			zap.String("session_id", req.SessionID),
			zap.String("terminal_id", terminalID),
			zap.String("label", label),
			zap.String("script_id", req.ScriptID),
			zap.String("initial_command", initialCommand))
	} else {
		// Creating a plain shell terminal
		result := interactiveRunner.CreateUserShell(req.SessionID)
		terminalID = result.TerminalID
		label = result.Label
		closable = result.Closable

		h.logger.Info("created user shell",
			zap.String("session_id", req.SessionID),
			zap.String("terminal_id", terminalID),
			zap.String("label", label),
			zap.Bool("closable", closable))
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"terminal_id":     terminalID,
		"label":           label,
		"closable":        closable,
		"initial_command": initialCommand,
	})
}

// UserShellStopRequest for user_shell.stop action
type UserShellStopRequest struct {
	SessionID  string `json:"session_id"`
	TerminalID string `json:"terminal_id"`
}

// wsUserShellStop stops a user shell terminal process
func (h *ShellHandlers) wsUserShellStop(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellStopRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.TerminalID == "" {
		return nil, fmt.Errorf("terminal_id is required")
	}

	// Get the interactive runner
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available")
	}

	// Stop the user shell
	if err := interactiveRunner.StopUserShell(ctx, req.SessionID, req.TerminalID); err != nil {
		h.logger.Warn("failed to stop user shell",
			zap.String("session_id", req.SessionID),
			zap.String("terminal_id", req.TerminalID),
			zap.Error(err))
		// Don't return error - shell may already be stopped
	}

	h.logger.Info("user shell stopped",
		zap.String("session_id", req.SessionID),
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
	api.GET("/sessions/:id/terminals", handlers.httpListTerminals)
}

// httpListTerminals returns all terminals for a session (for SSR).
func (h *ShellHandlers) httpListTerminals(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	// Get the interactive runner
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		c.JSON(http.StatusOK, gin.H{"terminals": []interface{}{}})
		return
	}

	// Get list of user shells
	shells := interactiveRunner.ListUserShells(sessionID)

	h.logger.Debug("listing terminals via HTTP",
		zap.String("session_id", sessionID),
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
