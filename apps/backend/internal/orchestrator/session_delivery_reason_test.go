package orchestrator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicDeliveryRecoveryReasonsRemainStableAcrossResponseMapping(t *testing.T) {
	for _, reason := range []string{"unknown_prompt_outcome", "delivery_output_paused", "delivery_cancellation_pending", "delivery_storage_pressure"} {
		response := sessionDeliveryRecoveryResponse("task", "session", SessionDeliveryRecoveryBlocked, publicDeliveryRecoveryReason(reason), 1, nil)
		require.Equal(t, reason, response.Reason)
	}
}
