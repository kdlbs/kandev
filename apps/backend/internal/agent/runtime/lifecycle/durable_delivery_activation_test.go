package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
)

func TestDurableDeliveryLegacyPeerCompatibility(t *testing.T) {
	decision := NegotiateDurableDelivery(journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, 0, false, false)
	if decision.Mode != DurableDeliveryLegacy {
		t.Fatalf("legacy peer decision = %#v", decision)
	}
}

func TestDurableDeliveryRollbackPreservesUncertainty(t *testing.T) {
	decision := NegotiateDurableDelivery(journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, 0, false, true)
	if decision.Mode != DurableDeliveryBlocked || decision.Reason != "unresolved_durable_work" {
		t.Fatalf("rollback decision = %#v", decision)
	}
}

func TestDurableDeliveryActivatesWithoutConfiguration(t *testing.T) {
	decision := NegotiateDurableDelivery(journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, journal.CurrentVersion, true, false)
	if decision.Mode != DurableDeliveryV1 {
		t.Fatalf("automatic activation decision = %#v", decision)
	}
}

func TestDurableDeliveryKeepsV1ForUnresolvedCompatibleWork(t *testing.T) {
	decision := NegotiateDurableDelivery(journal.StorageCapability{Version: journal.CurrentVersion, Durable: true}, journal.CurrentVersion, true, true)
	if decision.Mode != DurableDeliveryV1 || decision.Reason != "unresolved_durable_work" {
		t.Fatalf("compatible unresolved decision = %#v", decision)
	}
}

func TestJournalFailureNeverSelectsLegacyDelivery(t *testing.T) {
	decision := NegotiateDurableDelivery(journal.StorageCapability{Version: journal.CurrentVersion, Reason: "storage_unavailable"}, 0, false, false)
	if decision.Mode != DurableDeliveryBlocked {
		t.Fatalf("journal failure decision = %#v", decision)
	}
}
