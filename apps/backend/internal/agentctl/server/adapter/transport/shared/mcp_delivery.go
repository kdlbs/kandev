package shared

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// RequireBuiltinMCPDelivery verifies every host-injected package has an
// admitted wire transport or an execution-bound project delivery record.
func RequireBuiltinMCPDelivery(requested, selected []types.McpServer, project *types.ProjectMCPDelivery) error {
	if project != nil && project.Validate() != nil {
		project = nil
	}
	delivered := make(map[p.PackageID]bool, len(selected))
	for _, server := range selected {
		if server.IsInjectedBuiltin() {
			delivered[server.BuiltinPackage] = true
		}
	}
	for _, server := range requested {
		if server.IsInjectedBuiltin() && !delivered[server.BuiltinPackage] && !project.Delivers(server.BuiltinPackage) {
			return fmt.Errorf("selected built-in MCP server %q is unsupported by this agent; use an agent with HTTP or SSE MCP support", server.Name)
		}
	}
	return nil
}
