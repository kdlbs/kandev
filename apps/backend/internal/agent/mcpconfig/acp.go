package mcpconfig

import "github.com/kandev/kandev/internal/agentctl/types"

// ToACPServers converts resolved MCP servers into ACP server list.
// Supports stdio, SSE, HTTP, and streamable HTTP transports.
func ToACPServers(resolved []ResolvedServer) []types.McpServer {
	servers := make([]types.McpServer, 0, len(resolved))
	for _, server := range resolved {
		base := types.McpServer{
			Name: server.Name, DefinitionID: server.DefinitionID,
			DefinitionRevision: server.DefinitionRevision,
			Origins:            acpOrigins(server.Origins),
		}
		switch server.Type {
		case ServerTypeStdio:
			base.Type = string(ServerTypeStdio)
			base.Command = server.Command
			base.Args = append([]string{}, server.Args...)
			base.Env = cloneStringMap(server.Env)
		case ServerTypeSSE:
			base.Type = string(ServerTypeSSE)
			base.URL = server.URL
			base.Headers = cloneStringMap(server.Headers)
		case ServerTypeHTTP:
			base.Type = string(ServerTypeHTTP)
			base.URL = server.URL
			base.Headers = cloneStringMap(server.Headers)
		case ServerTypeStreamableHTTP:
			base.Type = "streamable_http"
			base.URL = server.URL
			base.Headers = cloneStringMap(server.Headers)
		default:
			continue
		}
		servers = append(servers, base)
	}
	return servers
}

func acpOrigins(origins []SelectionOrigin) []types.McpServerOrigin {
	result := make([]types.McpServerOrigin, 0, len(origins))
	for _, origin := range origins {
		result = append(result, types.McpServerOrigin{
			Scope: string(origin.Scope), WorkspaceID: origin.WorkspaceID, OwnerID: origin.OwnerID,
		})
	}
	return result
}
