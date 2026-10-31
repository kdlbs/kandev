package lifecycle

import (
	"context"
	"errors"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"go.uber.org/zap"
)

const (
	pluginExecutorCapabilityDurableBootstrap     = "durable_bootstrap"
	pluginExecutorCleanupReasonBootstrapConsumed = "bootstrap_consumed"
	pluginRuntimeSecretKindBootstrap             = "agentctl-bootstrap"
	pluginRuntimeSecretKindAuth                  = "agentctl-auth"
)

// PluginExecutorRuntimeSecrets is the secret-store surface a durable-bootstrap
// provider needs: the nonce and the agentctl token live there, never in the
// inventory envelope.
type PluginExecutorRuntimeSecrets interface {
	Create(context.Context, *secrets.SecretWithValue) error
	Get(context.Context, string) (*secrets.Secret, error)
	Reveal(context.Context, string) (string, error)
	Update(context.Context, string, *secrets.UpdateSecretRequest) error
	Delete(context.Context, string) error
}

// SetRuntimeSecretStore wires the store behind the durable-bootstrap secrets.
func (r *PluginRemoteExecutor) SetRuntimeSecretStore(store PluginExecutorRuntimeSecrets) {
	r.runtimeSecrets = store
}

func pluginExecutorDurableBootstrap(inventory pluginExecutorInventory) bool {
	return negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDurableBootstrap)
}

// putPluginRuntimeSecret stores value under the deterministic ID of the
// execution's secret of this kind and returns the ID.
func (r *PluginRemoteExecutor) putPluginRuntimeSecret(ctx context.Context, executionID, kind, value string) (string, error) {
	if r.runtimeSecrets == nil {
		return "", errors.New("plugin executor runtime secret store is unavailable")
	}
	id := kubernetesRuntimeSecretID(executionID, kind)
	name := kind + "-" + truncateID(executionID, 12)
	_, err := r.runtimeSecrets.Get(ctx, id)
	switch {
	case err == nil:
		err = r.runtimeSecrets.Update(ctx, id, &secrets.UpdateSecretRequest{Name: &name, Value: &value})
	case errors.Is(err, secrets.ErrNotFound):
		err = r.runtimeSecrets.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: id, Name: name}, Value: value})
	}
	if err != nil {
		return "", errors.New("plugin executor runtime secret could not be stored")
	}
	return id, nil
}

// loadPluginExecutorCleanupToken fills the agentctl credential a cleanup needs
// to drain the recorded runtime. The inventory store never returns the token,
// so it comes from the live instance when there is one and otherwise from the
// runtime secret the record references. A missing credential leaves the record
// without one, which the drain reports as not_attempted.
func (r *PluginRemoteExecutor) loadPluginExecutorCleanupToken(ctx context.Context, record *models.ExecutorRunning, instanceToken string) {
	if record.TransientAuthToken != "" {
		return
	}
	if instanceToken != "" {
		record.TransientAuthToken = instanceToken
		return
	}
	id := getMetadataString(record.Metadata, MetadataKeyAuthTokenSecret)
	if id == "" || r.runtimeSecrets == nil {
		return
	}
	value, err := revealPluginRuntimeSecret(ctx, r.runtimeSecrets, id)
	if err != nil {
		r.logger.Warn("plugin executor cleanup could not load the agentctl credential", zap.String("execution_id", record.AgentExecutionID))
		return
	}
	record.TransientAuthToken = value
}

func revealPluginRuntimeSecret(ctx context.Context, store PluginExecutorRuntimeSecrets, id string) (string, error) {
	if scoped, ok := store.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, id)
	}
	return store.Reveal(ctx, id)
}

