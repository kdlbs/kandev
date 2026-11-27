package sqlite

import (
	"encoding/json"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.8
func TestPrunedDeliveryRestoreClearsOnlyExactGapFailure(t *testing.T) {
	for _, detail := range []string{"delivery_history_gap: stream old, missing event range 2..6", "delivery_history_gap: stream other, missing event range 2..6", "delivery_history_gap: stream old, missing event range 2..7"} {
		t.Run(detail, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"code": "delivery_history_gap", "details": detail, "message": "The agent could not start"})
			require.NoError(t, err)
			metadata := map[string]json.RawMessage{models.SessionMetaKeyLastAgentError: raw}
			clearSilentRestoreUncertainError(metadata, models.AgentDeliveryRecovery{StreamID: "old", HistoryGap: &models.AgentDeliveryHistoryGap{FirstSequence: 2, LastSequence: 6}})
			_, exists := metadata[models.SessionMetaKeyLastAgentError]
			require.Equal(t, detail != "delivery_history_gap: stream old, missing event range 2..6", exists)
		})
	}
}
