package lifecycle

import (
	"context"
	"errors"
	"strings"

	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (r *PluginRemoteExecutor) attachRetainedPluginExecutor(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	state, err := r.loadRetainedPluginExecutor(ctx, req)
	if err != nil {
		return nil, err
	}
	resource, inspection, err := r.inspectAndAttachRetainedPluginExecutor(ctx, state)
	if err != nil {
		return nil, err
	}
	inventory := state.inventory
	inventory.Resource = resource
	inventory.RuntimeIdentity = state.runtimeIdentity
	inventory.StateVersion = resource.GetStateVersion()
	inventory.Platform = resource.GetPlatform()
	inventory.ExpiresAt, err = pluginExecutorEarlierExpiry(inventory.ExpiresAt, inspection.GetExpiresAt())
	if err != nil {
		return nil, err
	}
	inventory.LastInspectionState = pluginExecutorStateRunning
	inventory.Phase = pluginExecutorPhaseReady
	inventory.Revision++
	inventory.Generation++
	client, err := r.newRecoveredAgentctlClient(ctx, r.connectionResolver(state.operationContext, resource, state.inventory.InstancePort), r.logger, state.runtimeIdentity, state.token)
	if err != nil {
		return nil, errors.New("plugin executor retained agentctl transport is unavailable")
	}
	if err := r.ready(ctx, client); err != nil {
		client.Close()
		return nil, errors.New("plugin executor retained agentctl readiness check failed")
	}
	metadata := clonePluginExecutorMetadata(req.Metadata)
	metadata[MetadataKeyPluginExecutor] = inventory
	return &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: agentruntime.RuntimePluginRemote, Client: client, WorkspacePath: pluginExecutorWorkspacePath,
		Metadata: metadata, AuthToken: state.token, BootstrapNonce: req.BootstrapNonce,
		AgentProfileID: req.AgentProfileID,
	}, nil
}

type retainedPluginExecutorState struct {
	record           *models.ExecutorRunning
	inventory        pluginExecutorInventory
	profile          *models.ExecutorProviderLaunchProfile
	token            string
	runtimeIdentity  string
	operationContext *pluginsdk.ExecutorProviderRequestContext
}

func (r *PluginRemoteExecutor) loadRetainedPluginExecutor(ctx context.Context, req *ExecutorCreateRequest) (*retainedPluginExecutorState, error) {
	if err := r.validateRetainedPluginExecutorRequest(req); err != nil {
		return nil, err
	}
	record, err := r.inventoryStore.GetExecutorRunningBySessionID(ctx, req.SessionID)
	if err != nil || record == nil || record.Runtime != agentruntime.RuntimePluginRemote ||
		record.TaskID != req.TaskID || record.AgentExecutionID != req.PreviousExecutionID {
		return nil, models.ErrExecutionRotated
	}
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return nil, err
	}
	if !pluginExecutorInventoryRetainedAttachable(inventory, req.TaskEnvironmentID) {
		return nil, errors.New("plugin executor retained environment is not attachable")
	}
	profile, err := r.profileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, record.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil || profile == nil {
		return nil, errors.New("plugin executor retained provider profile is unavailable")
	}
	provider := profile.Provider
	if !pluginExecutorProviderMatchesInventory(provider, inventory) {
		return nil, errors.New("recorded plugin executor provider is unavailable or incompatible")
	}
	if err := validatePluginExecutorResourceForProvider(provider, inventory.Resource); err != nil {
		return nil, err
	}
	token := req.AuthToken
	if token == "" {
		token = record.TransientAuthToken
	}
	if token == "" {
		return nil, errors.New("plugin executor agentctl authorization is unavailable")
	}
	return &retainedPluginExecutorState{
		record: record, inventory: inventory, profile: profile, token: token,
		runtimeIdentity:  pluginExecutorRuntimeIdentity(inventory, record.AgentExecutionID),
		operationContext: pluginExecutorRequestContextForRecord(record, inventory, *profile),
	}, nil
}

func (r *PluginRemoteExecutor) validateRetainedPluginExecutorRequest(req *ExecutorCreateRequest) error {
	if r.operations == nil || r.profileLoader == nil || r.inventoryStore == nil {
		return errors.New("plugin executor retained-environment dependencies are unavailable")
	}
	if !pluginExecutorRetainedRequestIdentityComplete(req) {
		return errors.New("plugin executor retained-environment identity is incomplete")
	}
	if isPluginExecutorSharedWorkspace(req.Metadata) {
		return errors.New("plugin executor requires an isolated session workspace")
	}
	return nil
}

