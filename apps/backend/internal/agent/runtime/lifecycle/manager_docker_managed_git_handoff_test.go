package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	processconfig "github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

// @covers AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.15
func TestDockerManagedGitConfigureHandoff(t *testing.T) {
	for _, runtimeName := range []agentruntime.Runtime{agentruntime.RuntimeDocker, agentruntime.RuntimeRemoteDocker} {
		for _, scenario := range []string{"fresh", "refresh", "reconnect", "nil", "empty", "partial"} {
			t.Run(runtimeName.String()+"/"+scenario, func(t *testing.T) {
				mgr := newTestManager(t)
				var configured, requested map[string]string
				var startCalls int
				client := newDockerManagedGitConfigureClient(t, &configured, &requested, &startCalls, false)
				env := managedGitHandoffEnvironment("launch")
				env["PROFILE_SETTING"] = "preserved"
				if scenario == "reconnect" {
					delete(env, githubauth.CredentialHelperPathEnv)
				}
				instance := &ExecutorInstance{InstanceID: "docker-exec", RuntimeName: runtimeName, Client: client}
				execution := instance.ToAgentExecution(&ExecutorCreateRequest{
					TaskID: "task-1", SessionID: "session-1", Env: env, WorkspacePath: t.TempDir(),
				})
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

				_, err := mgr.configureAndStartAgent(context.Background(), execution)
				require.NoError(t, err)
				require.Equal(t, "preserved", configured["PROFILE_SETTING"])
				require.Equal(t, 1, startCalls)
				if scenario == "nil" || scenario == "empty" || scenario == "partial" {
					require.Empty(t, configured[githubauth.CredentialBrokerURLEnv])
					require.Empty(t, configured[githubauth.CredentialLeaseEnv])
					require.Empty(t, configured[githubauth.CredentialHelperPathEnv])
					require.Empty(t, configured[githubauth.CredentialCLIShimDirEnv])
					require.Empty(t, configured[githubauth.CredentialCLIBashEnvEnv])
					require.NotContains(t, configured, "GIT_CONFIG_VALUE_2")
					require.NotContains(t, execution.RuntimeEnvironment(), githubauth.CredentialBrokerURLEnv)
					return
				}

				lease := "launch"
				if scenario == "refresh" {
					lease = "current"
				}
				require.Equal(t, lease, configured[githubauth.CredentialLeaseEnv])
				require.Equal(t, remoteAgentctlExecutablePath, requested[githubauth.CredentialHelperPathEnv])
				require.Equal(t, remoteAgentctlExecutablePath, execution.RuntimeEnvironment()[githubauth.CredentialHelperPathEnv])
				require.Equal(t, remoteAgentctlExecutablePath, configured[githubauth.CredentialHelperPathEnv])
				shimDir := configured[githubauth.CredentialCLIShimDirEnv]
				require.NotEmpty(t, shimDir)
				require.Equal(t, filepath.Join(shimDir, githubauth.CLIBashEnvFilename), configured[githubauth.CredentialCLIBashEnvEnv])
				require.Contains(t, configured["PATH"], shimDir)
				require.NotEmpty(t, configured[githubauth.CredentialCLIBashEnvEnv])
				if scenario == "refresh" {
					require.Equal(t, "current", execution.RuntimeEnvironment()[githubauth.CredentialLeaseEnv])
				}
				if scenario == "reconnect" {
					require.Equal(t, "launch", execution.RuntimeEnvironment()[githubauth.CredentialLeaseEnv])
				}
				if scenario == "fresh" {
					require.Equal(t, "launch", execution.RuntimeEnvironment()[githubauth.CredentialLeaseEnv])
				}
			})
		}
	}
}

