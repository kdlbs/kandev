package usage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ExecutionUsageReader is installed only by the enabled runtime composition.
// Inputs are owned by the execution, not a decoded profile or API assertion.
type ExecutionUsageReader interface {
	BindExecutionUsage(context.Context, ExecutionUsageInput) (ExecutionUsageInput, string, error)
	CheckExecutionUsage(context.Context, ExecutionUsageInput) (*ExecutionUsage, error)
}

// ExecutionUsageLimitError reports an observed provider limit. A nil ResetAt
// means the provider supplied no trustworthy reset; no deadline is invented.
type ExecutionUsageLimitError struct {
	Provider, Window string
	ResetAt          *time.Time
}

func (e *ExecutionUsageLimitError) Error() string {
	return fmt.Sprintf("provider usage limit: %s %s", e.Provider, e.Window)
}

type ExecutionUsageBindingError struct{}

func (*ExecutionUsageBindingError) Error() string { return "execution subscription account changed" }

// BindExecutionUsage pins existing provider identifiers before a new local
// process starts. An unavailable binding stays unknown for that process.
func (s *UsageService) BindExecutionUsage(ctx context.Context, input ExecutionUsageInput) (ExecutionUsageInput, string, error) {
	source, reason := resolveExecutionUsageSource(input)
	if reason != "" {
		return input, reason, nil
	}
	account, err := s.executionClient(input.ProfileID, source).accountIdentity(ctx)
	if err != nil {
		return input, "account_unavailable", err
	}
	if account.account == "" {
		return input, "account_unknown", nil
	}
	input.ExpectedAccountRef = account.account
	input.ExpectedOrganizationRef = account.organization
	return input, "ok", nil
}

func (s *UsageService) CheckExecutionUsage(ctx context.Context, input ExecutionUsageInput) (*ExecutionUsage, error) {
	if input.ExpectedAccountRef == "" {
		return &ExecutionUsage{Reason: "account_unknown"}, nil
	}
	result, err := s.GetExecutionUsage(ctx, input)
	if result != nil && result.Reason == "binding_changed" {
		return result, &ExecutionUsageBindingError{}
	}
	if result != nil && result.SourceKnown {
		if limit := executionUsageLimit(result.Current, input.Model, time.Now()); limit != nil {
			return result, limit
		}
		return result, err
	}
	// Failed refreshes cannot erase an already measured hard limit for this
	// exact pinned account. The preserved sample is never used for ranking.
	source, reason := resolveExecutionUsageSource(input)
	if reason != "" {
		return result, &ExecutionUsageBindingError{}
	}
	address, _ := json.Marshal([]string{source.provider, input.ExpectedOrganizationRef, input.ExpectedAccountRef})
	s.cache.mu.RLock()
	entry := s.cache.entries["execution_usage:"+string(address)]
	var last *ProviderUsage
	if entry != nil {
		last = cloneProviderUsage(entry.usage)
	}
	s.cache.mu.RUnlock()
	if limit := executionUsageLimit(last, input.Model, time.Now()); limit != nil {
		return result, limit
	}
	return result, err
}

func executionUsageLimit(sample *ProviderUsage, model string, now time.Time) *ExecutionUsageLimitError {
	if sample == nil || sample.AccountRef == "" {
		return nil
	}
	var result *ExecutionUsageLimitError
	unknownReset := false
	for _, window := range sample.Windows {
		if !observedHardUsageWindow(window, model) {
			continue
		}
		blocked, reset := blockingUsageReset(sample.FetchedAt, window, now)
		if !blocked {
			continue
		}
		if result == nil {
			result = &ExecutionUsageLimitError{Provider: sample.Provider, Window: window.Label}
		}
		if reset == nil {
			unknownReset = true
			continue
		}
		if result.ResetAt == nil || reset.After(*result.ResetAt) {
			result.ResetAt = reset
		}
	}
	if result != nil && unknownReset {
		result.ResetAt = nil
	}
	return result
}

func observedHardUsageWindow(window UtilizationWindow, model string) bool {
	return window.Observation != nil && window.Observation.UtilizationObserved && window.UtilizationPct >= 100 && nativeUtilizationObserved(&window.UtilizationPct) && usageWindowMayApply(window, model)
}

func blockingUsageReset(fetchedAt time.Time, window UtilizationWindow, now time.Time) (bool, *time.Time) {
	if !window.Observation.ResetAtObserved || window.ResetAt.IsZero() {
		return true, nil
	}
	if window.ResetAt.After(now) {
		reset := window.ResetAt
		return true, &reset
	}
	// A new100% observation after a reported reset is still a hard limit,
	// but its past deadline cannot authorize another retry.
	return !fetchedAt.Before(window.ResetAt), nil
}

func usageWindowMayApply(window UtilizationWindow, model string) bool {
	scope := strings.ToLower(strings.TrimSpace(window.Model))
	if scope != "opus" && scope != "sonnet" && scope != "haiku" {
		return true
	}
	family := usageModelFamily(model)
	return family == "" || family == scope
}

func usageModelFamily(model string) string {
	for _, family := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(strings.ToLower(model), family) {
			return family
		}
	}
	return ""
}