// settlePluginExecutorSecrets deletes the durable-bootstrap secrets of an
// execution whose record has settled. Failure is logged and never blocks the
// settlement; the IDs are deterministic, so a later settlement retries them.
func (r *PluginRemoteExecutor) settlePluginExecutorSecrets(ctx context.Context, inventory pluginExecutorInventory, executionID string) {
	if r.runtimeSecrets == nil || !pluginExecutorDurableBootstrap(inventory) {
		return
	}
	for _, kind := range []string{pluginRuntimeSecretKindBootstrap, pluginRuntimeSecretKindAuth} {
		if err := r.runtimeSecrets.Delete(context.WithoutCancel(ctx), kubernetesRuntimeSecretID(executionID, kind)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			r.logger.Warn("plugin executor runtime secret was not deleted", zap.String("execution_id", executionID), zap.String("kind", kind))
		}
	}
}

func setPluginLaunchSecretRef(req *ExecutorCreateRequest, metadataKey, secretID string) {
	if req.Metadata == nil {
		req.Metadata = map[string]interface{}{}
	}
	req.Metadata[metadataKey] = secretID
}

// persistDurableNonce stores the bootstrap nonce and records its reference in
// the launch metadata, so the allocation checkpoint carries it.
func (r *PluginRemoteExecutor) persistDurableNonce(ctx context.Context, req *ExecutorCreateRequest, nonce string) error {
	id, err := r.putPluginRuntimeSecret(ctx, req.InstanceID, pluginRuntimeSecretKindBootstrap, nonce)
	if err != nil {
		return err
	}
	setPluginLaunchSecretRef(req, MetadataKeyBootstrapNonceSecret, id)
	return nil
}

// persistDurableToken stores the agentctl token and records its reference in
// the launch metadata before any instance is created with it.
func (r *PluginRemoteExecutor) persistDurableToken(ctx context.Context, req *ExecutorCreateRequest, token string) error {
	id, err := r.putPluginRuntimeSecret(ctx, req.InstanceID, pluginRuntimeSecretKindAuth, token)
	if err != nil {
		return err
	}
	setPluginLaunchSecretRef(req, MetadataKeyAuthTokenSecret, id)
	return nil
}

func (r *PluginRemoteExecutor) acquirePluginExecutorClaim(
	ctx context.Context,
	taskID, sessionID string,
	inventory pluginExecutorInventory,
	kind string,
) (*models.TaskEnvironmentRecoveryClaim, error) {
	if r.inventoryStore == nil {
		return nil, errors.New("plugin executor inventory store is unavailable")
	}
	request := models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: inventory.EnvironmentID, OwnerTaskID: taskID,
		OwnershipGeneration: inventory.EnvironmentGeneration, SessionID: sessionID,
		OperationID: r.newPluginExecutorClaimOperationID(inventory.OperationID, kind), ExecutorType: string(models.ExecutorTypePluginRemote),
		AllowCurrentSessionRuntime: true,
		SupersedeOperationPrefix:   pluginExecutorClaimPrefix(inventory.OperationID, kind), LiveOperationMarker: r.claimEpoch,
	}
	if job, ok := recoveryclaim.TaskCleanupJobFromContext(ctx); ok && job.TaskID == taskID {
		request.CleanupJobID = job.ID
	}
	return r.inventoryStore.AcquireTaskEnvironmentRecoveryClaim(ctx, request)
}

// resumeDurableBootstrap continues a launch that stopped between the bootstrap
// nonce and the persisted agentctl token. It returns an instance when agentctl
// accepted the repeated handshake and the environment was published ready.
// The cleanup claim and the recovery claim are one environment's fence, so a
// consumed nonce is cleaned up only after the recovery claim is released.
func (r *PluginRemoteExecutor) resumeDurableBootstrap(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) (*ExecutorInstance, error) {
	instance, consumed, err := r.resumeUnderRecoveryClaim(ctx, record, state)
	if err != nil {
		return nil, err
	}
	if consumed {
		return nil, r.destroyPluginExecutorRecord(ctx, record, state.inventory, state.resource, pluginExecutorCleanupReasonBootstrapConsumed, state.profile)
	}
	if instance == nil {
		return nil, nil
	}
	return r.honorCleanupRequestedAfterRecovery(ctx, record, state, instance)
}

