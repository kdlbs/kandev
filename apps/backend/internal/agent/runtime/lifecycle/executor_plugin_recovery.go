package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentruntime "github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"go.uber.org/zap"
)

type pluginExecutorRecoveryState struct {
	inventory        pluginExecutorInventory
	profile          *models.ExecutorProviderLaunchProfile
	operationContext *pluginsdk.ExecutorProviderRequestContext
	resource         *pluginsdk.ExecutorResourceDescriptor
	wasAllocating    bool
	cleanupPending   bool
}

func (r *PluginRemoteExecutor) recoverPluginExecutor(ctx context.Context, record *models.ExecutorRunning) (*ExecutorInstance, error) {
	state, err := r.loadPluginExecutorRecoveryState(ctx, record)
	if err != nil || state == nil {
		return nil, err
	}
	if pluginExecutorNeedsOperationRecovery(state) {
		resource, continueRecovery, err := r.recoverPluginExecutorAllocation(ctx, record, state)
		if err != nil || !continueRecovery {
			return nil, err
		}
		state.resource = resource
	}
	if shouldCleanupRecoveredPluginExecutor(state, record) {
		if state.resource == nil {
			return nil, errors.New("plugin executor cleanup inventory has no resource handle")
		}
		if err := r.destroyPluginExecutorRecord(ctx, record, state.inventory, state.resource, "recovery_cleanup"); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if isTerminalExecutorRunningStatus(record.Status) {
		return nil, nil
	}
	return r.attachRecoveredPluginExecutor(ctx, record, state)
}

func (r *PluginRemoteExecutor) loadPluginExecutorRecoveryState(ctx context.Context, record *models.ExecutorRunning) (*pluginExecutorRecoveryState, error) {
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return nil, err
	}
	if inventory.Phase == pluginExecutorPhaseExpired {
		return nil, nil
	}
	if !pluginExecutorRecoveryIdentityComplete(inventory) {
		return nil, errors.New("plugin executor recovery inventory is incomplete")
	}
	profile, err := r.profileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, record.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil {
		return nil, err
	}
	if profile == nil || !pluginExecutorProviderMatchesInventory(profile.Provider, inventory) {
		return nil, errors.New("recorded plugin executor provider is unavailable or incompatible")
	}
	return &pluginExecutorRecoveryState{
		inventory: inventory, profile: profile, resource: inventory.Resource,
		operationContext: pluginExecutorRequestContextForRecord(record, inventory, *profile),
		wasAllocating:    inventory.Phase == pluginExecutorPhaseAllocating,
		cleanupPending:   inventory.Phase == pluginExecutorPhaseCleanupPending,
	}, nil
}

func pluginExecutorRecoveryIdentityComplete(inventory pluginExecutorInventory) bool {
	return inventory.EnvironmentID != "" && inventory.EnvironmentGeneration > 0 && inventory.OperationID != "" &&
		inventory.InputDigest != "" && inventory.ProfileID != "" && inventory.ProviderIdentity != ""
}

func pluginExecutorNeedsOperationRecovery(state *pluginExecutorRecoveryState) bool {
	return state.inventory.Phase == pluginExecutorPhaseAllocating || state.resource == nil
}

func (r *PluginRemoteExecutor) recoverPluginExecutorAllocation(
	ctx context.Context,
	record *models.ExecutorRunning,
	state *pluginExecutorRecoveryState,
) (*pluginsdk.ExecutorResourceDescriptor, bool, error) {
	recovery, err := r.operations.RecoverExecutorOperation(ctx, &pluginsdk.RecoverExecutorOperationRequest{
		Context: state.operationContext,
		Profile: &pluginsdk.ExecutorProfileSnapshot{ProfileId: state.profile.ProfileID, Config: state.profile.Config, SecretValues: state.profile.SecretValues},
	})
	if err != nil || recovery == nil || recovery.GetError() != nil {
		return nil, false, errors.New("plugin executor operation recovery is unavailable")
	}
	switch strings.ToLower(strings.TrimSpace(recovery.GetOutcome())) {
	case pluginExecutorStateUnknown:
		r.logger.Warn("plugin executor allocation outcome remains unknown; inventory retained",
			zap.String("session_id", record.SessionID), zap.String("task_id", record.TaskID))
		return nil, false, nil
	case pluginExecutorPhaseAbsent:
		return nil, false, r.settleAbsentPluginExecutor(ctx, record, state.inventory, nil)
	case "found":
		return r.acceptRecoveredPluginExecutorResource(ctx, record, state, recovery.GetResource())
	default:
		return nil, false, errors.New("provider returned an unsupported operation recovery outcome")
	}
}

