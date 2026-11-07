package hostutility

import (
	"context"
	"github.com/kandev/kandev/internal/agent/usage"
	util "github.com/kandev/kandev/internal/agentctl/server/utility"
)

// PreviewPreparedExecutionUsage uses only an existing instance in the current
// shared local runtime owner epoch. Its control process owns the inherited
// native environment; no backend environment or representative account is used.
func (m *Manager) PreviewPreparedExecutionUsage(ctx context.Context, input *util.UsagePreviewRequest) *usage.ExecutionUsageReport {
	if !m.providerUsageLimits || input == nil || m.runtimeOwner == nil {
		return nil
	}
	inst := m.existingUsageInstance(input.AgentID)
	if inst == nil {
		return nil
	}
	release, err := inst.acquireOperation(ctx, false)
	if err != nil {
		return nil
	}
	defer release()
	copy := *input
	copy.ExecutionID = inst.instanceID
	report, err := inst.client.PreviewInferenceUsage(ctx, &copy)
	if err != nil {
		return nil
	}
	return report
}
