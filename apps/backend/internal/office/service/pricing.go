package service

// The old hardcoded pricingTable was removed when the cost subscriber was
// rewritten to use the three-layer lookup (provider-reported cost →
// models.dev → estimated). See internal/office/costs/pricing.go for the
// CalculateCostSubcents + ProviderForModel helpers, and
// internal/office/costs/modelsdev for the live pricing lookup. The
// subscriber in event_subscribers.go calls those directly.
