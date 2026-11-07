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
		return &usage.ExecutionUsageReport{Reason: "provider_selection_unknown", Model: req.Model}
	}
	policy := *req.UsagePolicy
	policy.AgentID = req.AgentID
	executionID := req.AttemptID
	if executionID == "" {
		executionID = req.CallID
	}
	input := usage.ExecutionInputFromEnvironment(policy, executionID, req.Model, env)
	input.WorkDir = req.InferenceConfig.WorkDir
	input.Command = append(append(append([]string(nil), req.InferenceConfig.CommandPrefix...), req.InferenceConfig.Command...), req.InferenceConfig.CLIFlags...)
	if !usage.NativeProviderSelectionKnown(input) {
		return &usage.ExecutionUsageReport{Reason: "provider_selection_unknown", Model: req.Model}
	}
	input, reason, _ := e.executionUsage.BindExecutionUsage(ctx, input)
	if reason != "ok" {
		return &usage.ExecutionUsageReport{Reason: reason, Model: req.Model}
	}
	return usage.ReadExecutionUsageReport(ctx, e.executionUsage, input)
}
