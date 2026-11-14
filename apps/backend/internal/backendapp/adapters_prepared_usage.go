package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

func (a *lifecycleAdapter) CandidateExecutionUsageEnabled() bool {
	return a.mgr.CandidateExecutionUsageEnabled()
}
func (a *lifecycleAdapter) PreviewLaunchExecutionUsage(ctx context.Context, req *executor.LaunchAgentRequest) *usage.ExecutionUsageReport {
	if req == nil {
		return nil
	}
	return a.mgr.PreviewLaunchExecutionUsage(ctx, buildLifecycleLaunchRequest(req, req.WorkspacePath, req.OfficeAgentProfileID))
}
