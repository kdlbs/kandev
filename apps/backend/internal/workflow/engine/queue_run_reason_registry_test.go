package engine_test

// TestDefaultQueueReasonResolvesInWakeReasonRegistry is the
// internal/workflow/engine-side half of AC-OFFICE-BACKPRESSURE-001.4/.8's
// exhaustiveness requirement. shared.WakeReasonRegistry's own completeness
// test (internal/office/shared/wakereasons_completeness_test.go) walks
// internal/office for RunReasonXxx/legacyRunReasonXxx constants; it cannot
// reach defaultQueueReasonR here because internal/office/shared imports
// internal/workflow/engine (QueueRunCallback's RunQueueAdapter dependency),
// so this package cannot alias shared's constants without an import cycle,
// and the raw literal is declared under a name that walk does not match.
// This test closes that gap directly: it fails if "queue_run" — the value
// QueueRunCallback.queueRunReason falls back to when an action configures
// no reason and the trigger name is empty — is ever removed from the
// registry.
import (
	"testing"

	officeshared "github.com/kandev/kandev/internal/office/shared"
)

func TestDefaultQueueReasonResolvesInWakeReasonRegistry(t *testing.T) {
	if _, ok := officeshared.PriorityClassForReason("queue_run"); !ok {
		t.Error(`shared.WakeReasonRegistry is missing "queue_run" (internal/workflow/engine's ` +
			"defaultQueueReasonR). AC-OFFICE-BACKPRESSURE-001.4/.8 requires every declared wake " +
			"reason to resolve through an explicit registry rule. Add it to WakeReasonRegistry in " +
			"internal/office/shared/wakereasons.go.")
	}
}