func (r *PluginRemoteExecutor) acceptRecoveredPluginExecutorResource(
	ctx context.Context,
	record *models.ExecutorRunning,
	state *pluginExecutorRecoveryState,
	resource *pluginsdk.ExecutorResourceDescriptor,
) (*pluginsdk.ExecutorResourceDescriptor, bool, error) {
	if err := validatePluginExecutorResourceForProvider(state.profile.Provider, resource); err != nil {
		return nil, false, err
	}
	if state.inventory.Resource != nil && state.inventory.Resource.GetResourceHandle() != resource.GetResourceHandle() {
		return nil, false, errors.New("provider recovery returned a different resource handle")
	}
	var err error
	state.inventory.Resource = resource
	state.inventory.StateVersion = resource.GetStateVersion()
	state.inventory.Platform = resource.GetPlatform()
	state.inventory.ExpiresAt, err = pluginExecutorEarlierExpiry(state.inventory.ExpiresAt, resource.GetExpiresAt())
	if err != nil {
		return nil, false, err
	}
	state.inventory.Phase = pluginExecutorPhaseProvisioned
	if err := r.checkpointPluginExecutorRecord(ctx, record, state.inventory); err != nil {
		return nil, false, err
	}
	return resource, true, nil
}

func shouldCleanupRecoveredPluginExecutor(state *pluginExecutorRecoveryState, record *models.ExecutorRunning) bool {
	return state.cleanupPending || state.wasAllocating && isTerminalExecutorRunningStatus(record.Status)
}

func (r *PluginRemoteExecutor) attachRecoveredPluginExecutor(
	ctx context.Context,
	record *models.ExecutorRunning,
	state *pluginExecutorRecoveryState,
) (*ExecutorInstance, error) {
	if !pluginExecutorPhaseCanAttach(state.inventory.Phase) {
		return nil, fmt.Errorf("plugin executor inventory phase %q cannot be attached", state.inventory.Phase)
	}
	if err := validatePluginExecutorResourceForProvider(state.profile.Provider, state.resource); err != nil {
		return nil, err
	}
	if record.TransientAuthToken == "" {
		if err := r.destroyPluginExecutorRecord(ctx, record, state.inventory, state.resource, "incomplete_bootstrap"); err != nil {
			return nil, err
		}
		return nil, nil
	}
	resource, _, canAttach, err := r.inspectPluginExecutorBeforeAttach(ctx, record, state, state.resource)
	if err != nil || !canAttach {
		return nil, err
	}
	attachedResource, inspection, canAttach, err := r.attachAndVerifyRecoveredPluginExecutor(ctx, record, state, resource)
	if err != nil || !canAttach {
		return nil, err
	}
	state.inventory.Resource = attachedResource
	state.inventory.StateVersion = attachedResource.GetStateVersion()
	state.inventory.Platform = attachedResource.GetPlatform()
	state.inventory.ExpiresAt, err = pluginExecutorEarlierExpiry(state.inventory.ExpiresAt, inspection.GetExpiresAt())
	if err != nil {
		return nil, err
	}
	state.inventory.Phase = pluginExecutorPhaseReady
	state.inventory.Generation++
	if err := r.checkpointPluginExecutorRecord(ctx, record, state.inventory); err != nil {
		return nil, err
	}
	return r.newRecoveredPluginExecutorInstance(ctx, record, state)
}

func pluginExecutorPhaseCanAttach(phase string) bool {
	switch phase {
	case pluginExecutorPhaseArtifactStaging, pluginExecutorPhaseBootstrapping, pluginExecutorPhaseProvisioned, pluginExecutorPhaseReady:
		return true
	default:
		return false
	}
}