func pluginExecutorRetainedRequestIdentityComplete(req *ExecutorCreateRequest) bool {
	return req != nil && req.InstanceID != "" && req.TaskID != "" && req.SessionID != "" &&
		req.TaskEnvironmentID != "" && req.PreviousExecutionID != ""
}

func pluginExecutorInventoryRetainedAttachable(inventory pluginExecutorInventory, environmentID string) bool {
	return inventory.Phase == pluginExecutorPhaseReady && inventory.Resource != nil && inventory.InstancePort != 0 && inventory.EnvironmentID == environmentID &&
		inventory.EnvironmentGeneration > 0 && inventory.ProfileID != "" && inventory.OperationID != "" && inventory.InputDigest != ""
}

func pluginExecutorProviderMatchesInventory(provider models.ExecutorProvider, inventory pluginExecutorInventory) bool {
	return provider.Available && provider.PluginID == inventory.PluginID && provider.InstallationID == inventory.InstallationID &&
		provider.Key == inventory.ProviderKey && provider.Identity == inventory.ProviderIdentity &&
		provider.ContractVersion == inventory.ContractVersion
}

func (r *PluginRemoteExecutor) inspectAndAttachRetainedPluginExecutor(ctx context.Context, state *retainedPluginExecutorState) (*pluginsdk.ExecutorResourceDescriptor, *pluginsdk.InspectExecutorEnvironmentResponse, error) {
	inventory, provider := state.inventory, state.profile.Provider
	if err := r.validateRetainedPluginExecutorRunning(ctx, state); err != nil {
		return nil, nil, err
	}
	attached, err := r.operations.AttachExecutorEnvironment(ctx, &pluginsdk.AttachExecutorEnvironmentRequest{
		Context: state.operationContext, Resource: inventory.Resource, ExpectedRuntimeIdentity: state.runtimeIdentity,
	})
	if err != nil || attached == nil || attached.GetError() != nil {
		return nil, nil, errors.New("plugin executor retained environment attachment is unavailable")
	}
	resource := attached.GetResource()
	if err := validatePluginExecutorResourceForProvider(provider, resource); err != nil {
		return nil, nil, err
	}
	if resource.GetResourceHandle() != inventory.Resource.GetResourceHandle() || resource.GetStateVersion() != inventory.Resource.GetStateVersion() {
		return nil, nil, errors.New("provider attachment changed the recorded resource identity")
	}
	inspection, err := r.inspectAttachedRetainedPluginExecutor(ctx, state, resource)
	if err != nil {
		return nil, nil, err
	}
	return resource, inspection, nil
}

func (r *PluginRemoteExecutor) validateRetainedPluginExecutorRunning(ctx context.Context, state *retainedPluginExecutorState) error {
	inspection, err := r.operations.InspectExecutorEnvironment(ctx, &pluginsdk.InspectExecutorEnvironmentRequest{
		Context: state.operationContext, Resource: state.inventory.Resource,
	})
	if err != nil || inspection == nil || inspection.GetError() != nil {
		return errors.New("plugin executor retained environment inspection is unavailable")
	}
	switch strings.ToLower(strings.TrimSpace(inspection.GetState())) {
	case pluginExecutorPhaseAbsent:
		if err := r.settleAbsentPluginExecutor(ctx, state.record, state.inventory, state.inventory.Resource); err != nil {
			return err
		}
		return errors.New("plugin executor retained environment is absent; replacement provisioning is not automatic")
	case pluginExecutorStateRunning, pluginExecutorStateSuspended:
		return nil
	default:
		return errors.New("plugin executor retained environment is not running")
	}
}

func (r *PluginRemoteExecutor) inspectAttachedRetainedPluginExecutor(
	ctx context.Context,
	state *retainedPluginExecutorState,
	resource *pluginsdk.ExecutorResourceDescriptor,
) (*pluginsdk.InspectExecutorEnvironmentResponse, error) {
	inspection, err := r.operations.InspectExecutorEnvironment(ctx, &pluginsdk.InspectExecutorEnvironmentRequest{
		Context: state.operationContext, Resource: resource,
	})
	if err != nil || inspection == nil || inspection.GetError() != nil ||
		!strings.EqualFold(strings.TrimSpace(inspection.GetState()), pluginExecutorStateRunning) {
		return nil, errors.New("plugin executor retained environment did not become ready after attachment")
	}
	return inspection, nil
}

func pluginExecutorRuntimeIdentity(inventory pluginExecutorInventory, fallback string) string {
	if strings.TrimSpace(inventory.RuntimeIdentity) != "" {
		return inventory.RuntimeIdentity
	}
	return fallback
}
