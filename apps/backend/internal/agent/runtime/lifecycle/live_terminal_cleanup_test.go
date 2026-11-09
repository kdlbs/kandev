package lifecycle

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCleanupLiveExecutionKeepsInstanceWithWorkspaceProcesses(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body          string
		wantConfirmed bool
	}{
		{name: "running shell command", status: http.StatusOK, body: `[{"id":"p1","session_id":"live-session","kind":"custom","status":"running"}]`},
		{name: "starting dev server", status: http.StatusOK, body: `[{"id":"p1","session_id":"live-session","kind":"dev","status":"starting"}]`},
		{name: "unreadable process list", status: http.StatusInternalServerError, body: `{}`},
		{name: "only finished processes", status: http.StatusOK, body: `[{"id":"p1","session_id":"live-session","kind":"custom","status":"exited"}]`, wantConfirmed: true},
		{name: "no processes", status: http.StatusOK, body: `[]`, wantConfirmed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/processes" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			client := agentctl.NewClient("127.0.0.1", server.Listener.Addr().(*net.TCPAddr).Port, newTestLogger())

			repo, _ := runOwnerTestRepository(t)
			mgr := newTestManager(t)
			mgr.SetExecutorRunningWriter(repo)
			backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
			mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
			mgr.executorRegistry.Register(backend)
			execution := &AgentExecution{ID: "live", SessionID: "live-session", TaskID: "live-task",
				Status: v1.AgentStatusReady, RuntimeName: executor.NameStandalone,
				standaloneInstanceID: "live", agentctl: client}
			require.NoError(t, mgr.executionStore.Add(execution))
			require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
			row, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
			require.NoError(t, err)

			confirmed, err := mgr.CleanupLiveExecutionIfCurrent(ctx, execution.SessionID, execution.ID, row.UpdatedAt)
			require.NoError(t, err)
			require.Equal(t, tc.wantConfirmed, confirmed)
			_, retained := mgr.executionStore.Get(execution.ID)
			if tc.wantConfirmed {
				require.Equal(t, "live", backend.stoppedID)
				require.False(t, retained)
				return
			}
			require.Empty(t, backend.stoppedID, "active or unknown workspace processes must not be stopped")
			require.True(t, retained)
		})
	}
}
