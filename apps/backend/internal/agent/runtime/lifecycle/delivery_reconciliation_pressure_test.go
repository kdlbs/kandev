package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/stretchr/testify/require"
)

func TestDeliveryRetryReportsPausedProducerWithoutReplacingLiveProcess(t *testing.T) {
	for _, testcase := range []struct {
		health string
		reason string
	}{
		{`{"state":"guarded","producer_paused":true}`, "delivery_output_paused"},
		{`{"state":"cancelling","cancellation_pending":true}`, "delivery_cancellation_pending"},
		{`{"state":"guarded","producer_paused":false}`, "delivery_storage_pressure"},
	} {
		t.Run(testcase.reason, func(t *testing.T) {
			sm := newDeliveryReconciliationTestManager(newReconciliationTestClock())
			peer := newDeliveryReconciliationTestPeer()
			peer.streamOpen = true
			peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) {
				var status agentctl.StatusResponse
				require.NoError(t, json.Unmarshal([]byte(`{"agent_status":"running","delivery_health":`+testcase.health+`}`), &status))
				return &status, nil
			}
			execution := newDeliveryReconciliationTestExecution()
			result := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
			require.Equal(t, DeliveryReconciliationBlocked, result.Outcome)
			require.Equal(t, testcase.reason, result.Reason)
			require.False(t, result.ProcessTerminated)
			require.Equal(t, "running", string(execution.Status))
		})
	}
}
