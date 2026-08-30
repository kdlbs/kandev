package costs

import commoncosts "github.com/kandev/kandev/internal/common/costs"

// ModelPricing is an alias of the shared costs package's type
// (docs/specs/task-cost-ledger/spec.md) so existing costs.ModelPricing{...}
// call sites and struct literals in this package keep working unchanged.
type ModelPricing = commoncosts.ModelPricing

// CalculateCostSubcents delegates to internal/common/costs, the single
// source of truth for the cost formula shared with the task usage ledger
// writer.
func CalculateCostSubcents(
	tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut int64,
	pricing ModelPricing,
) int64 {
	return commoncosts.CalculateCostSubcents(tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut, pricing)
}

// CalculateCostSubcentsChecked exposes the shared overflow-aware calculation
// to Office callers that need to distinguish an unpriced result from a real
// zero-cost calculation.
func CalculateCostSubcentsChecked(
	tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut int64,
	pricing ModelPricing,
) (int64, bool) {
	return commoncosts.CalculateCostSubcentsChecked(tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut, pricing)
}

// ProviderForModel delegates to internal/common/costs.
func ProviderForModel(model string) string {
	return commoncosts.ProviderForModel(model)
}
