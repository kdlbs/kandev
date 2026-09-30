package requiredstores

import (
	"testing"

	"github.com/kandev/kandev/internal/startup"
)

// TestCatalogSweepAssignmentIsCompleteAndNonOverlapping is completeness test 1
// from docs/specs/platform/system-design/startup-progress-visibility.md:
// every requiredstores.Catalog() entry is claimed by exactly one store-
// admission sweep step (AC-PLATFORM-STARTUP-PROGRESS-005.2). A Descriptor's
// Sweep field can only ever hold one value, so "exactly one" reduces to
// "a valid one"; this test also locks in the current runtime-order split
// (19 stores.repositories admissions in storage.go, 22 stores.services
// admissions elsewhere) so a future catalog entry silently landing on the
// wrong side of storage.go:215 fails loudly instead of only at startup.
func TestCatalogSweepAssignmentIsCompleteAndNonOverlapping(t *testing.T) {
	catalog := Catalog()

	var repositories, services int
	for _, descriptor := range catalog {
		switch descriptor.Sweep {
		case startup.StepStoresRepositories:
			repositories++
		case startup.StepStoresServices:
			services++
		default:
			t.Errorf("catalog entry %q has sweep %q, want one of stores.repositories/stores.services", descriptor.ID, descriptor.Sweep)
		}
	}

	if total := repositories + services; total != len(catalog) {
		t.Fatalf("sweep-claimed entries = %d, want %d (every catalog entry claimed exactly once)", total, len(catalog))
	}
	if repositories != 19 {
		t.Errorf("stores.repositories claims %d catalog entries, want 19", repositories)
	}
	if services != 22 {
		t.Errorf("stores.services claims %d catalog entries, want 22", services)
	}
}
