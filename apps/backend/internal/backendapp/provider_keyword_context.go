package backendapp

import (
	"context"
	"os"
	"path/filepath"

	"github.com/kandev/kandev/internal/agent/registry"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

type keywordExecutionLookup interface {
	GetExecutionBySessionID(string) (*agentruntime.AgentExecution, bool)
}

// These overrides make the backend's existing billing detector inapplicable to
// the launch. Only key presence is inspected, never credential values or files.
var keywordCredentialOverrideKeys = []string{
	"CLAUDE_CONFIG_DIR", "CODEX_HOME", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
	"CLAUDE_CODE_OAUTH_TOKEN", "ANTHROPIC_BASE_URL", "OPENAI_API_KEY", "CODEX_API_KEY", "OPENAI_BASE_URL",
}

func keywordCredentialBindingResolver(manager keywordExecutionLookup, agents *registry.Registry) func(context.Context, string, *settingsmodels.AgentProfile, string) dynamic.CredentialBindingDescriptor {
	return func(_ context.Context, sessionID string, profile *settingsmodels.AgentProfile, agentName string) dynamic.CredentialBindingDescriptor {
		execution, ok := manager.GetExecutionBySessionID(sessionID)
		if !ok || execution == nil || execution.AgentID != agentName || !execution.LaunchWrapper.IsZero() {
			return dynamic.CredentialBindingDescriptor{}
		}
		agent, ok := agents.Get(agentName)
		if !ok {
			return dynamic.CredentialBindingDescriptor{}
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return dynamic.CredentialBindingDescriptor{}
		}
		for _, key := range []string{"HOME", "USERPROFILE"} {
			if value, exists := os.LookupEnv(key); exists && !sameKeywordCredentialHome(value, home) {
				return dynamic.CredentialBindingDescriptor{}
			}
		}
		for _, key := range keywordCredentialOverrideKeys {
			if _, exists := os.LookupEnv(key); exists {
				return dynamic.CredentialBindingDescriptor{}
			}
		}
		return keywordBindingFromLaunch(execution.ExecutorType, execution.RuntimeEnvironment(), profile, home,
			func() string { return string(agent.BillingType()) })
	}
}

func keywordBindingFromLaunch(executorType string, env map[string]string, profile *settingsmodels.AgentProfile, home string, billingType func() string) dynamic.CredentialBindingDescriptor {
	if !keywordLaunchUsesHostCredentials(executorType, env, home) || !keywordProfileUsesHostCredentials(profile, home) {
		return dynamic.CredentialBindingDescriptor{}
	}
	// Reuse existing registered billing detection only for a proven local launch
	// using the same default credential home. Other bindings remain unknown.
	method := billingType()
	if method != "subscription" && method != "api_key" {
		return dynamic.CredentialBindingDescriptor{}
	}
	return dynamic.CredentialBindingDescriptor{
		Version: 1, AgentFamilyID: profile.AgentID, AuthenticationMethod: method,
		CredentialSourceKind: "agent_credentials", CredentialLocator: filepath.Clean(home),
		ExecutorNamespace: hostSourceLocal, AuthorizationScope: "agent_runtime",
	}
}

func keywordLaunchUsesHostCredentials(executorType string, env map[string]string, home string) bool {
	if (executorType != hostSourceLocal && executorType != "worktree" && executorType != legacyLocalPCExecutor) || env == nil || !filepath.IsAbs(home) {
		return false
	}
	for _, key := range []string{"HOME", "USERPROFILE"} {
		if value, exists := env[key]; exists && !sameKeywordCredentialHome(value, home) {
			return false
		}
	}
	for _, key := range keywordCredentialOverrideKeys {
		if _, exists := env[key]; exists {
			return false
		}
	}
	return true
}

func keywordProfileUsesHostCredentials(profile *settingsmodels.AgentProfile, home string) bool {
	if profile == nil || profile.CommandPrefix != "" || len(profile.CLIFlags) != 0 ||
		profile.ProviderKind != "" || profile.ProviderAPIKeySecretID != "" {
		return false
	}
	for _, variable := range profile.EnvVars {
		if variable.SecretID != "" || (variable.Key != "HOME" && variable.Key != "USERPROFILE") || !sameKeywordCredentialHome(variable.Value, home) {
			return false
		}
	}
	return true
}

func sameKeywordCredentialHome(value, home string) bool {
	return filepath.IsAbs(value) && filepath.Clean(value) == filepath.Clean(home)
}