func (r *PluginRemoteExecutor) inspectPluginExecutorBeforeAttach(
	ctx context.Context,
	record *models.ExecutorRunning,
	state *pluginExecutorRecoveryState,
	resource *pluginsdk.ExecutorResourceDescriptor,
) (*pluginsdk.ExecutorResourceDescriptor, *pluginsdk.InspectExecutorEnvironmentResponse, bool, error) {
	inspection, err := r.operations.InspectExecutorEnvironment(ctx, &pluginsdk.InspectExecutorEnvironmentRequest{
		Context: state.operationContext, Resource: resource,
	})
	if err != nil || inspection == nil || inspection.GetError() != nil {
		return nil, nil, false, errors.New("plugin executor environment inspection is unavailable")
	}
	if err := r.applyPluginExecutorInspection(ctx, record, &state.inventory, inspection); err != nil {
		return nil, nil, false, err
	}
	if state.inventory.Phase == pluginExecutorPhaseExpired {
		return nil, nil, false, nil
	}
	currentState := strings.ToLower(strings.TrimSpace(inspection.GetState()))
	if currentState == pluginExecutorPhaseAbsent {
		return nil, nil, false, r.settleAbsentPluginExecutor(ctx, record, state.inventory, resource)
	}
	if currentState != pluginExecutorStateRunning && currentState != pluginExecutorStateSuspended {
		return nil, nil, false, nil
	}
	return resource, inspection, true, nil
}

func (r *PluginRemoteExecutor) attachAndVerifyRecoveredPluginExecutor(
	ctx context.Context,
	record *models.ExecutorRunning,
	state *pluginExecutorRecoveryState,
	resource *pluginsdk.ExecutorResourceDescriptor,
) (*pluginsdk.ExecutorResourceDescriptor, *pluginsdk.InspectExecutorEnvironmentResponse, bool, error) {
	attached, err := r.operations.AttachExecutorEnvironment(ctx, &pluginsdk.AttachExecutorEnvironmentRequest{
		Context: state.operationContext, Resource: resource,
		ExpectedRuntimeIdentity: pluginExecutorRuntimeIdentity(state.inventory, record.AgentExecutionID),
	})
	if err != nil || attached == nil || attached.GetError() != nil {
		return nil, nil, false, errors.New("plugin executor environment attachment is unavailable")
	}
	attachedResource := attached.GetResource()
	if err := validatePluginExecutorResourceForProvider(state.profile.Provider, attachedResource); err != nil {
		return nil, nil, false, err
	}
	if attachedResource.GetResourceHandle() != resource.GetResourceHandle() || attachedResource.GetStateVersion() != resource.GetStateVersion() {
		return nil, nil, false, errors.New("provider attachment changed the recorded resource identity")
	}
	inspection, err := r.operations.InspectExecutorEnvironment(ctx, &pluginsdk.InspectExecutorEnvironmentRequest{
		Context: state.operationContext, Resource: attachedResource,
	})
	if err != nil || inspection == nil || inspection.GetError() != nil {
		return nil, nil, false, errors.New("plugin executor environment inspection is unavailable after attachment")
	}
	if err := r.applyPluginExecutorInspection(ctx, record, &state.inventory, inspection); err != nil {
		return nil, nil, false, err
	}
	if state.inventory.Phase == pluginExecutorPhaseExpired {
		return nil, nil, false, nil
	}
	currentState := strings.ToLower(strings.TrimSpace(inspection.GetState()))
	if currentState == pluginExecutorPhaseAbsent {
		return nil, nil, false, r.settleAbsentPluginExecutor(ctx, record, state.inventory, attachedResource)
	}
	if currentState != pluginExecutorStateRunning {
		return nil, nil, false, nil
	}
	return attachedResource, inspection, true, nil
}

func (r *PluginRemoteExecutor) newRecoveredPluginExecutorInstance(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) (*ExecutorInstance, error) {
	client, err := r.newRecoveredAgentctlClient(
		ctx, r.connectionResolver(state.operationContext, state.inventory.Resource), r.logger,
		record.AgentExecutionID, record.TransientAuthToken,
	)
	if err != nil {
		return nil, errors.New("plugin executor agentctl transport is unavailable")
	}
	if err := r.ready(ctx, client); err != nil {
		client.Close()
		return nil, errors.New("plugin executor agentctl readiness check failed")
	}
	metadata := clonePluginExecutorMetadata(record.Metadata)
	metadata[MetadataKeyPluginExecutor] = state.inventory
	return &ExecutorInstance{
		InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		RuntimeName: agentruntime.RuntimePluginRemote, Client: client, WorkspacePath: "/workspace",
		Metadata: metadata, AuthToken: record.TransientAuthToken, AgentProfileID: record.ExecutionProfileID,
	}, nil
}

