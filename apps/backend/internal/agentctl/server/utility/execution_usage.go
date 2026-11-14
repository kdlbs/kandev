package utility

import (
	"context"
	"github.com/kandev/kandev/internal/agent/usage"
)

// Exact same sanitized env slice is subsequently assigned to cmd.Env. Unknown
// providers do not borrow a host subscription; probes never call this path.
func (e *ACPInferenceExecutor) checkExecutionUsage(ctx context.Context, req *PromptRequest, env []string) *usage.ExecutionUsageReport {
	if req.UsagePolicy == nil {
		return nil
	}
	if req.InferenceConfig.ProviderGatewayAuth != nil {
		return unavailableSelectedUsage(req.UsagePolicy, "provider_selection_unknown", req.Model)
	}
	policy := *req.UsagePolicy
	input := nativeInferenceUsageInput(req, env)
	if !usage.NativeProviderSelectionKnown(input) {
		return unavailableSelectedUsage(req.UsagePolicy, "provider_selection_unknown", req.Model)
	}
	input, reason, _ := e.executionUsage.BindExecutionUsage(ctx, input)
	if usage.ExecutionUsageSelectionChanged(policy, input, reason, req.Model, true) {
		return &usage.ExecutionUsageReport{Reason: "binding_changed", BindingChanged: true, Model: req.Model}
	}
	if reason != "ok" {
		return &usage.ExecutionUsageReport{Reason: reason, Model: req.Model}
	}
	return usage.ReadExecutionUsageReport(ctx, e.executionUsage, input)
}

func unavailableSelectedUsage(policy *usage.ExecutionUsagePolicy, reason, model string) *usage.ExecutionUsageReport {
	changed := policy != nil && policy.ExpectedAccountRef != ""
	if changed {
		reason = "binding_changed"
	}
	return &usage.ExecutionUsageReport{Reason: reason, Model: model, BindingChanged: changed}
}
