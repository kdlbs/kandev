package lifecycle

import (
	"encoding/json"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

type BuiltinActionResolver func(p.Context) ([]string, error)

type builtinDispatchState struct {
	encoded     string
	taskID      string
	workspaceID string
	policy      streams.BuiltinMCPAuthority
}

func (e *AgentExecution) builtinMCPAuthority(resolve BuiltinActionResolver) streams.BuiltinMCPAuthority {
	e.builtinMCPMu.Lock()
	defer e.builtinMCPMu.Unlock()
	value, required := e.metadataValue(p.BuiltinBindingMetadataKey)
	if !required {
		return streams.BuiltinMCPAuthority{}
	}
	if resolve == nil {
		return streams.BuiltinMCPAuthority{Required: true}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return streams.BuiltinMCPAuthority{Required: true}
	}
	if e.builtinDispatch != nil && e.builtinDispatch.encoded == string(encoded) && e.builtinDispatch.taskID == e.TaskID && e.builtinDispatch.workspaceID == e.WorkspaceID {
		return e.builtinDispatch.policy
	}
	authority := streams.BuiltinMCPAuthority{Required: true}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err == nil && profile.BuiltinPackages.TaskID == e.TaskID && profile.BuiltinPackages.WorkspaceID == e.WorkspaceID {
		authority.Actions, err = resolve(*profile)
		if err == nil {
			authority.Manifest = profile.BuiltinPackages.Clone()
		}
	}
	e.builtinDispatch = &builtinDispatchState{encoded: string(encoded), taskID: e.TaskID, workspaceID: e.WorkspaceID, policy: authority}
	return authority
}

func putBuiltinLaunchMetadata(metadata map[string]any, profile *p.Context) {
	delete(metadata, p.BuiltinBindingMetadataKey)
	if profile == nil || profile.BuiltinPackages == nil {
		return
	}
	encoded, err := p.MarshalBuiltinBinding(*profile)
	if err != nil {
		metadata[p.BuiltinBindingMetadataKey] = map[string]any{"invalid": true}
		return
	}
	metadata[p.BuiltinBindingMetadataKey] = encoded
}

// SetBuiltinActionResolver binds backend authorization to the native tool registry.
// It must be installed during dependency wiring before a package-mode launch.
func (m *Manager) SetBuiltinActionResolver(resolve BuiltinActionResolver) {
	m.streamManager.mcpMu.Lock()
	defer m.streamManager.mcpMu.Unlock()
	m.streamManager.builtinActionResolver = resolve
}

func (sm *StreamManager) builtinResolver() BuiltinActionResolver {
	sm.mcpMu.RLock()
	defer sm.mcpMu.RUnlock()
	return sm.builtinActionResolver
}
