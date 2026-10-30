package lifecycle

import (
	"context"
	"errors"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

func (m *Manager) prepareBuiltinWorkspaceExecution(ctx context.Context, execution *AgentExecution, req *LaunchRequest) error {
	if req.McpProfile == nil || req.McpProfile.BuiltinPackages == nil {
		if _, bound := execution.metadataValue(p.BuiltinBindingMetadataKey); bound {
			return errors.New("cannot discard a prepared built-in MCP binding")
		}
		return nil
	}
	// A restored execution keeps its original authority, even if desired policy changed.
	if _, bound := execution.metadataValue(p.BuiltinBindingMetadataKey); bound {
		return m.verifyBuiltinExecution(ctx, execution)
	}
	profile := p.Normalize(*req.McpProfile)
	encoded, err := p.MarshalBuiltinBinding(profile)
	if err != nil {
		return err
	}
	if profile.BuiltinPackages.TaskID != execution.TaskID || profile.BuiltinPackages.WorkspaceID != execution.WorkspaceID {
		return errors.New("built-in MCP binding belongs to another workspace or task")
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return errors.New("workspace helper is unavailable for built-in MCP preparation")
	}
	execution.builtinMCPMu.Lock()
	defer execution.builtinMCPMu.Unlock()
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, map[string]any{"preparation_incomplete": true})
	if err := m.persistBuiltinMCPBinding(ctx, execution); err != nil {
		return err
	}
	if err := client.PrepareBuiltinMCP(ctx, profile); err != nil {
		return err
	}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	if err := m.persistBuiltinMCPBinding(ctx, execution); err != nil {
		execution.setMetadataValue(p.BuiltinBindingMetadataKey, map[string]any{"preparation_incomplete": true})
		return err
	}
	return nil
}

// PrepareBuiltinWorkspace covers prepared-workspace starts that reuse the
// execution without passing through Launch again.
func (m *Manager) PrepareBuiltinWorkspace(ctx context.Context, executionID string, profile p.Context) error {
	execution, exists := m.executionStore.Get(executionID)
	if !exists {
		return ErrExecutionNotFound
	}
	_, err := m.doSerializedExecution(ctx, execution.SessionID, func(sharedCtx context.Context) (interface{}, error) {
		if err := m.ensureLaunchSessionStillActive(sharedCtx, execution.SessionID, executionAdmissionAgent); err != nil {
			return nil, err
		}
		current, exists := m.executionStore.GetBySessionID(execution.SessionID)
		if !exists || current != execution {
			return nil, errors.New("workspace execution changed before built-in MCP preparation")
		}
		if err := m.validateBuiltinOwner(sharedCtx, execution.TaskID, execution.SessionID, execution.WorkspaceID, &profile); err != nil {
			return nil, err
		}
		return nil, m.prepareBuiltinWorkspaceExecution(sharedCtx, execution, &LaunchRequest{McpProfile: &profile})
	})
	return err
}
