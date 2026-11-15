package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

const (
	managedRuntimePreferOfflineArg = "--prefer-offline"
	managedRuntimePreferOnlineArg  = "--prefer-online"
)

func managedRuntimeSpecForArgs(agent agents.Agent, args []string) (agents.ManagedNPMRuntimeSpec, bool) {
	managed, ok := agent.(agents.ManagedNPMRuntimeAgent)
	if !ok {
		return agents.ManagedNPMRuntimeSpec{}, false
	}
	if openCode, ok := agent.(*agents.OpenCodeACP); ok {
		for _, family := range []managedruntime.OpenCodeFamily{
			managedruntime.OpenCodeFamilyV1,
			managedruntime.OpenCodeFamilyV2,
		} {
			spec, err := openCode.ManagedNPMRuntimeForFamily(family)
			if err != nil {
				continue
			}
			if _, _, found := onlineManagedRuntimeArgs(args, spec); found {
				return spec, true
			}
		}
		return agents.ManagedNPMRuntimeSpec{}, false
	}
	spec := managed.ManagedNPMRuntime()
	_, _, found := onlineManagedRuntimeArgs(args, spec)
	return spec, found
}

// onlineManagedRuntimeArgs returns a copy of a trusted managed-npm launch
// command with only npm's metadata preference changed. The package spec is
// returned separately so cache invalidation can target the exact execution
// tree used by the failed launch.
func onlineManagedRuntimeArgs(args []string, spec agents.ManagedNPMRuntimeSpec) ([]string, string, bool) {
	invocation, ok := spec.MatchRuntimeCommand(args)
	if !ok {
		return nil, "", false
	}
	return invocation.WithNpmPreference(true), invocation.PackageSpec, true
}
