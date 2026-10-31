package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/retention"
)

type workspaceRetentionReader interface {
	HasRetainedTaskWorkspace(context.Context, string) (bool, error)
}

// Provider stops can destroy the only copy of a workspace. Standalone stops
// terminate local processes without removing the checkout.
func (m *Manager) stopWorkspaceRuntime(ctx context.Context, backend ExecutorBackend, instance *ExecutorInstance, force bool) error {
	// An admitted stop must finish disposal even after the requesting client leaves.
	if ctx.Err() != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
	}
	ctx, releaseRetention, retentionErr := retention.AcquireMutation(ctx)
	if retentionErr != nil {
		return retentionErr
	}
	defer releaseRetention()

	if backend.Name() != executor.NameStandalone {
		sandboxID, _ := instance.Metadata[MetadataKeySpriteName].(string)
		if err := m.checkRetainedWorkspaceResource(ctx, instance.TaskID, instance.InstanceID, instance.ContainerID, sandboxID); err != nil {
			return err
		}
	}

	return backend.StopInstance(ctx, instance, force)
}

func (m *Manager) stopWorkspaceRuntimeAndRelease(ctx context.Context, backend ExecutorBackend, instance *ExecutorInstance, force bool) error {
	if backend == nil || instance == nil {
		return nil
	}
	if instance.StopReason == "" {
		instance.StopReason = StopReasonLaunchRollback
	}
	if err := m.stopWorkspaceRuntime(ctx, backend, instance, force); err != nil {
		return err
	}
	return releaseExecutorInstanceRuntimeInventory(ctx, instance)
}

// rollbackPreservesRuntime reports whether a failed launch attempt attached to
// existing compute and therefore must not destroy it. Before an AgentExecution
// exists, ExecutorInstance.ReusedWorkspace is the only provenance; afterwards
// the execution's immutable attachment record is authoritative, with the
// instance field retaining its value as a defensive fallback.
func rollbackPreservesRuntime(execInstance *ExecutorInstance, execution *AgentExecution) bool {
	if execution != nil && retainsRuntimeOnLaunchRollback(execution) {
		return true
	}
	if execInstance == nil || !execInstance.ReusedWorkspace {
		return false
	}
	switch execInstance.RuntimeName {
	case agentruntime.RuntimeRemoteDocker, agentruntime.RuntimeKubernetes:
		return true
	default:
		return false
	}
}

// stopLaunchRuntimeOnRollback routes every post-attachment launch failure
// through one retention policy. A retained runtime releases only attempt-local
// transport; fresh allocations and explicit cleanup keep destructive authority.
func (m *Manager) stopLaunchRuntimeOnRollback(
	ctx context.Context,
	backend ExecutorBackend,
	instance *ExecutorInstance,
	execution *AgentExecution,
) error {
	if backend == nil || instance == nil {
		return nil
	}
	if !rollbackPreservesRuntime(instance, execution) {
		return m.stopWorkspaceRuntimeAndRelease(ctx, backend, instance, true)
	}
	if instance.RuntimeName == agentruntime.RuntimeRemoteDocker {
		// Preserve the retained container even if an earlier stop attempt was
		// unconfirmed. This reason is also what a later retry expects.
		instance.StopReason = StopReasonAgentBootstrapFailed
	}
	return m.stopWorkspaceRuntime(ctx, backend, instance, false)
}

func (m *Manager) checkRetainedWorkspaceResource(ctx context.Context, taskID string, resourceIDs ...string) error {
	var retained bool
	var err error
	if reader, ok := m.runningWriter.(interface {
		HasRetainedWorkspaceResource(context.Context, string, ...string) (bool, error)
	}); ok {
		retained, err = reader.HasRetainedWorkspaceResource(ctx, taskID, resourceIDs...)
	} else if reader, ok := m.runningWriter.(workspaceRetentionReader); ok {
		retained, err = reader.HasRetainedTaskWorkspace(ctx, taskID)
	}
	if err != nil {
		return fmt.Errorf("check workspace retention: %w", err)
	}
	if retained {
		return fmt.Errorf("workspace retained; destructive runtime stop is disabled")
	}
	return nil
}
