package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotWorkspaceRebindAndRollback(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		server := newWorkspaceRebindAgentctlServer(t, false)
		t.Cleanup(server.Close)
		t.Cleanup(server.closeConnections)
		manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
		execution.Status = v1.AgentStatusReady
		execution.ACPSessionID = "acp-existing"
		execution.WorkspacePath = "/old-workspace"
		paths := []string{}
		manager.SetExecutionSnapshotRecorder(func(_ context.Context, got *AgentExecution, snapshot models.ExecutionSnapshot, launch *contract.Launch) error {
			require.Nil(t, launch)
			require.Equal(t, "chat", snapshot.Transport)
			paths = append(paths, got.WorkspacePath)
			if refuse {
				return errors.New("snapshot write failed")
			}
			return nil
		})
		err := manager.RebindWorkspaceForSession(t.Context(), execution.SessionID, "/new-workspace", []string{"/attached"})
		server.mu.Lock()
		starts := server.startCount
		server.mu.Unlock()
		if refuse {
			require.Error(t, err)
			require.Zero(t, starts)
			require.Equal(t, []string{"/new-workspace", "/old-workspace"}, paths)
		} else {
			require.NoError(t, err)
			require.Equal(t, 1, starts)
			require.Equal(t, []string{"/new-workspace"}, paths)
		}
	}
}

func TestExecutionSnapshotWorkspaceRebindPreservesSandboxScope(t *testing.T) {
	server := newWorkspaceRebindAgentctlServer(t, false)
	t.Cleanup(server.Close)
	t.Cleanup(server.closeConnections)
	manager, execution := workspaceSourceTestManager(t, server.URL, []string{"/old"})
	execution.Status = v1.AgentStatusReady
	execution.ACPSessionID = "acp-existing"
	execution.WorkspacePath = "/old-workspace"
	execution.SandboxSelection = &sandbox.SelectionContext{OrgID: "org"}
	stops := 0
	server.onStop = func() { stops++ }
	require.Error(t, manager.RebindWorkspaceForSession(t.Context(), execution.SessionID, "/new-workspace"))
	require.Empty(t, server.reboundPaths())
	require.Zero(t, stops)
	require.Equal(t, "/old-workspace", execution.WorkspacePath)
}
