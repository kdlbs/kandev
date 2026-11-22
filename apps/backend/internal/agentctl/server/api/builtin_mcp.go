package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/mcpurl"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"go.uber.org/zap"
)

func (s *Server) builtinSessionMCPServers(requested []types.McpServer) ([]types.McpServer, error) {
	manifest := s.cfg.McpProfile.BuiltinPackages
	if s.mcpServer == nil || !reflect.DeepEqual(s.mcpServer.Profile().BuiltinPackages, manifest) {
		return nil, errors.New("bound built-in MCP package host is unavailable")
	}
	for _, server := range requested {
		if p.ReservedBuiltinRuntimeName(server.Name) {
			return nil, fmt.Errorf("reserved built-in MCP name %q must be renamed", server.Name)
		}
	}
	injected, err := config.BuiltinMCPServersForInstance(manifest, s.cfg.MCPHost, s.cfg.Port, s.cfg.InstanceID, s.cfg.ScopedMCPRoutes)
	if err != nil {
		return nil, err
	}
	servers := make([]types.McpServer, 0, len(injected)+len(requested))
	for _, server := range injected {
		servers = append(servers, types.McpServer{Name: server.Name, Type: server.Type, URL: server.URL, BuiltinPackage: p.BuiltinPackageForRuntimeName(server.Name)})
	}
	return append(servers, requested...), nil
}

func builtinMCPExemptPaths(cfg *config.InstanceConfig) []string {
	return builtinMCPRoutePaths(cfg)
}

func builtinMCPEndpoints() []string {
	paths := []string{"/mcp", "/sse", "/message"}
	for _, descriptor := range p.BuiltinPackageCatalog() {
		id := descriptor.ID
		if id == p.PackageCore {
			continue
		}
		suffix := "/packages/" + string(id)
		for _, route := range []string{"/mcp", "/sse", "/message"} {
			paths = append(paths, route+suffix)
		}
	}
	return paths
}

func builtinMCPRoutePaths(cfg *config.InstanceConfig) []string {
	paths := []string{"/health"}
	for _, endpoint := range builtinMCPEndpoints() {
		path, err := mcpurl.BuiltinPath(cfg.InstanceID, cfg.ScopedMCPRoutes, endpoint)
		if err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

func (s *Server) rejectUnboundBuiltinMCPRoutes() gin.HandlerFunc {
	allowed := make(map[string]struct{})
	for _, path := range builtinMCPRoutePaths(s.cfg) {
		allowed[path] = struct{}{}
	}
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !isBuiltinMCPRequestPath(path) {
			c.Next()
			return
		}
		if _, ok := allowed[path]; ok {
			c.Next()
			return
		}
		if s.builtinMCPRejectionLogLimiter.allow() {
			s.logger.Warn("rejected stale built-in MCP route", zap.String("reason", "wrong_instance_route"), zap.String("route_template", "native_mcp"))
		}
		c.JSON(http.StatusNotFound, gin.H{"code": "stale_mcp_endpoint", "hint": "Use the MCP endpoint provided by this agent."})
		c.Abort()
	}
}

func isBuiltinMCPRequestPath(path string) bool {
	if isBuiltinMCPEndpointPath(path) {
		return true
	}
	if !strings.HasPrefix(path, mcpurl.InstanceRoutePrefix) {
		return false
	}
	instanceAndEndpoint := strings.TrimPrefix(path, mcpurl.InstanceRoutePrefix)
	separator := strings.IndexByte(instanceAndEndpoint, '/')
	return separator > 0 && isBuiltinMCPEndpointPath(instanceAndEndpoint[separator:])
}

func isBuiltinMCPEndpointPath(path string) bool {
	if mcpurl.IsBuiltinEndpoint(path) {
		return true
	}
	parts := strings.Split(path, "/")
	return len(parts) == 4 && parts[0] == "" &&
		(parts[1] == "mcp" || parts[1] == "sse" || parts[1] == "message") &&
		parts[2] == "packages" && parts[3] != "" && !strings.ContainsAny(parts[3], "?#%")
}

func (s *Server) handlePrepareBuiltinMCP(c *gin.Context) {
	var profile p.Context
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16384))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&profile) != nil || decoder.Decode(new(any)) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid built-in MCP profile"})
		return
	}
	err := s.procMgr.PrepareBuiltinProfile(profile, func(value p.Context) error { return s.mcpServer.BindInitialProfile(value, s.cfg.Port) })
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.mcpServer.Profile())
}
