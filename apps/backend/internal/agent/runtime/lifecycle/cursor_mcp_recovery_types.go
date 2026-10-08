package lifecycle

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// ErrCursorMCPRecoverySessionBusy is returned when recovery would interfere
// with an active conversational turn. Recovery never stops or replays work.
var ErrCursorMCPRecoverySessionBusy = errors.New("cursor MCP recovery session is busy")

// ErrCursorMCPRecoveryUnavailable is returned when the requested native MCP
// server is no longer eligible for recovery in this session.
var ErrCursorMCPRecoveryUnavailable = errors.New("cursor MCP recovery server is unavailable")

// CursorMCPAuthenticationSpec contains trusted task context used by the HTTP
// transport to create or reuse an ordinary terminal. InitialCommand is private
// to the backend and must never be serialized to a client.
type CursorMCPAuthenticationSpec struct {
	TaskID            string
	TaskEnvironmentID string
	WorkspacePath     string
	InitialCommand    string
	Label             string
	ServerID          string
}

// CursorMCPRetryResult is the sanitized readiness result returned after a
// same-session native connection recheck.
type CursorMCPRetryResult struct {
	ProviderID string
	ServerID   string
	Status     string
	ReasonCode string
	ToolCount  int
	Diagnostic *mcpconfig.NativeMCPDiagnostic `json:"mcp_diagnostic,omitempty"`
}
