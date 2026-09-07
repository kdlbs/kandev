package costs

import (
	"testing"

	"github.com/kandev/kandev/internal/office/shared"
)

// TestDegradationBlocks pins REQ-OFFICE-BUDGET-004: an unpriced event inside
// an unattended run's window blocks admission once priced spend alone
// reaches 50% of the limit, applying regardless of the policy's configured
// action (AC-OFFICE-BUDGET-004.3).
func TestDegradationBlocks(t *testing.T) {
	tests := []struct {
		name           string
		degraded       bool
		pricedSubcents int64
		limitSubcents  int64
		provenance     shared.RunProvenance
		want           bool
	}{
		{
			name:     "degraded unattended at exactly 50% blocks (non-truncating threshold)",
			degraded: true, pricedSubcents: 500, limitSubcents: 1000,
			provenance: shared.RunProvenanceUnattended, want: true,
		},
		{
			name:     "degraded unattended above 50% blocks",
			degraded: true, pricedSubcents: 600, limitSubcents: 1000,
			provenance: shared.RunProvenanceUnattended, want: true,
		},
		{
			name:     "degraded unattended below 50% does not block",
			degraded: true, pricedSubcents: 499, limitSubcents: 1000,
			provenance: shared.RunProvenanceUnattended, want: false,
		},
		{
			name:     "not degraded never blocks regardless of spend",
			degraded: false, pricedSubcents: 999999, limitSubcents: 1000,
			provenance: shared.RunProvenanceUnattended, want: false,
		},
		{
			name:     "attended run is exempt even when degraded and over threshold",
			degraded: true, pricedSubcents: 600, limitSubcents: 1000,
			provenance: shared.RunProvenanceAttended, want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := degradationBlocks(tt.degraded, tt.pricedSubcents, tt.limitSubcents, tt.provenance)
			if got != tt.want {
				t.Errorf("degradationBlocks(%v, %d, %d, %v) = %v, want %v",
					tt.degraded, tt.pricedSubcents, tt.limitSubcents, tt.provenance, got, tt.want)
			}
		})
	}
}
