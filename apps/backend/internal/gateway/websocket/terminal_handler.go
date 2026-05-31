// Package websocket provides WebSocket handlers for the gateway.
package websocket

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
)

// UserService interface for getting user preferences.
type UserService interface {
	PreferredShell(ctx context.Context) (string, error)
}

// TerminalHandler handles dedicated binary WebSocket connections for terminal I/O.
// This bypasses JSON encoding for raw PTY communication via xterm.js AttachAddon.
type TerminalHandler struct {
	lifecycleMgr  *lifecycle.Manager
	userService   UserService
	scriptService scripts.ScriptService
	logger        *logger.Logger
}

// NewTerminalHandler creates a new TerminalHandler instance.
func NewTerminalHandler(lifecycleMgr *lifecycle.Manager, userService UserService, scriptService scripts.ScriptService, log *logger.Logger) *TerminalHandler {
	return &TerminalHandler{
		lifecycleMgr:  lifecycleMgr,
		userService:   userService,
		scriptService: scriptService,
		logger:        log.WithFields(zap.String("component", "terminal_handler")),
	}
}

// terminalUpgrader is the WebSocket upgrader for terminal connections.
// Uses larger buffers for better TUI performance.
var terminalUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     checkWebSocketOrigin,
}

// checkWebSocketOrigin validates the Origin header for WebSocket connections.
// This prevents cross-site WebSocket hijacking attacks.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No origin header - allow (could be a non-browser client)
		return true
	}

	// Allow localhost origins for development
	if strings.HasPrefix(origin, "http://localhost") ||
		strings.HasPrefix(origin, "http://127.0.0.1") ||
		strings.HasPrefix(origin, "https://localhost") ||
		strings.HasPrefix(origin, "https://127.0.0.1") {
		return true
	}

	// Check same-origin: Origin should match the Host header
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}

	// Parse the origin URL to get its host
	originURL, err := url.Parse(origin)
	if err != nil {
		return false
	}

	// Compare hosts (ignoring port for flexibility)
	originHost := originURL.Hostname()
	requestHost := host
	if colonIdx := strings.LastIndex(requestHost, ":"); colonIdx != -1 {
		// Strip port from host if present (but be careful with IPv6)
		if !strings.Contains(requestHost, "]") || colonIdx > strings.Index(requestHost, "]") {
			requestHost = requestHost[:colonIdx]
		}
	}

	return originHost == requestHost
}

type terminalRoute struct {
	kind string
	id   string
}

const (
	terminalRouteEnvironment = "environment"
	terminalRouteSession     = "session"
)

func parseTerminalRoute(c *gin.Context) terminalRoute {
	target := strings.Trim(c.Param("target"), "/")
	if target == "" {
		return terminalRoute{}
	}
	if id, ok := strings.CutPrefix(target, "environment/"); ok {
		return terminalRoute{kind: terminalRouteEnvironment, id: id}
	}
	if id, ok := strings.CutPrefix(target, "session/"); ok {
		return terminalRoute{kind: terminalRouteSession, id: id}
	}
	return terminalRoute{}
}

// HandleTerminalWS handles terminal WebSocket connections.
// Explicit routes:
// - /terminal/session/:sessionId?mode=agent for agent passthrough terminals
// - /terminal/environment/:environmentId?terminalId=... for user shell terminals
func (h *TerminalHandler) HandleTerminalWS(c *gin.Context) {
	route := parseTerminalRoute(c)
	if route.id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "terminal route must be /environment/:environmentId for shell terminals or /session/:sessionId?mode=agent for agent terminals"})
		return
	}

	switch route.kind {
	case terminalRouteEnvironment:
		h.handleEnvironmentTerminalRoute(c, route.id)
	case terminalRouteSession:
		h.handleSessionTerminalRoute(c, route.id)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid terminal route"})
	}
}

func (h *TerminalHandler) handleSessionTerminalRoute(c *gin.Context, sessionID string) {
	if c.Query("mode") != "agent" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session terminal route requires mode=agent; shell terminals must use /terminal/environment/:environmentId"})
		return
	}
	h.handleAgentTerminalRoute(c, sessionID)
}

