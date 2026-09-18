package api

import "github.com/kandev/kandev/internal/agentctl/types"

// sessionMcpServers keeps the managed assistant broker as the sole attachment
// across new sessions, restored sessions and context resets.
func (s *Server) sessionMcpServers(requested []types.McpServer) []types.McpServer {
	if s.cfg.AssistantRestricted() {
		servers := make([]types.McpServer, 0, len(s.cfg.McpServers))
		for _, server := range s.cfg.McpServers {
			servers = append(servers, types.McpServer{
				Name: server.Name, Type: server.Type, Command: server.Command,
				Args: server.Args, Env: server.Env, URL: server.URL, Headers: server.Headers,
			})
		}
		return servers
	}
	if s.mcpServer != nil {
		return s.injectKandevMcpServers(requested)
	}
	return requested
}
