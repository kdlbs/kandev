// Package usage provides subscription utilization tracking for agent providers.
// It fetches utilization data from provider APIs (Anthropic, OpenAI) for agents
// authenticated via OAuth/subscription credentials rather than API keys.
package usage

import (
	"context"
	"time"
)

// BillingType identifies how an agent is billed.
type BillingType string

const (
	// BillingTypeAPIKey means the agent uses an API key with per-token billing.
	BillingTypeAPIKey BillingType = "api_key"
	// BillingTypeSubscription means the agent uses OAuth/subscription credentials.
	BillingTypeSubscription BillingType = "subscription"
)

// UtilizationWindow represents one rate-limit window's utilization.
type UtilizationWindow struct {
	Label          string    `json:"label"`           // e.g. "5-hour", "7-day"
	UtilizationPct float64   `json:"utilization_pct"` // 0–100
	ResetAt        time.Time `json:"reset_at"`
	// Scope identifies the model class covered by this account window.
	Scope           string    `json:"scope,omitempty"`
	LimitReached    bool      `json:"limit_reached,omitempty"`
	StartAt         time.Time `json:"start_at,omitempty"`
	DurationSeconds int64     `json:"duration_seconds,omitempty"`
	// Observation is additive provenance for strict readers; legacy values and wire data are unchanged.
	Observation *WindowObservation `json:"-"`
}

// WindowObservation distinguishes provider measurements from legacy defaults.
type WindowObservation struct {
	UtilizationObserved bool
	ResetAtObserved     bool
}

func legacyUtilization(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func utilizationObserved(value *float64) bool {
	return value != nil && *value >= 0
}

// ProviderUsage is the full utilization response for one provider credential.
type ProviderUsage struct {
	Provider  string              `json:"provider"`       // "anthropic", "openai"
	Plan      string              `json:"plan,omitempty"` // e.g. "max", "pro", "plus", "free"
	Windows   []UtilizationWindow `json:"windows"`
	FetchedAt time.Time           `json:"fetched_at"`
	// Stale marks a preserved success served while the provider refused a
	// refresh: the reading is real, but FetchedAt may be well behind now.
	Stale bool `json:"stale,omitempty"`
}

// ProviderUsageClient fetches live utilization from a provider API.
type ProviderUsageClient interface {
	FetchUsage(ctx context.Context) (*ProviderUsage, error)
}
