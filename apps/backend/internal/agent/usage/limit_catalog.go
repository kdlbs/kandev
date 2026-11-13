package usage

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// limit_catalog.json holds installation default values and their provenance.
// Routing code reads it through catalogDefaults; owner and learned values
// still take precedence.
//
//go:embed limit_catalog.json
var limitCatalogJSON []byte

type limitCatalogEntry struct {
	Providers            []string          `json:"providers"`
	Tier                 BillingTier       `json:"tier"`
	PurchasedCredits     *bool             `json:"purchased_credits,omitempty"`
	Plans                []string          `json:"plans,omitempty"`
	Provenance           map[string]string `json:"provenance,omitempty"`
	Scopes               []RequestScope    `json:"scopes,omitempty"`
	RequestsPerMinute    *float64          `json:"requests_per_minute,omitempty"`
	RequestsPerDay       *float64          `json:"requests_per_day,omitempty"`
	RequestsPerWindow    *float64          `json:"requests_per_window,omitempty"`
	RequestWindowSeconds int64             `json:"request_window_seconds,omitempty"`
	MaxConcurrentTurns   *float64          `json:"max_concurrent_turns,omitempty"`
	RequestsPerModel     bool              `json:"requests_per_model,omitempty"`
	DailyReset           string            `json:"daily_reset,omitempty"`
	Windows              []LimitBudget     `json:"windows,omitempty"`
}

type limitCatalog struct {
	Entries []limitCatalogEntry `json:"entries"`
}

var loadLimitCatalog = sync.OnceValues(func() (limitCatalog, error) {
	return decodeLimitCatalog(limitCatalogJSON)
})

func decodeLimitCatalog(data []byte) (limitCatalog, error) {
	var catalog limitCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return limitCatalog{}, err
	}
	for _, entry := range catalog.Entries {
		for _, scope := range entry.Scopes {
			if err := validateRequestScope(scope); err != nil {
				return limitCatalog{}, err
			}
		}
		for _, budget := range entry.Windows {
			if err := validateLimitBudget(budget); err != nil {
				return limitCatalog{}, err
			}
		}
	}
	return catalog, nil
}

func (e limitCatalogEntry) matches(provider string, tier BillingTier, purchased bool, plan string) bool {
	if e.Tier != tier || (e.PurchasedCredits != nil && *e.PurchasedCredits != purchased) {
		return false
	}
	if len(e.Plans) > 0 {
		matched := false
		for _, p := range e.Plans {
			if strings.EqualFold(p, plan) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, p := range e.Providers {
		if strings.EqualFold(p, provider) {
			return true
		}
	}
	return false
}

// CatalogFreeModel reports whether the embedded catalog lists the model in a
// free-tier scope. The second result is false when the catalog has no opinion.
func CatalogFreeModel(modelID string) (bool, bool) {
	provider := providerPrefix(modelID)
	model := ScopeModelID(modelID)
	if model == "" {
		return false, false
	}
	catalog, err := loadLimitCatalog()
	if err != nil {
		return false, false
	}
	for _, entry := range catalog.Entries {
		if entry.Tier != TierFree {
			continue
		}
		if provider != "" && !entry.matchesProvider(provider) {
			continue
		}
		for _, scope := range entry.Scopes {
			for _, listed := range scope.Models {
				if strings.EqualFold(listed, model) {
					return true, true
				}
			}
		}
	}
	return false, false
}

func (e limitCatalogEntry) matchesProvider(provider string) bool {
	for _, listed := range e.Providers {
		if strings.EqualFold(listed, provider) {
			return true
		}
	}
	return false
}

func providerPrefix(modelID string) string {
	model := strings.TrimSpace(modelID)
	if index := strings.Index(model, "/"); index > 0 {
		return strings.ToLower(model[:index])
	}
	return ""
}

// catalogTierDefaults returns the catalog-owned installation defaults for one
// provider, tier, and plan. The second result is false when no entry matches,
// so callers can keep an explicit fallback for an unknown provider.
func catalogTierDefaults(provider string, tier BillingTier, plan string, purchased bool) (TierLimits, bool) {
	catalog, err := loadLimitCatalog()
	if err != nil {
		return TierLimits{}, false
	}
	limits := TierLimits{}
	matched := false
	for _, entry := range catalog.Entries {
		if !entry.matches(provider, tier, purchased, plan) {
			continue
		}
		matched = true
		if entry.RequestsPerMinute != nil {
			limits.RequestsPerMinute = entry.RequestsPerMinute
		}
		if entry.RequestsPerDay != nil {
			limits.RequestsPerDay = entry.RequestsPerDay
		}
		if entry.RequestsPerWindow != nil {
			limits.RequestsPerWindow = entry.RequestsPerWindow
		}
		if entry.RequestWindowSeconds > 0 {
			limits.RequestWindowSeconds = entry.RequestWindowSeconds
		}
		if entry.MaxConcurrentTurns != nil {
			limits.MaxConcurrentTurns = entry.MaxConcurrentTurns
		}
		if entry.RequestsPerModel {
			limits.RequestsPerModel = true
		}
		if entry.DailyReset != "" {
			limits.DailyReset = entry.DailyReset
		}
		if len(entry.Windows) > 0 {
			limits.Windows = append(limits.Windows, cloneLimitBudgets(entry.Windows)...)
		}
		for field, provenance := range entry.Provenance {
			if limits.Provenance == nil {
				limits.Provenance = make(map[string]string)
			}
			limits.Provenance[field] = provenance
		}
		for _, scope := range entry.Scopes {
			scope.Models = append([]string(nil), scope.Models...)
			limits.Scopes = append(limits.Scopes, scope)
		}
	}
	return limits, matched
}

func cloneLimitBudgets(budgets []LimitBudget) []LimitBudget {
	result := make([]LimitBudget, len(budgets))
	copy(result, budgets)
	return result
}
