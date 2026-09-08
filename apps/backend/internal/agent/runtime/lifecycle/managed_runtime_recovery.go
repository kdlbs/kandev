package lifecycle

import (
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

// onlineManagedRuntimeArgs returns a copy of a trusted managed-npm launch
// command with only npm's metadata preference changed. The package spec is
// returned separately so cache invalidation can target the exact execution
// tree used by the failed launch.
func onlineManagedRuntimeArgs(args []string, spec agents.ManagedNPMRuntimeSpec) ([]string, string, bool) {
	packageName := strings.TrimSpace(spec.Package)
	if packageName == "" || len(args) < 4 {
		return nil, "", false
	}

	for npxIndex, arg := range args {
		if arg != "npx" || npxIndex+3 >= len(args) {
			continue
		}
		if args[npxIndex+1] != "--yes" || args[npxIndex+2] != "--prefer-offline" {
			continue
		}

		packageSpec := args[npxIndex+3]
		versionPrefix := packageName + "@"
		if !strings.HasPrefix(packageSpec, versionPrefix) {
			return nil, "", false
		}
		if err := managedruntime.ValidateExactPackageSpec(packageSpec); err != nil {
			return nil, "", false
		}

		recovered := append([]string(nil), args...)
		recovered[npxIndex+2] = "--prefer-online"
		return recovered, packageSpec, true
	}

	return nil, "", false
}
