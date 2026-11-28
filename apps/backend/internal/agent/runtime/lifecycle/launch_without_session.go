package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

func (m *Manager) prepareLaunchAdmission(ctx context.Context, req *LaunchRequest) (context.Context, error) {
	if req.McpProfile != nil && req.McpProfile.BuiltinPackages != nil {
		if err := m.validateBuiltinOwner(ctx, req.TaskID, req.SessionID, req.WorkspaceID, req.McpProfile); err != nil {
			return ctx, err
		}
	}
	if m.executorAdmission != nil {
		var err error
		ctx, err = m.executorAdmission(ctx, req.SessionID, getMetadataString(req.Metadata, "executor_id"), getMetadataString(req.Metadata, MetadataKeyExecutorProfileID))
		if err != nil {
			return ctx, err
		}
	}
	return ctx, m.validateManagedToolPolicyAdmission(ctx, req)
}

func (m *Manager) launchWithoutSession(ctx context.Context, req *LaunchRequest) (*AgentExecution, error) {
	activityLease, err := m.acquireActivity(ctx, activity.KindExecutionStarting)
	if err != nil {
		return nil, err
	}
	transferredActivity := false
	defer func() {
		if !transferredActivity {
			activityLease.Release()
		}
	}()
	activityLease.SetKind(activity.KindExecutionPreparing)
	execution, launchErr := m.launchInternal(ctx, req)
	if launchErr == nil && req.StartAgent {
		m.markAgentStartPending(execution)
		m.trackActivity(executionActivityKey(execution.ID), activityLease)
		transferredActivity = true
	}
	return execution, launchErr
}
