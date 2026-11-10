package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type recoveryLeaseWriter struct {
	captureExecutorRunningWriter
	execution      *AgentExecution
	leaseAvailable bool
	reads          int
}

func (w *recoveryLeaseWriter) GetExecutorRunningBySessionID(ctx context.Context, id string) (*models.ExecutorRunning, error) {
	w.reads++
	w.leaseAvailable = w.execution.agentctlLifecycleMu.TryLock()
	if w.leaseAvailable {
		w.execution.agentctlLifecycleMu.Unlock()
	}
	return w.captureExecutorRunningWriter.GetExecutorRunningBySessionID(ctx, id)
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.2
func TestRecoverAgentPromptStreamReleasesLeaseBeforePersistence(t *testing.T) {
	peer := newMockAgentServer(t)
	t.Cleanup(peer.Close)
	client := createTestClient(t, peer.server.URL)
	t.Cleanup(client.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, client.StreamUpdates(ctx, func(agentctl.AgentEvent) {}, nil, nil))
	manager := newTestManager(t)
	execution := &AgentExecution{ID: "execution", SessionID: "session", ACPSessionID: "native-session",
		Status: v1.AgentStatusFailed, agentctl: client, sessionInitialized: true,
		promptDoneCh: make(chan PromptCompletionSignal, 1)}
	require.NoError(t, manager.executionStore.Add(execution))
	writer := &recoveryLeaseWriter{execution: execution}
	manager.SetExecutorRunningWriter(writer)
	require.NoError(t, manager.RecoverAgentPromptStream(ctx, execution.SessionID))
	require.Equal(t, 1, writer.reads, "the real readiness persistence must run")
	require.True(t, writer.leaseAvailable, "recovery must release the lease before persistence begins")
	require.NotNil(t, writer.running)
	require.Equal(t, models.ExecutorRunningStatusReady, writer.running.Status)
	require.Equal(t, client.BaseURL(), writer.running.AgentctlURL)
}

func TestRestoreRecoveredFailedExecutionReleasesLeaseOnErrors(t *testing.T) {
	for _, scenario := range []string{"missing client", "status error", "stopped", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if scenario == "status error" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				_, _ = w.Write([]byte(`{"agent_status":"stopped"}`))
			}))
			t.Cleanup(peer.Close)
			execution := &AgentExecution{ID: "execution", Status: v1.AgentStatusFailed}
			if scenario != "missing client" {
				execution.agentctl = createTestClient(t, peer.URL)
				t.Cleanup(execution.agentctl.Close)
			}
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			if scenario == "cancelled" {
				cancel()
			}
			manager := newTestManager(t)
			require.Error(t, manager.restoreRecoveredFailedExecution(ctx, execution))
			available := execution.agentctlLifecycleMu.TryLock()
			if available {
				execution.agentctlLifecycleMu.Unlock()
			}
			require.True(t, available, "failed status verification must release its client lease")
			require.Equal(t, v1.AgentStatusFailed, execution.Status)
		})
	}
}
