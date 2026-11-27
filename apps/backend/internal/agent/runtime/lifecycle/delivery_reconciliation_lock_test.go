package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

func TestDeliveryReconciliationAllowsQueuedClientReplacement(t *testing.T) {
	entered, respond := make(chan struct{}), make(chan struct{})
	execution := newDeliveryReconciliationTestExecution()
	execution.agentctl = ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/status":
			close(entered)
			<-respond
			_ = json.NewEncoder(w).Encode(agentctl.StatusResponse{AgentStatus: "running"})
		case "/api/v1/agent/delivery":
			_ = json.NewEncoder(w).Encode(agentctl.DeliveryStatus{StorageCapability: journal.StorageCapability{Durable: true}, SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID})
		default:
			_ = json.NewEncoder(w).Encode(journal.Submission{ID: "submission-1", SessionID: execution.SessionID, IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration, StreamID: execution.DeliveryStreamID, State: journal.SubmissionDispatching})
		}
	})
	t.Cleanup(func() {
		select {
		case <-respond:
		default:
			close(respond)
		}
	})
	sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
	result := make(chan DeliveryReconciliationResult, 1)
	go func() { result <- sm.ReconcileAgentDelivery(context.Background(), execution) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("reconciliation did not query the peer")
	}
	replaced := make(chan struct{})
	go func() {
		execution.agentctlLifecycleMu.Lock()
		execution.replaceAgentctlClient(agentctl.NewClient("127.0.0.1", 1, newTestLogger()))
		execution.agentctlLifecycleMu.Unlock()
		close(replaced)
	}()
	require.Eventually(t, func() bool {
		if execution.agentctlLifecycleMu.TryRLock() {
			execution.agentctlLifecycleMu.RUnlock()
			return false
		}
		return true
	}, 5*time.Second, time.Millisecond)
	close(respond)
	select {
	case got := <-result:
		require.Equal(t, DeliveryReconciliationOwnerMismatch, got.Outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("reconciliation deadlocked behind the queued replacement")
	}
	select {
	case <-replaced:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement remained blocked")
	}
}
