package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
)

// unrecordedPluginResource is a provisioned resource whose launch could neither
// take the cleanup claim nor record the handle. The environment cleanup, which
// runs under a persisted claim, destroys it.
type unrecordedPluginResource struct {
	taskID        string
	environmentID string
	generation    int64
	resource      *pluginsdk.ExecutorResourceDescriptor
}

// holdUnrecordedPluginExecutorResource keeps the handle of a resource that has
// neither a durable inventory record nor a cleanup claim, as when an archive
// rotated the execution after the provider returned. A provider mutates a
// resource only under a persisted claim, so the launch never destroys it: the
// handle waits for the environment cleanup and that cleanup fails until it has
// removed the resource.
func (r *PluginRemoteExecutor) holdUnrecordedPluginExecutorResource(
	req *ExecutorCreateRequest,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	stage string,
	cause error,
) error {
	r.unrecorded.Store(req.InstanceID, &unrecordedPluginResource{
		taskID: req.TaskID, environmentID: inventory.EnvironmentID,
		generation: inventory.EnvironmentGeneration, resource: resource,
	})
	r.logger.Error("plugin executor resource is unrecorded; its removal is left to the environment cleanup",
		zap.String("execution_id", req.InstanceID), zap.String("environment_id", inventory.EnvironmentID),
		zap.String("stage", stage), zap.String("resource_handle", resource.GetResourceHandle()),
		zap.String("cleanup_outcome", "handed_off"), zap.NamedError("launch_cause", cause))
	return withPluginExecutorLaunchCode(cause, errors.New("plugin executor launch failed; allocated resource was not recorded and will be removed by the environment cleanup"))
}

// unrecordedResourceExecutions lists the handed-off executions of an
// environment generation.
func (r *PluginRemoteExecutor) unrecordedResourceExecutions(environment *models.TaskEnvironment) []string {
	var executions []string
	r.unrecorded.Range(func(key, value any) bool {
		held := value.(*unrecordedPluginResource)
		if held.taskID == environment.TaskID && held.environmentID == environment.ID && held.generation == environment.OwnershipGeneration {
			executions = append(executions, key.(string))
		}
		return true
	})
	return executions
}

// destroyHandedOffPluginExecutorResource destroys the resource a failed launch
// could not record, under the cleanup claim, and forgets the handle only once
// the destroy is settled.
func (r *PluginRemoteExecutor) destroyHandedOffPluginExecutorResource(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	held *unrecordedPluginResource,
	profile *models.ExecutorProviderLaunchProfile,
) error {
	if err := r.destroyPluginExecutorRecord(ctx, record, inventory, held.resource, "launch_unrecorded", profile); err != nil {
		return err
	}
	r.unrecorded.Delete(record.AgentExecutionID)
	return nil
}
