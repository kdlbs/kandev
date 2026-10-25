package mcp

import (
	"context"
	"slices"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func builtinEvidencePackage(manifest *p.BuiltinManifest, name string) p.PackageID {
	id := p.BuiltinPackageForRuntimeName(name)
	if manifest != nil && slices.Contains(manifest.Packages, id) {
		return id
	}
	return ""
}

func mcpEvidenceServerName(names ...string) string {
	if len(names) > 0 {
		return names[0]
	}
	return "kandev"
}

func mcpAttachmentKey(connectionID string, names ...string) string {
	name := mcpEvidenceServerName(names...)
	if name == "kandev" {
		return connectionID
	}
	return name + "\x00" + connectionID
}

func (s *Server) addBuiltinObservationHooks(hooks *server.Hooks, name string) {
	hooks.AddBeforeAny(func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any) {
		if server.IsModernRequest(ctx) {
			s.observeMCPRequest(ctx, streams.MCPAttachmentEvidenceProtocolAccepted, 0, "", name)
		}
	})
	hooks.AddOnRegisterSession(func(_ context.Context, session server.ClientSession) {
		s.registerMCPConnection(session.SessionID(), name)
	})
	hooks.AddAfterInitialize(func(ctx context.Context, _ any, _ *mcp.InitializeRequest, _ *mcp.InitializeResult) {
		s.observeMCPRequest(ctx, streams.MCPAttachmentEvidenceInitializeObserved, 0, "", name)
	})
	hooks.AddAfterCallTool(func(ctx context.Context, _ any, _ *mcp.CallToolRequest, _ any) {
		s.observeMCPRequest(ctx, streams.MCPAttachmentEvidenceToolCallObserved, 0, "", name)
	})
	hooks.AddOnError(func(ctx context.Context, _ any, _ mcp.MCPMethod, _ any, err error) {
		s.observeMCPRequest(ctx, streams.MCPAttachmentEvidenceExplicitError, 0, err.Error(), name)
	})
	hooks.AddOnUnregisterSession(func(_ context.Context, session server.ClientSession) {
		s.unregisterMCPConnection(session.SessionID(), name)
	})
}
