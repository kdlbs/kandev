package service

// ModelPricing holds per-million-token pricing for a model.
type ModelPricing struct {
	InputPerMillion  int // cost in hundredths of a cent per million input tokens
	CachedPerMillion int // cost in hundredths of a cent per million cached input tokens
	OutputPerMillion int // cost in hundredths of a cent per million output tokens
}

// pricingTable is a hardcoded in-memory pricing map for common models.
// Prices are in hundredths of a cent per million tokens.
// Example: $3/M input = 300 (cents per million).
var pricingTable = map[string]ModelPricing{
	// Anthropic
	"claude-sonnet-4-20250514": {InputPerMillion: 300, CachedPerMillion: 30, OutputPerMillion: 1500},
	"claude-opus-4-20250514":   {InputPerMillion: 1500, CachedPerMillion: 150, OutputPerMillion: 7500},
	"claude-haiku-35-20241022": {InputPerMillion: 80, CachedPerMillion: 8, OutputPerMillion: 400},

	// Aliases without date suffix
	"claude-sonnet-4":  {InputPerMillion: 300, CachedPerMillion: 30, OutputPerMillion: 1500},
	"claude-opus-4":    {InputPerMillion: 1500, CachedPerMillion: 150, OutputPerMillion: 7500},
	"claude-haiku-3.5": {InputPerMillion: 80, CachedPerMillion: 8, OutputPerMillion: 400},

	// OpenAI
	"gpt-4o":       {InputPerMillion: 250, CachedPerMillion: 125, OutputPerMillion: 1000},
	"gpt-4o-mini":  {InputPerMillion: 15, CachedPerMillion: 8, OutputPerMillion: 60},
	"gpt-4.1":      {InputPerMillion: 200, CachedPerMillion: 50, OutputPerMillion: 800},
	"gpt-4.1-mini": {InputPerMillion: 40, CachedPerMillion: 10, OutputPerMillion: 160},
	"o3":           {InputPerMillion: 1000, CachedPerMillion: 250, OutputPerMillion: 4000},
	"o3-mini":      {InputPerMillion: 110, CachedPerMillion: 28, OutputPerMillion: 440},
	"o4-mini":      {InputPerMillion: 110, CachedPerMillion: 28, OutputPerMillion: 440},

	// Google
	"gemini-2.5-pro": {InputPerMillion: 125, CachedPerMillion: 31, OutputPerMillion: 1000},
}

// GetModelPricing returns pricing for a model. found is false if not in table.
func GetModelPricing(model string) (pricing ModelPricing, found bool) {
	p, ok := pricingTable[model]
	return p, ok
}

// CalculateCostCents computes estimated cost from token counts and pricing.
// Returns 0 if pricing is unavailable.
func CalculateCostCents(
	tokensIn, tokensCachedIn, tokensOut int,
	pricing ModelPricing,
) int {
	cost := int64(tokensIn)*int64(pricing.InputPerMillion) +
		int64(tokensCachedIn)*int64(pricing.CachedPerMillion) +
		int64(tokensOut)*int64(pricing.OutputPerMillion)
	return int(cost / 1_000_000)
}
