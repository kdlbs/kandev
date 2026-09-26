package lifecycle

import (
	"strconv"

	"github.com/kandev/kandev/internal/githubauth"
)

// managedGitCredentialBrokerEnvKeys retain the existing agentctl environment
// contract while carrying provider-neutral opaque leases and helper scopes.
// None of these values is a Git access token.
var managedGitCredentialBrokerEnvKeys = []string{
	githubauth.CredentialBrokerURLEnv,
	githubauth.CredentialLeaseEnv,
	githubauth.CredentialReissueCapabilityEnv,
	githubauth.CredentialTaskIDEnv,
	githubauth.CredentialSessionIDEnv,
	githubauth.CredentialRepositoryEnv,
	githubauth.CredentialOwnerEnv,
	githubauth.CredentialRepoEnv,
	githubauth.CredentialHostEnv,
	githubauth.CredentialScopesEnv,
	"GIT_TERMINAL_PROMPT",
}

// managedGitCredentialBrokerEnv returns only runtime values needed by the
// broker-backed git helper and gh shim. It deliberately excludes unrelated
// profile and control-plane secrets from the long-lived agentctl process.
func managedGitCredentialBrokerEnv(env map[string]string) map[string]string {
	if env[githubauth.CredentialBrokerURLEnv] == "" {
		return nil
	}
	result := make(map[string]string, len(managedGitCredentialBrokerEnvKeys)+1)
	for _, key := range managedGitCredentialBrokerEnvKeys {
		if value := env[key]; value != "" {
			result[key] = value
		}
	}
	copyIndexedGitConfig(env, result)
	return result
}

func copyIndexedGitConfig(source, target map[string]string) {
	count, err := strconv.Atoi(source["GIT_CONFIG_COUNT"])
	if err != nil || count < 1 || count > 64 {
		return
	}
	target["GIT_CONFIG_COUNT"] = strconv.Itoa(count)
	for index := range count {
		keyName := "GIT_CONFIG_KEY_" + strconv.Itoa(index)
		valueName := "GIT_CONFIG_VALUE_" + strconv.Itoa(index)
		if key := source[keyName]; key != "" {
			target[keyName] = key
			target[valueName] = source[valueName]
		}
	}
}

func hasManagedGitCredentialBrokerEnv(env map[string]string) bool {
	return env[githubauth.CredentialBrokerURLEnv] != "" && env[githubauth.CredentialLeaseEnv] != ""
}

// Deprecated compatibility wrappers keep existing lifecycle call sites and
// agentctl contract tests stable while their behavior is provider-neutral.
func managedGitHubBrokerEnv(env map[string]string) map[string]string {
	return managedGitCredentialBrokerEnv(env)
}

func hasManagedGitHubBrokerEnv(env map[string]string) bool {
	return hasManagedGitCredentialBrokerEnv(env)
}
