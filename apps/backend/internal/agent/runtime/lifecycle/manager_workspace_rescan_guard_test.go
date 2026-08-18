package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
)

type recordedRescan struct {
	WorkDir     string   `json:"work_dir"`
	SourceRoots []string `json:"workspace_source_roots"`
}

// newRescanServer serves a fixed status for the lifetime of the server. The
// status is passed by value rather than by pointer so the handler goroutine
// cannot race a mid-run write: these tests each want one status throughout.
// (fakeAgentctlProcessServer uses atomic.Int32 because it genuinely does flip
// its status mid-test; matching the shape to the requirement keeps the
// difference between the two helpers visible.)
func newRescanServer(t *testing.T, status int) (*[]recordedRescan, string) {
	t.Helper()
	var mu sync.Mutex
	recorded := &[]recordedRescan{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body recordedRescan
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		*recorded = append(*recorded, body)
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	return recorded, server.URL
}

// TestRescanWorkspaceForSessionForwardsWorkDirAndRoots pins what reaches
// agentctl and, critically, that the execution's recorded source roots are
// updated to match — a rescan that succeeded but left the old roots on the
// execution would send stale roots on the next call.
func TestRescanWorkspaceForSessionForwardsWorkDirAndRoots(t *testing.T) {
	recorded, url := newRescanServer(t, http.StatusOK)
	mgr := newTestManager(t)
	execution := &AgentExecution{
		ID: "exec-1", SessionID: "session-1",
		WorkspaceSourceRoots: []string{"/work/task-1/backend"},
		agentctl:             newTestAgentctlClient(t, url, newTestLogger()),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	newRoots := []string{"/work/task-1/backend", "/work/task-1/frontend"}
	require.NoError(t, mgr.RescanWorkspaceForSession(
		context.Background(), "session-1", "/work/task-1", newRoots))

	require.Len(t, *recorded, 1)
	require.Equal(t, "/work/task-1", (*recorded)[0].WorkDir)
	require.Equal(t, newRoots, (*recorded)[0].SourceRoots)
	require.Equal(t, newRoots, execution.WorkspaceSourceRoots,
		"a successful rescan commits the new roots to the execution")
}

// TestRescanWorkspaceForSessionRollsBackRootsOnFailure pins the rollback: a
// failed rescan must not leave the execution claiming roots agentctl never
// accepted.
func TestRescanWorkspaceForSessionRollsBackRootsOnFailure(t *testing.T) {
	_, url := newRescanServer(t, http.StatusInternalServerError)
	mgr := newTestManager(t)
	original := []string{"/work/task-1/backend"}
	execution := &AgentExecution{
		ID: "exec-1", SessionID: "session-1",
		WorkspaceSourceRoots: original,
		agentctl:             newTestAgentctlClient(t, url, newTestLogger()),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.RescanWorkspaceForSession(context.Background(), "session-1", "/work/task-1",
		[]string{"/work/task-1/backend", "/work/task-1/frontend"})

	require.ErrorContains(t, err, "rescan workspace via agentctl")
	require.Equal(t, original, execution.WorkspaceSourceRoots,
		"a failed rescan must restore the roots agentctl is actually tracking")
}

func TestRescanWorkspaceForSessionSkipsWithoutExecutionOrClient(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	require.ErrorContains(t, mgr.RescanWorkspaceForSession(ctx, "", ""), "session_id is required")
	require.NoError(t, mgr.RescanWorkspaceForSession(ctx, "session-absent", ""),
		"no execution means agentctl is not running; the next launch rebuilds trackers")

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "exec-1", SessionID: "session-1"}))
	require.NoError(t, mgr.RescanWorkspaceForSession(ctx, "session-1", ""),
		"an execution without an agentctl client has nothing to rescan")
}

func TestOptionalWorkspaceSourceRoots(t *testing.T) {
	current := []string{"/work/backend"}

	unchanged := optionalWorkspaceSourceRoots(current, nil)
	require.Equal(t, current, unchanged)
	require.NotSame(t, &current[0], &unchanged[0],
		"the caller must get a copy so a later mutation cannot reach the execution")

	replaced := optionalWorkspaceSourceRoots(current, [][]string{{"/work/frontend"}})
	require.Equal(t, []string{"/work/frontend"}, replaced)

	cleared := optionalWorkspaceSourceRoots(current, [][]string{nil})
	require.Empty(t, cleared, "an explicit empty root list clears the tracked roots")
}

// TestManagerStartRecoversInstancesIntoStore pins the restart path: instances
// a backend reports as still running are re-tracked so the frontend reattaches
// instead of creating a duplicate execution.
func TestManagerStartRecoversInstancesIntoStore(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID:    "exec-recovered",
			TaskID:        "task-1",
			SessionID:     "session-1",
			ContainerID:   "container-1",
			ContainerIP:   "10.0.0.2",
			WorkspacePath: "/workspace",
			RuntimeName:   executor.NameStandalone,
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })

	require.NoError(t, mgr.Start(context.Background()))

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	require.True(t, ok, "a recovered instance must be re-tracked under its session")
	require.Equal(t, "exec-recovered", execution.ID)
	require.Equal(t, "container-1", execution.ContainerID)
	require.Equal(t, "/workspace", execution.WorkspacePath)
}

func TestManagerStartWithoutRegistryIsNoOp(t *testing.T) {
	mgr := newTestManager(t)

	require.NoError(t, mgr.Start(context.Background()))
	require.Empty(t, mgr.ListExecutions())
}
