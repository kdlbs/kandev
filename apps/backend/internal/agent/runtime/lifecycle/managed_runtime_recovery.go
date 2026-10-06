package lifecycle

import (
	"strings"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

const (
	managedRuntimePreferOfflineArg = "--prefer-offline"
	managedRuntimePreferOnlineArg  = "--prefer-online"
)

// onlineManagedRuntimeArgs validates a trusted managed-npm launch command and
// returns an online-preferred copy plus its exact package spec.
func onlineManagedRuntimeArgs(args []string, spec agents.ManagedNPMRuntimeSpec) ([]string, string, bool) {
	packageName := strings.TrimSpace(spec.Package)
	if packageName == "" || len(args) < 4 {
		return nil, "", false
	}

	for npxIndex, arg := range args {
		if arg != "npx" || npxIndex+5 >= len(args) {
			continue
		}
		if args[npxIndex+1] != "--yes" ||
			(args[npxIndex+2] != managedRuntimePreferOfflineArg && args[npxIndex+2] != managedRuntimePreferOnlineArg) ||
			args[npxIndex+3] != "--prefix" || args[npxIndex+4] != managedruntime.NPMProjectPrefix || npxIndex+5 >= len(args) {
			continue
		}

		packageSpec := args[npxIndex+5]
		versionPrefix := packageName + "@"
		if !strings.HasPrefix(packageSpec, versionPrefix) {
			return nil, "", false
		}
		if err := managedruntime.ValidateExactPackageSpec(packageSpec); err != nil {
			return nil, "", false
		}

		recovered := append([]string(nil), args...)
		recovered[npxIndex+2] = managedRuntimePreferOnlineArg
		return recovered, packageSpec, true
	}

	return nil, "", false
}