func (h *TerminalHandler) handleEnvironmentTerminalRoute(c *gin.Context, environmentID string) {
	mode := c.Query("mode")
	if mode != "" && mode != "shell" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "environment terminal route only supports shell mode"})
		return
	}
	terminalID := c.Query("terminalId")
	if terminalID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "terminalId required for shell mode"})
		return
	}
	h.handleEnvironmentUserShellWS(c, environmentID, terminalID)
}

func (h *TerminalHandler) handleAgentTerminalRoute(c *gin.Context, sessionID string) {
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sessionId is required"})
		return
	}
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		h.logger.Error("interactive runner not available")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interactive runner not available"})
		return
	}
	h.logger.Info("terminal WebSocket connection request",
		zap.String("session_id", sessionID),
		zap.String("mode", "agent"),
		zap.String("remote_addr", c.Request.RemoteAddr))
	h.handleAgentPassthroughWS(c, sessionID, interactiveRunner)
}

func (h *TerminalHandler) handleEnvironmentUserShellWS(c *gin.Context, environmentID, terminalID string) {
	execution, err := h.lifecycleMgr.GetOrEnsureExecutionForEnvironment(c.Request.Context(), environmentID)
	if err != nil {
		h.logger.Warn("environment terminal execution not ready",
			zap.String("task_environment_id", environmentID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	if h.shouldUseWorkspaceShell(c.Request.Context(), execution.SessionID) {
		remoteExecution, ok := h.waitForRemoteExecutionReadyWithTimeout(
			c.Request.Context(), execution.SessionID, passthroughReadyTimeout,
		)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remote execution not available"})
			return
		}
		execution = remoteExecution
		h.handleRemoteUserShellWS(c, execution, environmentID, terminalID)
		return
	}
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		h.logger.Error("interactive runner not available")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "interactive runner not available"})
		return
	}
	h.logger.Info("terminal WebSocket connection request",
		zap.String("task_environment_id", environmentID),
		zap.String("session_id", execution.SessionID),
		zap.String("mode", "shell"),
		zap.String("terminal_id", terminalID),
		zap.String("remote_addr", c.Request.RemoteAddr))
	h.handleUserShellWS(c, execution, environmentID, terminalID, interactiveRunner)
}

