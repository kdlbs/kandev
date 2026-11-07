package usage

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ExecutionUsagePolicy is backend-resolved metadata, installed only when the
// native feature is enabled. Credential values never travel in this policy.
type ExecutionUsagePolicy struct {
	ExpectedProvider        string `json:"expected_provider,omitempty"`
	ExpectedAccountRef      string `json:"expected_account_ref,omitempty"`
	ExpectedOrganizationRef string `json:"expected_organization_ref,omitempty"`
	ExpectedModel           string `json:"expected_model,omitempty"`
	ProfileID               string `json:"profile_id"`
	AgentID                 string `json:"agent_id"`
	ExecutorType            string `json:"executor_type"`
}

type ObservedUsageWindow struct {
	Label               string    `json:"label"`
	Model               string    `json:"model,omitempty"`
	UtilizationPct      float64   `json:"utilization_pct"`
	ResetAt             time.Time `json:"reset_at"`
	UtilizationObserved bool      `json:"utilization_observed"`
	ResetAtObserved     bool      `json:"reset_at_observed"`
}
type ObservedUsageSample struct {
	Provider        string                `json:"provider"`
	AccountRef      string                `json:"account_ref"`
	OrganizationRef string                `json:"organization_ref,omitempty"`
	Plan            string                `json:"plan,omitempty"`
	FetchedAt       time.Time             `json:"fetched_at"`
	Windows         []ObservedUsageWindow `json:"windows"`
}

// ExecutionUsageReport contains provider facts only. No environment, token,
// credential filename, profile-derived account ID, or invented reset is sent.
type ExecutionUsageReport struct {
	SourceKnown    bool                 `json:"source_known"`
	Reason         string               `json:"reason"`
	Model          string               `json:"model,omitempty"`
	Current        *ObservedUsageSample `json:"current,omitempty"`
	Previous       *ObservedUsageSample `json:"previous,omitempty"`
	Limited        bool                 `json:"limited"`
	BlockingWindow string               `json:"blocking_window,omitempty"`
	ResetAt        *time.Time           `json:"reset_at,omitempty"`
	BindingChanged bool                 `json:"binding_changed,omitempty"`
}

// ExecutionInputFromEnvironment must receive the native child's final env,
// after adapter contributions and stripping. It never inherits backend env.
func ExecutionInputFromEnvironment(policy ExecutionUsagePolicy, executionID, model string, environment []string) ExecutionUsageInput {
	input := ExecutionUsageInput{ExecutionID: executionID, ProfileID: policy.ProfileID, AgentID: policy.AgentID, ExecutorType: policy.ExecutorType, Model: model}
	if !localUsageExecution(policy.ExecutorType) || environment == nil {
		return input
	}
	input.Environment = make(map[string]string)
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok && usageEnvironmentKey(key) {
			input.Environment[usageEnvironmentName(key)] = value
		}
	}
	return input
}

func ReadExecutionUsageReport(ctx context.Context, service ExecutionUsageReader, input ExecutionUsageInput) *ExecutionUsageReport {
	report := &ExecutionUsageReport{Reason: "account_unknown", Model: input.Model}
	result, err := service.CheckExecutionUsage(ctx, input)
	if result != nil {
		report.SourceKnown, report.Reason = result.SourceKnown, result.Reason
		report.Current, report.Previous = observedUsageSample(result.Current), observedUsageSample(result.Previous)
	}
	var limit *ExecutionUsageLimitError
	if errors.As(err, &limit) {
		report.Limited = true
		report.BlockingWindow = limit.Window
		report.ResetAt = limit.ResetAt
	}
	var changed *ExecutionUsageBindingError
	if errors.As(err, &changed) {
		report.BindingChanged = true
		report.Reason = executionBindingChanged
	}
	return report
}

func observedUsageSample(sample *ProviderUsage) *ObservedUsageSample {
	if sample == nil {
		return nil
	}
	result := &ObservedUsageSample{Provider: sample.Provider, AccountRef: sample.AccountRef, OrganizationRef: sample.OrganizationRef, Plan: sample.Plan, FetchedAt: sample.FetchedAt, Windows: make([]ObservedUsageWindow, 0, len(sample.Windows))}
	for _, window := range sample.Windows {
		item := ObservedUsageWindow{Label: window.Label, Model: window.Model, UtilizationPct: window.UtilizationPct}
		if window.Observation != nil {
			item.UtilizationObserved = window.Observation.UtilizationObserved
			item.ResetAtObserved = window.Observation.ResetAtObserved
		}
		if item.ResetAtObserved {
			item.ResetAt = window.ResetAt
		}
		result.Windows = append(result.Windows, item)
	}
	return result
}

func (sample *ObservedUsageSample) ProviderSample() *ProviderUsage {
	if sample == nil {
		return nil
	}
	result := &ProviderUsage{Provider: sample.Provider, AccountRef: sample.AccountRef, OrganizationRef: sample.OrganizationRef, Plan: sample.Plan, FetchedAt: sample.FetchedAt, Windows: make([]UtilizationWindow, 0, len(sample.Windows))}
	for _, window := range sample.Windows {
		result.Windows = append(result.Windows, UtilizationWindow{Label: window.Label, Model: window.Model, UtilizationPct: window.UtilizationPct, ResetAt: window.ResetAt, Observation: &WindowObservation{UtilizationObserved: window.UtilizationObserved, ResetAtObserved: window.ResetAtObserved}})
	}
	return result
}
