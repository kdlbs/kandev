package costs

import "strings"

// ModelPricing holds per-million-token pricing for a model. All units
// are hundredths of a cent (subcents) per million tokens — keeps the math
// integer-only and matches the storage unit on office_cost_events.cost_subcents.
type ModelPricing struct {
	InputPerMillion       int64
	CachedReadPerMillion  int64
	CachedWritePerMillion int64
	OutputPerMillion      int64
}

// CalculateCostSubcents computes estimated cost from token counts and pricing.
// All token counts are int64 to match the wire types from streams.PromptUsage.
// Returns 0 if pricing is the zero value. Cached read and cached write are
// passed separately because Anthropic charges different rates (cached writes
// cost ~25% more than the base input rate).
func CalculateCostSubcents(
	tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut int64,
	pricing ModelPricing,
) int64 {
	cost := tokensIn*pricing.InputPerMillion +
		tokensCachedRead*pricing.CachedReadPerMillion +
		tokensCachedWrite*pricing.CachedWritePerMillion +
		tokensOut*pricing.OutputPerMillion
	return cost / 1_000_000
}

// ProviderForModel returns a best-guess provider id for a model name, used
// when the CLI payload doesn't already carry a provider (it does for claude-acp
// once the subscriber sets it from the CLI id). Returns "" when the prefix is
// unknown.
func ProviderForModel(model string) string {
	switch {
	case strings.HasPrefix(model, "claude"):
		return "anthropic"
	case strings.HasPrefix(model, "gpt") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4"):
		return "openai"
	case strings.HasPrefix(model, "gemini"):
		return "google"
	}
	return ""
}
