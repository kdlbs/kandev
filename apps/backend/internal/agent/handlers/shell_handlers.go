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
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// errTaskEnvIDRequired is the canonical error string for missing
// task_environment_id on user-shell RPCs.
const errTaskEnvIDRequired = "task_environment_id is required"

// JSON keys shared by user_shell.* responses (extracted so the goconst
// linter doesn't complain about literal duplication across handlers).
const (
	shellResponseKindField       = "kind"
	shellResponseTerminalIDField = "terminal_id"
	shellResponseLabelField      = "label"
	shellResponseClosableField   = "closable"
	shellResponseInitialCmdField = "initial_command"
)

// shellKindScript is the discriminator value for script terminals (id
// prefix `script-`). Bottom-panel is "fixed"; ordinary uses the constant
// from internal/terminal/service.
const shellKindScript = "script"

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
	// terminalSvc, when set, owns the ordinary user-terminal path
	// (DB-backed, sticky, renameable). Non-ordinary ids — bottom-panel and
	// script-* — keep going through the original agentctl-direct code
	// regardless. Wired via SetTerminalService once the storage layer is
	// up; left nil in unit tests, where the legacy in-memory runner path
	// is exercised directly.
	terminalSvc *terminalservice.Service
}

// NewShellHandlers creates a new ShellHandlers instance
func NewShellHandlers(lifecycleMgr *lifecycle.Manager, scriptService scripts.ScriptService, log *logger.Logger) *ShellHandlers {
	return &ShellHandlers{
		lifecycleMgr:  lifecycleMgr,
		scriptService: scriptService,
		logger:        log.WithFields(zap.String("component", "shell_handlers")),
	}
}

// SetTerminalService injects the terminal service after construction. Done
// post-hoc so the storage-layer wiring (which creates the service) can run
// after handler construction without breaking existing test wiring.
func (h *ShellHandlers) SetTerminalService(svc *terminalservice.Service) {
	h.terminalSvc = svc
}

