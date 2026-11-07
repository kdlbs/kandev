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
	// Model is the explicit provider scope, empty for shared account windows.
	Model          string    `json:"-"`
	Label          string    `json:"label"`           // e.g. "5-hour", "7-day"
	UtilizationPct float64   `json:"utilization_pct"` // 0-100
	ResetAt        time.Time `json:"reset_at"`
	// DurationSeconds and StartAt are the numeric shape of the window.
	DurationSeconds int64     `json:"duration_seconds,omitempty"`
	StartAt         time.Time `json:"start_at,omitempty"`
	// ModelID is the concrete model the window applies to, when scoped.
	ModelID string `json:"model_id,omitempty"`
	// Scope names which class of the account's models the window limits.
	Scope WindowScope `json:"scope,omitempty"`
	// LimitReached records that the provider reports the window exhausted.
	LimitReached bool `json:"limit_reached,omitempty"`
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

// WindowScope names the class of models a provider window limits.
type WindowScope string

const (
	// WindowScopeAllModels is an account-wide window.
	WindowScopeAllModels WindowScope = ""
	// WindowScopePaidModels limits only models billed against the plan.
	WindowScopePaidModels WindowScope = "paid_models"
	// WindowScopeFreeModels limits only the provider's free models.
	WindowScopeFreeModels WindowScope = "free_models"
	// WindowScopePremiumModels limits only premium models.
	WindowScopePremiumModels WindowScope = "premium_models"
)

// Exhausted reports whether the window is known to have no remaining capacity.
func (w UtilizationWindow) Exhausted() bool {
	return w.LimitReached || w.UtilizationPct >= 100
}

// ModelClass is the provider's cost class for a model.
type ModelClass string

const (
	ModelClassUnknown ModelClass = "unknown"
	ModelClassFree    ModelClass = "free"
	ModelClassPremium ModelClass = "premium"
)

// ProviderUsage is the full utilization response for one provider credential.
type ProviderUsage struct {
	AccountRef             string              `json:"-"`
	OrganizationRef        string              `json:"-"`
	FreeModelDailyRequests *FreeModelCounter   `json:"free_model_daily_requests,omitempty"`
	Provider               string              `json:"provider"`       // "anthropic", "openai"
	Plan                   string              `json:"plan,omitempty"` // e.g. "max", "pro", "plus", "free"
	Windows                []UtilizationWindow `json:"windows"`
	FetchedAt              time.Time           `json:"fetched_at"`
	// Stale marks a preserved success served while the provider refused a
	// refresh: the reading is real, but FetchedAt may be well behind now.
	Stale bool `json:"stale,omitempty"`
}

// FreeModelCounter is a provider's free-model request counter.
type FreeModelCounter struct {
	Used      float64 `json:"used"`
	Limit     float64 `json:"limit"`
	Remaining float64 `json:"remaining"`
}

// ProviderUsageClient fetches live utilization from a provider API.
type ProviderUsageClient interface {
	FetchUsage(ctx context.Context) (*ProviderUsage, error)
}