func decodePluginExecutorInventory(metadata map[string]interface{}) (pluginExecutorInventory, error) {
	value, ok := metadata[MetadataKeyPluginExecutor]
	if !ok || value == nil {
		return pluginExecutorInventory{}, errors.New("plugin executor recovery inventory is missing")
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > pluginExecutorInventoryMaxBytes {
		return pluginExecutorInventory{}, errors.New("plugin executor recovery inventory is invalid")
	}
	var inventory pluginExecutorInventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return pluginExecutorInventory{}, errors.New("plugin executor recovery inventory is invalid")
	}
	return inventory, nil
}

func pluginExecutorRequestContextForRecord(
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	profile models.ExecutorProviderLaunchProfile,
) *pluginsdk.ExecutorProviderRequestContext {
	return &pluginsdk.ExecutorProviderRequestContext{
		PluginId: inventory.PluginID, InstallationId: inventory.InstallationID, ProviderKey: inventory.ProviderKey,
		ContractVersion: int32(inventory.ContractVersion), TaskId: record.TaskID, SessionId: record.SessionID,
		EnvironmentId: inventory.EnvironmentID, ExecutionId: record.AgentExecutionID,
		OperationId: inventory.OperationID, Deadline: time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339Nano),
		EnvironmentGeneration: uint64(profile.OwnershipGeneration), InputDigest: inventory.InputDigest,
	}
}

func (r *PluginRemoteExecutor) applyPluginExecutorInspection(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory *pluginExecutorInventory,
	inspection *pluginsdk.InspectExecutorEnvironmentResponse,
) error {
	state := strings.ToLower(strings.TrimSpace(inspection.GetState()))
	switch state {
	case pluginExecutorStateRunning, pluginExecutorStateSuspended, pluginExecutorStateTerminated, pluginExecutorPhaseAbsent, pluginExecutorStateUnknown:
	default:
		return errors.New("provider returned an unsupported environment state")
	}
	var err error
	inventory.ExpiresAt, err = pluginExecutorEarlierExpiry(inventory.ExpiresAt, inspection.GetExpiresAt())
	if err != nil {
		return err
	}
	now := time.Now()
	deadlinePassed := false
	if inventory.ExpiresAt != "" {
		if expiresAt, parseErr := time.Parse(time.RFC3339Nano, inventory.ExpiresAt); parseErr == nil {
			deadlinePassed = !now.Before(expiresAt)
		}
	}
	inventory.ExpiryCheckAt = now.UTC().Format(time.RFC3339Nano)
	if pluginExecutorInspectionConfirmsExpiry(inspection, deadlinePassed) {
		inventory.Phase = pluginExecutorPhaseExpired
		inventory.Resource = nil
		inventory.LastInspectionState = pluginExecutorPhaseExpired
	} else {
		inventory.LastInspectionState = state
	}
	return r.checkpointPluginExecutorRecord(ctx, record, *inventory)
}

func (r *PluginRemoteExecutor) checkpointPluginExecutorRecord(ctx context.Context, record *models.ExecutorRunning, inventory pluginExecutorInventory) error {
	metadata := clonePluginExecutorMetadata(record.Metadata)
	metadata[MetadataKeyPluginExecutor] = inventory
	updated := *record
	updated.Metadata = metadata
	updated.Runtime = agentruntime.RuntimePluginRemote
	current, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return err
	}
	updated.ExpectedPluginExecutorRevision = current.Revision
	if err := r.inventoryStore.CheckpointPluginExecutorInventory(ctx, &updated); err != nil {
		return err
	}
	record.UpdatedAt = updated.UpdatedAt
	record.Metadata = updated.Metadata
	return nil
}