// RegisterHandlers registers shell handlers with the WebSocket dispatcher
func (h *ShellHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionShellStatus, h.wsShellStatus)
	d.RegisterFunc(ws.ActionShellSubscribe, h.wsShellSubscribe)
	d.RegisterFunc(ws.ActionShellInput, h.wsShellInput)
	d.RegisterFunc(ws.ActionUserShellList, h.wsUserShellList)
	d.RegisterFunc(ws.ActionUserShellCreate, h.wsUserShellCreate)
	d.RegisterFunc(ws.ActionUserShellStop, h.wsUserShellStop)
	d.RegisterFunc(ws.ActionUserShellDestroy, h.wsUserShellStop)
	d.RegisterFunc(ws.ActionUserShellRename, h.wsUserShellRename)
	d.RegisterFunc(ws.ActionUserShellPark, h.wsUserShellPark)
	d.RegisterFunc(ws.ActionUserShellResume, h.wsUserShellResume)
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

	execution, err := h.ensureShellExecution(ctx, req.SessionID)
	if err != nil {
		return nil, err
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

	execution, err := h.ensureShellExecution(ctx, req.SessionID)
	if err != nil {
		return nil, err
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

// shellLifecycle is the narrow slice of lifecycle.Manager that
// ensureShellExecution needs. *lifecycle.Manager satisfies it; tests can
// stub it without touching the rest of the manager.
type shellLifecycle interface {
	GetOrEnsureExecution(ctx context.Context, sessionID string) (*lifecycle.AgentExecution, error)
	CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error
}

func (h *ShellHandlers) ensureShellExecution(ctx context.Context, sessionID string) (*lifecycle.AgentExecution, error) {
	return ensureShellExecution(ctx, h.lifecycleMgr, h.logger, sessionID)
}

// ensureShellExecution returns a live shell execution for sessionID, recovering
// from a stale (Failed) execution by cleaning it up and re-launching once.
// Extracted as a free function so the recovery branch is directly testable
// without standing up a real lifecycle.Manager.
func ensureShellExecution(ctx context.Context, lifecycleMgr shellLifecycle, log *logger.Logger, sessionID string) (*lifecycle.AgentExecution, error) {
	execution, err := lifecycleMgr.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		log.Debug("failed to ensure shell execution",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return nil, fmt.Errorf("no agent running for session %s", sessionID)
	}
	if execution.Status != v1.AgentStatusFailed {
		return execution, nil
	}
	if err := lifecycleMgr.CleanupStaleExecutionBySessionID(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("cleanup stale execution for session %s: %w", sessionID, err)
	}
	execution, err = lifecycleMgr.GetOrEnsureExecution(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("recover execution for session %s: %w", sessionID, err)
	}
	return execution, nil
}

// UserShellListRequest for user_shell.list action.
//
// task_environment_id is required (agentctl scope key). task_id is
// required when the terminal service is wired — only ordinary user
// terminals are listed from the DB, so the service needs a task to scope
// the query. Tests that exercise the legacy direct-runner path leave
// task_id empty.
//
// include_parked toggles whether parked tabs are returned (default false,
// strip view); the "Parked terminals" submenu sends true.
type UserShellListRequest struct {
	TaskID            string `json:"task_id"`
	TaskEnvironmentID string `json:"task_environment_id"`
	IncludeParked     bool   `json:"include_parked"`
}

// userShellListItem is the wire-shape this handler returns. Discriminated
// by Kind: "ordinary" items carry seq + display_name + state; "fixed"
// (bottom-panel) and "script" items carry just the id + pty_status so the
// frontend can keep dispatching them unchanged.
type userShellListItem struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Seq            int     `json:"seq,omitempty"`
	DisplayName    string  `json:"display_name,omitempty"`
	CustomName     *string `json:"custom_name,omitempty"`
	State          string  `json:"state,omitempty"`
	PTYStatus      string  `json:"pty_status"`
	Label          string  `json:"label,omitempty"`
	InitialCommand string  `json:"initial_command,omitempty"`
}

// wsUserShellList resolves the union of (DB ordinary terminals) and
// (agentctl non-managed terminals) for a task.
func (h *ShellHandlers) wsUserShellList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellListRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if req.TaskEnvironmentID == "" {
		return nil, errors.New(errTaskEnvIDRequired)
	}

	items := []userShellListItem{}

	// Ordinary terminals — backed by the terminal service when wired.
	if h.terminalSvc != nil && req.TaskID != "" {
		ordinary, err := h.terminalSvc.List(ctx, req.TaskID, req.IncludeParked)
		if err != nil {
			return nil, fmt.Errorf("list ordinary terminals: %w", err)
		}
		for _, it := range ordinary {
			items = append(items, userShellListItem{
				ID:             it.ID,
				Kind:           it.Kind,
				Seq:            it.Seq,
				DisplayName:    it.DisplayName,
				CustomName:     it.CustomName,
				State:          it.State,
				PTYStatus:      it.PTYStatus,
				InitialCommand: it.InitialCommand,
			})
		}
	}

	// Non-managed terminals — passthrough straight from agentctl. Filters
	// out anything an ordinary row already covers so a managed id never
	// appears twice.
	// When the terminal service is wired and a task scope was supplied,
	// managed-shaped orphans are dropped (no DB row → not user-visible).
	// Without those preconditions we're on the legacy passthrough path,
	// so any agentctl-side shell stays visible regardless of id shape.
	managedCovered := h.terminalSvc != nil && req.TaskID != ""
	items = appendUnmanagedShells(items, h.lifecycleMgr.GetInteractiveRunner(), req.TaskEnvironmentID, managedCovered)

	h.logger.Debug("listing user shells",
		zap.String("task_id", req.TaskID),
		zap.String("task_environment_id", req.TaskEnvironmentID),
		zap.Int("count", len(items)))

	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"shells": items,
	})
}

// appendUnmanagedShells merges agentctl's view of unmanaged terminals
// (bottom-panel + scripts + orphans) into the items list. Skips ids the
// terminal service already returned (`seen` set).
//
// managedCovered=true means the caller already loaded ordinary terminals
// from the service for this task — so any managed-shaped id we see here
// without a matching DB row is an orphan and should be silently dropped
// (plan §7).
//
// managedCovered=false means we're on the legacy passthrough path
// (terminal service disabled or task_id missing). In that case every
// agentctl-side shell stays visible regardless of id shape — otherwise
// older clients lose their previously created shells.
func appendUnmanagedShells(
	items []userShellListItem,
	runner *process.InteractiveRunner,
	scopeID string,
	managedCovered bool,
) []userShellListItem {
	if runner == nil || scopeID == "" {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		seen[it.ID] = struct{}{}
	}
	for _, s := range runner.ListUserShells(scopeID) {
		if _, dup := seen[s.TerminalID]; dup {
			continue
		}
		if managedCovered && terminalservice.IsManaged(s.TerminalID) {
			// Orphan ordinary shell (no DB row) — drop silently per
			// plan §7; it'll be GC'd next agentctl restart.
			continue
		}
		items = append(items, userShellListItem{
			ID:             s.TerminalID,
			Kind:           kindForUnmanaged(s.TerminalID),
			Label:          s.Label,
			PTYStatus:      ptyStatusFromRunning(s.Running),
			InitialCommand: s.InitialCommand,
		})
	}
	return items
}

func kindForUnmanaged(id string) string {
	if id == terminalservice.BottomPanelID {
		return "fixed"
	}
	return shellKindScript
}

func ptyStatusFromRunning(running bool) string {
	if running {
		return terminalservice.PTYStatusRunning
	}
	return terminalservice.PTYStatusStopped
}

