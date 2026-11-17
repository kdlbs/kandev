package config

import (
	"errors"
	"github.com/kandev/kandev/internal/common/mcpurl"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const providerMCPHTTPTransport = "http"

// ProviderMCPConfig is private provider attachment state. Workspace process
// environments and public configuration/status responses must not receive it.
type ProviderMCPConfig struct {
	Scope      p.ProviderMCPScope
	Credential string
}

// ProviderBuiltinMCPServers materializes one authenticated HTTP attachment per
// selected package. It never reuses an unscoped endpoint or instance credential.
func ProviderBuiltinMCPServers(profile p.Context, provider ProviderMCPConfig, host string, port int) ([]McpServerConfig, error) {
	return ProviderBuiltinMCPServersForInstance(profile, provider, host, port, "", false)
}

// ProviderBuiltinMCPServersForInstance retains the helper route boundary while
// assigning a private provider-generation credential.
func ProviderBuiltinMCPServersForInstance(profile p.Context, provider ProviderMCPConfig, host string, port int, instanceID string, scopedRoutes bool) ([]McpServerConfig, error) {
	if err := provider.Scope.Validate(); err != nil {
		return nil, err
	}
	digest, err := p.BuiltinBindingDigest(profile)
	if err != nil {
		return nil, err
	}
	if digest != provider.Scope.ProfileDigest || len(provider.Credential) < 32 || len(provider.Credential) > 256 || port <= 0 {
		return nil, errors.New("provider MCP binding configuration is invalid")
	}
	prefix, err := mcpurl.RoutePrefix(instanceID, scopedRoutes)
	if err != nil {
		return nil, err
	}
	if scopedRoutes && provider.Scope.ExecutionID != instanceID {
		return nil, errors.New("provider MCP scope belongs to another helper instance")
	}
	origin, err := mcpurl.OriginURL(host, port)
	if err != nil {
		return nil, err
	}
	servers, err := BuiltinMCPServers(profile.BuiltinPackages, host, port)
	if err != nil {
		return nil, err
	}
	scoped := make([]McpServerConfig, 0, len(servers)/2)
	for _, server := range servers {
		if server.Type != providerMCPHTTPTransport {
			continue
		}
		suffix := ""
		if id := p.BuiltinPackageForRuntimeName(server.Name); id != p.PackageCore {
			suffix = "/packages/" + string(id)
		}
		server.URL = origin + prefix + "/mcp/generations/" + provider.Scope.BindingID + suffix
		server.Headers = map[string]string{"Authorization": "Bearer " + provider.Credential}
		scoped = append(scoped, server)
	}
	return scoped, nil
}
