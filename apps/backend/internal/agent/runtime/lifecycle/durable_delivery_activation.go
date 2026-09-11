package lifecycle

import "github.com/kandev/kandev/internal/agentctl/journal"

type DurableDeliveryMode string

const (
	DurableDeliveryV1      DurableDeliveryMode = "durable_v1"
	DurableDeliveryLegacy  DurableDeliveryMode = "legacy"
	DurableDeliveryBlocked DurableDeliveryMode = "blocked"
)

type DurableDeliveryDecision struct {
	Mode   DurableDeliveryMode
	Reason string
}

// DurableDeliveryCapability is the negotiated transport capability exposed
// to admission owners that must choose a queue protocol before dispatch.
type DurableDeliveryCapability struct {
	Version    uint32
	Durable    bool
	Unresolved bool
	Reason     string
}

// NegotiateDurableDelivery activates v1 only when both sides advertise the
// same supported contract and the local journal is healthy. An existing v1
// owner with unresolved work cannot be silently downgraded to legacy.
func NegotiateDurableDelivery(local journal.StorageCapability, peerVersion uint32, peerDurable, unresolved bool) DurableDeliveryDecision {
	if !local.Durable {
		if local.Reason == "storage_not_durable" && !unresolved && !peerDurable && peerVersion == 0 {
			return DurableDeliveryDecision{Mode: DurableDeliveryLegacy, Reason: "legacy_peer"}
		}
		return DurableDeliveryDecision{Mode: DurableDeliveryBlocked, Reason: local.Reason}
	}
	if local.Version != journal.CurrentVersion || peerVersion != journal.CurrentVersion || !peerDurable {
		if peerVersion == 0 && !peerDurable {
			if unresolved {
				return DurableDeliveryDecision{Mode: DurableDeliveryBlocked, Reason: "unresolved_durable_work"}
			}
			return DurableDeliveryDecision{Mode: DurableDeliveryLegacy, Reason: "legacy_peer"}
		}
		return DurableDeliveryDecision{Mode: DurableDeliveryBlocked, Reason: "incompatible_delivery_version"}
	}
	if unresolved {
		return DurableDeliveryDecision{Mode: DurableDeliveryV1, Reason: "unresolved_durable_work"}
	}
	return DurableDeliveryDecision{Mode: DurableDeliveryV1, Reason: "compatible_retained_storage"}
}
