package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// fakeAgentctlProcessServer serves the agentctl process endpoints the lifecycle
// manager drives, recording every request so the arguments the manager sent can
// be asserted rather than merely that the call returned nil.
type fakeAgentctlProcessServer struct {
	mu sync.Mutex

	startRequests []agentctl.StartProcessRequest
	stopIDs       []string
	listSessions  []string
	fetchedIDs    []string

	startStatus   agentctl.ProcessStatus
	knownProcess  string
	sessionProcs  []agentctl.ProcessInfo
	stopShouldErr bool
	healthy       bool
}

func newFakeAgentctlProcessServer(t *testing.T) (*fakeAgentctlProcessServer, *agentctl.Client) {
	t.Helper()
	f := &fakeAgentctlProcessServer{startStatus: "running", healthy: true}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/health":
			if !f.healthy {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/processes/start":
			var req agentctl.StartProcessRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			f.startRequests = append(f.startRequests, req)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"process": agentctl.ProcessInfo{
					ID: "proc-1", SessionID: req.SessionID, Command: req.Command, Status: f.startStatus,
				},
			})
		case r.URL.Path == "/api/v1/processes/stop":
			var req struct {
				ProcessID string `json:"process_id"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			f.stopIDs = append(f.stopIDs, req.ProcessID)
			if f.stopShouldErr {
				_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": "already gone"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		case r.URL.Path == "/api/v1/processes":
			f.listSessions = append(f.listSessions, r.URL.Query().Get("session_id"))
			_ = json.NewEncoder(w).Encode(f.sessionProcs)
		case strings.HasPrefix(r.URL.Path, "/api/v1/processes/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v1/processes/")
			f.fetchedIDs = append(f.fetchedIDs, id)
			if f.knownProcess == "" || id != f.knownProcess {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(agentctl.ProcessInfo{ID: id, Status: "running"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	return f, newTestAgentctlClient(t, server.URL, newTestLogger())
}

func (f *fakeAgentctlProcessServer) snapshotStarts() []agentctl.StartProcessRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agentctl.StartProcessRequest(nil), f.startRequests...)
}

func (f *fakeAgentctlProcessServer) snapshotStops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stopIDs...)
}

func newProcessRunnerManager(t *testing.T, client *agentctl.Client) (*Manager, *AgentExecution) {
	t.Helper()
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID: "exec-1", TaskID: "task-1", SessionID: "session-1", agentctl: client,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	return mgr, execution
}

// TestStartProcessForwardsRequestAndManagedGoCache pins what actually reaches
// agentctl, including the managed GOCACHE overlay that the execution carries in
// metadata rather than in the caller's request.
func TestStartProcessForwardsRequestAndManagedGoCache(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	mgr, execution := newProcessRunnerManager(t, client)
	execution.setMetadataValue(managedGoCacheMetadataKey, "/cache/go-build")

	info, err := mgr.StartProcess(context.Background(), StartProcessRequest{
		SessionID:  "session-1",
		Kind:       "script",
		ScriptName: "setup",
		Command:    "make setup",
		WorkingDir: "/workspace",
		Env:        map[string]string{"CI": "1"},
	})

	require.NoError(t, err)
	require.Equal(t, "proc-1", info.ID)

	starts := fake.snapshotStarts()
	require.Len(t, starts, 1)
	require.Equal(t, "session-1", starts[0].SessionID)
	require.Equal(t, agentctl.ProcessKind("script"), starts[0].Kind)
	require.Equal(t, "setup", starts[0].ScriptName)
	require.Equal(t, "make setup", starts[0].Command)
	require.Equal(t, "/workspace", starts[0].WorkingDir)
	require.Equal(t, "1", starts[0].Env["CI"], "the caller's env must not be dropped")
	require.Equal(t, "/cache/go-build", starts[0].Env["GOCACHE"],
		"the managed cache is an install-wide setting the caller never sends")
}

func TestProcessEnvironmentLeavesRequestUntouchedWithoutManagedCache(t *testing.T) {
	execution := &AgentExecution{ID: "exec-1"}
	requested := map[string]string{"CI": "1"}

	require.Equal(t, requested, processEnvironment(execution, requested))

	execution.setMetadataValue(managedGoCacheMetadataKey, "/cache/go-build")
	merged := processEnvironment(execution, requested)
	require.Equal(t, "/cache/go-build", merged["GOCACHE"])
	require.NotContains(t, requested, "GOCACHE",
		"the caller's map must not be mutated in place")
}

func TestProcessRunnerEntryPointsValidateArguments(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	_, err := mgr.StartProcess(ctx, StartProcessRequest{})
	require.ErrorContains(t, err, "session_id is required")
	_, err = mgr.StartProcess(ctx, StartProcessRequest{SessionID: "session-absent"})
	require.ErrorIs(t, err, ErrNoExecutionForSession)

	require.ErrorContains(t, mgr.WaitForAgentctlReadyForSession(ctx, ""), "session_id is required")
	require.ErrorIs(t, mgr.WaitForAgentctlReadyForSession(ctx, "session-absent"), ErrNoExecutionForSession)

	require.ErrorContains(t, mgr.StopProcessForSession(ctx, "", "proc-1"), "session_id is required")
	require.ErrorContains(t, mgr.StopProcessForSession(ctx, "session-1", ""), "process_id is required")
	require.ErrorIs(t, mgr.StopProcessForSession(ctx, "session-absent", "proc-1"), ErrNoExecutionForSession)

	_, err = mgr.ListProcesses(ctx, "")
	require.ErrorContains(t, err, "session_id is required")
	_, err = mgr.ListProcesses(ctx, "session-absent")
	require.ErrorIs(t, err, ErrNoExecutionForSession)

	_, err = mgr.GetProcess(ctx, "", false)
	require.ErrorContains(t, err, "process_id is required")
	require.ErrorContains(t, mgr.StopProcess(ctx, ""), "process_id is required")
}

func TestProcessRunnerEntryPointsRequireAgentctlClient(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-1", SessionID: "session-1"}))

	_, err := mgr.StartProcess(ctx, StartProcessRequest{SessionID: "session-1"})
	require.ErrorContains(t, err, "agentctl client not available for session session-1")
	require.ErrorContains(t, mgr.WaitForAgentctlReadyForSession(ctx, "session-1"),
		"agentctl client not available")
	require.ErrorContains(t, mgr.StopProcessForSession(ctx, "session-1", "proc-1"),
		"agentctl client not available")
	_, err = mgr.ListProcesses(ctx, "session-1")
	require.ErrorContains(t, err, "agentctl client not available")

	// Executions without a client are skipped by the cross-execution scans.
	_, err = mgr.GetProcess(ctx, "proc-1", false)
	require.ErrorContains(t, err, "process not found: proc-1")
	require.ErrorContains(t, mgr.StopProcess(ctx, "proc-1"), "process not found: proc-1")
	require.NoError(t, mgr.StopAllProcesses(ctx))
}

func TestWaitForAgentctlReadyForSessionProbesHealth(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	mgr, _ := newProcessRunnerManager(t, client)

	require.NoError(t, mgr.WaitForAgentctlReadyForSession(context.Background(), "session-1"))

	fake.mu.Lock()
	fake.healthy = false
	fake.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Error(t, mgr.WaitForAgentctlReadyForSession(ctx, "session-1"))
}

func TestStopProcessForSessionForwardsProcessID(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	mgr, _ := newProcessRunnerManager(t, client)

	require.NoError(t, mgr.StopProcessForSession(context.Background(), "session-1", "proc-7"))

	require.Equal(t, []string{"proc-7"}, fake.snapshotStops())
}

func TestStopProcessForSessionSurfacesAgentctlFailure(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	fake.stopShouldErr = true
	mgr, _ := newProcessRunnerManager(t, client)

	err := mgr.StopProcessForSession(context.Background(), "session-1", "proc-7")

	require.ErrorContains(t, err, "already gone")
}

// TestStopProcessScansExecutionsForOwner pins the cross-execution scan: the
// caller supplies only a process ID, so the manager must locate the owning
// execution before issuing the stop.
func TestStopProcessScansExecutionsForOwner(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	fake.knownProcess = "proc-7"
	mgr, _ := newProcessRunnerManager(t, client)

	require.NoError(t, mgr.StopProcess(context.Background(), "proc-7"))
	require.Equal(t, []string{"proc-7"}, fake.snapshotStops())

	require.ErrorContains(t, mgr.StopProcess(context.Background(), "proc-unknown"),
		"process not found: proc-unknown")
	require.Equal(t, []string{"proc-7"}, fake.snapshotStops(),
		"an unowned process ID must not produce a stop call")
}

func TestGetProcessScansExecutions(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	fake.knownProcess = "proc-7"
	mgr, _ := newProcessRunnerManager(t, client)

	got, err := mgr.GetProcess(context.Background(), "proc-7", true)
	require.NoError(t, err)
	require.Equal(t, "proc-7", got.ID)

	_, err = mgr.GetProcess(context.Background(), "proc-unknown", false)
	require.ErrorContains(t, err, "process not found: proc-unknown")
}

func TestListProcessesScopesToSession(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	fake.sessionProcs = []agentctl.ProcessInfo{{ID: "proc-1"}, {ID: "proc-2"}}
	mgr, _ := newProcessRunnerManager(t, client)

	got, err := mgr.ListProcesses(context.Background(), "session-1")

	require.NoError(t, err)
	require.Len(t, got, 2)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, []string{"session-1"}, fake.listSessions,
		"the list must be scoped to the session, not the whole agentctl instance")
}

// TestStopAllProcessesStopsEveryListedProcess pins the shutdown sweep: every
// process agentctl reports for the session is stopped, and per-process failures
// are aggregated rather than aborting the sweep.
func TestStopAllProcessesStopsEveryListedProcess(t *testing.T) {
	fake, client := newFakeAgentctlProcessServer(t)
	fake.sessionProcs = []agentctl.ProcessInfo{{ID: "proc-1"}, {ID: "proc-2"}}
	mgr, _ := newProcessRunnerManager(t, client)

	require.NoError(t, mgr.StopAllProcesses(context.Background()))
	require.Equal(t, []string{"proc-1", "proc-2"}, fake.snapshotStops())

	fake.mu.Lock()
	fake.stopShouldErr = true
	fake.mu.Unlock()
	err := mgr.StopAllProcesses(context.Background())
	require.ErrorContains(t, err, "already gone")
	require.Len(t, fake.snapshotStops(), 4,
		"a failing stop must not abort the sweep over the remaining processes")
}

func TestIsTerminalProcessStatus(t *testing.T) {
	require.True(t, isTerminalProcessStatus("exited"))
	require.True(t, isTerminalProcessStatus("failed"))
	require.True(t, isTerminalProcessStatus("stopped"))
	require.False(t, isTerminalProcessStatus("running"))
	require.False(t, isTerminalProcessStatus(""))
}