func (h *TerminalHandler) handleRemoteUserShellWS(
	c *gin.Context,
	execution *lifecycle.AgentExecution,
	scopeID string,
	terminalID string,
) {
	sessionID := execution.SessionID
	if execution.GetAgentCtlClient() == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "remote execution not available"})
		return
	}
	client := execution.GetAgentCtlClient()

	_, initialCommand, httpErr := h.resolveShellLabel(c)
	if httpErr != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": httpErr})
		return
	}
	// The per-terminal WS URL carries only env + terminalId, so recover the
	// command pre-registered by wsUserShellCreate for script/dev terminals.
	if initialCommand == "" {
		if runner := h.lifecycleMgr.GetInteractiveRunner(); runner != nil {
			initialCommand = runner.LookupShellInitialCommand(scopeID, terminalID)
		}
	}

	// Create a per-terminal shell on agentctl (idempotent if already exists)
	if err := client.StartShellTerminal(c.Request.Context(), terminalID, 80, 24); err != nil {
		h.logger.Error("failed to start remote terminal shell",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// Open binary WS to the per-terminal shell on agentctl
	agentctlConn, err := client.StreamShellTerminal(c.Request.Context(), terminalID)
	if err != nil {
		h.logger.Error("failed to connect to remote terminal shell stream",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// Upgrade the client-facing WebSocket
	clientConn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		_ = agentctlConn.Close()
		h.logger.Error("failed to upgrade remote shell websocket",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}

	defer func() {
		_ = agentctlConn.Close()
		_ = clientConn.Close()
	}()

	h.logger.Info("remote terminal shell WebSocket connected",
		zap.String("task_environment_id", scopeID),
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID))

	// Send initial command if specified
	if initialCommand != "" {
		if err := agentctlConn.WriteMessage(gorillaws.BinaryMessage, []byte(initialCommand+"\n")); err != nil {
			h.logger.Debug("failed to send initial command to remote terminal shell",
				zap.String("terminal_id", terminalID),
				zap.Error(err))
		}
	}

	h.bridgeBinaryWebSockets(clientConn, agentctlConn, terminalID)
}

// handleAgentPassthroughWS handles WebSocket connections for agent passthrough terminals.
// It ensures a passthrough execution exists, upgrades to WebSocket, and bridges I/O.
func (h *TerminalHandler) handleAgentPassthroughWS(
	c *gin.Context,
	sessionID string,
	interactiveRunner *process.InteractiveRunner,
) {
	// Ensure passthrough execution exists and is running.
	// This handles:
	// 1. Normal case: execution exists with running process
	// 2. Backend restart: no execution, need to create and start
	// 3. Process died: execution exists but process not running, need to restart
	execution, err := h.ensurePassthroughExecutionReady(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Warn("failed to ensure passthrough execution",
			zap.String("session_id", sessionID),
			zap.Error(err))
		if errors.Is(err, lifecycle.ErrSessionWorkspaceNotReady) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	processID := execution.PassthroughProcessID
	if processID == "" {
		h.logger.Error("passthrough process ID is empty after ensure",
			zap.String("session_id", sessionID))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "passthrough process not started"})
		return
	}

	// Verify the process exists (either running or pending start for deferred-start processes)
	if !interactiveRunner.IsProcessReadyOrPending(processID) {
		h.logger.Error("passthrough process not found or exited",
			zap.String("session_id", sessionID),
			zap.String("process_id", processID))
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "passthrough process failed to start"})
		return
	}

	// Upgrade to WebSocket - we'll get PTY access after the first resize
	// triggers the lazy process start
	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade to WebSocket",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	h.logger.Info("terminal WebSocket connected",
		zap.String("session_id", sessionID),
		zap.String("process_id", processID))

	// Create WebSocket writer for output
	wsw := newWsWriter(conn)
	interactiveRunner.TrackSessionWebSocket(sessionID, wsw)

	// Run the terminal bridge
	h.runTerminalBridge(conn, sessionID, processID, interactiveRunner, wsw)
}

// shellScriptInfo holds the label and initial command resolved from a script.
type shellScriptInfo struct {
	label          string
	initialCommand string
}

// resolveShellScript looks up a script by ID and returns its label and command.
// Returns an error if the script cannot be retrieved.
func (h *TerminalHandler) resolveShellScript(ctx context.Context, scriptID string) (*shellScriptInfo, error) {
	script, err := h.scriptService.GetRepositoryScript(ctx, scriptID)
	if err != nil {
		h.logger.Error("failed to get repository script",
			zap.String("script_id", scriptID),
			zap.Error(err))
		return nil, err
	}
	h.logger.Info("handleUserShellWS: resolved script",
		zap.String("script_id", scriptID),
		zap.String("script_name", script.Name),
		zap.String("script_command", script.Command))
	return &shellScriptInfo{label: script.Name, initialCommand: script.Command}, nil
}

// resolvePreferredShell returns the user's preferred shell, or empty string on error.
func (h *TerminalHandler) resolvePreferredShell(ctx context.Context) string {
	if h.userService == nil {
		return ""
	}
	shell, err := h.userService.PreferredShell(ctx)
	if err != nil {
		h.logger.Debug("failed to get preferred shell, using default", zap.Error(err))
		return ""
	}
	return shell
}

// resolveWorkingDir returns the workspace path from the environment execution.
func (h *TerminalHandler) resolveWorkingDir(execution *lifecycle.AgentExecution) (string, error) {
	if execution == nil {
		return "", fmt.Errorf("environment execution is not ready")
	}
	if execution.WorkspacePath == "" {
		return "", fmt.Errorf("%w: task environment %s has no workspace path configured",
			lifecycle.ErrSessionWorkspaceNotReady, execution.TaskEnvironmentID)
	}
	h.logger.Info("handleUserShellWS: got working directory from environment execution",
		zap.String("working_dir", execution.WorkspacePath))
	return execution.WorkspacePath, nil
}