func (r *PluginRemoteExecutor) settleAbsentPluginExecutor(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
) error {
	inventory.Phase = pluginExecutorPhaseAbsent
	inventory.Resource = nil
	if resource != nil {
		inventory.StateVersion = resource.GetStateVersion()
	}
	session, err := r.inventoryStore.GetTaskSession(ctx, record.SessionID)
	if err != nil || session == nil {
		return r.checkpointPluginExecutorRecord(ctx, record, inventory)
	}
	if models.RowMustBePreserved(record, session.State) {
		return r.checkpointPluginExecutorRecord(ctx, record, inventory)
	}
	claim, err := r.acquirePluginExecutorCleanupClaim(ctx, record.TaskID, record.SessionID, inventory)
	if err != nil {
		return r.checkpointPluginExecutorRecord(ctx, record, inventory)
	}
	defer r.releasePluginExecutorCleanupClaim(claim)
	claimedCtx := recoveryclaim.WithClaim(ctx, claim)
	if err := r.inventoryStore.DeletePluginExecutorInventoryIfCurrent(claimedCtx, record.SessionID, record.AgentExecutionID, inventory.EnvironmentGeneration); err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return err
	}
	return nil
}

func (r *PluginRemoteExecutor) cleanupPluginExecutorInstance(ctx context.Context, instance *ExecutorInstance) error {
	inventory, err := decodePluginExecutorInventory(instance.Metadata)
	if err != nil {
		return err
	}
	if inventory.Resource == nil {
		return errors.New("plugin executor cleanup resource is unavailable")
	}
	profile, err := r.profileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, instance.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil {
		return err
	}
	record, err := r.currentPluginExecutorRecord(ctx, instance.SessionID, instance.InstanceID)
	if err != nil {
		return err
	}
	if profile.Provider.Identity != inventory.ProviderIdentity || profile.Provider.InstallationID != inventory.InstallationID {
		return errors.New("recorded plugin executor provider is unavailable")
	}
	return r.destroyPluginExecutorRecord(ctx, record, inventory, inventory.Resource, "task_cleanup")
}

