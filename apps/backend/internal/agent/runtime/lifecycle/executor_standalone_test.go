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
	"github.com/kandev/kandev/internal/task/models"
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
	// listInstances is what GET /api/v1/instances returns; nil (vs. an empty
	// non-nil slice) tells the fake to answer with a listing failure instead,
	// for exercising AC-EXECUTORS-SURVIVAL-002.12.
	listInstances    []*agentctlclient.InstanceInfo
	listInstancesErr bool
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
		case r.URL.Path == "/api/v1/instances" && r.Method == http.MethodGet:
			if s.listInstancesErr {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(struct {
				Instances []*agentctlclient.InstanceInfo `json:"instances"`
			}{Instances: s.listInstances})
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
	instances, err := exec.RecoverInstances(context.Background(), nil)
	if err != nil || len(instances) != 0 {
		t.Fatalf("RecoverInstances() = %v, %v; want none recovered, no error", instances, err)
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

// TestStandaloneExecutorRecoverInstancesReTracksWinnerAndStopsOrphan pins
// AC-EXECUTORS-SURVIVAL-002.1/002.2/002.6: a live instance whose session
// matches a recovery-inventory record is re-tracked and returned; a live
// instance with no matching record is stopped as an orphan.
func TestStandaloneExecutorRecoverInstancesReTracksWinnerAndStopsOrphan(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "instance-1", Port: 5001, SessionID: "session-1", TaskID: "task-1", WorkspacePath: "/ws/1"},
		{ID: "orphan-instance", Port: 5002, SessionID: "session-orphan"},
	}
	exec := control.executor(t)
	exec.SetAuthToken("survival-token")

	records := []*models.ExecutorRunning{
		{SessionID: "session-1", TaskID: "task-1", AgentExecutionID: "instance-1", Metadata: map[string]interface{}{"k": "v"}},
	}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered = %+v, want exactly one winner", recovered)
	}
	got := recovered[0]
	if got.SessionID != "session-1" || got.TaskID != "task-1" || got.InstanceID != "instance-1" {
		t.Fatalf("recovered instance identity = %+v", got)
	}
	if got.WorkspacePath != "/ws/1" {
		t.Fatalf("WorkspacePath = %q, want the instance's live workspace path", got.WorkspacePath)
	}
	if got.Metadata["k"] != "v" {
		t.Fatalf("Metadata = %+v, want the record's persisted metadata", got.Metadata)
	}
	if got.Client == nil {
		t.Fatal("recovered instance must carry a usable agentctl client")
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 1 || control.deleted[0] != "orphan-instance" {
		t.Fatalf("deleted = %v, want the orphan instance stopped", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesStopsDuplicateLoser pins
// AC-EXECUTORS-SURVIVAL-002.10: of two live instances for one session, the
// one matching the record's agent execution identifier is re-tracked and the
// other is stopped.
func TestStandaloneExecutorRecoverInstancesStopsDuplicateLoser(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = []*agentctlclient.InstanceInfo{
		{ID: "winner", Port: 5001, SessionID: "session-1"},
		{ID: "loser", Port: 5002, SessionID: "session-1"},
	}
	exec := control.executor(t)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "winner"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 1 || recovered[0].InstanceID != "winner" {
		t.Fatalf("recovered = %+v, want only the winner", recovered)
	}

	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 1 || control.deleted[0] != "loser" {
		t.Fatalf("deleted = %v, want the loser stopped", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesRecordWithNoInstanceIsUntouched pins
// AC-EXECUTORS-SURVIVAL-002.7: a record with no live instance is left alone,
// not stopped and not reported recovered.
func TestStandaloneExecutorRecoverInstancesRecordWithNoInstanceIsUntouched(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstances = nil
	exec := control.executor(t)

	records := []*models.ExecutorRunning{{SessionID: "session-cold", AgentExecutionID: "instance-cold"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 0 {
		t.Fatalf("deleted = %v, want none: a record with no instance is left to the existing repair path", control.deleted)
	}
}

// TestStandaloneExecutorRecoverInstancesEnumerationFailureRecoversNothing
// pins AC-EXECUTORS-SURVIVAL-002.12: when the adopted server cannot be
// enumerated, recovery reports nothing recovered, stops nothing, and does
// not error out (a hard error here would fail backend startup outright).
func TestStandaloneExecutorRecoverInstancesEnumerationFailureRecoversNothing(t *testing.T) {
	control := newStandaloneControlServer(t, true)
	control.listInstancesErr = true
	exec := control.executor(t)

	records := []*models.ExecutorRunning{{SessionID: "session-1", AgentExecutionID: "instance-1"}}

	recovered, err := exec.RecoverInstances(context.Background(), records)
	if err != nil {
		t.Fatalf("RecoverInstances: %v, want a logged-and-swallowed enumeration failure", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered = %+v, want none", recovered)
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if len(control.deleted) != 0 {
		t.Fatalf("deleted = %v, want none: an unenumerable server must not have instances stopped blind", control.deleted)
	}
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
