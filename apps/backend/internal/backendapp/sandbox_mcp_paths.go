package backendapp

import (
	"github.com/kandev/kandev/internal/agent/runtime"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/common/sandbox"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func sandboxMCPPathsForExecution(execution *runtime.AgentExecution) ([]string, error) {
	value, bound := execution.MetadataSnapshot()[p.BuiltinBindingMetadataKey]
	if !bound {
		return nil, nil
	}
	profile, err := p.ParseBuiltinBindingMetadata(value)
	if err != nil {
		return nil, policy.ErrIneligible
	}
	manifest := profile.BuiltinPackages
	if manifest.TaskID != execution.TaskID || manifest.WorkspaceID != execution.WorkspaceID {
		return nil, policy.ErrIneligible
	}
	paths := []string{sandbox.MCPRootPath}
	for _, id := range manifest.Packages {
		if id != p.PackageCore {
			paths = append(paths, sandbox.MCPPackagePathPrefix+string(id))
		}
	}
	if len(paths) == 1 {
		return nil, nil
	}
	return sandbox.CanonicalMCPPaths(paths)
}