func (r *PluginRemoteExecutor) resumeUnderRecoveryClaim(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) (*ExecutorInstance, bool, error) {
	claim, err := r.acquirePluginExecutorClaim(ctx, record.TaskID, record.SessionID, state.inventory, models.PluginClaimKindRecovery)
	if errors.Is(err, recoveryclaim.ErrBusy) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("plugin executor recovery is fenced: %w", err)
	}
	defer r.releasePluginExecutorCleanupClaim(claim)
	claimedCtx := recoveryclaim.WithClaim(ctx, claim)
	state.inventory.HandshakeStarted = true
	if err := r.checkpointPluginExecutorRecord(claimedCtx, record, state.inventory); err != nil {
		return nil, false, err
	}
	resolver := r.connectionResolver(state.operationContext, state.resource, pluginExecutorRuntimePort, state.snapshot)
	var control *agentctl.ControlClient
	token := record.TransientAuthToken
	if token != "" {
		control, err = r.newTokenControlClient(claimedCtx, resolver, r.logger, token)
	} else {
		control, token, err = r.newAgentctlControlClient(claimedCtx, resolver, r.logger, record.TransientBootstrapNonce)
	}
	if errors.Is(err, agentctl.ErrHandshakeRejected) {
		state.inventory.HandshakeRejected = true
		if err := r.checkpointPluginExecutorRecord(claimedCtx, record, state.inventory); err != nil {
			return nil, false, errors.New("plugin executor handshake rejection was not recorded; recovery stays retryable")
		}
		return nil, true, nil
	}
	if err != nil {
		return nil, false, errors.New("plugin executor bootstrap handshake is unavailable; recovery stays retryable")
	}
	defer control.Close()
	r.reachCrashBarrier(claimedCtx, BarrierAfterHandshake)
	if err := r.followRecoveryClaimCheckpoints(claimedCtx, record, state); err != nil {
		return nil, false, err
	}
	instance, err := r.adoptDurableBootstrap(claimedCtx, record, state, control, token)
	if err != nil {
		return nil, false, err
	}
	if err := r.publishRecoveredPluginExecutor(claimedCtx, claim, record, state); err != nil {
		instance.Client.Close()
		return nil, false, err
	}
	r.clearPublishPending(claimedCtx, record, state)
	return instance, false, nil
}

// followRecoveryClaimCheckpoints adopts the revision and claim state a
// provider's recovery_progress checkpoint left in the stored envelope while the
// host held the recovery claim. The claim holder is the only other writer, so
// following the stored envelope cannot overwrite a competing host write.
func (r *PluginRemoteExecutor) followRecoveryClaimCheckpoints(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) error {
	current, err := r.currentPluginExecutorRecord(ctx, record.SessionID, record.AgentExecutionID)
	if err != nil {
		return err
	}
	stored, err := decodePluginExecutorInventory(current.Metadata)
	if err != nil {
		return err
	}
	local, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return err
	}
	local.Revision = stored.Revision
	metadata := clonePluginExecutorMetadata(record.Metadata)
	metadata[MetadataKeyPluginExecutor] = local
	record.Metadata = metadata
	state.inventory.Revision, state.inventory.Claim = stored.Revision, stored.Claim
	return nil
}

