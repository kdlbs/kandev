package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

// standaloneControlServer is an in-process stand-in for the host agentctl
// control API the standalone executor talks to.
type standaloneControlServer struct {
	mu             sync.Mutex
	healthy        bool
	healthCalls    int
	createRequests []agentctlclient.CreateInstanceRequest
	deleted        []string
	createStatus   int
	server         *httptest.Server
}

func newStandaloneControlServer(t *testing.T, healthy bool) *standaloneControlServer {
	t.Helper()
	s := &standaloneControlServer{healthy: healthy, createStatus: http.StatusOK}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case r.URL.Path == "/health":
			s.healthCalls++
			if !s.healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodPost:
			var req agentctlclient.CreateInstanceRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			s.createRequests = append(s.createRequests, req)
			if s.createStatus != http.StatusOK {
				w.WriteHeader(s.createStatus)
				_, _ = w.Write([]byte(`{"error":"cannot create"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlclient.CreateInstanceResponse{ID: "std-1", Port: 45678})
		case strings.HasPrefix(r.URL.Path, "/api/v1/instances/") && r.Method == http.MethodDelete:
			s.deleted = append(s.deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/instances/"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *standaloneControlServer) executor(t *testing.T) *StandaloneExecutor {
	t.Helper()
	parsed, err := url.Parse(s.server.URL)
	if err != nil {
		t.Fatalf("parse control URL: %v", err)
	}
	port, _ := strconv.Atoi(parsed.Port())
	ctl := agentctlclient.NewControlClient(parsed.Hostname(), port, newTestLogger())
	return NewStandaloneExecutor(ctl, parsed.Hostname(), port, newTestLogger())
}

func (s *standaloneControlServer) lastCreateRequest(t *testing.T) agentctlclient.CreateInstanceRequest {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.createRequests) == 0 {
		t.Fatal("no create-instance request recorded")
	}
	return s.createRequests[len(s.createRequests)-1]
}

func TestStandaloneExecutorStaticSurface(t *testing.T) {
	exec := newStandaloneControlServer(t, true).executor(t)
	if exec.Name() != executor.NameStandalone {
		t.Fatalf("Name() = %q", exec.Name())
	}
	if exec.RequiresCloneURL() {
		t.Fatal("standalone runs in an existing host workspace, so no clone URL is required")
	}
	if !exec.ShouldApplyPreferredShell() {
		t.Fatal("standalone runs on the host, so the user's preferred shell applies")
	}
	if exec.IsAlwaysResumable() {
		t.Fatal("standalone instances are transient and not always resumable")
	}
	instances, err := exec.RecoverInstances(context.Background())
	if err != nil || instances != nil {
		t.Fatalf("RecoverInstances() = %v, %v; want nil, nil", instances, err)
	}

	runner := &process.InteractiveRunner{}
	exec.SetInteractiveRunner(runner)
	if exec.GetInteractiveRunner() != runner {
		t.Fatal("interactive runner round-trip failed")
	}
}

func TestStandaloneExecutorHealthCheck(t *testing.T) {
	healthy := newStandaloneControlServer(t, true)
	if err := healthy.executor(t).HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}

	unhealthy := newStandaloneControlServer(t, false)
	if err := unhealthy.executor(t).HealthCheck(context.Background()); err == nil {
		t.Fatal("expected an unhealthy control server to fail the health check")
	}
}

func TestStandaloneExecutorCreateInstance(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	exec := control.executor(t)
	exec.SetAuthToken("launch-token")

	agent := &fakeRuntimeAgent{
		MockAgent:    agents.NewMockAgent(),
		id:           "opencode",
		requiresKill: true,
		stripEnv:     []string{"NODE_OPTIONS"},
	}
	req := &ExecutorCreateRequest{
		InstanceID:           "instance-1",
		TaskID:               "task-1",
		SessionID:            "session-1",
		WorkspacePath:        "/host/workspace",
		WorkspaceSourceRoots: []string{"/host/sources"},
		Protocol:             "acp",
		McpMode:              "task",
		AgentConfig:          agent,
		Env:                  map[string]string{"EXISTING": "value"},
		Metadata: map[string]interface{}{
			MetadataKeyWorktreeID:     "wt-1",
			MetadataKeyWorktreeBranch: "feature/task-1",
			MetadataKeyBaseBranches:   map[string]string{"": "main"},
		},
	}

	instance, err := exec.CreateInstance(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	if instance.StandaloneInstanceID != "std-1" || instance.StandalonePort != 45678 {
		t.Fatalf("instance = %+v", instance)
	}
	if instance.RuntimeName != executor.NameStandalone {
		t.Fatalf("RuntimeName = %q", instance.RuntimeName)
	}
	if instance.WorkspacePath != "/host/workspace" {
		t.Fatalf("WorkspacePath = %q, want the host path verbatim", instance.WorkspacePath)
	}
	if instance.Metadata["standalone_port"] != 45678 {
		t.Fatalf("standalone_port metadata = %v", instance.Metadata["standalone_port"])
	}
	if instance.Metadata["worktree_id"] != "wt-1" ||
		instance.Metadata["worktree_path"] != "/host/workspace" ||
		instance.Metadata["worktree_branch"] != "feature/task-1" {
		t.Fatalf("worktree metadata = %+v", instance.Metadata)
	}

	got := control.lastCreateRequest(t)
	if got.ID != "instance-1" || got.SessionID != "session-1" || got.TaskID != "task-1" {
		t.Fatalf("create request identity = %+v", got)
	}
	if got.AgentCommand != "" {
		t.Fatalf("AgentCommand = %q, want empty (the agent starts via a later Configure call)", got.AgentCommand)
	}
	if got.AutoStart {
		t.Fatal("standalone instances must not auto-start the agent")
	}
	if got.AgentType != "opencode" || !got.RequiresProcessKill {
		t.Fatalf("agent runtime fields = %+v", got)
	}
	if !equalStrings(got.StripEnv, []string{"NODE_OPTIONS"}) {
		t.Fatalf("StripEnv = %v", got.StripEnv)
	}
	if got.BaseBranches[""] != "main" {
		t.Fatalf("BaseBranches = %+v", got.BaseBranches)
	}
	if !equalStrings(got.WorkspaceSourceRoots, []string{"/host/sources"}) {
		t.Fatalf("WorkspaceSourceRoots = %v", got.WorkspaceSourceRoots)
	}
	// The executor injects the task/session identity into the child env.
	if got.Env["KANDEV_TASK_ID"] != "task-1" || got.Env["KANDEV_SESSION_ID"] != "session-1" {
		t.Fatalf("Env = %+v, want the task and session IDs injected", got.Env)
	}
	if got.Env["EXISTING"] != "value" {
		t.Fatalf("Env = %+v, want the caller's env preserved", got.Env)
	}
}

func TestStandaloneExecutorCreateInstanceWithoutWorktreeMetadata(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	instance, err := control.executor(t).CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID:    "instance-1",
		TaskID:        "task-1",
		SessionID:     "session-1",
		WorkspacePath: "/host/workspace",
	})
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	for _, key := range []string{"worktree_id", "worktree_path", "worktree_branch"} {
		if _, present := instance.Metadata[key]; present {
			t.Fatalf("metadata %q must be absent without a worktree: %+v", key, instance.Metadata)
		}
	}
	if got := control.lastCreateRequest(t); got.AgentType != "" {
		t.Fatalf("AgentType = %q, want empty without an agent config", got.AgentType)
	}
}

func TestStandaloneExecutorCreateInstanceSurfacesControlFailure(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.createStatus = http.StatusInternalServerError
	_, err := control.executor(t).CreateInstance(context.Background(), &ExecutorCreateRequest{
		InstanceID: "instance-1",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to create standalone instance") {
		t.Fatalf("error = %v", err)
	}
}

func TestStandaloneExecutorCreateInstanceFailsWhenAgentctlNeverBecomesReady(t *testing.T) {
	control := newStandaloneControlServer(t, false)
	// waitForReady polls every 500ms; give the deadline a wide enough margin
	// that a loaded CI runner still observes several retries.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := control.executor(t).CreateInstance(ctx, &ExecutorCreateRequest{InstanceID: "instance-1"})
	if err == nil || !strings.Contains(err.Error(), "agentctl not ready") {
		t.Fatalf("error = %v, want the readiness wait to fail", err)
	}
	control.mu.Lock()
	calls := control.healthCalls
	control.mu.Unlock()
	if calls < 2 {
		t.Fatalf("health calls = %d, want the wait loop to retry", calls)
	}
}

func TestStandaloneExecutorStopInstance(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	exec := control.executor(t)

	t.Run("no standalone instance is a no-op", func(t *testing.T) {
		if err := exec.StopInstance(context.Background(), &ExecutorInstance{}, false); err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		control.mu.Lock()
		defer control.mu.Unlock()
		if len(control.deleted) != 0 {
			t.Fatalf("deleted = %v, want none", control.deleted)
		}
	})

	t.Run("deletes the tracked instance", func(t *testing.T) {
		err := exec.StopInstance(context.Background(),
			&ExecutorInstance{StandaloneInstanceID: "std-1"}, false)
		if err != nil {
			t.Fatalf("StopInstance: %v", err)
		}
		control.mu.Lock()
		defer control.mu.Unlock()
		if len(control.deleted) != 1 || control.deleted[0] != "std-1" {
			t.Fatalf("deleted = %v", control.deleted)
		}
	})
}

func TestBuildStandaloneCreateInstanceRequestMapsEveryField(t *testing.T) {
	autoApprove := true
	req := &ExecutorCreateRequest{
		InstanceID:                     "instance-2",
		TaskID:                         "task-2",
		SessionID:                      "session-2",
		WorkspacePath:                  "/host/workspace",
		WorkspaceSourceRoots:           []string{"/host/sources"},
		Protocol:                       "acp",
		McpMode:                        "office",
		McpProviders:                   []string{"github"},
		AutoApprovePermissionsOverride: &autoApprove,
		Metadata:                       map[string]interface{}{MetadataKeyBaseBranches: map[string]string{"repo": "dev"}},
	}

	got := buildStandaloneCreateInstanceRequest(req, map[string]string{"K": "V"}, "codex",
		true, true, false, true, []string{"STRIP"})

	if got.ID != "instance-2" || got.WorkspacePath != "/host/workspace" || got.AgentType != "codex" {
		t.Fatalf("request = %+v", got)
	}
	if !got.DisableAskQuestion || !got.AssumeMcpSse || got.AssumeMcpHttp || !got.RequiresProcessKill {
		t.Fatalf("capability flags = %+v", got)
	}
	if got.AutoApprovePermissions == nil || !*got.AutoApprovePermissions {
		t.Fatalf("AutoApprovePermissions = %v", got.AutoApprovePermissions)
	}
	if got.Env["K"] != "V" || !equalStrings(got.StripEnv, []string{"STRIP"}) {
		t.Fatalf("env/strip = %+v / %v", got.Env, got.StripEnv)
	}
	if got.BaseBranches["repo"] != "dev" {
		t.Fatalf("BaseBranches = %+v", got.BaseBranches)
	}
	if got.McpMode != "office" || !equalStrings(got.McpProviders, []string{"github"}) {
		t.Fatalf("mcp routing = %q / %v", got.McpMode, got.McpProviders)
	}
	if !equalStrings(got.WorkspaceSourceRoots, []string{"/host/sources"}) {
		t.Fatalf("WorkspaceSourceRoots = %v", got.WorkspaceSourceRoots)
	}
}
