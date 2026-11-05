package lifecycle

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

func (r *PluginRemoteExecutor) acquirePluginExecutorCleanupClaim(
	ctx context.Context,
	taskID, sessionID string,
	inventory pluginExecutorInventory,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	return r.acquirePluginExecutorClaim(ctx, taskID, sessionID, inventory, models.PluginClaimKindCleanup)
}

func (r *PluginRemoteExecutor) releasePluginExecutorCleanupClaim(claim *models.TaskEnvironmentRecoveryClaim) {
	if claim == nil || r.inventoryStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.inventoryStore.ReleaseTaskEnvironmentRecoveryClaim(ctx, claim); err != nil {
		r.logger.Warn("plugin executor cleanup claim release failed", zap.String("operation_id", claim.OperationID), zap.Error(err))
	}
}

// refreshAfterClaimCheckpoints reloads the record after a destroy call for a
// provider that checkpoints under the claim, so later writes carry the revision
// those checkpoints produced and a retained cleanup keeps the latest
// checkpointed resource. Other providers keep the state the call started from.
func (r *PluginRemoteExecutor) refreshAfterClaimCheckpoints(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
) (*models.ExecutorRunning, pluginExecutorInventory, *pluginsdk.ExecutorResourceDescriptor) {
	if !negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDestroyCheckpoints) {
		return record, inventory, resource
	}
	current, err := r.currentPluginExecutorRecord(ctx, record.SessionID, record.AgentExecutionID)
	if err != nil {
		return record, inventory, resource
	}
	latest, err := decodePluginExecutorInventory(current.Metadata)
	if err != nil || latest.EnvironmentID != inventory.EnvironmentID || latest.EnvironmentGeneration != inventory.EnvironmentGeneration {
		return record, inventory, resource
	}
	if latest.Resource != nil && latest.Resource.GetResourceHandle() == resource.GetResourceHandle() {
		resource = latest.Resource
	}
	return current, latest, resource
}

// latestPluginExecutorClaimCheckpointPersisted reports whether a provider that
// declared destroy_checkpoints has at least one accepted checkpoint recorded
// for this claim. Confirmed absence without one is never settled.
func latestPluginExecutorClaimCheckpointPersisted(inventory pluginExecutorInventory, claim *models.TaskEnvironmentRecoveryClaim) bool {
	if !negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDestroyCheckpoints) {
		return true
	}
	return inventory.Claim != nil && inventory.Claim.OperationID == claim.OperationID &&
		inventory.Claim.AcquiredAt == pluginExecutorClaimAcquiredAt(claim) && inventory.Claim.Sequence >= 1
}