// adoptDurableBootstrap stores the token, creates or adopts the session's
// agentctl instance, verifies it, and checkpoints ready.
func (r *PluginRemoteExecutor) adoptDurableBootstrap(
	ctx context.Context,
	record *models.ExecutorRunning,
	state *pluginExecutorRecoveryState,
	control *agentctl.ControlClient,
	token string,
) (*ExecutorInstance, error) {
	id, err := r.putPluginRuntimeSecret(ctx, record.AgentExecutionID, pluginRuntimeSecretKindAuth, token)
	if err != nil {
		return nil, err
	}
	metadata := clonePluginExecutorMetadata(record.Metadata)
	metadata[MetadataKeyAuthTokenSecret] = id
	record.Metadata = metadata
	record.TransientAuthToken = token
	state.inventory.recordCredential()
	if err := r.checkpointPluginExecutorRecord(ctx, record, state.inventory); err != nil {
		return nil, err
	}
	port, err := r.createOrAdoptPluginAgentctlInstance(ctx, control, record, state)
	if err != nil {
		return nil, err
	}
	state.inventory.InstancePort = port
	if err := r.followRecoveryClaimCheckpoints(ctx, record, state); err != nil {
		return nil, err
	}
	instance, err := r.newRecoveredPluginExecutorInstance(ctx, record, state)
	if err != nil {
		return nil, err
	}
	state.inventory.Phase = pluginExecutorPhaseReady
	state.inventory.PublishPending = true
	state.inventory.Generation++
	record.Metadata = clonePluginExecutorMetadata(record.Metadata)
	delete(record.Metadata, MetadataKeyBootstrapNonceSecret)
	if err := r.checkpointPluginExecutorRecord(ctx, record, state.inventory); err != nil {
		instance.Client.Close()
		return nil, err
	}
	instance.Metadata = pluginRecoveredMetadataWithoutNonce(record.Metadata, state.inventory)
	r.settlePluginExecutorNonce(ctx, record.AgentExecutionID)
	return instance, nil
}

// createOrAdoptPluginAgentctlInstance returns the port of the session's
// instance, creating it with the identity the record holds when agentctl has
// none. The recorded port, when present, must match.
func (r *PluginRemoteExecutor) createOrAdoptPluginAgentctlInstance(ctx context.Context, control *agentctl.ControlClient, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) (uint32, error) {
	existing, err := control.GetInstance(ctx, record.AgentExecutionID)
	port := uint32(0)
	switch {
	case err == nil && existing != nil:
		port = uint32(existing.Port)
	case err == nil || errors.Is(err, agentctl.ErrInstanceNotFound):
		created, createErr := control.CreateInstance(ctx, &agentctl.CreateInstanceRequest{
			ID: record.AgentExecutionID, WorkspacePath: state.inventory.workspacePath(),
			SessionID: record.SessionID, TaskID: record.TaskID,
		})
		if createErr != nil {
			return 0, errors.New("plugin executor agentctl instance could not be created")
		}
		port = uint32(created.Port)
	default:
		return 0, errors.New("plugin executor agentctl instance lookup failed")
	}
	if port == 0 || port > 65535 || (state.inventory.InstancePort != 0 && state.inventory.InstancePort != port) {
		return 0, errors.New("plugin executor agentctl instance port does not match the record")
	}
	return port, nil
}

func (r *PluginRemoteExecutor) settlePluginExecutorNonce(ctx context.Context, executionID string) {
	if r.runtimeSecrets == nil {
		return
	}
	if err := r.runtimeSecrets.Delete(context.WithoutCancel(ctx), kubernetesRuntimeSecretID(executionID, pluginRuntimeSecretKindBootstrap)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		r.logger.Warn("plugin executor bootstrap secret was not deleted", zap.String("execution_id", executionID))
	}
}

func pluginRecoveredMetadataWithoutNonce(metadata map[string]interface{}, inventory pluginExecutorInventory) map[string]interface{} {
	cloned := clonePluginExecutorMetadata(metadata)
	delete(cloned, MetadataKeyBootstrapNonceSecret)
	cloned[MetadataKeyPluginExecutor] = inventory
	return cloned
}

// publishRecoveredPluginExecutor flips the creating environment to ready under
// the held recovery claim. Re-reading the record after the ready checkpoint
// supplies the revision the publication is fenced on.
func (r *PluginRemoteExecutor) publishRecoveredPluginExecutor(ctx context.Context, claim *models.TaskEnvironmentRecoveryClaim, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) error {
	r.reachCrashBarrier(ctx, BarrierBeforePublish)
	publisher, ok := r.inventoryStore.(PluginExecutorReadyPublisher)
	if !ok {
		return errors.New("plugin executor ready publication is unavailable")
	}
	current, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return err
	}
	_, err = publisher.PublishRecoveredTaskEnvironment(ctx, models.PublishRecoveredTaskEnvironmentParams{
		Claim: claim, SessionID: record.SessionID, EnvironmentID: state.inventory.EnvironmentID,
		OperationID: state.inventory.OperationID, RuntimeIdentity: pluginExecutorRuntimeIdentity(state.inventory, record.AgentExecutionID),
		OwnershipGeneration: state.inventory.EnvironmentGeneration, ExpectedRevision: current.Revision,
		Repos: publishedPluginExecutorRepos(current),
	})
	return err
}

