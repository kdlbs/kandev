package acp

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types"
)

func requireBuiltinMCPDelivery(requested, selected []types.McpServer) error {
	delivered := make(map[string]bool, len(selected))
	for _, server := range selected {
		if server.IsInjectedBuiltin() {
			delivered[server.Name] = true
		}
	}
	for _, server := range requested {
		if server.IsInjectedBuiltin() && !delivered[server.Name] {
			return fmt.Errorf("selected built-in MCP server %q is unsupported by this agent; use an agent with HTTP or SSE MCP support", server.Name)
		}
	}
	return nil
}
