package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
)

// A fresh launch has no per-run runtime_env overlay. The managed credential
// broker values issued for that launch live only in the runtime snapshot, so
// configuring the agent must keep them alongside the managed helper entry
// that expands KANDEV_GITHUB_CREDENTIAL_HELPER_PATH.
func TestConfigureAndStartAgentKeepsLaunchManagedGitCredentials(t *testing.T) {
	mgr := newTestManager(t)
	var configuredEnv map[string]string
	var replaced bool
	client := newConfigureCaptureAgentctlClient(t, newTestLogger(), &configuredEnv, &replaced)
	execution := &AgentExecution{
		ID:            "exec-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		AgentCommand:  "npx -y @agentclientprotocol/codex-acp",
		WorkspacePath: t.TempDir(),
		metadata:      map[string]interface{}{},
		agentctl:      client,
	}
	execution.setRuntimeEnvironment(map[string]string{
		githubauth.CredentialHelperPathEnv: "/opt/kandev/agentctl",
		githubauth.CredentialBrokerURLEnv:  "http://127.0.0.1:38429/broker",
		githubauth.CredentialLeaseEnv:      "lease-1",
		"GIT_CONFIG_COUNT":                 "2",
		"GIT_CONFIG_KEY_0":                 "credential.https://git.example.com.helper",
		"GIT_CONFIG_VALUE_0":               "",
		"GIT_CONFIG_KEY_1":                 "credential.https://git.example.com.helper",
		"GIT_CONFIG_VALUE_1":               githubauth.ManagedGitCredentialHelper,
	})

	if _, err := mgr.configureAndStartAgent(context.Background(), execution, "never"); err != nil {
		t.Fatalf("configureAndStartAgent() error = %v", err)
	}
	for _, key := range []string{
		githubauth.CredentialHelperPathEnv,
		githubauth.CredentialBrokerURLEnv,
		githubauth.CredentialLeaseEnv,
	} {
		if configuredEnv[key] == "" {
			t.Errorf("%s missing from configured env; managed helper entry would expand to an empty command", key)
		}
	}
	if configuredEnv["GIT_CONFIG_VALUE_1"] != githubauth.ManagedGitCredentialHelper {
		t.Errorf("managed helper entry = %q, want %q", configuredEnv["GIT_CONFIG_VALUE_1"], githubauth.ManagedGitCredentialHelper)
	}
	if replaced {
		t.Fatal("overlay-free launch must use agentctl's overlay configuration mode")
	}
}

// An empty runtime_env is an explicit overlay and must keep its composition semantics.
func TestHasRuntimeEnvOverlayDistinguishesAbsentAndEmptyOverlay(t *testing.T) {
	if hasRuntimeEnvOverlay(nil) {
		t.Fatal("nil metadata must not report an overlay")
	}
	if hasRuntimeEnvOverlay(map[string]interface{}{}) {
		t.Fatal("metadata without runtime_env must not report an overlay")
	}
	if !hasRuntimeEnvOverlay(map[string]interface{}{
		"runtime_env": map[string]string{},
	}) {
		t.Fatal("an explicitly delivered empty runtime_env must report an overlay")
	}
}
