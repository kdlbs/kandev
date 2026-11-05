package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

// A recovered execution on an agentctl without retained storage has no pinned
// delivery stream: the peer names its stream "<incarnation>:g<generation>",
// not the session ID that keys the legacy replay cursor. The preflight must
// ask the peer for its current stream instead of the session ID, or the peer
// answers OWNER_MISMATCH and the recovery never completes.
func TestDeliveryPreflightRecoveredLegacyExecutionReadsPeerStream(t *testing.T) {
	const peerStream = "incarnation:g1"
	for _, tc := range []struct {
		name        string
		incarnation string
		allowed     bool
	}{
		{"matching legacy owner", "incarnation", true},
		{"different owner is refused", "other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, _ := runOwnerTestRepository(t)
			client := ackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/api/v1/agent/delivery", r.URL.Path)
				if stream := r.URL.Query().Get("stream_id"); stream != "" && stream != peerStream {
					w.WriteHeader(http.StatusConflict)
					require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"code": "OWNER_MISMATCH", "message": "OWNER_MISMATCH"}))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(agentctl.DeliveryStatus{
					SessionID: "session", IncarnationID: tc.incarnation, HarnessGeneration: 1, StreamID: peerStream,
					StorageCapability: journal.StorageCapability{Version: journal.CurrentVersion, Reason: durableDeliveryStorageNotDurable},
				}))
			})
			sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
			t.Cleanup(sm.Wait)
			execution := &AgentExecution{SessionID: "session", DeliveryMode: DurableDeliveryLegacy, DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 1}
			after, err := sm.initialDeliveryReplayCursor(context.Background(), execution, repo, client, "session")
			if tc.allowed {
				require.NoError(t, err)
				require.Zero(t, after)
			} else {
				require.Error(t, err)
			}
		})
	}
}
