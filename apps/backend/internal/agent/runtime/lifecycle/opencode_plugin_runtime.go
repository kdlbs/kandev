package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	commonopencodeplugin "github.com/kandev/kandev/internal/common/opencodeplugin"
)

// openCodePluginRuntimeFor resolves the typed evidence a supported launch
// needs. Unsupported families, versions, sources, or disabled launches return
// nil so no plugin files or listeners are prepared.
func openCodePluginRuntimeFor(
	agentConfig agents.Agent,
	options agents.CommandOptions,
	enabled bool,
) *agentctltypes.OpenCodePluginRuntime {
	if !enabled || agentConfig == nil || agentConfig.ID() != agents.OpenCodeACPAgentID {
		return nil
	}
	if options.ManagedRuntimeFamily != managedruntime.OpenCodeFamilyV2 {
		return nil
	}
	if options.ManagedRuntimeSource != managedruntime.OpenCodeSourceManaged &&
		options.ManagedRuntimeSource != managedruntime.OpenCodeSourceNative {
		return nil
	}
	version := options.ManagedRuntimeVersion
	if options.ManagedRuntimeSource == managedruntime.OpenCodeSourceNative && options.NativeRuntimeVersion != "" {
		version = options.NativeRuntimeVersion
	}
	if version != commonopencodeplugin.SupportedRuntimeVersion {
		return nil
	}
	return &agentctltypes.OpenCodePluginRuntime{
		Family:  string(options.ManagedRuntimeFamily),
		Source:  string(options.ManagedRuntimeSource),
		Version: version,
	}
}
