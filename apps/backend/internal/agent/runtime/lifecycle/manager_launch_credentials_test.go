package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

func TestConfigureAndStartAgentRejectsUnsupportedOpenCodeCredentialRuntime(t *testing.T) {
	mgr := newTestManager(t)
	var configuredEnv map[string]string
	client := newConfigureCaptureAgentctlClient(t, newTestLogger(), &configuredEnv)
	execution := &AgentExecution{
		ID: "exec-opencode", AgentID: "opencode-acp", RuntimeName: agentruntime.RuntimeKubernetes,
		OpenCodeRuntime: managedruntime.OpenCodeRuntime{Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged, Version: "2.0.18"},
		AgentArgs:       []string{"npx", "--yes", "--prefer-offline", "--prefix", managedruntime.NPMProjectPrefix, "@opencode/cli@2.0.18", "acp"},
		WorkspacePath:   t.TempDir(), agentctl: client,
		metadata: map[string]interface{}{"remote_credentials": `["agent:opencode-acp:files:0"]`},
	}
	if _, err := mgr.configureAndStartAgent(context.Background(), execution); !errors.Is(err, managedruntime.ErrOpenCodeCredentialRuntime) {
		t.Fatalf("launch error = %v, want unsupported OpenCode credential runtime", err)
	}
}

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

	if _, err := mgr.configureAndStartAgent(context.Background(), execution); err != nil {
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

func TestLaunch_PromotionConfiguresFreshManagedGitEnvironment(t *testing.T) {
	for _, overlay := range []struct {
		name string
		env  map[string]string
	}{
		{name: "workspace-only"},
		{name: "previous lease-only overlay", env: map[string]string{
			githubauth.CredentialBrokerURLEnv: "http://127.0.0.1:38429/previous-broker",
			githubauth.CredentialLeaseEnv:     "previous-lease",
			"RUN_LABEL":                       "kept",
		}},
	} {
		t.Run(overlay.name, func(t *testing.T) {
			mgr, execution, configuredEnv := newManagedGitWorkspaceForPromotion(t)
			if overlay.env != nil {
				require.NoError(t, mgr.SetExecutionEnv(t.Context(), execution.ID, overlay.env))
			}
			req := managedGitWorkspacePromotionRequest(execution)

			promoted, err := mgr.Launch(t.Context(), req)
			require.NoError(t, err)
			require.Same(t, execution, promoted)
			_, err = mgr.configureAndStartAgent(t.Context(), promoted)
			require.NoError(t, err)

			for _, key := range []string{
				githubauth.CredentialBrokerURLEnv,
				githubauth.CredentialLeaseEnv,
				githubauth.CredentialReissueCapabilityEnv,
				githubauth.CredentialSessionIDEnv,
				githubauth.CredentialHelperPathEnv,
			} {
				require.Equal(t, req.Env[key], (*configuredEnv)[key], key)
			}
			require.Equal(t, "3", (*configuredEnv)["GIT_CONFIG_COUNT"])
			require.Equal(t, "http.version", (*configuredEnv)["GIT_CONFIG_KEY_0"])
			require.Equal(t, githubauth.ManagedGitCredentialHelper, (*configuredEnv)["GIT_CONFIG_VALUE_2"])
			require.Equal(t, overlay.env["RUN_LABEL"], (*configuredEnv)["RUN_LABEL"])
			require.Nil(t, managedRuntimeInstallContext(promoted), "the workspace installer digest predates the launch environment")
		})
	}
}

func TestLaunch_PromotionRejectsManagedGitEnvironmentWithoutLease(t *testing.T) {
	mgr, execution, configuredEnv := newManagedGitWorkspaceForPromotion(t)
	req := managedGitWorkspacePromotionRequest(execution)
	delete(req.Env, githubauth.CredentialLeaseEnv)

	_, err := mgr.Launch(t.Context(), req)
	require.ErrorContains(t, err, "managed Git credential lease is missing")
	require.Empty(t, execution.AgentCommand)
	require.Empty(t, *configuredEnv)
}

func newManagedGitWorkspaceForPromotion(t *testing.T) (*Manager, *AgentExecution, *map[string]string) {
	t.Helper()
	mgr := newTestManager(t)
	mgr.profileResolver = &countingProfileResolver{info: &AgentProfileInfo{
		ProfileID: "profile-1", AgentName: "auggie",
	}}
	var configuredEnv map[string]string
	execution := &AgentExecution{
		ID: "exec-workspace-only", TaskID: "task-1", SessionID: "session-1",
		WorkspacePath:                    t.TempDir(),
		ManagedRuntimeOwnerUserID:        "user-1",
		ManagedRuntimeInstallerEnvDigest: "workspace-environment-digest",
		agentctl:                         newConfigureCaptureAgentctlClient(t, mgr.logger, &configuredEnv),
	}
	execution.setRuntimeEnvironment(map[string]string{
		"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "http.version", "GIT_CONFIG_VALUE_0": "HTTP/1.1",
	})
	require.NoError(t, mgr.executionStore.Add(execution))
	return mgr, execution, &configuredEnv
}

func managedGitWorkspacePromotionRequest(execution *AgentExecution) *LaunchRequest {
	return &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID, AgentProfileID: "profile-1",
		StartAgent: true, PreviousExecutionID: "exec-previous", ACPSessionID: "acp-session-previous",
		Env: map[string]string{
			githubauth.CredentialBrokerURLEnv:         "http://127.0.0.1:38429/broker",
			githubauth.CredentialLeaseEnv:             "fresh-lease",
			githubauth.CredentialReissueCapabilityEnv: "fresh-capability",
			githubauth.CredentialSessionIDEnv:         execution.SessionID,
			githubauth.CredentialHelperPathEnv:        "/opt/kandev/agentctl",
			"GIT_CONFIG_COUNT":                        "2",
			"GIT_CONFIG_KEY_0":                        "credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_0":                      "",
			"GIT_CONFIG_KEY_1":                        "credential.https://github.com.helper",
			"GIT_CONFIG_VALUE_1":                      githubauth.ManagedGitCredentialHelper,
		},
	}
}
