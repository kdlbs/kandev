package lifecycle

import (
	"context"
	"errors"
	"fmt"

	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
)

// verifyBuiltinExecution does not resolve desired policy. A retained process
// must agree with its original binding before it can accept work.
func (m *Manager) verifyBuiltinExecution(ctx context.Context, execution *AgentExecution) error {
	err := m.verifyBuiltinExecutionBinding(ctx, execution)
	if err != nil {
		return &BuiltinMCPRecoveryError{Cause: err}
	}
	return nil
}

// BuiltinMCPRecoveryError prevents a retained binding mismatch from becoming a
// broader fresh-session retry.
type BuiltinMCPRecoveryError struct{ Cause error }

func (e *BuiltinMCPRecoveryError) Error() string          { return "built-in MCP binding requires recovery" }
func (e *BuiltinMCPRecoveryError) Unwrap() error          { return e.Cause }
func (e *BuiltinMCPRecoveryError) RecoveryReason() string { return "builtin_mcp_binding" }

func (m *Manager) verifyBuiltinExecutionBinding(ctx context.Context, execution *AgentExecution) error {
	execution.builtinMCPMu.Lock()
	defer execution.builtinMCPMu.Unlock()
	value, bound := execution.metadataValue(p.BuiltinBindingMetadataKey)
	if !bound {
		return nil
	}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil {
		return fmt.Errorf("invalid built-in MCP execution binding: %w", err)
	}
	if err := m.validateBuiltinOwner(ctx, execution.TaskID, execution.SessionID, execution.WorkspaceID, profile); err != nil {
		return err
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return errors.New("built-in MCP helper is unavailable")
	}
	if err := client.VerifyBuiltinMCP(ctx, *profile); err != nil {
		return fmt.Errorf("verify built-in MCP execution binding: %w", err)
	}
	return nil
}

func (m *Manager) validateBuiltinOwner(ctx context.Context, taskID, sessionID, workspaceID string, profile *p.Context) error {
	if _, err := p.MarshalBuiltinBinding(*profile); err != nil {
		return fmt.Errorf("invalid built-in MCP profile: %w", err)
	}
	manifest := profile.BuiltinPackages
	if taskID == "" || sessionID == "" || manifest.TaskID != taskID || manifest.WorkspaceID != workspaceID {
		return errors.New("built-in MCP binding does not match the execution owner")
	}
	_, _, scope := m.streamManager.mcpDispatchState()
	if scope == nil {
		return errors.New("built-in MCP principal scope is unavailable")
	}
	scoped, err := scope(ctx, taskID, sessionID)
	if err != nil {
		return fmt.Errorf("resolve built-in MCP principal: %w", err)
	}
	if !mcpscope.BuiltinPrincipalMatches(scoped, manifest) {
		return errors.New("built-in MCP binding does not match the current principal")
	}
	if m.streamManager.builtinResolver() == nil {
		return errors.New("built-in MCP action registry is unavailable")
	}
	return nil
}
