package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"reflect"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
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
	injected, err := config.BuiltinMCPServers(manifest, s.cfg.MCPHost, s.cfg.Port)
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
	paths := []string{"/health", "/sse", "/message", "/mcp"}
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
