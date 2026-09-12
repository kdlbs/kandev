package backendapp

import (
	"net"
	"net/url"
	"strings"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

const teamClaudeBaseURLEnv = "ANTHROPIC_BASE_URL"
const usageProxyVendorEnv = "USAGE_PROXY_VENDOR"
const claudeACPAgentID = "claude-acp"

// usageProxyResolver recognizes a profile that is served by a proxy and
// returns a proxy-native usage client. Resolvers own their protocol and never
// cause the generic usage adapter to branch on a particular proxy name.
//
// New proxies add a resolver and register it in defaultUsageProxyResolver;
// direct provider usage clients remain untouched.
type usageProxyResolver interface {
	Resolve(profile *settingsmodels.AgentProfile) (client agentusage.ProviderUsageClient, cacheKey string, ok bool)
}

type usageProxyResolvers []usageProxyResolver

func (resolvers usageProxyResolvers) Resolve(profile *settingsmodels.AgentProfile) (agentusage.ProviderUsageClient, string, bool) {
	for _, resolver := range resolvers {
		if resolver == nil {
			continue
		}
		if client, cacheKey, ok := resolver.Resolve(profile); ok {
			return client, cacheKey, true
		}
	}
	return nil, "", false
}

func defaultUsageProxyResolver() usageProxyResolver {
	return usageProxyResolvers{teamClaudeUsageResolver{}}
}

// teamClaudeUsageResolver recognizes an explicitly selected local TeamClaude
// control plane. The profile must set USAGE_PROXY_VENDOR=teamclaude; the
// upstream URL alone is never treated as proof of a TeamClaude endpoint.
// Discovery accepts loopback endpoints only: Kandev never sends a profile's
// environment or proxy key to a remote control plane while rendering usage.
type teamClaudeUsageResolver struct{}

func (teamClaudeUsageResolver) Resolve(profile *settingsmodels.AgentProfile) (agentusage.ProviderUsageClient, string, bool) {
	statusURL, ok := teamClaudeStatusURL(profile)
	if !ok {
		return nil, "", false
	}
	return agentusage.NewTeamClaudeUsageClient(statusURL), agentusage.CacheKey("teamclaude", statusURL), true
}

func teamClaudeStatusURL(profile *settingsmodels.AgentProfile) (string, bool) {
	if profile == nil || profile.AgentID != claudeACPAgentID {
		return "", false
	}
	if profileEnvValue(profile, usageProxyVendorEnv) != "teamclaude" {
		return "", false
	}
	for _, env := range profile.EnvVars {
		if env.Key != teamClaudeBaseURLEnv || strings.TrimSpace(env.Value) == "" {
			continue
		}
		base, err := url.Parse(strings.TrimSpace(env.Value))
		if err != nil || base.Scheme != "http" || base.Host == "" || base.User != nil || !isLoopbackHost(base.Hostname()) {
			return "", false
		}
		// TeamClaude always serves its control-plane status at
		// /teamclaude/status regardless of any path prefix in the configured
		// ANTHROPIC_BASE_URL; only the host:port is used.
		return "http://" + base.Host + "/teamclaude/status", true
	}
	return "", false
}

func profileEnvValue(profile *settingsmodels.AgentProfile, key string) string {
	for _, env := range profile.EnvVars {
		if env.Key == key {
			return strings.ToLower(strings.TrimSpace(env.Value))
		}
	}
	return ""
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
