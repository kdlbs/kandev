package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

var errPluginExecutorHandoffUnavailable = errors.New("plugin executor launch failed; allocated resource was not recorded and was not removed")

// handOffUnrecordedPluginExecutorResource durably records a resource that has
// neither an inventory row nor a cleanup claim to hold it, as when an archive
// holds the environment while the provider returns. A provider mutates a
// resource only under a persisted claim, so the launch never destroys it: the
// environment cleanup adopts the record, destroys the resource under its own
// claim, and deletes the record only after the destroy is confirmed.
func (r *PluginRemoteExecutor) handOffUnrecordedPluginExecutorResource(
	ctx context.Context,
	req *ExecutorCreateRequest,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	stage string,
	cause error,
) error {
	// The table is the last writer of the resource, so a state that carries a
	// revealed secret is cut down to what cleanup needs whatever the caller held.
	if req.PluginExecutor != nil && rejectSecretInProviderState(req.PluginExecutor.Profile.SecretValues, resource) != nil {
		resource = fixedSizePluginExecutorResource(resource)
	}
	inventory.Resource = resource
	inventory.Phase = pluginExecutorPhaseCleanupPending
	var recordErr error
	if req.RecordUnrecordedRuntimeInventory == nil {
		recordErr = errors.New("durable handoff is unavailable")
	} else {
		recordErr = req.RecordUnrecordedRuntimeInventory(ctx, map[string]interface{}{MetadataKeyPluginExecutor: inventory})
	}
	fields := []zap.Field{
		zap.String("execution_id", req.InstanceID), zap.String("environment_id", inventory.EnvironmentID),
		zap.String("stage", stage), zap.String("resource_handle", resource.GetResourceHandle()), zap.NamedError("launch_cause", cause),
	}
	if recordErr != nil {
		r.logger.Error("plugin executor resource is unrecorded and its handoff failed",
			append(fields, zap.String("cleanup_outcome", "handoff_failed"), zap.Error(recordErr))...)
		return withPluginExecutorLaunchCode(cause, errPluginExecutorHandoffUnavailable)
	}
	r.logger.Error("plugin executor resource is unrecorded; its removal is left to the environment cleanup",
		append(fields, zap.String("cleanup_outcome", "handed_off"))...)
	return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; allocated resource was not recorded and will be removed by the environment cleanup"))
}

// adoptUnrecordedPluginExecutors returns the durable handoffs of an environment
// by execution, inserting the inventory row each one needs so the cleanup claim
// can accept the provider's checkpoints. A handoff that cannot be adopted stays
// recorded and fails the cleanup.
func (r *PluginRemoteExecutor) adoptUnrecordedPluginExecutors(ctx context.Context, environment *models.TaskEnvironment) (map[string]*models.UnrecordedPluginExecutor, error) {
	listed, err := r.inventoryStore.ListUnrecordedPluginExecutors(ctx, environment.TaskID, environment.ID)
	if err != nil {
		return nil, fmt.Errorf("read unrecorded plugin executors: %w", err)
	}
	adopted := make(map[string]*models.UnrecordedPluginExecutor, len(listed))
	var adoptErrors []error
	for _, item := range listed {
		adopted[item.ExecutionID] = item
		if err := r.adoptUnrecordedPluginExecutor(ctx, environment, item); err != nil {
			adoptErrors = append(adoptErrors, fmt.Errorf("adopt unrecorded plugin executor %s: %w", item.ExecutionID, err))
		}
	}
	return adopted, errors.Join(adoptErrors...)
}

