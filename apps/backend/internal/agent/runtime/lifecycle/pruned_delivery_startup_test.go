package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.2
func TestPrunedDeliveryStartupReportsAuthenticatedGapBeforeWebSocketAndACP(t *testing.T) {
	for _, mode := range []DurableDeliveryMode{DurableDeliveryV1, DurableDeliveryLegacy} {
		t.Run(string(mode), func(t *testing.T) {
			repo, connection := runOwnerTestRepository(t)
			// Bound recovery rather than database schema initialization.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Gap diagnosis"}))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "session"}))
			_, err := repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1,
				StreamID: "session:g1", Sequence: 1, EventType: "context_window", Payload: []byte(`{}`)}, 1)
			require.NoError(t, err)
			_, err = connection.ExecContext(ctx, `UPDATE agent_delivery_cursors SET projected_sequence = 1 WHERE stream_id = 'session:g1'`)
			require.NoError(t, err)
			var streamDials atomic.Int32
			client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/agent/delivery" {
					require.Equal(t, "session:g1", r.URL.Query().Get("stream_id"))
					status := agentctl.DeliveryStatus{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session:g1",
						StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Durable: true},
						Stream:            &journal.Stream{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session:g1", HighWater: 6, Acknowledged: 6, FirstRetained: 7}}
					require.NoError(t, json.NewEncoder(w).Encode(status))
					return
				}
				if r.URL.Path == "/api/v1/agent/stream" {
					streamDials.Add(1)
				}
				w.WriteHeader(http.StatusNotFound)
			})
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			sm.setAgentDeliveryRepository(repo)
			execution := &AgentExecution{ID: "execution", SessionID: "session", TaskID: "task", agentctl: client,
				DeliveryMode: mode, DeliveryStreamID: "session:g1", DeliveryIncarnationID: "session", DeliveryHarnessGeneration: 1}
			err = sm.ConnectAllAndWait(ctx, execution)
			require.ErrorContains(t, err, "delivery_history_gap")
			require.ErrorContains(t, err, "2..6")
			require.Zero(t, streamDials.Load(), "a diagnosed gap must not fall through to a doomed socket or generic ACP timeout")
		})
	}
}
