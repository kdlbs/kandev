package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	terminalmodels "github.com/kandev/kandev/internal/terminal/models"
	terminalservice "github.com/kandev/kandev/internal/terminal/service"
)

const maxCursorMCPRecoveryServerIDLength = 512

type cursorMCPRecoveryManager interface {
	CursorMCPAuthenticationSpec(context.Context, string, string) (runtime.CursorMCPAuthenticationSpec, error)
	RetryCursorMCPConnection(context.Context, string, string) (runtime.CursorMCPRetryResult, error)
}

type cursorMCPRecoveryTerminalService interface {
	CreateWithOneShotInitialCommand(context.Context, string, string, string) (*terminalmodels.Terminal, error)
	List(context.Context, string, bool) ([]terminalservice.ListItem, error)
	ClaimShellLaunchInfo(context.Context, string, string, string) (terminalservice.ShellLaunchInfo, error)
	MarkCursorMCPAuthenticationAttemptPending(string)
	CursorMCPAuthenticationAttemptPending(string) bool
	MarkCursorMCPAuthenticationAttemptFailed(string)
	Rename(context.Context, string, string, *string) error
	Resume(context.Context, string, string) error
	Destroy(context.Context, string, string) error
}

type cursorMCPRecoveryRequest struct {
	ServerID string `json:"server_id"`
}

type cursorMCPAuthenticationResponse struct {
	TerminalID        string `json:"terminal_id"`
	TaskEnvironmentID string `json:"task_environment_id"`
	Label             string `json:"label"`
	Reused            bool   `json:"reused"`
}

type cursorMCPRetryResponse struct {
	ProviderID string                         `json:"provider_id"`
	ServerID   string                         `json:"server_id"`
	Status     string                         `json:"status"`
	ReasonCode string                         `json:"reason_code,omitempty"`
	ToolCount  int                            `json:"tool_count,omitempty"`
	Diagnostic *mcpconfig.NativeMCPDiagnostic `json:"mcp_diagnostic,omitempty"`
}

func (h *ProcessHandlers) httpAuthenticateCursorMCP(c *gin.Context) {
	if !h.authorizeCursorMCPRecovery(c) {
		return
	}
	request, ok := h.bindCursorMCPRecoveryRequest(c)
	if !ok {
		return
	}
	if h.cursorMCPRecovery == nil || h.terminalSvc == nil {
		writeCursorMCPRecoveryDependencyUnavailable(c)
		return
	}
	h.createCursorMCPAuthenticationTerminal(c, request)
}

func (h *ProcessHandlers) createCursorMCPAuthenticationTerminal(c *gin.Context, request cursorMCPRecoveryRequest) {
	spec, err := h.cursorMCPRecovery.CursorMCPAuthenticationSpec(c.Request.Context(), c.Param("id"), request.ServerID)
	if err != nil {
		h.writeCursorMCPRecoveryError(c, err)
		return
	}
	if spec.ServerID != request.ServerID || spec.TaskID == "" || spec.TaskEnvironmentID == "" ||
		spec.InitialCommand == "" || spec.Label == "" {
		writeCursorMCPRecoveryUnavailable(c, "server_unavailable")
		return
	}

	h.cursorMCPRecoveryMu.Lock()
	defer h.cursorMCPRecoveryMu.Unlock()
	if h.reuseCursorMCPAuthenticationTerminal(c, spec) {
		return
	}
	h.createNewCursorMCPAuthenticationTerminal(c, spec)
}

func (h *ProcessHandlers) reuseCursorMCPAuthenticationTerminal(c *gin.Context, spec runtime.CursorMCPAuthenticationSpec) bool {
	items, err := h.terminalSvc.List(c.Request.Context(), spec.TaskID, true)
	if err != nil {
		h.logger.Error("failed to list terminals for Cursor MCP authentication", zap.Error(err))
		writeCursorMCPRecoveryInternalError(c)
		return true
	}
	for _, item := range items {
		if !h.isReusableCursorMCPAuthenticationTerminal(item, spec) {
			continue
		}
		if item.State == "parked" {
			if err := h.terminalSvc.Resume(c.Request.Context(), spec.TaskID, item.ID); err != nil {
				h.logger.Error("failed to resume Cursor MCP authentication terminal", zap.Error(err))
				writeCursorMCPRecoveryInternalError(c)
				return true
			}
		}
		c.JSON(http.StatusOK, cursorMCPAuthenticationResponse{
			TerminalID: item.ID, TaskEnvironmentID: spec.TaskEnvironmentID, Label: spec.Label, Reused: true,
		})
		return true
	}
	return false
}

func (h *ProcessHandlers) isReusableCursorMCPAuthenticationTerminal(item terminalservice.ListItem, spec runtime.CursorMCPAuthenticationSpec) bool {
	if item.Kind != terminalservice.KindOrdinary || item.EnvironmentID != spec.TaskEnvironmentID ||
		item.DisplayName != spec.Label || item.InitialCommand != spec.InitialCommand ||
		(item.State != "open" && item.State != "parked") {
		return false
	}
	return item.PTYStatus == terminalservice.PTYStatusRunning || h.terminalSvc.CursorMCPAuthenticationAttemptPending(item.ID)
}

