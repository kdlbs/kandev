package mcp

import (
	"context"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type builtinPackageServer struct {
	registry *server.MCPServer
	http     *server.StreamableHTTPServer
	sse      *server.SSEServer
}

func builtinToolPackage(name string) mcpprofile.PackageID {
	return mcpprofile.BuiltinToolPackage(name)
}

func (s *Server) initializeBuiltinPackages() {
	manifest := s.profile.BuiltinPackages
	if manifest == nil {
		return
	}
	s.builtinPackages = make(map[mcpprofile.PackageID]*builtinPackageServer)
	if manifest.Validate() != nil || manifest.Surface != s.profile.Surface || manifest.TaskID != s.taskID {
		s.mcpServer.SetTools()
		return
	}
	for _, id := range manifest.Packages {
		if id == mcpprofile.PackageCore {
			continue
		}
		s.builtinPackages[id] = &builtinPackageServer{registry: s.newBuiltinRegistry(id)}
	}
	tools := s.mcpServer.ListTools()
	selected := make([]server.ServerTool, 0, len(tools))
	for _, entry := range tools {
		selected = append(selected, *entry)
	}
	s.publishBuiltinTools(selected)
}

func (s *Server) publishBuiltinTools(tools []server.ServerTool) {
	if s.profile.BuiltinPackages == nil {
		s.mcpServer.SetTools(tools...)
		return
	}
	grouped := make(map[mcpprofile.PackageID][]server.ServerTool)
	manifest := s.profile.BuiltinPackages
	if manifest.Validate() == nil && manifest.Surface == s.profile.Surface && manifest.TaskID == s.taskID {
		for _, tool := range tools {
			id := s.registeredToolPackage(tool.Tool.Name)
			if slices.Contains(manifest.Packages, id) {
				grouped[id] = append(grouped[id], tool)
			}
		}
	}
	s.mcpServer.SetTools(grouped[mcpprofile.PackageCore]...)
	for id, view := range s.builtinPackages {
		view.registry.SetTools(grouped[id]...)
	}
}

func (s *Server) registeredToolPackage(name string) mcpprofile.PackageID {
	if id := builtinToolPackage(name); id != "" {
		return id
	}
	s.pluginToolsMu.Lock()
	defer s.pluginToolsMu.Unlock()
	for _, definition := range s.pluginTools.Tools {
		if definition.ExposedName == name && pluginToolSupportsSurface(definition, string(s.profile.Surface)) {
			return mcpprofile.PackageCore
		}
	}
	return ""
}

func (s *Server) initializeBuiltinTransports(port int) {
	for id, view := range s.builtinPackages {
		suffix := "/packages/" + string(id)
		view.http = server.NewStreamableHTTPServer(view.registry, server.WithEndpointPath("/mcp"+suffix), server.WithStateful(true), server.WithHTTPContextFunc(s.mcpHTTPContext))
		view.sse = server.NewSSEServer(view.registry, server.WithBaseURL(s.sseBaseURLForPort(port)), server.WithSSEEndpoint("/sse"+suffix), server.WithMessageEndpoint("/message"+suffix))
	}
}

func (s *Server) registerBuiltinRoutes(router gin.IRouter) {
	for _, descriptor := range mcpprofile.BuiltinPackageCatalog() {
		id := descriptor.ID
		if id == mcpprofile.PackageCore {
			continue
		}
		suffix := "/packages/" + string(id)
		router.Any("/mcp"+suffix, s.serveBuiltinPackage(id, "http"))
		router.GET("/sse"+suffix, s.serveBuiltinPackage(id, "sse"))
		router.POST("/message"+suffix, s.serveBuiltinPackage(id, "message"))
	}
}

func (s *Server) serveBuiltinPackage(id mcpprofile.PackageID, transport string) gin.HandlerFunc {
	return func(c *gin.Context) {
		s.mu.RLock()
		view := s.builtinPackages[id]
		s.mu.RUnlock()
		if view == nil {
			c.Status(http.StatusNotFound)
			return
		}
		switch transport {
		case "http":
			compatibleStreamableHTTPHandler(view.http).ServeHTTP(c.Writer, c.Request)
		case "sse":
			view.sse.SSEHandler().ServeHTTP(c.Writer, c.Request)
		case "message":
			view.sse.MessageHandler().ServeHTTP(c.Writer, c.Request)
		}
	}
}

func (s *Server) closeBuiltinPackages(ctx context.Context) {
	for _, view := range s.builtinPackages {
		if view.http != nil {
			_ = view.http.Shutdown(ctx)
		}
		if view.sse != nil {
			_ = view.sse.Shutdown(ctx)
		}
	}
}

// BuiltinCatalog returns expected canonical tool names for the bound packages.
// Registration is configuration evidence, not an observation of a connection.
func (s *Server) BuiltinCatalog() map[mcpprofile.PackageID][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.profile.BuiltinPackages == nil {
		return nil
	}
	result := make(map[mcpprofile.PackageID][]string, len(s.builtinPackages)+1)
	result[mcpprofile.PackageCore] = registeredPackageNames(s.mcpServer)
	for id, view := range s.builtinPackages {
		result[id] = registeredPackageNames(view.registry)
	}
	return result
}

func registeredPackageNames(registry *server.MCPServer) []string {
	tools := registry.ListTools()
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (s *Server) newBuiltinRegistry(id mcpprofile.PackageID) *server.MCPServer {
	hooks := new(server.Hooks)
	registry := server.NewMCPServer("kandev-"+string(id), "1.0.0", server.WithToolCapabilities(true), server.WithHooks(hooks))
	hooks.AddBeforeCallTool(func(_ context.Context, requestID any, request *mcp.CallToolRequest) {
		s.restorePackageToolName(registry, request)
		stampTransportRequestID(request, requestID)
	})
	hooks.AddAfterListTools(func(ctx context.Context, _ any, _ *mcp.ListToolsRequest, result *mcp.ListToolsResult) {
		s.presentPackageToolNames(registry, result)
		s.observeMCPToolsListForRequest(ctx, result.Tools, "kandev-"+string(id))
	})
	s.addBuiltinObservationHooks(hooks, "kandev-"+string(id))
	return registry
}