// UserShellCreateRequest for user_shell.create action.
// Exactly one of (ScriptID) or (Command) may be set; if both are empty the
// handler creates a plain ordinary shell (DB-backed when the terminal
// service is wired). When Command is set, the result is always a script
// terminal (passthrough to agentctl).
//
// TaskEnvironmentID is the agentctl scope; TaskID is needed for the DB
// row on ordinary creates.
type UserShellCreateRequest struct {
	TaskID            string `json:"task_id"`
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
	label, command, err := h.resolveShellScript(ctx, &req)
	if err != nil {
		return nil, err
	}

	scopeID := req.TaskEnvironmentID

	// Script terminal — always passthrough to agentctl, never DB-backed.
	if command != "" {
		if interactiveRunner == nil {
			return nil, fmt.Errorf("interactive runner not available")
		}
		terminalID := "script-" + uuid.New().String()
		interactiveRunner.RegisterScriptShell(scopeID, terminalID, label, command)
		h.logger.Info("created script terminal",
			zap.String("task_environment_id", scopeID),
			zap.String("terminal_id", terminalID),
			zap.String("label", label),
			zap.String("initial_command", command))
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			shellResponseTerminalIDField: terminalID,
			shellResponseKindField:       shellKindScript,
			shellResponseLabelField:      label,
			shellResponseClosableField:   true,
			shellResponseInitialCmdField: command,
		})
	}

	// Plain ordinary shell — DB-backed when the terminal service is wired.
	if h.terminalSvc != nil && req.TaskID != "" {
		term, err := h.terminalSvc.Create(ctx, req.TaskID, scopeID, "")
		if err != nil {
			return nil, fmt.Errorf("create ordinary terminal: %w", err)
		}
		h.logger.Info("created ordinary terminal",
			zap.String("task_id", req.TaskID),
			zap.String("task_environment_id", scopeID),
			zap.String("terminal_id", term.ID),
			zap.Int("seq", term.Seq))
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{
			shellResponseTerminalIDField: term.ID,
			shellResponseKindField:       terminalservice.KindOrdinary,
			"seq":                        term.Seq,
			"display_name":               term.DisplayName(),
			"state":                      string(term.State),
			"pty_status":                 terminalservice.PTYStatusStopped,
		})
	}

	// Legacy path — used by older clients that don't send task_id, and by
	// unit tests that exercise the in-memory runner directly.
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available")
	}
	result := interactiveRunner.CreateUserShell(scopeID)
	h.logger.Info("created user shell (legacy path)",
		zap.String("task_environment_id", scopeID),
		zap.String("terminal_id", result.TerminalID))
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		shellResponseTerminalIDField: result.TerminalID,
		shellResponseKindField:       shellKindScript,
		shellResponseLabelField:      result.Label,
		shellResponseClosableField:   result.Closable,
		shellResponseInitialCmdField: "",
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
	TaskID            string `json:"task_id"`
	TaskEnvironmentID string `json:"task_environment_id"`
	TerminalID        string `json:"terminal_id"`
}

// wsUserShellStop stops a user shell terminal process. Also serves
// user_shell.destroy — semantically identical for managed terminals: stop
// the PTY and delete the DB row. For non-managed ids (bottom-panel,
// script-*) only the PTY is stopped.
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

	// Managed terminal — route through the service so the DB row is
	// deleted alongside the PTY tear-down. Destroy errors propagate so
	// the frontend doesn't optimistically remove a row that the backend
	// failed to delete.
	//
	// task_id is required for ordinary terminals. Older clients (and
	// legacy dockview panels persisted before task_id was stamped) may
	// still send empty task_id and a `shell-…` id that matches IsManaged.
	// For those, fall through to the passthrough stop so legacy callers
	// keep working — the service rejects with ErrTaskMismatch on empty
	// task_id, which we treat as "this isn't a DB-backed terminal,
	// behave like the old code did".
	if h.terminalSvc != nil && req.TaskID != "" && terminalservice.IsManaged(req.TerminalID) {
		if err := h.terminalSvc.Destroy(ctx, req.TaskID, req.TerminalID); err != nil {
			h.logger.Warn("destroy ordinary terminal",
				zap.String("terminal_id", req.TerminalID), zap.Error(err))
			return nil, fmt.Errorf("destroy terminal: %w", err)
		}
		return ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
	}

	// Non-managed, no service wired, or legacy caller without task_id —
	// best-effort PTY tear-down via the runner.
	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		return nil, fmt.Errorf("interactive runner not available")
	}
	if err := interactiveRunner.StopUserShell(ctx, req.TaskEnvironmentID, req.TerminalID); err != nil {
		h.logger.Warn("failed to stop user shell",
			zap.String("task_environment_id", req.TaskEnvironmentID),
			zap.String("terminal_id", req.TerminalID),
			zap.Error(err))
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
}

