package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func isolateGlobalGitConfig(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	// The shell client inherits the test environment. Clear its global override
	// and isolate XDG paths so Git cannot mutate a host configuration file.
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	if err := os.Unsetenv("GIT_CONFIG_GLOBAL"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

func TestApplyPluginExecutorGitIdentityUsesAgentGlobalConfig(t *testing.T) {
	hostXDG := t.TempDir()
	hostGitConfig := filepath.Join(hostXDG, "git", "config")
	if err := os.MkdirAll(filepath.Dir(hostGitConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	hostGitConfigContents := []byte("[user]\n\tname = Host identity\n\temail = host@example.test\n")
	if err := os.WriteFile(hostGitConfig, hostGitConfigContents, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", hostXDG)
	isolateGlobalGitConfig(t)
	agentHome := t.TempDir()
	remoteAuthHome := t.TempDir()
	t.Setenv("HOME", agentHome)
	name, email := `Ada O'Brien "Lovelace"`, "ada@example.test"
	client := &shellProcessClient{}
	t.Cleanup(func() { stopAll(client) })

	executor := NewPluginRemoteExecutor(nil, newTestLogger())
	executor.applyPluginExecutorGitIdentity(context.Background(), client, &ExecutorCreateRequest{
		SessionID: "session-1",
		Metadata: map[string]interface{}{
			MetadataKeyGitUserName:    name,
			MetadataKeyGitUserEmail:   email,
			MetadataKeyRemoteAuthHome: remoteAuthHome,
		},
	})

	for key, want := range map[string]string{"user.name": name, "user.email": email} {
		out, err := exec.Command("git", "config", "--global", key).Output()
		if err != nil {
			t.Fatalf("read effective agent %s: %v", key, err)
		}
		if got := strings.TrimSpace(string(out)); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(remoteAuthHome, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("credential target home received Git configuration: %v", err)
	}
	if got, err := os.ReadFile(hostGitConfig); err != nil || string(got) != string(hostGitConfigContents) {
		t.Fatalf("host XDG Git config changed: content=%q err=%v", got, err)
	}
	for _, command := range client.commands {
		if strings.Contains(command, email) || strings.Contains(command, "Lovelace") {
			t.Fatalf("command line carries the identity: %q", command)
		}
	}
}

func TestPluginExecutorLaunchUsesAgentGitConfig(t *testing.T) {
	testPluginExecutorGitIdentityLifecycle(t, false, false, "")
}

func TestPluginExecutorRetainedAttachmentUsesAgentGitConfig(t *testing.T) {
	testPluginExecutorGitIdentityLifecycle(t, true, false, "")
}

func TestPluginExecutorLaunchContinuesWhenGitIdentitySetupFails(t *testing.T) {
	testPluginExecutorGitIdentityLifecycle(t, false, true, "")
}

func TestPluginExecutorLaunchKeepsEffectiveGitConfigOverride(t *testing.T) {
	testPluginExecutorGitIdentityLifecycle(t, false, false, filepath.Join(t.TempDir(), "agent-gitconfig"))
}

func testPluginExecutorGitIdentityLifecycle(t *testing.T, retained, failSetup bool, globalConfig string) {
	t.Helper()
	hostXDG := t.TempDir()
	hostGitConfig := filepath.Join(hostXDG, "git", "config")
	if err := os.MkdirAll(filepath.Dir(hostGitConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	hostGitConfigContents := []byte("[user]\n\tname = Host identity\n\temail = host@example.test\n")
	if err := os.WriteFile(hostGitConfig, hostGitConfigContents, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", hostXDG)
	isolateGlobalGitConfig(t)
	if globalConfig != "" {
		t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	}
	agentHome := t.TempDir()
	remoteAuthHome := t.TempDir()
	t.Setenv("HOME", agentHome)

	processServer := newPluginGitIdentityProcessServer(t, failSetup)
	resource := &pluginsdk.ExecutorResourceDescriptor{
		ResourceHandle: "resource-git-identity", StateJson: `{"resource_id":"git-identity"}`,
		Platform: "linux-amd64", StateVersion: 1,
		Capabilities: &pluginsdk.ExecutorProviderCapabilities{Terminal: true, Files: true, Git: true},
	}
	provider := testPluginExecutorLaunchProvider()
	operations := &pluginExecutorOperationsFake{
		provisionResponse: &pluginsdk.ProvisionExecutorEnvironmentResponse{Resource: resource},
		attachResponse:    &pluginsdk.AttachExecutorEnvironmentResponse{Resource: resource},
		inspectResponses: []*pluginsdk.InspectExecutorEnvironmentResponse{
			{State: "running", ExpiresAt: "2027-01-01T00:00:00Z"},
			{State: "running", ExpiresAt: "2027-01-01T00:00:00Z"},
		},
		connectionResponse: &pluginsdk.ResolveExecutorConnectionResponse{Lease: &pluginsdk.ExecutorConnectionLease{
			BaseUrl: processServer.server.URL, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano), Generation: "git-identity-lease",
		}},
	}
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewPluginRemoteExecutor(operations, log)
	runtime.ready = func(context.Context, *agentctl.Client) error { return nil }
	name, email := `Ada O'Brien "Lovelace"`, "ada@example.test"
	metadata := map[string]interface{}{
		MetadataKeyGitUserName:    name,
		MetadataKeyGitUserEmail:   email,
		MetadataKeyRemoteAuthHome: remoteAuthHome,
	}
	var agentInstanceRequest *agentctl.CreateInstanceRequest
	runtime.newAgentctlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, executionID, token string) (*agentctl.Client, error) {
		if _, err := resolver(ctx); err != nil {
			return nil, err
		}
		endpoint, err := url.Parse(processServer.server.URL)
		if err != nil {
			return nil, err
		}
		port, err := strconv.Atoi(endpoint.Port())
		if err != nil {
			return nil, err
		}
		return agentctl.NewClient(endpoint.Hostname(), port, log,
			agentctl.WithExecutionID(executionID), agentctl.WithAuthToken(token)), nil
	}
	var instance *ExecutorInstance
	if retained {
		operations.provisionResponse = nil
		operations.connectionResponse.Lease.BaseUrl = processServer.server.URL
		operations.attachResponse.Resource = resource
		loader := &pluginExecutorRecoveryProfileLoaderFake{profile: models.ExecutorProviderLaunchProfile{
			Provider: provider, ProfileID: "profile-plugin-recovery",
		}}
		store := &pluginExecutorInventoryStoreFake{record: pluginExecutorRecoveryRecord(t, pluginExecutorPhaseReady, resource)}
		runtime.SetRecoveryDependencies(loader, store)
		request := &ExecutorCreateRequest{
			InstanceID: "execution-plugin-recovery", TaskID: "task-plugin-recovery", SessionID: "session-plugin-recovery",
			TaskEnvironmentID: "environment-plugin-recovery", PreviousExecutionID: "execution-plugin-recovery",
			WorkspaceReuseRequired: true, Metadata: metadata,
		}
		instance, err = runtime.CreateInstance(context.Background(), request)
		if err != nil {
			t.Fatalf("retained CreateInstance(): %v; logs=%#v", err, observed.All())
		}
	} else {
		request := pluginExecutorLaunchRequest(provider)
		request.Env["HOME"] = agentHome
		if globalConfig != "" {
			request.Env["GIT_CONFIG_GLOBAL"] = globalConfig
		}
		request.Metadata = metadata
		request.CheckpointRuntimeInventory = func(context.Context, map[string]interface{}) error { return nil }
		request.ReleaseRuntimeInventory = func(context.Context) error { return nil }
		agentInstanceRequest = startPluginExecutorGitIdentityControl(t, runtime, instancePortForPluginGitIdentity)
		instance, err = runtime.CreateInstance(context.Background(), request)
		if err != nil {
			t.Fatalf("fresh CreateInstance(): %v; logs=%#v", err, observed.All())
		}
	}
	defer instance.Client.Close()
	if !retained {
		if got := agentInstanceRequest.Env["HOME"]; got != agentHome {
			t.Fatalf("agent HOME = %q, want %q", got, agentHome)
		}
		if globalConfig != "" && agentInstanceRequest.Env["GIT_CONFIG_GLOBAL"] != globalConfig {
			t.Fatalf("agent GIT_CONFIG_GLOBAL = %q, want %q", agentInstanceRequest.Env["GIT_CONFIG_GLOBAL"], globalConfig)
		}
	}

	processServer.mu.Lock()
	startCalls := processServer.startCalls
	processServer.mu.Unlock()
	if startCalls != 1 {
		t.Fatalf("identity process start calls = %d, want 1", startCalls)
	}
	if failSetup {
		if instance.Client == nil {
			t.Fatal("launch returned no agentctl client after identity setup failure")
		}
	} else {
		for key, want := range map[string]string{"user.name": name, "user.email": email} {
			cmd := exec.Command("git", "config", "--global", key)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("read agent %s: %v", key, err)
			}
			if got := strings.TrimSpace(string(out)); got != want {
				t.Errorf("agent %s = %q, want %q", key, got, want)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(remoteAuthHome, ".gitconfig")); !os.IsNotExist(err) {
		t.Fatalf("credential target home received Git configuration: %v", err)
	}
	if got, err := os.ReadFile(hostGitConfig); err != nil || string(got) != string(hostGitConfigContents) {
		t.Fatalf("host XDG Git config changed: content=%q err=%v", got, err)
	}
}

const instancePortForPluginGitIdentity = 41234

type pluginGitIdentityProcessServer struct {
	client     *shellProcessClient
	mu         sync.Mutex
	startCalls int
	server     *httptest.Server
}

func newPluginGitIdentityProcessServer(t *testing.T, failStart bool) *pluginGitIdentityProcessServer {
	t.Helper()
	state := &pluginGitIdentityProcessServer{client: &shellProcessClient{}}
	state.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/processes/start":
			state.mu.Lock()
			state.startCalls++
			state.mu.Unlock()
			if failStart {
				http.Error(w, "injected process start failure", http.StatusInternalServerError)
				return
			}
			var request agentctl.StartProcessRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			process, err := state.client.StartProcess(r.Context(), request)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"process": process})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/processes/"):
			processID := path.Base(r.URL.Path)
			process, err := state.client.GetProcess(r.Context(), processID, true)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(process)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/processes/stop":
			var request struct {
				ProcessID string `json:"process_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = state.client.StopProcess(r.Context(), request.ProcessID)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(func() {
		state.server.Close()
		stopAll(state.client)
	})
	return state
}

func startPluginExecutorGitIdentityControl(t *testing.T, runtime *PluginRemoteExecutor, instancePort int) *agentctl.CreateInstanceRequest {
	t.Helper()
	instanceRequest := &agentctl.CreateInstanceRequest{}
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/instances" {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(instanceRequest); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: instanceRequest.ID, Port: instancePort})
	}))
	t.Cleanup(control.Close)
	endpoint, err := url.Parse(control.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(endpoint.Port())
	if err != nil {
		t.Fatal(err)
	}
	runtime.newAgentctlControlClient = func(ctx context.Context, resolver agentctl.ConnectionLeaseResolver, log *logger.Logger, _ string) (*agentctl.ControlClient, string, error) {
		if _, err := resolver(ctx); err != nil {
			return nil, "", err
		}
		return agentctl.NewControlClient(endpoint.Hostname(), port, log), "agentctl-token", nil
	}
	return instanceRequest
}

func TestApplyPluginExecutorGitIdentityNeedsNameAndEmail(t *testing.T) {
	for name, metadata := range map[string]map[string]interface{}{
		"neither":    {},
		"name only":  {MetadataKeyGitUserName: "Ada"},
		"email only": {MetadataKeyGitUserEmail: "ada@example.test"},
	} {
		t.Run(name, func(t *testing.T) {
			client := &shellProcessClient{}
			executor := NewPluginRemoteExecutor(nil, newTestLogger())
			executor.applyPluginExecutorGitIdentity(context.Background(), client, &ExecutorCreateRequest{
				SessionID: "session-1", Metadata: metadata,
			})
			if len(client.commands) != 0 {
				t.Fatalf("commands = %v, want none", client.commands)
			}
		})
	}
}
