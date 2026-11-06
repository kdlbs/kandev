package mcp

import (
	"context"
	"errors"
	"slices"

	"github.com/gin-gonic/gin"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/mark3labs/mcp-go/server"
)

// BindInitialProfile is called only under the process manager's pre-start lock.
// It can initialize a workspace helper once; later selection changes require a
// new process and helper instance.
func (s *Server) BindInitialProfile(profile p.Context, port int) error {
	profile = p.Normalize(profile)
	if profile.BuiltinPackages == nil {
		return errors.New("built-in MCP binding is required")
	}
	if _, err := p.MarshalBuiltinBinding(profile); err != nil {
		return err
	}
	if profile.BuiltinPackages.TaskID != s.taskID {
		return errors.New("built-in MCP binding belongs to another task")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.profile.BuiltinPackages != nil {
		if sameProfile(s.profile, profile) {
			return nil
		}
		return errors.New("built-in MCP packages are already bound")
	}
	switch s.profile.Surface {
	case p.SurfaceExternal, p.SurfaceAutomation, p.SurfaceCoordinator, p.SurfaceManagedConversation:
		return errors.New("this helper surface cannot bind task packages")
	}
	s.profile = profile
	s.mode = modeForProfile(profile)
	s.legacyModeCapabilities = slices.Clone(profile.Capabilities)
	s.disableAskQuestion = !profile.HasCapability(p.CapabilityUserQuestion)
	s.mcpProviders = slices.Clone(profile.Providers)
	s.builtinPackages = make(map[p.PackageID]*builtinPackageServer)
	for _, id := range profile.BuiltinPackages.Packages {
		if id != p.PackageCore {
			s.builtinPackages[id] = &builtinPackageServer{registry: s.newBuiltinRegistry(id)}
		}
	}
	s.initializeBuiltinTransports(port)
	s.rebuildTools()
	_ = s.httpServer.Shutdown(context.Background())
	s.httpServer = server.NewStreamableHTTPServer(s.mcpServer, server.WithEndpointPath("/mcp"), server.WithStateful(true), server.WithHTTPContextFunc(s.mcpHTTPContext))
	return nil
}

func (s *Server) serveCurrentStreamableHTTP(c *gin.Context) {
	s.mu.RLock()
	transport := s.httpServer
	s.mu.RUnlock()
	compatibleStreamableHTTPHandler(transport).ServeHTTP(c.Writer, c.Request)
}
