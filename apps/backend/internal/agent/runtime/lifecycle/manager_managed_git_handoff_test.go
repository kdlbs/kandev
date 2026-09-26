package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	processconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

// @covers AC-EXECUTORS-KUBERNETES-TASK-POD-001.9
// @covers AC-EXECUTORS-KUBERNETES-TASK-POD-001.10
func TestKubernetesManagedGitConfigureHandoff(t *testing.T) {
	for _, scenario := range []string{"fresh", "refresh", "nil", "empty", "partial"} {
		t.Run(scenario, func(t *testing.T) {
			mgr := newTestManager(t)
			var configured map[string]string
			client := newManagedGitConfigureClient(t, &configured)
			env := managedGitHandoffEnvironment("launch")
			env["SSL_CERT_FILE"] = "/worker/ca.pem"
			env["PROFILE_SETTING"] = "preserved"
			req := &ExecutorCreateRequest{TaskID: "task-1", SessionID: "session-1", Env: env, WorkspacePath: t.TempDir()}
			instance := &ExecutorInstance{InstanceID: "exec-1", RuntimeName: "k8s", Client: client}
			execution := instance.ToAgentExecution(req)
			execution.AgentCommand = "echo"
			require.NoError(t, mgr.executionStore.Add(execution))
			switch scenario {
			case "refresh":
				require.NoError(t, mgr.SetExecutionEnv(context.Background(), execution.ID, managedGitHandoffEnvironment("current")))
			case "nil":
				require.NoError(t, mgr.SetExecutionEnv(context.Background(), execution.ID, nil))
			case "empty":
				require.NoError(t, mgr.SetExecutionEnv(context.Background(), execution.ID, map[string]string{}))
			case "partial":
				require.NoError(t, mgr.SetExecutionEnv(context.Background(), execution.ID, map[string]string{"CURRENT": "yes"}))
			}
			_, err := mgr.configureAndStartAgent(context.Background(), execution, "never")
			require.NoError(t, err)
			require.Equal(t, "/worker/ca.pem", configured["SSL_CERT_FILE"])
			require.Equal(t, "preserved", configured["PROFILE_SETTING"])
			if scenario != "fresh" && scenario != "refresh" {
				require.Empty(t, configured[githubauth.CredentialLeaseEnv])
				require.Empty(t, configured[githubauth.CredentialHelperPathEnv])
				require.NotContains(t, configured, "GIT_CONFIG_VALUE_1")
				require.Equal(t, "core.hooksPath", configured["GIT_CONFIG_KEY_0"])
				return
			}
			require.NotEmpty(t, configured[githubauth.CredentialBrokerURLEnv])
			lease := "launch"
			if scenario == "refresh" {
				lease = "current"
			}
			require.Equal(t, lease, configured[githubauth.CredentialLeaseEnv])
			require.Equal(t, kubernetesAgentctlPath, configured[githubauth.CredentialHelperPathEnv])
			require.Equal(t, configured, execution.RuntimeEnvironment())
			// A retained container restart receives exactly the effective launch snapshot.
			_, err = mgr.prepareRestartedKubernetesAgentctl(context.Background(), execution, &RemoteInstanceRefresh{Instance: instance})
			require.NoError(t, err)
			require.Equal(t, lease, configured[githubauth.CredentialLeaseEnv])
			require.Equal(t, kubernetesAgentctlPath, configured[githubauth.CredentialHelperPathEnv])
		})
	}
}

func managedGitHandoffEnvironment(lease string) map[string]string {
	return map[string]string{
		githubauth.CredentialBrokerURLEnv:  "https://broker.example/resolve",
		githubauth.CredentialLeaseEnv:      lease,
		githubauth.CredentialHelperPathEnv: "/host/bin/agentctl",
		"GIT_CONFIG_COUNT":                 "3",
		"GIT_CONFIG_KEY_0":                 "core.hooksPath",
		"GIT_CONFIG_VALUE_0":               "/user/hooks",
		"GIT_CONFIG_KEY_1":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_1":               "",
		"GIT_CONFIG_KEY_2":                 "credential.https://github.com.helper",
		"GIT_CONFIG_VALUE_2":               githubauth.ManagedGitCredentialHelper,
	}
}

func TestRestartedKubernetesManagedGitNormalizesRecoveredSnapshot(t *testing.T) {
	mgr := newTestManager(t)
	var configured map[string]string
	client := newManagedGitConfigureClient(t, &configured)
	instance := &ExecutorInstance{InstanceID: "exec-1", RuntimeName: "k8s", Client: client}
	env := managedGitHandoffEnvironment("recovered")
	delete(env, githubauth.CredentialHelperPathEnv)
	execution := instance.ToAgentExecution(&ExecutorCreateRequest{Env: env, WorkspacePath: t.TempDir()})
	execution.AgentCommand = "echo"
	_, err := mgr.prepareRestartedKubernetesAgentctl(context.Background(), execution, &RemoteInstanceRefresh{Instance: instance})
	require.NoError(t, err)
	require.Equal(t, kubernetesAgentctlPath, configured[githubauth.CredentialHelperPathEnv])
	require.Equal(t, "recovered", configured[githubauth.CredentialLeaseEnv])
}

func newManagedGitConfigureClient(t *testing.T, captured *map[string]string) *agentctlclient.Client {
	t.Helper()
	cfg := &processconfig.InstanceConfig{WorkDir: t.TempDir()}
	for key, value := range managedGitHandoffEnvironment("inherited-stale") {
		cfg.AgentEnv = append(cfg.AgentEnv, key+"="+value)
	}
	log := newTestLogger()
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/configure":
			var request struct {
				Env map[string]string `json:"env"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if err := manager.Configure("echo", nil, false, request.Env, "", "", nil, false); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			effective := make(map[string]string)
			for _, entry := range cfg.AgentEnv {
				key, value, ok := strings.Cut(entry, "=")
				if ok {
					effective[key] = value
				}
			}
			*captured = effective
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			_, _ = w.Write([]byte(`{"success":true,"command":"echo"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return newTestAgentctlClient(t, server.URL, log)
}

func TestComposeExecutionRuntimeEnvironmentRemovesManagedConfig(t *testing.T) {
	env := managedGitHandoffEnvironment("previous")
	env["GIT_CONFIG_COUNT"] = "4"
	env["GIT_CONFIG_KEY_3"] = "credential.useHttpPath"
	env["GIT_CONFIG_VALUE_3"] = "true"
	got, err := composeExecutionRuntimeEnvironment(env, nil)
	require.NoError(t, err)
	require.Equal(t, "1", got["GIT_CONFIG_COUNT"])
	require.Equal(t, "core.hooksPath", got["GIT_CONFIG_KEY_0"])
}
