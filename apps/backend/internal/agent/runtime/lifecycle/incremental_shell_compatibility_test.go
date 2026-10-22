package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/stretchr/testify/require"
)

func TestIncrementalShellRetainedRuntimeCompatibility(t *testing.T) {
	for _, version := range []uint32{1, 2} {
		decision := NegotiateDurableDelivery(journal.StorageCapability{Version: 2, Durable: true}, version, true, true)
		require.Equal(t, DurableDeliveryV1, decision.Mode)
	}
	decision := NegotiateDurableDelivery(journal.StorageCapability{Version: 2, Durable: true}, 3, true, true)
	require.Equal(t, DurableDeliveryBlocked, decision.Mode)
}
