package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDeliveryPreflightRetainedStartupEventBeforeNativeInitialization(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Managed startup"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
	descriptor := agentctl.DeliveryStatus{
		StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
		SessionID:         "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1",
		Stream: &journal.Stream{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "incarnation:g1", HighWater: 1, FirstRetained: 1},
	}
	client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/agent/delivery", r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(descriptor))
	})
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	t.Cleanup(sm.Wait)
	execution := &AgentExecution{SessionID: "session", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1, DeliveryStreamID: "incarnation:g1"}
	execution.deliveryBootstrap.Store(true)
	after, err := sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.NoError(t, err, "a retained pre-initialize child exit must not prevent ACP initialization and its managed-runtime retry")
	require.Zero(t, after, "replay the real startup event; do not fabricate a receipt")
	_, err = repo.GetAgentDeliveryCursor(ctx, execution.DeliveryStreamID)
	require.Error(t, err, "preflight must leave the absent SQL receipt absent")

	execution.ACPSessionID = "existing-native"
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "a restored native conversation is not a fresh bootstrap")
	execution.ACPSessionID = ""
	execution.isResumedSession = true
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "replacement recovery is not a fresh bootstrap")
	execution.isResumedSession = false
	descriptor.SubmissionCount = 1
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "retained prompt work must be reconciled")
	descriptor.SubmissionCount = 0
	descriptor.Stream.Acknowledged = 1
	descriptor.Stream.FirstRetained = 2
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.ErrorIs(t, err, ErrAgentDeliveryHistoryGap, "fresh bootstrap cannot bypass pruned history")
	descriptor.Stream.Acknowledged = 0
	descriptor.Stream.FirstRetained = 1
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 1, NativeSessionID: "committed-native", CreationReason: "native_started",
	}))
	_, err = sm.initialDeliveryReplayCursor(ctx, execution, repo, client, execution.DeliveryStreamID)
	require.Error(t, err, "an existing SQL native generation cannot bypass a missing checkpoint")
}
