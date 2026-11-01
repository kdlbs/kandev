package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func newSSHProbeManager(t *testing.T, rt ExecutorBackend) (*Manager, *AgentExecution) {
	t.Helper()
	mgr := newTestManager(t)
	mgr.executorRegistry = NewExecutorRegistry(newTestLogger())
	mgr.executorRegistry.Register(rt)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "status unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	client := agentctl.NewClient("127.0.0.1", server.Listener.Addr().(*net.TCPAddr).Port, newTestLogger())
	t.Cleanup(client.Close)
	execution := &AgentExecution{ID: "execution", SessionID: "session", RuntimeName: rt.Name(), agentctl: client}
	require.NoError(t, mgr.executionStore.Add(execution))
	return mgr, execution
}

func TestManagerSSHStatusFailureRequiresConfirmedAbsence(t *testing.T) {
	for _, name := range []string{"absent", "alive", "permission", "inconclusive", "transport closed", "transport lost", "transport unresponsive", "missing pid", "missing client", "missing instance", "non SSH", "missing registry", "unregistered runtime", "missing runtime name"} {
		t.Run(name, func(t *testing.T) {
			rt := NewSSHExecutor(nil, nil, nil, newTestLogger())
			server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
				if command != "kill -0 4242" {
					t.Errorf("unexpected remote command: %s", command)
					return sshFail("unexpected command")
				}
				switch name {
				case "alive":
					return sshOK
				case "permission":
					return sshFail("kill: Operation not permitted")
				case "inconclusive":
					return sshFail("")
				default:
					return sshFail("kill: no such process")
				}
			})
			state := &sshSessionState{client: server.dial(t), pid: 4242}
			switch name {
			case "transport closed":
				require.NoError(t, state.client.Close())
			case "transport lost":
				state.transportLost = true
			case "transport unresponsive":
				state.watchdog = &sshKeepaliveWatchdog{interval: time.Second}
				state.lastProbeReply = time.Now().Add(-time.Hour)
			case "missing pid":
				state.pid = 0
			case "missing client":
				state.client = nil
			}
			if name != "missing instance" {
				rt.sessions["execution"] = state
			}
			var backend ExecutorBackend = rt
			if name == "non SSH" {
				backend = &MockExecutor{name: executor.NameStandalone}
			}
			mgr, execution := newSSHProbeManager(t, backend)
			switch name {
			case "missing registry":
				mgr.executorRegistry = nil
			case "unregistered runtime":
				execution.RuntimeName = executor.NameDocker
			case "missing runtime name":
				execution.RuntimeName = ""
			}
			running, err := mgr.ProbeAgentRunningForSession(context.Background(), "session")
			require.False(t, running)
			if name == "absent" {
				require.NoError(t, err, "confirmed remote absence must permit idle reclaim")
			} else {
				require.ErrorContains(t, err, "503", "uncertain liveness must retain status failure")
			}
			switch name {
			case "absent", "alive", "permission", "inconclusive":
				require.Len(t, server.execCalls(), 1)
			default:
				require.Empty(t, server.execCalls())
			}
		})
	}
}

func TestManagerSSHAbsenceProbeRejectsConcurrentInvalidation(t *testing.T) {
	for _, name := range []string{"replacement", "removal", "transport lost", "cancellation", "unchanged"} {
		t.Run(name, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			// Release the server even if a foreground assertion fails.
			defer close(release)
			rt := NewSSHExecutor(nil, nil, nil, newTestLogger())
			server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
				if command != "kill -0 4242" {
					return sshFail("unexpected command")
				}
				close(entered)
				<-release
				return sshFail("no such process")
			})
			state := &sshSessionState{client: server.dial(t), pid: 4242}
			rt.sessions["execution"] = state
			mgr, _ := newSSHProbeManager(t, rt)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := mgr.ProbeAgentRunningForSession(ctx, "session")
				done <- err
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("remote probe did not start")
			}
			replacement := &sshSessionState{pid: 8888}
			rt.mu.Lock()
			switch name {
			case "replacement":
				rt.sessions["execution"] = replacement
			case "removal":
				delete(rt.sessions, "execution")
			case "transport lost":
				state.transportLost = true
			case "cancellation":
				cancel()
			}
			rt.mu.Unlock()
			// A token releases one response; deferred close also handles test failures.
			release <- struct{}{}
			select {
			case err := <-done:
				if name == "unchanged" {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "503")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("probe did not finish")
			}
			if name == "replacement" {
				require.Same(t, replacement, rt.sessions["execution"])
				require.Equal(t, 8888, replacement.pid)
			}
		})
	}
}

func TestManagerSSHConfirmedAbsenceAllowsResumeSafeCleanup(t *testing.T) {
	rt := NewSSHExecutor(nil, nil, nil, newTestLogger())
	server := newFakeSSHServer(t, func(command, _ string) sshExecResult {
		if command == "kill -0 4242" {
			return sshFail("no such process")
		}
		if !strings.Contains(command, "kill 4242") {
			t.Errorf("cleanup used unexpected remote command: %s", command)
			return sshFail("unexpected command")
		}
		return sshOK
	})
	rt.sessions["execution"] = &sshSessionState{client: server.dial(t), pid: 4242, remoteDir: "/owned/session"}
	mgr, execution := newSSHProbeManager(t, rt)
	writer := &invariantWriter{prior: &models.ExecutorRunning{
		SessionID: execution.SessionID, AgentExecutionID: execution.ID,
		Runtime: executor.NameSSH, Status: models.ExecutorRunningStatusRunning,
		ResumeToken: "preserve-conversation", WorktreePath: "/owned/worktree",
	}}
	mgr.runningWriter = writer
	running, err := mgr.ProbeAgentRunningForSession(context.Background(), execution.SessionID)
	require.NoError(t, err)
	require.False(t, running)
	require.NoError(t, mgr.CleanupStaleExecutionBySessionIDIfCurrent(context.Background(), execution.SessionID, execution.ID, time.Time{}))
	_, tracked := mgr.GetExecutionBySessionID(execution.SessionID)
	require.False(t, tracked, "stale execution must release in-memory ownership")
	require.Empty(t, rt.sessions, "SSH instance must be released")
	require.False(t, writer.deleted)
	require.True(t, writer.repaired)
	require.Equal(t, models.ExecutorRunningStatusStopped, writer.prior.Status)
	require.Equal(t, "preserve-conversation", writer.prior.ResumeToken)
	require.Equal(t, "/owned/worktree", writer.prior.WorktreePath)
}

func TestManagerSSHSuccessfulStatusDoesNotProbeRemoteProcess(t *testing.T) {
	for _, status := range []string{"running", "stopped"} {
		t.Run(status, func(t *testing.T) {
			rt := NewSSHExecutor(nil, nil, nil, newTestLogger())
			sshServer := newFakeSSHServer(t, func(string, string) sshExecResult {
				t.Error("successful status must not consult remote absence")
				return sshFail("no such process")
			})
			rt.sessions["execution"] = &sshSessionState{client: sshServer.dial(t), pid: 4242}
			mgr, execution := newSSHProbeManager(t, rt)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"agent_status":"` + status + `"}`))
			}))
			t.Cleanup(server.Close)
			execution.agentctl = agentctl.NewClient("127.0.0.1", server.Listener.Addr().(*net.TCPAddr).Port, newTestLogger())
			t.Cleanup(execution.agentctl.Close)
			running, err := mgr.ProbeAgentRunningForSession(context.Background(), execution.SessionID)
			require.NoError(t, err)
			require.Equal(t, status == "running", running)
			require.Empty(t, sshServer.execCalls())
		})
	}
}
