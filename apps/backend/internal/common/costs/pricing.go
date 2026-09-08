// Package costs holds cost-arithmetic and provider-derivation logic shared
// across the backend and agentctl tiers (docs/specs/task-cost-ledger/spec.md).
// It must not import internal/office/** or internal/task/**: both the Office
// cost pipeline and the task usage ledger delegate to this package, not the
// other way around.
package costs

import (
	"math"
	"strings"
)

// ModelPricing holds per-million-token pricing for a model. All units
// are hundredths of a cent (subcents) per million tokens — keeps the math
// integer-only and matches the storage unit on office_cost_events.cost_subcents
// and task_usage_events.cost_subcents.
type ModelPricing struct {
	InputPerMillion       int64
	CachedReadPerMillion  int64
	CachedWritePerMillion int64
	OutputPerMillion      int64
}

// CalculateCostSubcents computes estimated cost from token counts and pricing.
// All token counts are int64 to match the wire types from streams.PromptUsage.
// Returns 0 if pricing is the zero value or an intermediate operation
// overflows int64. Cached read and cached write are passed separately because
// Anthropic charges different rates (cached writes cost ~25% more than the
// base input rate). Call CalculateCostSubcentsChecked when the caller needs
// to distinguish overflow from a legitimate zero cost.
func CalculateCostSubcents(
	tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut int64,
	pricing ModelPricing,
) int64 {
	cost, ok := CalculateCostSubcentsChecked(tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut, pricing)
	if !ok {
		return 0
	}
	return cost
}

// CalculateCostSubcentsChecked computes estimated cost and reports whether
// every intermediate operation fits in int64. A false result prevents a
// wrapped value from entering a durable cost ledger.
func CalculateCostSubcentsChecked(
	tokensIn, tokensCachedRead, tokensCachedWrite, tokensOut int64,
	pricing ModelPricing,
) (int64, bool) {
	terms := [][2]int64{
		{tokensIn, pricing.InputPerMillion},
		{tokensCachedRead, pricing.CachedReadPerMillion},
		{tokensCachedWrite, pricing.CachedWritePerMillion},
		{tokensOut, pricing.OutputPerMillion},
	}
	var total int64
	for _, term := range terms {
		product, ok := checkedMul(term[0], term[1])
		if !ok {
			return 0, false
		}
		total, ok = checkedAdd(total, product)
		if !ok {
			return 0, false
		}
	}
	return total / 1_000_000, true
}

func checkedAdd(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

func checkedMul(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, false
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	return product, true
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

// ProviderFromCLI maps the upstream CLI id (the agent_id/agent_type stream
// field) to a provider name. Used because claude-acp emits logical model
// aliases (sonnet / haiku) that can't be matched on prefix.
func ProviderFromCLI(cli string) string {
	switch cli {
	case "claude-acp":
		return "anthropic"
	case "codex-acp", "openai-acp":
		return "openai"
	case "gemini", "gemini-acp":
		return "google"
	}
	return ""
}
