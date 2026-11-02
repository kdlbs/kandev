package config

import (
	"errors"
	"os"

	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/githubauth"
)

// CollectInstanceAgentEnvWithError inherits runtime tools and user configuration,
// while managed Git authorization must come from the instance request.
func CollectInstanceAgentEnvWithError(additional map[string]string) ([]string, error) {
	return collectAgentEnvWithError(additional, true)
}

// InstalledGitCredentialHelperPath captures runtime-installed tooling separately
// from session authorization. Agentctl itself implements the helper command.
func InstalledGitCredentialHelperPath() string {
	if path := os.Getenv(githubauth.CredentialHelperPathEnv); path != "" {
		return path
	}
	path, _ := os.Executable()
	return path
}

func withoutInheritedManagedGitAuthorization(env map[string]string) (map[string]string, error) {
	managed := env[githubauth.CredentialBrokerURLEnv] != "" || env[githubauth.CredentialLeaseEnv] != ""
	DeactivateManagedGitTools(env, env[githubauth.CredentialCLIShimDirEnv], env[githubauth.CredentialCLIBashEnvEnv])
	filtered, err := gitconfigenv.Filter(env, func(index int, entries []gitconfigenv.Entry) bool {
		return !githubauth.IsManagedGitCredentialConfigEntry(index, entries, managed) && !githubauth.IsHostGitHubCredentialHelperEntry(entries[index].Key, entries[index].Value)
	})
	if err != nil {
		return nil, errors.New("invalid inherited Git configuration")
	}
	helper, shim, bash := filtered[githubauth.CredentialHelperPathEnv], filtered[githubauth.CredentialCLIShimDirEnv], filtered[githubauth.CredentialCLIBashEnvEnv]
	githubauth.RemoveManagedCredentialEnvironment(filtered)
	for key, value := range map[string]string{githubauth.CredentialHelperPathEnv: helper, githubauth.CredentialCLIShimDirEnv: shim, githubauth.CredentialCLIBashEnvEnv: bash} {
		if value != "" {
			filtered[key] = value
		}
	}
	return filtered, nil
}