func (h *ProcessHandlers) createNewCursorMCPAuthenticationTerminal(c *gin.Context, spec runtime.CursorMCPAuthenticationSpec) {
	terminal, err := h.terminalSvc.CreateWithOneShotInitialCommand(c.Request.Context(), spec.TaskID, spec.TaskEnvironmentID, spec.InitialCommand)
	if err != nil {
		h.logger.Error("failed to create Cursor MCP authentication terminal", zap.Error(err))
		writeCursorMCPRecoveryInternalError(c)
		return
	}
	if err := h.terminalSvc.Rename(c.Request.Context(), spec.TaskID, terminal.ID, &spec.Label); err != nil {
		if cleanupErr := h.terminalSvc.Destroy(c.Request.Context(), spec.TaskID, terminal.ID); cleanupErr != nil {
			h.logger.Warn("failed to clean up Cursor MCP authentication terminal", zap.Error(cleanupErr))
		}
		h.logger.Error("failed to label Cursor MCP authentication terminal", zap.Error(err))
		writeCursorMCPRecoveryInternalError(c)
		return
	}
	h.terminalSvc.MarkCursorMCPAuthenticationAttemptPending(terminal.ID)
	c.JSON(http.StatusOK, cursorMCPAuthenticationResponse{
		TerminalID: terminal.ID, TaskEnvironmentID: spec.TaskEnvironmentID, Label: spec.Label, Reused: false,
	})
}

func (h *ProcessHandlers) httpRetryCursorMCP(c *gin.Context) {
	if !h.authorizeCursorMCPRecovery(c) {
		return
	}
	request, ok := h.bindCursorMCPRecoveryRequest(c)
	if !ok {
		return
	}
	if h.cursorMCPRecovery == nil {
		writeCursorMCPRecoveryDependencyUnavailable(c)
		return
	}

	result, err := h.cursorMCPRecovery.RetryCursorMCPConnection(c.Request.Context(), c.Param("id"), request.ServerID)
	if err != nil {
		h.writeCursorMCPRecoveryError(c, err)
		return
	}
	if !validCursorMCPRetryResult(result, request.ServerID) {
		h.logger.Warn("Cursor MCP retry returned an invalid readiness result")
		writeCursorMCPRecoveryUnavailable(c, "server_unavailable")
		return
	}
	c.JSON(http.StatusOK, cursorMCPRetryResponse{
		ProviderID: result.ProviderID, ServerID: result.ServerID, Status: result.Status,
		ReasonCode: result.ReasonCode, ToolCount: result.ToolCount,
		Diagnostic: normalizeCursorMCPRetryDiagnostic(result.Diagnostic),
	})
}

func normalizeCursorMCPRetryDiagnostic(diagnostic *mcpconfig.NativeMCPDiagnostic) *mcpconfig.NativeMCPDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return mcpconfig.NormalizeNativeMCPDiagnostic(diagnostic, diagnostic.Operation)
}

func (h *ProcessHandlers) authorizeCursorMCPRecovery(c *gin.Context) bool {
	if sessionID := c.Param("id"); sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return false
	}
	return !h.denySessionExecutionAccess(c, c.Param("id"))
}

func (h *ProcessHandlers) bindCursorMCPRecoveryRequest(c *gin.Context) (cursorMCPRecoveryRequest, bool) {
	var request cursorMCPRecoveryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return request, false
	}
	if !validCursorMCPRecoveryServerID(request.ServerID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_server_id"})
		return request, false
	}
	return request, true
}

func validCursorMCPRecoveryServerID(serverID string) bool {
	if serverID == "" || len(serverID) > maxCursorMCPRecoveryServerIDLength {
		return false
	}
	return strings.TrimFunc(serverID, unicode.IsSpace) == serverID &&
		!strings.HasPrefix(serverID, "-") && !strings.ContainsAny(serverID, "\x00\r\n")
}

func (h *ProcessHandlers) writeCursorMCPRecoveryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtime.ErrCursorMCPAuthenticationUnsupported):
		c.JSON(http.StatusNotImplemented, gin.H{"error": "mcp_recovery_unsupported", "error_code": "mcp_recovery_unsupported", "reason_code": "unsupported_shell"})
	case errors.Is(err, runtime.ErrCursorMCPRecoverySessionBusy):
		writeCursorMCPRecoveryUnavailable(c, "session_busy")
	case errors.Is(err, runtime.ErrCursorMCPRecoveryUnavailable):
		writeCursorMCPRecoveryUnavailable(c, "server_unavailable")
	case errors.Is(err, repoerrors.ErrTaskNotFound), errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
	default:
		h.logger.Error("Cursor MCP recovery failed", zap.Error(err))
		writeCursorMCPRecoveryInternalError(c)
	}
}

func writeCursorMCPRecoveryUnavailable(c *gin.Context, reasonCode string) {
	errorCode := "mcp_recovery_unavailable"
	if reasonCode == "session_busy" {
		errorCode = "mcp_recovery_session_busy"
	}
	c.JSON(http.StatusConflict, gin.H{"error": errorCode, "error_code": errorCode, "reason_code": reasonCode})
}

func writeCursorMCPRecoveryInternalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "mcp_recovery_failed", "error_code": "mcp_recovery_failed"})
}

func writeCursorMCPRecoveryDependencyUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp_recovery_unavailable", "error_code": "mcp_recovery_unavailable"})
}

func validCursorMCPRetryResult(result runtime.CursorMCPRetryResult, serverID string) bool {
	if result.ProviderID != "cursor" || result.ServerID != serverID || result.ToolCount < 0 {
		return false
	}
	switch result.Status {
	case "ready", "authentication_required", "approval_failed", "connection_failed", "unavailable":
	default:
		return false
	}
	if result.ReasonCode == "" {
		return result.Status == "ready"
	}
	switch result.ReasonCode {
	case "authentication_required", "approval_failed", "connection_failed", "unavailable", "canceled", "session_reload_unsupported":
		return true
	default:
		return false
	}
}