// resolveShellLabel returns the label and initial command for a user shell, derived
// from either a script ID lookup or a plain label query parameter.
// Returns an HTTP error string (non-empty) when the script lookup fails.
func (h *TerminalHandler) resolveShellLabel(c *gin.Context) (label, initialCommand, httpErr string) {
	scriptID := c.Query("scriptId")
	labelParam := c.Query("label")

	if scriptID != "" && h.scriptService != nil {
		info, err := h.resolveShellScript(c.Request.Context(), scriptID)
		if err != nil {
			return "", "", "invalid script ID"
		}
		return info.label, info.initialCommand, ""
	}
	if labelParam != "" {
		return labelParam, "", ""
	}
	return "", "", ""
}

// startUserShellProcess resolves shell parameters and starts (or reconnects to) the
// user shell process. Returns the process ID on success, or an HTTP status + message on failure.
func (h *TerminalHandler) startUserShellProcess(
	c *gin.Context,
	execution *lifecycle.AgentExecution,
	scopeID string,
	terminalID string,
	interactiveRunner *process.InteractiveRunner,
) (string, int, string) {
	sessionID := execution.SessionID
	label, initialCommand, httpErr := h.resolveShellLabel(c)
	if httpErr != "" {
		return "", http.StatusBadRequest, httpErr
	}

	h.logger.Info("handleUserShellWS: starting user shell handling",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("label", label),
		zap.String("initial_command", initialCommand))

	preferredShell := h.resolvePreferredShell(c.Request.Context())
	workingDir, err := h.resolveWorkingDir(execution)
	if err != nil {
		h.logger.Warn("handleUserShellWS: workspace path not ready",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return "", http.StatusServiceUnavailable, err.Error()
	}

	opts := &process.UserShellOptions{Label: label, InitialCommand: initialCommand}

	h.logger.Info("handleUserShellWS: calling StartUserShell",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("working_dir", workingDir),
		zap.String("preferred_shell", preferredShell),
		zap.String("label", opts.Label),
		zap.String("initial_command", opts.InitialCommand))

	info, err := interactiveRunner.StartUserShell(
		c.Request.Context(), scopeID, sessionID, terminalID, workingDir, preferredShell, opts,
	)
	if err != nil {
		h.logger.Error("failed to start user shell",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return "", http.StatusServiceUnavailable, err.Error()
	}

	h.logger.Info("handleUserShellWS: user shell started successfully",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", info.ID),
		zap.Int("os_pid", info.OSPID))

	return info.ID, 0, ""
}

// handleUserShellWS handles WebSocket connections for user shell terminals.
// Each terminal tab gets its own independent shell process.
func (h *TerminalHandler) handleUserShellWS(
	c *gin.Context,
	execution *lifecycle.AgentExecution,
	scopeID string,
	terminalID string,
	interactiveRunner *process.InteractiveRunner,
) {
	sessionID := execution.SessionID
	processID, httpStatus, errMsg := h.startUserShellProcess(
		c, execution, scopeID, terminalID, interactiveRunner,
	)
	if errMsg != "" {
		c.JSON(httpStatus, gin.H{"error": errMsg})
		return
	}

	conn, err := terminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade to WebSocket",
			zap.String("session_id", sessionID),
			zap.String("terminal_id", terminalID),
			zap.Error(err))
		return
	}

	h.logger.Info("user shell WebSocket connected",
		zap.String("session_id", sessionID),
		zap.String("terminal_id", terminalID),
		zap.String("process_id", processID),
		zap.Int("os_pid", osPIDForInteractiveProcess(interactiveRunner, processID)))

	wsw := newWsWriter(conn)
	h.runUserShellBridge(conn, sessionID, scopeID, terminalID, processID, interactiveRunner, wsw)
}

func osPIDForInteractiveProcess(runner *process.InteractiveRunner, processID string) int {
	if runner == nil || processID == "" {
		return 0
	}
	pid, ok := runner.GetOSPID(processID)
	if !ok {
		return 0
	}
	return pid
}
