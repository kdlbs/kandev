package lifecycle

import (
	"strconv"

	"github.com/kandev/kandev/internal/githubauth"
)

var managedGitHubBrokerEnvKeys = []string{
	githubauth.CredentialBrokerURLEnv,
	githubauth.CredentialLeaseEnv,
	githubauth.CredentialTaskIDEnv,
	githubauth.CredentialSessionIDEnv,
	githubauth.CredentialRepositoryEnv,
	githubauth.CredentialOwnerEnv,
	githubauth.CredentialRepoEnv,
	githubauth.CredentialHostEnv,
	githubauth.CredentialScopesEnv,
	"GIT_TERMINAL_PROMPT",
}

// managedGitHubBrokerEnv returns only the runtime values needed by the
// broker-backed git helper and gh shim. It deliberately excludes unrelated
// profile and control-plane secrets from the long-lived agentctl process.
func managedGitHubBrokerEnv(env map[string]string) map[string]string {
	if env[githubauth.CredentialBrokerURLEnv] == "" {
		return nil
	}
	result := make(map[string]string, len(managedGitHubBrokerEnvKeys)+1)
	for _, key := range managedGitHubBrokerEnvKeys {
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

func hasManagedGitHubBrokerEnv(env map[string]string) bool {
	return env[githubauth.CredentialBrokerURLEnv] != "" && env[githubauth.CredentialLeaseEnv] != ""
}
