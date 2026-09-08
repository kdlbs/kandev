package costs

import "github.com/kandev/kandev/internal/office/shared"

// degradationBlocks implements REQ-OFFICE-BUDGET-004: an unpriced event
// inside an unattended run's window means priced spend alone can no longer
// be trusted as a ceiling, so this blocks once priced spend alone already
// reaches half the limit — applied identically to a policy and the
// built-in default, regardless of the policy's configured action
// (AC-OFFICE-BUDGET-004.3).
//
// The `pricedSubcents >= limitSubcents-pricedSubcents` form is algebraically
// `2*pricedSubcents >= limitSubcents`, AC-OFFICE-BUDGET-004.3's
// exact-50%-without-truncation requirement (comparing against limit/2 would
// round down and miss an odd limit's exact threshold) -- written this way so
// it never doubles pricedSubcents, which can overflow int64 and wrap
// negative for extreme inputs.
func degradationBlocks(degraded bool, pricedSubcents, limitSubcents int64, provenance shared.RunProvenance) bool {
	return degraded && provenance == shared.RunProvenanceUnattended && pricedSubcents >= limitSubcents-pricedSubcents
}
