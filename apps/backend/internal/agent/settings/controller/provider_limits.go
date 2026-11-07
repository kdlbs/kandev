package controller

import (
	"context"
	"errors"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"time"
)

// ErrProviderLimitsUnavailable means the provider limit service is not wired.
var ErrProviderLimitsUnavailable = errors.New("provider limits are not configured")

// ErrInvalidProviderLimit marks a rejected provider limit update.
var ErrInvalidProviderLimit = errors.New("invalid provider limit")

// ProviderLimitDTO is one provider's limit settings and the reset instants the
// router currently uses for it.
type ProviderLimitDTO struct {
	Accounts     []ProviderAccountLimitDTO `json:"accounts,omitempty"`
	Provider     string                    `json:"provider"`
	ProfileCount int                       `json:"profile_count"`
	// ModelScoped is true when usage limits of this provider pause one model
	// at a time.
	ModelScoped bool `json:"model_scoped"`
	// MonthlyResetAt and MonthlyResetTimezone are the operator-entered monthly
	// reset: one occurrence, repeated every month at the same local time.
	MonthlyResetAt       *time.Time `json:"monthly_reset_at,omitempty"`
	MonthlyResetTimezone string     `json:"monthly_reset_timezone,omitempty"`
	// NextMonthlyReset is the reset the router uses, from observed usage when
	// the provider reports one and otherwise from the operator setting.
	NextMonthlyReset       *time.Time `json:"next_monthly_reset,omitempty"`
	NextMonthlyResetSource string     `json:"next_monthly_reset_source,omitempty"`
	// BlockUntil pauses every model of the provider until that instant.
	BlockUntil *time.Time `json:"block_until,omitempty"`
	// ObservedExhaustedUntil is the latest reset among fully consumed usage
	// windows the provider reports.
	ObservedExhaustedUntil *time.Time `json:"observed_exhausted_until,omitempty"`
	UpdatedAt              *time.Time `json:"updated_at,omitempty"`
}

// ProviderLimitsResponse lists every provider known from agent profiles or
// saved settings.
type ProviderLimitsResponse struct {
	Providers []ProviderLimitDTO `json:"providers"`
}

// UpdateProviderLimitRequest replaces one provider's operator settings. A nil
// instant clears that setting.
type UpdateProviderLimitRequest struct {
	AccountID            string                        `json:"account_id,omitempty"`
	AccountLimits        *agentusage.AccountTierConfig `json:"account_limits,omitempty"`
	MonthlyResetAt       *time.Time                    `json:"monthly_reset_at"`
	MonthlyResetTimezone string                        `json:"monthly_reset_timezone"`
	BlockUntil           *time.Time                    `json:"block_until"`
}

type ProviderAccountLimitDTO struct {
	AccountID   string                                          `json:"account_id"`
	AccountKind string                                          `json:"account_kind"`
	Profiles    []ProviderTierProfileDTO                        `json:"profiles"`
	Tiers       map[agentusage.BillingTier]ProviderTierLimitDTO `json:"tiers"`
	Overrides   agentusage.AccountTierConfig                    `json:"overrides"`
}

type ProviderTierProfileDTO struct {
	ID   string                 `json:"id"`
	Name string                 `json:"name"`
	Tier agentusage.BillingTier `json:"tier"`
	// Model is the provider model ID without the provider prefix, so the card
	// can offer the account's models for a new request scope.
	Model string `json:"model,omitempty"`
}

type ProviderTierLimitDTO struct {
	Defaults  agentusage.TierLimits `json:"defaults"`
	Learned   agentusage.TierLimits `json:"learned"`
	Override  agentusage.TierLimits `json:"override"`
	Effective agentusage.TierLimits `json:"effective"`
	Usage     agentusage.LimitUsage `json:"usage"`
}

// ProviderLimitService owns provider limit settings. The runtime composition
// implements it because the effective reset reads observed provider usage.
type ProviderLimitService interface {
	ListProviderLimits(ctx context.Context) ([]ProviderLimitDTO, error)
	UpdateProviderLimit(ctx context.Context, provider string, request UpdateProviderLimitRequest) (*ProviderLimitDTO, error)
}

// SetProviderLimitService injects the provider limit service.
func (c *Controller) SetProviderLimitService(service ProviderLimitService) {
	c.providerLimits = service
}

// ListProviderLimits returns every provider's limit settings.
func (c *Controller) ListProviderLimits(ctx context.Context) (*ProviderLimitsResponse, error) {
	if c.providerLimits == nil {
		return nil, ErrProviderLimitsUnavailable
	}
	providers, err := c.providerLimits.ListProviderLimits(ctx)
	if err != nil {
		return nil, err
	}
	if providers == nil {
		providers = []ProviderLimitDTO{}
	}
	return &ProviderLimitsResponse{Providers: providers}, nil
}

// UpdateProviderLimit replaces one provider's operator settings.
func (c *Controller) UpdateProviderLimit(
	ctx context.Context,
	provider string,
	request UpdateProviderLimitRequest,
) (*ProviderLimitDTO, error) {
	if c.providerLimits == nil {
		return nil, ErrProviderLimitsUnavailable
	}
	return c.providerLimits.UpdateProviderLimit(ctx, provider, request)
}
