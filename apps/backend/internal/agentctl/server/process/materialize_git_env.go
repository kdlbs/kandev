package process

import (
	"errors"
	"sort"

	"github.com/kandev/kandev/internal/gitconfigenv"
	"github.com/kandev/kandev/internal/githubauth"
)

// MaterializationGitEnvironment returns one detached current-instance snapshot.
// Only installed tooling may fall back to startup state; authorization may not.
func (m *Manager) MaterializationGitEnvironment() ([]string, error) {
	env := environmentMapFromSlice(m.agentEnvSnapshot())
	if _, err := gitconfigenv.EnvironmentEntries(env); err != nil {
		return nil, errors.New("invalid materialization Git environment")
	}
	if env[githubauth.CredentialBrokerURLEnv] != "" && env[githubauth.CredentialLeaseEnv] != "" && env[githubauth.CredentialHelperPathEnv] == "" {
		if m.materializeGitHelperPath == "" {
			return nil, errors.New("materialization Git helper unavailable")
		}
		env[githubauth.CredentialHelperPathEnv] = m.materializeGitHelperPath
	}
	keys := make([]string, 0, len(env))
	for key := range env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+env[key])
	}
	return result, nil
}