// PluginExecutorReadyPublisher is the repository's guarded creating-to-ready
// publication.
type PluginExecutorReadyPublisher interface {
	PublishRecoveredTaskEnvironment(context.Context, models.PublishRecoveredTaskEnvironmentParams) (bool, error)
}

// honorCleanupRequestedAfterRecovery runs the cleanup an archive, delete or
// reset asked for while recovery held the claim.
func (r *PluginRemoteExecutor) honorCleanupRequestedAfterRecovery(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState, instance *ExecutorInstance) (*ExecutorInstance, error) {
	current, err := r.currentPluginExecutorRecord(ctx, record.SessionID, record.AgentExecutionID)
	if err != nil {
		return instance, nil
	}
	inventory, err := decodePluginExecutorInventory(current.Metadata)
	if err != nil || !inventory.CleanupRequested {
		return instance, nil
	}
	instance.Client.Close()
	return nil, r.destroyPluginExecutorRecord(ctx, current, inventory, inventory.Resource, pluginExecutorCleanupReasonTask, state.profile)
}

// recordCleanupRequested persists that a cleanup lost the claim race, so the
// sweep runs it once the claim is released. The store records it without
// waiting on the claim the other holder owns.
func (r *PluginRemoteExecutor) recordCleanupRequested(ctx context.Context, record *models.ExecutorRunning) {
	recorder, ok := r.inventoryStore.(PluginExecutorCleanupRequester)
	if !ok {
		r.logger.Warn("plugin executor cleanup request cannot be recorded", zap.String("session_id", record.SessionID))
		return
	}
	if err := recorder.RecordPluginExecutorCleanupRequested(ctx, record.SessionID, record.AgentExecutionID); err != nil {
		r.logger.Warn("plugin executor cleanup request was not recorded", zap.String("session_id", record.SessionID), zap.Error(err))
	}
}

// PluginExecutorCleanupRequester is the repository's claim-independent write
// of the cleanup-requested marker.
type PluginExecutorCleanupRequester interface {
	RecordPluginExecutorCleanupRequested(ctx context.Context, sessionID, executionID string) error
}

// publishPendingReady completes a creating-to-ready publication that a crash
// interrupted after the ready checkpoint: the envelope still marks it pending.
// A holder of the recovery claim elsewhere publishes it instead.
func (r *PluginRemoteExecutor) publishPendingReady(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) error {
	claim, err := r.acquirePluginExecutorClaim(ctx, record.TaskID, record.SessionID, state.inventory, models.PluginClaimKindRecovery)
	if errors.Is(err, recoveryclaim.ErrBusy) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("plugin executor recovery is fenced: %w", err)
	}
	defer r.releasePluginExecutorCleanupClaim(claim)
	claimedCtx := recoveryclaim.WithClaim(ctx, claim)
	if err := r.publishRecoveredPluginExecutor(claimedCtx, claim, record, state); err != nil {
		return err
	}
	r.clearPublishPending(claimedCtx, record, state)
	return nil
}

// clearPublishPending records that the publication happened. A failed write
// only repeats the idempotent publication on the next recovery.
func (r *PluginRemoteExecutor) clearPublishPending(ctx context.Context, record *models.ExecutorRunning, state *pluginExecutorRecoveryState) {
	state.inventory.PublishPending = false
	if err := r.checkpointPluginExecutorRecord(ctx, record, state.inventory); err != nil {
		r.logger.Warn("plugin executor publication marker was not cleared")
	}
}
