package runtime

import (
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

// DurableDeliveryMode and DurableDeliveryDecision expose the transport
// negotiation result through the runtime seam used by higher-level callers.
type DurableDeliveryMode = lifecycle.DurableDeliveryMode
type DurableDeliveryDecision = lifecycle.DurableDeliveryDecision
type DurableDeliveryCapability = lifecycle.DurableDeliveryCapability

const (
	DurableDeliveryV1      = lifecycle.DurableDeliveryV1
	DurableDeliveryLegacy  = lifecycle.DurableDeliveryLegacy
	DurableDeliveryBlocked = lifecycle.DurableDeliveryBlocked
)

// NegotiateDurableDelivery keeps protocol negotiation owned by the lifecycle
// implementation while allowing admission callers to use the runtime seam.
func NegotiateDurableDelivery(
	local journal.StorageCapability,
	peerVersion uint32,
	peerDurable, unresolved bool,
) DurableDeliveryDecision {
	return lifecycle.NegotiateDurableDelivery(local, peerVersion, peerDurable, unresolved)
}
