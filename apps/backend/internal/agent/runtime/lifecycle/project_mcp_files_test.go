package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/passthroughconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const remoteProjectMCPPort = 43210

type remoteProjectMCPHelper struct {
	mu            sync.Mutex
	server        *httptest.Server
	workspace     string
	instanceID    string
	sessionID     string
	capable       bool
	generation    uint64
	files         map[string][]byte
	requests      []types.ProjectMCPPrepareRequest
	configureBody map[string]any
	configureCode int
	starts        int
	cleanups      int
}

func newRemoteProjectMCPHelper(t *testing.T, capable bool) *remoteProjectMCPHelper {
	t.Helper()
	helper := &remoteProjectMCPHelper{
		workspace: t.TempDir(), instanceID: "instance-1", sessionID: "session-1",
		capable: capable, files: map[string][]byte{},
	}
	helper.server = httptest.NewServer(http.HandlerFunc(helper.serveHTTP))
	t.Cleanup(helper.server.Close)
	return helper
}

func (h *remoteProjectMCPHelper) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/identity":
		capabilities := []string{"agent-survival.v1"}
		if h.capable {
			capabilities = append(capabilities, types.ProjectMCPDeliveryCapability)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities})
	case r.URL.Path == "/api/v1/mcp/project" && r.Method == http.MethodGet:
		_ = json.NewEncoder(w).Encode(h.target())
	case r.URL.Path == "/api/v1/mcp/project/prepare" && r.Method == http.MethodPost:
		h.prepare(w, r)
	case r.URL.Path == "/api/v1/mcp/project/cleanup" && r.Method == http.MethodPost:
		h.cleanup(w)
	case r.URL.Path == "/api/v1/agent/configure" && r.Method == http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		h.mu.Lock()
		h.configureBody = body
		code := h.configureCode
		h.mu.Unlock()
		if code != 0 && code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	case r.URL.Path == "/api/v1/start" && r.Method == http.MethodPost:
		h.mu.Lock()
		h.starts++
		h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "command": "pi-acp"})
	default:
		http.NotFound(w, r)
	}
}

func (h *remoteProjectMCPHelper) target() types.ProjectMCPTarget {
	h.mu.Lock()
	defer h.mu.Unlock()
	return types.ProjectMCPTarget{
		Version: types.ProjectMCPFilesVersion, InstanceID: h.instanceID, SessionID: h.sessionID,
		WorkspacePath: h.workspace, MCPURL: remoteProjectMCPURL(), Generation: h.generation,
	}
}

func remoteProjectMCPURL() string {
	return "http://127.0.0.1:" + strconv.Itoa(remoteProjectMCPPort) + "/mcp"
}

func (h *remoteProjectMCPHelper) prepare(w http.ResponseWriter, r *http.Request) {
	var request types.ProjectMCPPrepareRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if request.Version != types.ProjectMCPFilesVersion || request.Generation != h.generation+1 {
		http.Error(w, "generation conflict", http.StatusConflict)
		return
	}
	evidence := make([]types.ProjectMCPPreparedFile, 0, len(request.Files))
	for _, file := range request.Files {
		relative, err := filepath.Rel(h.workspace, file.Path)
		if err != nil || strings.HasPrefix(relative, "..") {
			http.Error(w, "path escape", http.StatusBadRequest)
			return
		}
		content := file.Content
		if existing, readErr := os.ReadFile(file.Path); readErr == nil && file.MergeKey != "" {
			content, err = passthroughconfig.MergeJSONUnderKey(existing, content, file.MergeKey)
			if err != nil {
				http.Error(w, "merge failed", http.StatusBadRequest)
				return
			}
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o700); err != nil || os.WriteFile(file.Path, content, 0o600) != nil {
			http.Error(w, "write failed", http.StatusInternalServerError)
			return
		}
		fingerprint := sha256.Sum256(content)
		evidence = append(evidence, types.ProjectMCPPreparedFile{
			Path: file.Path, Fingerprint: hex.EncodeToString(fingerprint[:]),
		})
		h.files[file.Path] = content
	}
	h.generation = request.Generation
	h.requests = append(h.requests, request)
	_ = json.NewEncoder(w).Encode(types.ProjectMCPPrepareResponse{
		Version: types.ProjectMCPFilesVersion, InstanceID: h.instanceID, SessionID: h.sessionID,
		WorkspacePath: h.workspace, MCPURL: remoteProjectMCPURL(), Generation: h.generation, Files: evidence,
	})
}

func (h *remoteProjectMCPHelper) cleanup(w http.ResponseWriter) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanups++
	for path := range h.files {
		_ = os.Remove(path)
	}
	h.files = map[string][]byte{}
	h.generation = 0
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "cleaned"})
}

