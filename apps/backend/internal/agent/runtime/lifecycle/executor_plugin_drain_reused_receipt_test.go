package lifecycle

import "testing"

// A reused receipt is rebuilt from the stored claim, receipt id and runtime
// identity; the first drain's completion time is not persisted, and plugin.proto
// documents completed_at as empty on such a receipt.
func TestReusedDrainReceiptCarriesNoCompletionTime(t *testing.T) {
	inventory := pluginExecutorInventory{RuntimeIdentity: "runtime-1", DrainOutcome: pluginDrainOutcomeDrained}
	const claim = "claim-1"
	inventory.DrainClaim = claim
	inventory.DrainReceiptID = pluginDrainReceiptID(inventory, claim)

	decision, reused := storedPluginDrainDecision(inventory, claim)
	if !reused {
		t.Fatal("a matching stored receipt was not reused")
	}
	if decision.receipt.GetReceiptId() != inventory.DrainReceiptID || decision.receipt.GetCompletedAt() != "" {
		t.Fatalf("reused receipt = %v, want the stored id and no completion time", decision.receipt)
	}
}
