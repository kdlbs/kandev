package lifecycle

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

const (
	envKeyGitHubCredentialBrokerURL  = githubauth.CredentialBrokerURLEnv
	envKeyGitHubCredentialLease      = githubauth.CredentialLeaseEnv
	envKeyGitHubCredentialTaskID     = githubauth.CredentialTaskIDEnv
	envKeyGitHubCredentialSessionID  = githubauth.CredentialSessionIDEnv
	envKeyGitHubCredentialRepository = githubauth.CredentialRepositoryEnv
	envKeyGitHubCredentialOwner      = githubauth.CredentialOwnerEnv
	envKeyGitHubCredentialRepo       = githubauth.CredentialRepoEnv
	envKeyGitHubCredentialHost       = githubauth.CredentialHostEnv
	envKeyGitHubCredentialScopes     = githubauth.CredentialScopesEnv
)

func TestManagedGitHubBrokerEnvUsesStrictAllowlist(t *testing.T) {
	input := map[string]string{
		envKeyGitHubCredentialBrokerURL:  "https://kandev.example/api/v1/github/credentials/resolve",
		envKeyGitHubCredentialLease:      "opaque-lease",
		envKeyGitHubCredentialTaskID:     "task-1",
		envKeyGitHubCredentialSessionID:  "session-1",
		envKeyGitHubCredentialRepository: "repo-1",
		envKeyGitHubCredentialOwner:      "acme",
		envKeyGitHubCredentialRepo:       "widgets",
		envKeyGitHubCredentialHost:       "github.com",
		envKeyGitHubCredentialScopes:     `[{"lease":"opaque-lease"}]`,
		"GIT_TERMINAL_PROMPT":            "0",
		"GIT_CONFIG_COUNT":               "2",
		"GIT_CONFIG_KEY_0":               "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_0":             "!agentctl git-credential",
		"GIT_CONFIG_KEY_1":               "credential.useHttpPath",
		"GIT_CONFIG_VALUE_1":             "true",
		"OPENAI_API_KEY":                 "must-not-reach-agentctl",
		"GITHUB_TOKEN":                   "profile-token-must-not-reach-agentctl",
		"HOME":                           "/control-plane/home",
		"GIT_CONFIG_KEY_99":              "credential.helper",
		"GIT_CONFIG_VALUE_99":            "!ambient-helper",
	}

	got := managedGitHubBrokerEnv(input)
	for _, key := range []string{
		envKeyGitHubCredentialBrokerURL,
		envKeyGitHubCredentialLease,
		envKeyGitHubCredentialScopes,
		"GIT_TERMINAL_PROMPT",
		"GIT_CONFIG_COUNT",
		"GIT_CONFIG_KEY_0",
		"GIT_CONFIG_VALUE_0",
		"GIT_CONFIG_KEY_1",
		"GIT_CONFIG_VALUE_1",
	} {
		if got[key] != input[key] {
			t.Errorf("%s = %q, want %q", key, got[key], input[key])
		}
	}
	for _, key := range []string{"OPENAI_API_KEY", "GITHUB_TOKEN", "HOME", "GIT_CONFIG_KEY_99"} {
		if _, ok := got[key]; ok {
			t.Errorf("unapproved key %s reached remote agentctl env", key)
		}
	}
	launchInput := buildSSHEnvInitScript(got)
	if strings.Contains(launchInput, "must-not-reach-agentctl") ||
		strings.Contains(launchInput, "profile-token-must-not-reach-agentctl") ||
		strings.Contains(launchInput, "ambient-helper") {
		t.Fatalf("remote agentctl stdin leaked unapproved values: %s", launchInput)
	}
}

func TestManagedGitHubBrokerEnvDoesNotActivateForExplicitProfileToken(t *testing.T) {
	env := map[string]string{"GITHUB_TOKEN": "explicit-profile-token"}
	if got := managedGitHubBrokerEnv(env); got != nil {
		t.Fatalf("managedGitHubBrokerEnv() = %#v, want nil", got)
	}
	if hasManagedGitHubBrokerEnv(env) {
		t.Fatal("explicit profile token must not be treated as a managed broker lease")
	}
}
