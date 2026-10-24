package mcp

import (
	"slices"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/server"
)

// registerSelectedConfiguration adds only configuration-owned definitions. Shared
// discovery and task tools retain the current surface's schemas and handlers.
func (s *Server) registerSelectedConfiguration() {
	manifest := s.profile.BuiltinPackages
	if manifest == nil || s.profile.Surface != mcpprofile.SurfaceKanbanTask ||
		!slices.Contains(manifest.Packages, mcpprofile.PackageConfiguration) {
		return
	}
	active := s.mcpServer
	s.mcpServer = server.NewMCPServer("configuration-assembly", "1.0.0")
	s.registerConfigWorkflowTools()
	s.registerConfigAgentTools()
	s.registerConfigMcpTools()
	s.registerConfigPromptTools()
	s.registerConfigExecutorTools()
	s.registerConfigSettingsTools()
	s.registerConfigAutomationTools()
	tools := s.mcpServer.ListTools()
	s.mcpServer = active
	for name, tool := range tools {
		if mcpprofile.BuiltinToolPackage(name) == mcpprofile.PackageConfiguration {
			active.AddTool(tool.Tool, tool.Handler)
		}
	}
}