func (h *remoteProjectMCPHelper) snapshot() (uint64, map[string]any, int, []types.ProjectMCPPrepareRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.generation, h.configureBody, h.starts, append([]types.ProjectMCPPrepareRequest(nil), h.requests...)
}

func (h *remoteProjectMCPHelper) cleanupCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cleanups
}

func (h *remoteProjectMCPHelper) client(t *testing.T) *agentctl.Client {
	t.Helper()
	parsed, err := url.Parse(h.server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	return agentctl.NewClient(parsed.Hostname(), port, newTestLogger(), agentctl.WithExecutionID(h.instanceID))
}

func newRemoteProjectMCPExecution(t *testing.T, client *agentctl.Client) *AgentExecution {
	t.Helper()
	profile := projectMCPDeliveryTestProfile()
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution := &AgentExecution{
		ID: "exec-1", TaskID: profile.BuiltinPackages.TaskID, SessionID: "session-1",
		WorkspaceID: profile.BuiltinPackages.WorkspaceID, AgentProfileID: "profile-1",
		RuntimeName: agentruntime.RuntimeKubernetes, ExecutorType: "k8s",
		WorkspacePath: t.TempDir(), metadata: map[string]interface{}{}, agentctl: client,
	}
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	return execution
}

type remoteMCPServer struct {
	Transport string `json:"transport"`
	URL       string `json:"url"`
	Lifecycle string `json:"lifecycle"`
}

func readRemoteProjectMCP(t *testing.T, helper *remoteProjectMCPHelper) map[string]remoteMCPServer {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(helper.workspace, ".pi", "mcp.json"))
	require.NoError(t, err)
	var payload struct {
		MCPServers map[string]remoteMCPServer `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &payload))
	return payload.MCPServers
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6, AC-AGENTS-MCP-PREP-005.1
func TestRemoteProjectMCPDeliveryUsesExecutorLocalFilesystem(t *testing.T) {
	helper := newRemoteProjectMCPHelper(t, true)
	execution := newRemoteProjectMCPExecution(t, helper.client(t))
	manager := newTestManager(t)
	agentConfig, ok := manager.registry.Get("pi-acp")
	require.True(t, ok)

	require.NoError(t, manager.materializeRuntimeProjectMCP(
		context.Background(), execution, agentConfig, nil, "k8s",
	))

	// The backend-local workspace is a different filesystem from the helper's.
	_, statErr := os.Stat(filepath.Join(execution.WorkspacePath, ".pi", "mcp.json"))
	require.True(t, os.IsNotExist(statErr), "remote delivery must not write the backend-local workspace")

	servers := readRemoteProjectMCP(t, helper)
	require.Equal(t, remoteProjectMCPURL(), servers["kandev"].URL)
	require.Equal(t, remoteProjectMCPURL()+"/packages/diagnostics", servers["kandev-diagnostics"].URL)
	for _, server := range servers {
		require.Equal(t, "streamable-http", server.Transport)
		require.Equal(t, "eager", server.Lifecycle)
	}

	delivery := execution.projectMCPDeliverySnapshot()
	require.NotNil(t, delivery)
	require.Equal(t, types.CanonicalProjectMCPDeliveryWorkspace(helper.workspace), delivery.Workspace)
	require.Equal(t, "instance-1", delivery.InstanceID)
	require.Equal(t, []string{"core", "diagnostics"}, delivery.Packages)
	generation, _, starts, requests := helper.snapshot()
	require.Equal(t, uint64(1), generation)
	require.Zero(t, starts)
	require.Len(t, requests, 1)

	require.NoError(t, manager.cleanupProjectMCPFiles(context.Background(), execution, nil))
	_, statErr = os.Stat(filepath.Join(helper.workspace, ".pi", "mcp.json"))
	require.True(t, os.IsNotExist(statErr), "remote project files are released with the execution")
	_, preparedGeneration := execution.projectMCPRemotePrepared()
	require.Zero(t, preparedGeneration)
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6, AC-AGENTS-MCP-PREP-005.1
func TestRemoteProjectMCPDeliveryRequiresHelperSupport(t *testing.T) {
	helper := newRemoteProjectMCPHelper(t, false)
	execution := newRemoteProjectMCPExecution(t, helper.client(t))
	manager := newTestManager(t)
	agentConfig, ok := manager.registry.Get("pi-acp")
	require.True(t, ok)

	err := manager.materializeRuntimeProjectMCP(context.Background(), execution, agentConfig, nil, "k8s")
	require.ErrorContains(t, err, "project-mcp-delivery-v1")
	require.Nil(t, execution.projectMCPDeliverySnapshot())
	_, _, _, requests := helper.snapshot()
	require.Empty(t, requests, "an old helper must fail before any remote file operation")
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5, AC-AGENTS-BUILTIN-MCP-003.6
func TestKubernetesRestartConfiguresProjectDelivery(t *testing.T) {
	helper := newRemoteProjectMCPHelper(t, true)
	client := helper.client(t)
	execution := newRemoteProjectMCPExecution(t, client)
	execution.AgentCommand = "pi-acp"
	execution.AgentArgs = []string{"pi-acp"}
	manager := newTestManager(t)
	manager.sessionManager = nil
	agentConfig, ok := manager.registry.Get("pi-acp")
	require.True(t, ok)

	refresh := &RemoteInstanceRefresh{
		Instance:    &ExecutorInstance{InstanceID: helper.instanceID, Client: client},
		AgentConfig: agentConfig,
	}
	_, err := manager.prepareRestartedKubernetesAgentctl(context.Background(), execution, refresh)
	require.NoError(t, err)

	_, configured, starts, requests := helper.snapshot()
	require.Equal(t, 1, starts)
	require.Len(t, requests, 1, "the restarted helper materializes the project file before configure")
	raw, present := configured["project_mcp_delivery"]
	require.True(t, present, "Kubernetes restart must configure the project delivery record")
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	var delivery types.ProjectMCPDelivery
	require.NoError(t, json.Unmarshal(encoded, &delivery))
	require.Equal(t, "instance-1", delivery.InstanceID)
	require.Equal(t, []string{"core", "diagnostics"}, delivery.Packages)

	servers := readRemoteProjectMCP(t, helper)
	require.Equal(t, remoteProjectMCPURL()+"/packages/diagnostics", servers["kandev-diagnostics"].URL)
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestKubernetesRestartRollsBackProjectDeliveryOnFailure(t *testing.T) {
	helper := newRemoteProjectMCPHelper(t, true)
	helper.configureCode = http.StatusInternalServerError
	client := helper.client(t)
	execution := newRemoteProjectMCPExecution(t, client)
	execution.AgentCommand = "pi-acp"
	execution.AgentArgs = []string{"pi-acp"}
	prior := &types.ProjectMCPDelivery{
		Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
		Workspace: "prior-workspace", TaskID: "task-1", InstanceID: "instance-1",
		BindingDigest: strings.Repeat("a", 64), Packages: []string{"core"},
	}
	execution.setProjectMCPDelivery(prior)
	execution.setProjectMCPRemotePrepared(client, 7)
	manager := newTestManager(t)
	manager.sessionManager = nil
	agentConfig, ok := manager.registry.Get("pi-acp")
	require.True(t, ok)

	_, err := manager.prepareRestartedKubernetesAgentctl(context.Background(), execution, &RemoteInstanceRefresh{
		Instance:    &ExecutorInstance{InstanceID: helper.instanceID, Client: client},
		AgentConfig: agentConfig,
	})
	require.Error(t, err)
	restored := execution.projectMCPDeliverySnapshot()
	require.Equal(t, prior, restored, "failed restart must restore the prior evidence")
	_, generation := execution.projectMCPRemotePrepared()
	require.Equal(t, uint64(7), generation)
	require.Equal(t, 1, helper.cleanupCount(), "staged helper files must be released")
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6
func TestProjectMCPCleanupDoesNotReacquireLifecycleLock(t *testing.T) {
	helper := newRemoteProjectMCPHelper(t, true)
	client := helper.client(t)
	execution := newRemoteProjectMCPExecution(t, client)
	execution.setProjectMCPRemotePrepared(client, 1)
	manager := newTestManager(t)

	execution.agentctlLifecycleMu.Lock()
	done := make(chan error, 1)
	go func() {
		done <- manager.cleanupProjectMCPFiles(context.Background(), execution, client)
	}()
	select {
	case err := <-done:
		execution.agentctlLifecycleMu.Unlock()
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		execution.agentctlLifecycleMu.Unlock()
		t.Fatal("project MCP cleanup reacquired a lifecycle lock already held by a rollback")
	}
}

func TestProjectMCPDeliveryHandoffAcceptsLongWorkspace(t *testing.T) {
	delivery := &types.ProjectMCPDelivery{
		Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
		Workspace: "/workspace/" + strings.Repeat("nested/", 40) + "task",
		TaskID:    "task-1", InstanceID: "instance-1",
		BindingDigest: strings.Repeat("a", 64), Packages: []string{"core"},
	}
	require.NoError(t, delivery.Validate())
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1",
		agentctl: agentctl.NewClient("127.0.0.1", 1, newTestLogger(), agentctl.WithExecutionID("instance-1")),
	}
	execution.setProjectMCPDelivery(delivery)
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	restored, err := execution.mcpProjectDeliveryFor(client)
	require.NoError(t, err)
	require.Equal(t, delivery.Workspace, restored.Workspace)
}