// UserShellRenameRequest for user_shell.rename. CustomName==nil clears.
// TaskID is verified against the terminal's owning task on the service
// side; cross-task renames return ErrTaskMismatch.
type UserShellRenameRequest struct {
	TaskID     string  `json:"task_id"`
	TerminalID string  `json:"terminal_id"`
	CustomName *string `json:"custom_name"`
}

func (h *ShellHandlers) wsUserShellRename(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellRenameRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if req.TerminalID == "" {
		return nil, fmt.Errorf("terminal_id is required")
	}
	if h.terminalSvc == nil {
		return nil, fmt.Errorf("terminal service not available")
	}
	if err := h.terminalSvc.Rename(ctx, req.TaskID, req.TerminalID, req.CustomName); err != nil {
		return nil, err
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
}

// UserShellStateRequest shares its shape between park and resume.
// TaskID is verified against the terminal's owning task on the service
// side; cross-task mutations return ErrTaskMismatch.
type UserShellStateRequest struct {
	TaskID     string `json:"task_id"`
	TerminalID string `json:"terminal_id"`
}

func (h *ShellHandlers) wsUserShellPark(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellStateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if req.TerminalID == "" {
		return nil, fmt.Errorf("terminal_id is required")
	}
	if h.terminalSvc == nil {
		return nil, fmt.Errorf("terminal service not available")
	}
	if err := h.terminalSvc.Park(ctx, req.TaskID, req.TerminalID); err != nil {
		return nil, err
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
}

func (h *ShellHandlers) wsUserShellResume(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UserShellStateRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}
	if req.TerminalID == "" {
		return nil, fmt.Errorf("terminal_id is required")
	}
	if h.terminalSvc == nil {
		return nil, fmt.Errorf("terminal service not available")
	}
	if err := h.terminalSvc.Resume(ctx, req.TaskID, req.TerminalID); err != nil {
		return nil, err
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
}

// RegisterShellRoutes registers HTTP routes for shell operations.
// This is separate from RegisterHandlers which registers WebSocket handlers.
//
// The handlers value returned by gateway.go is the same instance whose
// terminalSvc gets set later via SetTerminalService — so we accept it here
// rather than constructing a fresh one. Backwards-compat: when called
// without a pre-built handler (helpers.go boot path), the legacy
// runner-only fallback still works because the SSR endpoint degrades
// gracefully when terminalSvc is nil.
func RegisterShellRoutes(router *gin.Engine, lifecycleMgr *lifecycle.Manager, log *logger.Logger) {
	handlers := NewShellHandlers(lifecycleMgr, nil, log)
	RegisterShellRoutesOn(router, handlers)
}

// RegisterShellRoutesOn binds the SSR routes on an existing ShellHandlers
// instance — used in gateway wiring where the handler is already created
// and the terminal service has been injected.
func RegisterShellRoutesOn(router *gin.Engine, handlers *ShellHandlers) {
	api := router.Group("/api/v1")
	api.GET("/environments/:id/terminals", handlers.httpListTerminals)
	api.GET("/tasks/:id/terminals", handlers.httpListTaskTerminals)
}

// httpListTerminals (env-keyed legacy SSR) — kept for backwards-compat with
// callers that don't yet have a task id at SSR time.
func (h *ShellHandlers) httpListTerminals(c *gin.Context) {
	environmentID := c.Param("id")
	if environmentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": errTaskEnvIDRequired})
		return
	}

	interactiveRunner := h.lifecycleMgr.GetInteractiveRunner()
	if interactiveRunner == nil {
		c.JSON(http.StatusOK, gin.H{"terminals": []interface{}{}})
		return
	}

	shells := interactiveRunner.ListUserShells(environmentID)
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

// httpListTaskTerminals — new SSR endpoint keyed by task_id. Returns the
// same union shape the WS user_shell.list returns.
func (h *ShellHandlers) httpListTaskTerminals(c *gin.Context) {
	taskID := c.Param("id")
	envID := c.Query("task_environment_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	items := []userShellListItem{}
	if h.terminalSvc != nil {
		ordinary, err := h.terminalSvc.List(c.Request.Context(), taskID, false)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		for _, it := range ordinary {
			items = append(items, userShellListItem{
				ID:             it.ID,
				Kind:           it.Kind,
				Seq:            it.Seq,
				DisplayName:    it.DisplayName,
				CustomName:     it.CustomName,
				State:          it.State,
				PTYStatus:      it.PTYStatus,
				InitialCommand: it.InitialCommand,
			})
		}
	}
	items = appendUnmanagedShells(items, h.lifecycleMgr.GetInteractiveRunner(), envID, h.terminalSvc != nil)

	c.JSON(http.StatusOK, gin.H{"terminals": items})
}
