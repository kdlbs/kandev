package lifecycle

import p "github.com/kandev/kandev/internal/mcp/profile"

// builtinMCPManifestSnapshot exposes only the execution's sealed package
// selection. It makes no claim that a process accepted it or connected.
func (e *AgentExecution) builtinMCPManifestSnapshot() *p.BuiltinManifest {
	if value, present := e.metadataValue(p.ProviderMCPBindingMetadataKey); present {
		binding, profile, err := p.ParseProviderMCPBinding(value)
		if err != nil || binding.Scope.ExecutionID != e.ID || profile.BuiltinPackages.TaskID != e.TaskID || profile.BuiltinPackages.WorkspaceID != e.WorkspaceID {
			return nil
		}
		return profile.BuiltinPackages.Clone()
	}
	value, present := e.metadataValue(p.BuiltinBindingMetadataKey)
	if !present {
		return nil
	}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil || profile.BuiltinPackages.TaskID != e.TaskID || profile.BuiltinPackages.WorkspaceID != e.WorkspaceID {
		return nil
	}
	return profile.BuiltinPackages.Clone()
}
