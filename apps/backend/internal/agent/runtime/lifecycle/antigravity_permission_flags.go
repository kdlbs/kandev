package lifecycle

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/antigravityruntime"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func antigravitySkipPermissionsAllowed(selection agents.LocalRuntimeSelection, req *LaunchRequest) bool {
	if selection.Variant != acpcompat.AntigravityRuntimeCLI || !selection.SupportsDangerouslySkipPermissions {
		return false
	}
	if req == nil || req.McpProfile == nil {
		return false
	}
	profile := req.McpProfile
	if profile.ManagedToolPolicy != nil || profile.CoordinatorToolPolicy != nil {
		return false
	}
	switch profile.Surface {
	case mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask:
		return true
	default:
		return false
	}
}

func filterAntigravitySkipPermissions(args []string, selection agents.LocalRuntimeSelection, req *LaunchRequest) []string {
	return antigravityruntime.FilterDangerouslySkipPermissions(
		args,
		antigravitySkipPermissionsAllowed(selection, req),
	)
}
