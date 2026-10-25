package orchestrator

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	backendexecutor "github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// absenceRuntime fakes only the executor boundary; HTTP probing, lifecycle
// cleanup, reaper eligibility, and resume-record repair use real implementations.
type absenceRuntime struct {
	lifecycle.ExecutorBackend
	absent   bool
	probeErr error
	stopped  []string
}

func (*absenceRuntime) Name() backendexecutor.Name { return backendexecutor.NameSSH }
func (r *absenceRuntime) ConfirmAgentctlAbsent(_ context.Context, _ string) (bool, error) {
	return r.absent, r.probeErr
}
func (r *absenceRuntime) StopInstance(_ context.Context, instance *lifecycle.ExecutorInstance, _ bool) error {
	r.stopped = append(r.stopped, instance.InstanceID)
	return nil
}

type absenceLifecycleManager struct {
	*mockAgentManager
	manager *lifecycle.Manager
}

func (a *absenceLifecycleManager) ProbeAgentRunningForSession(ctx context.Context, sessionID string) (bool, error) {
	return a.manager.ProbeAgentRunningForSession(ctx, sessionID)
}
func (a *absenceLifecycleManager) CleanupStaleExecutionBySessionIDIfCurrent(ctx context.Context, sessionID, executionID string, updatedAt time.Time) error {
	return a.manager.CleanupStaleExecutionBySessionIDIfCurrent(ctx, sessionID, executionID, updatedAt)
}

func TestIdleReaperRuntimeConfirmedAbsenceReachesCleanup(t *testing.T) {
	for _, name := range []string{"confirmed absent", "alive or unknown", "probe error"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedTaskAndSession(t, repo, "task", "session", models.TaskSessionStateWaitingForInput)
			row := &models.ExecutorRunning{
				ID: "session", SessionID: "session", TaskID: "task", AgentExecutionID: "execution",
				Runtime: backendexecutor.NameSSH, Status: models.ExecutorRunningStatusRunning,
				ResumeToken: "keep-conversation", WorktreePath: "/owned/worktree",
				CreatedAt: time.Now().UTC().Add(-time.Hour), UpdatedAt: time.Now().UTC().Add(-time.Hour),
			}
			require.NoError(t, repo.UpsertExecutorRunning(ctx, row))
			_, err := repo.DB().ExecContext(ctx, `UPDATE executors_running SET updated_at = ? WHERE session_id = ?`, time.Now().UTC().Add(-time.Hour), "session")
			require.NoError(t, err)
			rt := &absenceRuntime{absent: name == "confirmed absent"}
			if name == "probe error" {
				rt.probeErr = errors.New("SSH transport unavailable")
			}
			log := newTestLogger()
			registry := lifecycle.NewExecutorRegistry(log)
			registry.Register(rt)
			mgr := lifecycle.NewManager(nil, nil, registry, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
			t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
			mgr.SetExecutorRunningWriter(repo)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "controller unavailable", http.StatusServiceUnavailable)
			}))
			t.Cleanup(server.Close)
			client := agentctl.NewClient("127.0.0.1", server.Listener.Addr().(*net.TCPAddr).Port, log)
			t.Cleanup(client.Close)
			execution := &lifecycle.AgentExecution{ID: "execution", SessionID: "session", TaskID: "task", RuntimeName: backendexecutor.NameSSH}
			execution.SetAgentCtlClientForTesting(client)
			require.NoError(t, mgr.ExecutionStoreForTesting().Add(execution))
			agent := &absenceLifecycleManager{mockAgentManager: &mockAgentManager{}, manager: mgr}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agent)
			svc.turnService = &inactiveTurnService{}
			svc.idleReaper = newIdleSessionReaper()
			svc.reclaimIdleSessionsOnce(ctx)
			got, err := repo.GetExecutorRunningBySessionID(ctx, "session")
			require.NoError(t, err)
			_, tracked := mgr.GetExecutionBySessionID("session")
			if name == "confirmed absent" {
				require.Equal(t, []string{"execution"}, rt.stopped)
				require.False(t, tracked)
				require.Equal(t, models.ExecutorRunningStatusStopped, got.Status)
			} else {
				require.Empty(t, rt.stopped)
				require.True(t, tracked)
				require.Equal(t, models.ExecutorRunningStatusRunning, got.Status)
			}
			require.Equal(t, "keep-conversation", got.ResumeToken)
			require.Equal(t, "/owned/worktree", got.WorktreePath)
		})
	}
}
