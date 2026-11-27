package api

import (
	"context"
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/sysprompt"
)

func (s *Server) resetCanvasGuidance(attemptID string, servers []types.McpServer) {
	var pending *string
	if s.mcpServer != nil && slices.ContainsFunc(servers, func(server types.McpServer) bool { return server.Name == kandevMcpServerName }) {
		profile := s.mcpServer.Profile()
		if profile.Surface == mcpprofile.SurfaceKanbanTask && profile.HasCapability(mcpprofile.CapabilityCanvas) {
			pending = &attemptID
		}
	}
	s.pendingCanvasGuidance.Store(pending)
}

func (s *Server) promptWithCanvasGuidance(ctx context.Context, adpt adapter.AgentAdapter, req PromptRequest, pending *string) error {
	text := strings.TrimSpace(req.Text)
	if pending == nil || req.Steer || strings.HasPrefix(text, "/") || (text == "" && len(req.Attachments) == 0) {
		return promptOrSteer(ctx, adpt, req)
	}
	req.Text = sysprompt.WithCanvasGuidance(req.Text)
	err := promptOrSteer(ctx, adpt, req)
	if err == nil {
		// A late completion cannot acknowledge a newer session attachment.
		s.pendingCanvasGuidance.CompareAndSwap(pending, nil)
	}
	return err
}