func (r *PluginRemoteExecutor) adoptUnrecordedPluginExecutor(ctx context.Context, environment *models.TaskEnvironment, item *models.UnrecordedPluginExecutor) error {
	if _, err := r.inventoryStore.GetExecutorRunningBySessionID(ctx, item.SessionID); err == nil {
		return nil
	} else if !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return err
	}
	inventory, err := decodePluginExecutorInventory(item.Running.Metadata)
	if err != nil {
		return err
	}
	inventory.Phase, inventory.Revision = pluginExecutorPhaseAllocating, 0
	inventory.Resource = r.withoutRevealedSecret(ctx, item, inventory)
	inventory.EnvironmentGeneration = environment.OwnershipGeneration
	running := *item.Running
	running.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: inventory}
	running.ExpectedPluginExecutorRevision = 0
	return r.inventoryStore.CheckpointPluginExecutorInventory(ctx, &running)
}

// withoutRevealedSecret returns the handoff's resource, cut down to what cleanup
// needs when its state holds a secret of the recorded profile. A handoff written
// by this build never does; the check keeps an older row from being copied into
// the inventory. An unavailable profile leaves the resource as recorded.
func (r *PluginRemoteExecutor) withoutRevealedSecret(ctx context.Context, item *models.UnrecordedPluginExecutor, inventory pluginExecutorInventory) *pluginsdk.ExecutorResourceDescriptor {
	if inventory.Resource == nil || r.profileLoader == nil {
		return inventory.Resource
	}
	profile, err := r.profileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, item.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil || profile == nil || rejectSecretInProviderState(profile.SecretValues, inventory.Resource) == nil {
		return inventory.Resource
	}
	return fixedSizePluginExecutorResource(inventory.Resource)
}

// settleUnrecordedPluginExecutor forgets a handoff after its resource's destroy
// was confirmed.
func (r *PluginRemoteExecutor) settleUnrecordedPluginExecutor(ctx context.Context, item *models.UnrecordedPluginExecutor) error {
	if item == nil {
		return nil
	}
	return r.inventoryStore.DeleteUnrecordedPluginExecutor(ctx, item.ExecutionID)
}

// remainingUnrecordedPluginExecutors names the handoffs still recorded for the
// environment after the cleanup ran: a resource nothing has confirmed destroyed
// must keep the environment from being reported cleaned.
func (r *PluginRemoteExecutor) remainingUnrecordedPluginExecutors(ctx context.Context, environment *models.TaskEnvironment) []error {
	remaining, err := r.inventoryStore.ListUnrecordedPluginExecutors(ctx, environment.TaskID, environment.ID)
	if err != nil {
		return []error{fmt.Errorf("re-read unrecorded plugin executors: %w", err)}
	}
	unsettled := make([]error, 0, len(remaining))
	for _, item := range remaining {
		unsettled = append(unsettled, fmt.Errorf("plugin executor launch %s provisioned a resource that is not confirmed destroyed; cleanup cannot settle", item.ExecutionID))
	}
	return unsettled
}

// destroyHandedOffPluginExecutorResource destroys the resource a failed launch
// could not record, under the cleanup claim, and forgets the handoff only once
// the destroy is settled.
func (r *PluginRemoteExecutor) destroyHandedOffPluginExecutorResource(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	handoff *models.UnrecordedPluginExecutor,
	profile *models.ExecutorProviderLaunchProfile,
) error {
	resource := inventory.Resource
	if resource == nil {
		handed, err := decodePluginExecutorInventory(handoff.Running.Metadata)
		if err != nil || handed.Resource == nil {
			return errors.New("plugin executor handoff carries no resource handle")
		}
		resource = handed.Resource
	}
	fields := []zap.Field{zap.String("execution_id", handoff.ExecutionID), zap.String("environment_id", handoff.EnvironmentID)}
	if err := r.destroyPluginExecutorRecord(ctx, record, inventory, resource, pluginExecutorCleanupReasonLaunch, profile); err != nil {
		r.logger.Warn("plugin executor handoff destroy left the handoff recorded", append(fields, zap.Error(err))...)
		return err
	}
	r.logger.Info("plugin executor handoff destroyed and settled", fields...)
	return r.settleUnrecordedPluginExecutor(ctx, handoff)
}