func (r *PluginRemoteExecutor) DestroyTaskEnvironment(ctx context.Context, environment *models.TaskEnvironment) error {
	if environment == nil || environment.ID == "" || environment.TaskID == "" || environment.OwnershipGeneration <= 0 {
		return errors.New("plugin executor environment cleanup identity is incomplete")
	}
	if r.inventoryStore == nil {
		return errors.New("plugin executor cleanup dependencies are unavailable")
	}
	records, err := r.inventoryStore.ListExecutorsRunningPluginRemote(ctx)
	if err != nil {
		return fmt.Errorf("read plugin executor cleanup inventory: %w", err)
	}
	var cleanupErrors []error
	for _, record := range records {
		if err := r.destroyPluginExecutorEnvironmentRecord(ctx, environment, record); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func (r *PluginRemoteExecutor) destroyPluginExecutorEnvironmentRecord(ctx context.Context, environment *models.TaskEnvironment, record *models.ExecutorRunning) error {
	if !pluginExecutorRecordMatchesEnvironment(environment, record) {
		return nil
	}
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return err
	}
	if inventory.EnvironmentID != environment.ID || inventory.EnvironmentGeneration != environment.OwnershipGeneration || inventory.Phase == pluginExecutorPhaseAbsent {
		return nil
	}
	if inventory.Phase == pluginExecutorPhaseExpired {
		return r.settleAbsentPluginExecutor(ctx, record, inventory, nil)
	}
	profile, err := r.pluginExecutorCleanupProfile(ctx, record, inventory)
	if err != nil {
		return err
	}
	resource, shouldDestroy, err := r.recoverPluginExecutorCleanupResource(ctx, record, inventory, profile)
	if err != nil || !shouldDestroy {
		return err
	}
	return r.destroyPluginExecutorRecord(ctx, record, inventory, resource, "task_environment_reset")
}

func pluginExecutorRecordMatchesEnvironment(environment *models.TaskEnvironment, record *models.ExecutorRunning) bool {
	return record != nil && record.TaskID == environment.TaskID && record.Runtime == agentruntime.RuntimePluginRemote
}

func (r *PluginRemoteExecutor) pluginExecutorCleanupProfile(ctx context.Context, record *models.ExecutorRunning, inventory pluginExecutorInventory) (*models.ExecutorProviderLaunchProfile, error) {
	if r.profileLoader == nil || r.operations == nil {
		return nil, errors.New("plugin executor cleanup dependencies are unavailable")
	}
	profile, err := r.profileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, record.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil || profile == nil || profile.Provider.Identity != inventory.ProviderIdentity || profile.Provider.InstallationID != inventory.InstallationID {
		return nil, errors.New("recorded plugin executor provider is unavailable for cleanup")
	}
	return profile, nil
}

func (r *PluginRemoteExecutor) recoverPluginExecutorCleanupResource(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	profile *models.ExecutorProviderLaunchProfile,
) (*pluginsdk.ExecutorResourceDescriptor, bool, error) {
	if inventory.Resource != nil {
		return inventory.Resource, true, nil
	}
	switch inventory.Phase {
	case pluginExecutorPhaseAllocating, pluginExecutorPhaseArtifactStaging, pluginExecutorPhaseBootstrapping,
		pluginExecutorPhaseProvisioned, pluginExecutorPhaseReady, pluginExecutorPhaseCleanupPending:
	default:
		return nil, false, errors.New("plugin executor cleanup resource handle is missing")
	}
	operationContext := pluginExecutorRequestContextForRecord(record, inventory, *profile)
	recovery, err := r.operations.RecoverExecutorOperation(ctx, &pluginsdk.RecoverExecutorOperationRequest{
		Context: operationContext,
		Profile: &pluginsdk.ExecutorProfileSnapshot{ProfileId: profile.ProfileID, Config: profile.Config, SecretValues: profile.SecretValues},
	})
	if err != nil || recovery == nil || recovery.GetError() != nil {
		return nil, false, errors.New("plugin executor allocation outcome is unavailable for cleanup")
	}
	switch strings.ToLower(strings.TrimSpace(recovery.GetOutcome())) {
	case pluginExecutorStateUnknown:
		return nil, false, errors.New("plugin executor allocation outcome is unknown; cleanup was retained")
	case pluginExecutorPhaseAbsent:
		return nil, false, r.settleAbsentPluginExecutor(ctx, record, inventory, nil)
	case "found":
		return r.recordRecoveredCleanupResource(ctx, record, inventory, profile, recovery.GetResource())
	default:
		return nil, false, errors.New("provider returned an unsupported operation recovery outcome")
	}
}

func (r *PluginRemoteExecutor) recordRecoveredCleanupResource(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	profile *models.ExecutorProviderLaunchProfile,
	resource *pluginsdk.ExecutorResourceDescriptor,
) (*pluginsdk.ExecutorResourceDescriptor, bool, error) {
	if err := validatePluginExecutorResourceForProvider(profile.Provider, resource); err != nil {
		return nil, false, err
	}
	inventory.Resource = resource
	inventory.StateVersion = resource.GetStateVersion()
	inventory.Platform = resource.GetPlatform()
	var err error
	inventory.ExpiresAt, err = pluginExecutorEarlierExpiry(inventory.ExpiresAt, resource.GetExpiresAt())
	if err != nil {
		return nil, false, err
	}
	inventory.Phase = pluginExecutorPhaseProvisioned
	if err := r.checkpointPluginExecutorRecord(ctx, record, inventory); err != nil {
		return nil, false, err
	}
	return resource, true, nil
}

func (m *Manager) DestroyPluginExecutorEnvironment(ctx context.Context, environment *models.TaskEnvironment) error {
	backend, err := m.executorRegistry.GetBackend(executor.NamePluginRemote)
	if err != nil {
		return fmt.Errorf("plugin executor backend unavailable: %w", err)
	}
	destroyer, ok := backend.(*PluginRemoteExecutor)
	if !ok {
		return fmt.Errorf("plugin executor backend has unexpected type %T", backend)
	}
	return destroyer.DestroyTaskEnvironment(ctx, environment)
}

func (m *Manager) GetPluginExecutorEnvironmentStatus(ctx context.Context, record *models.ExecutorRunning) (*models.PluginExecutorEnvironmentStatus, error) {
	backend, err := m.executorRegistry.GetBackend(executor.NamePluginRemote)
	if err != nil {
		return &models.PluginExecutorEnvironmentStatus{State: pluginExecutorStateUnavailable, Retention: pluginExecutorStateUnknown, Reason: "provider_unavailable"}, nil
	}
	runtime, ok := backend.(*PluginRemoteExecutor)
	if !ok {
		return nil, fmt.Errorf("plugin executor backend has unexpected type %T", backend)
	}
	return runtime.GetEnvironmentStatus(ctx, record)
}

type pluginExecutorInventoryReader interface {
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
}

func (r *PluginRemoteExecutor) currentPluginExecutorRecord(ctx context.Context, sessionID, executionID string) (*models.ExecutorRunning, error) {
	reader, ok := r.inventoryStore.(pluginExecutorInventoryReader)
	if !ok {
		return nil, errors.New("plugin executor inventory reader is unavailable")
	}
	record, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil || record == nil || record.AgentExecutionID != executionID || record.Runtime != agentruntime.RuntimePluginRemote {
		return nil, models.ErrExecutionRotated
	}
	return record, nil
}

func (r *PluginRemoteExecutor) destroyPluginExecutorRecord(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	reason string,
) error {
	claim, err := r.acquirePluginExecutorCleanupClaim(ctx, record.TaskID, record.SessionID, inventory)
	if err != nil {
		return fmt.Errorf("plugin executor cleanup is fenced: %w", err)
	}
	defer r.releasePluginExecutorCleanupClaim(claim)
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	claimedCtx := recoveryclaim.WithClaim(cleanupCtx, claim)
	operationContext := pluginExecutorRequestContextForRecord(record, inventory, models.ExecutorProviderLaunchProfile{
		Provider:            models.ExecutorProvider{PluginID: inventory.PluginID, InstallationID: inventory.InstallationID, Key: inventory.ProviderKey, ContractVersion: inventory.ContractVersion},
		OwnershipGeneration: inventory.EnvironmentGeneration,
	})
	operationContext.Deadline = time.Now().Add(30 * time.Second).UTC().Format(time.RFC3339Nano)
	response, err := r.operations.DestroyExecutorEnvironment(claimedCtx, &pluginsdk.DestroyExecutorEnvironmentRequest{
		Context: operationContext, Resource: resource, CleanupReason: reason, CleanupClaim: claim.OperationID,
	})
	if err != nil || response == nil || response.GetError() != nil || !response.GetConfirmedAbsent() {
		inventory.Resource = resource
		inventory.Phase = pluginExecutorPhaseCleanupPending
		if checkpointErr := r.checkpointPluginExecutorRecord(claimedCtx, record, inventory); checkpointErr != nil {
			return fmt.Errorf("plugin executor cleanup is unconfirmed and inventory checkpoint failed: %w", checkpointErr)
		}
		return errors.New("plugin executor cleanup is unconfirmed; retry information was retained")
	}
	inventory.Resource = nil
	inventory.Phase = pluginExecutorPhaseAbsent
	session, sessionErr := r.inventoryStore.GetTaskSession(claimedCtx, record.SessionID)
	if sessionErr == nil && session != nil && !models.RowMustBePreserved(record, session.State) {
		if err := r.inventoryStore.DeletePluginExecutorInventoryIfCurrent(claimedCtx, record.SessionID, record.AgentExecutionID, inventory.EnvironmentGeneration); err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
			return fmt.Errorf("plugin executor cleanup succeeded but inventory removal failed: %w", err)
		}
	} else if err := r.checkpointPluginExecutorRecord(claimedCtx, record, inventory); err != nil {
		return fmt.Errorf("plugin executor cleanup succeeded but inventory checkpoint failed: %w", err)
	}
	return nil
}

func (r *PluginRemoteExecutor) acquirePluginExecutorCleanupClaim(
	ctx context.Context,
	taskID, sessionID string,
	inventory pluginExecutorInventory,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	if r.inventoryStore == nil {
		return nil, errors.New("plugin executor inventory store is unavailable")
	}
	session, err := r.inventoryStore.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load plugin executor cleanup session: %w", err)
	}
	if session == nil || session.TaskID != taskID || session.QueueIncarnationID == "" {
		return nil, errors.New("plugin executor cleanup session identity is unavailable")
	}
	request := models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: inventory.EnvironmentID, OwnerTaskID: taskID,
		OwnershipGeneration: inventory.EnvironmentGeneration, SessionID: sessionID,
		SessionIncarnationID: session.QueueIncarnationID,
		OperationID:          inventory.OperationID + ":cleanup", ExecutorType: string(models.ExecutorTypePluginRemote),
		AllowCurrentSessionRuntime: true,
	}
	if job, ok := recoveryclaim.TaskCleanupJobFromContext(ctx); ok && job.TaskID == taskID {
		request.CleanupJobID = job.ID
	}
	return r.inventoryStore.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
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