func TestNormalizeManagedGitHelperEnvironment(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		runtime agentruntime.Runtime
		want    string
	}{
		{name: "docker", runtime: agentruntime.RuntimeDocker, want: remoteAgentctlExecutablePath},
		{name: "remote docker", runtime: agentruntime.RuntimeRemoteDocker, want: remoteAgentctlExecutablePath},
		{name: "kubernetes", runtime: agentruntime.RuntimeKubernetes, want: kubernetesAgentctlPath},
		{name: "standalone", runtime: agentruntime.RuntimeStandalone, want: "/host/bin/agentctl"},
		{name: "ssh", runtime: agentruntime.RuntimeSSH, want: "/host/bin/agentctl"},
		{name: "sprites", runtime: agentruntime.RuntimeSprites, want: "/host/bin/agentctl"},
		{name: "plugin remote", runtime: agentruntime.RuntimePluginRemote, want: "/host/bin/agentctl"},
		{name: "unknown", runtime: agentruntime.Runtime("future"), want: "/host/bin/agentctl"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			env := managedGitHandoffEnvironment("current")
			normalizeManagedGitHelperEnvironment(testCase.runtime, env)
			require.Equal(t, testCase.want, env[githubauth.CredentialHelperPathEnv])

			unmanaged := map[string]string{githubauth.CredentialHelperPathEnv: "/host/bin/agentctl"}
			normalizeManagedGitHelperEnvironment(testCase.runtime, unmanaged)
			require.Equal(t, "/host/bin/agentctl", unmanaged[githubauth.CredentialHelperPathEnv])
		})
	}
}

func TestDockerManagedGitConfigureFailureDoesNotStartAgent(t *testing.T) {
	mgr := newTestManager(t)
	var configured, requested map[string]string
	var startCalls int
	client := newDockerManagedGitConfigureClient(t, &configured, &requested, &startCalls, true)
	instance := &ExecutorInstance{InstanceID: "docker-exec", RuntimeName: agentruntime.RuntimeDocker, Client: client}
	execution := instance.ToAgentExecution(&ExecutorCreateRequest{
		TaskID: "task-1", SessionID: "session-1", Env: managedGitHandoffEnvironment("current"), WorkspacePath: t.TempDir(),
	})
	execution.AgentCommand = "echo"

	_, err := mgr.configureAndStartAgent(context.Background(), execution)
	require.Error(t, err)
	require.Zero(t, startCalls)
}

func newDockerManagedGitConfigureClient(
	t *testing.T,
	configured, requested *map[string]string,
	startCalls *int,
	configureFailure bool,
) *agentctlclient.Client {
	t.Helper()
	shimDir := filepath.Join(t.TempDir(), "installed shims")
	startupEnv := filepath.Join(shimDir, githubauth.CLIBashEnvFilename)
	parentEnv := filepath.Join(t.TempDir(), "parent bash env.sh")
	initial := managedGitHandoffEnvironment("inherited-stale")
	initial[githubauth.CredentialHelperPathEnv] = remoteAgentctlExecutablePath
	initial[githubauth.CredentialCLIShimDirEnv] = shimDir
	initial[githubauth.CredentialCLIBashEnvEnv] = startupEnv
	initial[githubauth.CredentialParentBashEnv] = parentEnv
	initial["BASH_ENV"] = startupEnv
	initial["PATH"] = shimDir + string(filepath.ListSeparator) + "/usr/bin:/bin"
	cfg := &processconfig.InstanceConfig{WorkDir: t.TempDir()}
	for key, value := range initial {
		cfg.AgentEnv = append(cfg.AgentEnv, key+"="+value)
	}
	log := newTestLogger()
	manager := process.NewManager(cfg, log)
	t.Cleanup(func() { require.NoError(t, manager.StopForTeardown(context.Background())) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent/configure":
			if configureFailure {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var request struct {
				Env map[string]string `json:"env"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			*requested = request.Env
			if err := manager.Configure("echo", nil, false, request.Env, "", nil, false); err != nil {
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
			*configured = effective
			_, _ = w.Write([]byte(`{"success":true}`))
		case "/api/v1/start":
			*startCalls++
			_, _ = w.Write([]byte(`{"success":true,"command":"echo"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return newTestAgentctlClient(t, server.URL, log)
}
