package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

const (
	pluginExecutorCapabilityDestroyCheckpoints = "destroy_checkpoints"
	pluginExecutorClaimSuffixSeparator         = ":"
)

// pluginExecutorClaimState is the claim record the repository keeps in the
// envelope: the claim that last checkpointed and the highest sequence and
// digest accepted for it.
type pluginExecutorClaimState struct {
	Kind        string `json:"kind"`
	OperationID string `json:"operation_id"`
	AcquiredAt  string `json:"acquired_at"`
	Sequence    uint64 `json:"sequence"`
	Digest      string `json:"digest"`
}

// PluginExecutorClaimStore is the repository surface the claim-scoped
// checkpoint path needs.
type PluginExecutorClaimStore interface {
	GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	GetTaskEnvironmentRecoveryClaim(context.Context, string) (*models.TaskEnvironmentRecoveryClaim, error)
	CheckpointPluginExecutorClaimInventory(context.Context, *models.ExecutorRunning, models.PluginExecutorClaimCheckpoint) (bool, error)
}

// SetPluginExecutorClaimStore wires the store behind cleanup and recovery
// checkpoints. Without it those checkpoints fail closed.
func (m *Manager) SetPluginExecutorClaimStore(store PluginExecutorClaimStore) {
	m.pluginExecutorClaimStore = store
}

// pluginExecutorClaimPrefix is the part of a claim id shared by every
// acquisition of one kind for an operation. The kind sits in the prefix, so a
// checkpoint's claim kind is read from the claim row.
func pluginExecutorClaimPrefix(operationID, kind string) string {
	return operationID + pluginExecutorClaimSuffixSeparator + kind + pluginExecutorClaimSuffixSeparator
}

// newPluginExecutorClaimOperationID names one acquisition: the process epoch
// marks a live holder, and the nonce makes the id unique, so a callback or
// drain of an earlier acquisition can never be mistaken for this one.
func (r *PluginRemoteExecutor) newPluginExecutorClaimOperationID(operationID, kind string) string {
	return pluginExecutorClaimPrefix(operationID, kind) + r.claimEpoch + pluginExecutorClaimSuffixSeparator + uuid.NewString()
}

func pluginExecutorClaimKind(operationID string, claim *models.TaskEnvironmentRecoveryClaim) string {
	for _, kind := range []string{models.PluginClaimKindCleanup, models.PluginClaimKindRecovery} {
		if strings.HasPrefix(claim.OperationID, pluginExecutorClaimPrefix(operationID, kind)) {
			return kind
		}
	}
	return ""
}

// pluginExecutorClaimAcquiredAt is the acquisition stamp the repository keeps
// in the envelope's claim state.
func pluginExecutorClaimAcquiredAt(claim *models.TaskEnvironmentRecoveryClaim) string {
	return claim.CreatedAt.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)
}

func pluginExecutorClaimPhaseKind(phase string) string {
	switch phase {
	case pluginExecutorPhaseCleanupProgress:
		return models.PluginClaimKindCleanup
	case pluginExecutorPhaseRecoveryProgress:
		return models.PluginClaimKindRecovery
	default:
		return ""
	}
}

// pluginExecutorClaimDigest identifies one checkpoint payload so a replay of
// the same sequence can be told from a different payload at that sequence.
func pluginExecutorClaimDigest(phase string, resource *pluginsdk.ExecutorResourceDescriptor) (string, error) {
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(resource)
	if err != nil {
		return "", fmt.Errorf("%w: checkpoint resource cannot be encoded", models.ErrExecutorInvalidConfig)
	}
	sum := sha256.Sum256(append([]byte(phase+"\x00"), encoded...))
	return hex.EncodeToString(sum[:]), nil
}

// checkpointPluginExecutorClaim accepts a provider checkpoint issued under the
// active cleanup or recovery claim of its environment. Refusal order: shape
// (invalid_config), claim identity (not_found), then sequence and revision in
// the repository transaction.
func (m *Manager) checkpointPluginExecutorClaim(ctx context.Context, request *pluginsdk.CheckpointExecutorResourceRequest, phase string) error {
	return m.acceptPluginExecutorClaimCheckpoint(ctx, request, phase)
}

// logPluginExecutorCheckpointRefusal records why a provider checkpoint was
// refused, with the claim the host holds for the environment and the claim the
// provider echoed. Claim ids are not secret.
func (m *Manager) logPluginExecutorCheckpointRefusal(ctx context.Context, request *pluginsdk.CheckpointExecutorResourceRequest, phase string, cause error) {
	expected := ""
	if store := m.pluginExecutorClaimStore; store != nil {
		if claim, err := store.GetTaskEnvironmentRecoveryClaim(ctx, request.GetContext().GetEnvironmentId()); err == nil && claim != nil {
			expected = claim.OperationID
		}
	}
	m.logger.Warn("plugin executor checkpoint refused",
		zap.String("phase", phase),
		zap.Uint64("sequence", request.GetSequence()),
		zap.String("environment_id", request.GetContext().GetEnvironmentId()),
		zap.String("execution_id", request.GetContext().GetExecutionId()),
		zap.String("expected_claim", expected),
		zap.String("received_claim", request.GetClaim()),
		zap.Error(cause))
}

func (m *Manager) acceptPluginExecutorClaimCheckpoint(ctx context.Context, request *pluginsdk.CheckpointExecutorResourceRequest, phase string) error {
	if err := validatePluginExecutorCallbackLease(request.GetContext()); err != nil || request.GetSequence() < 1 || request.GetResource() == nil {
		return fmt.Errorf("%w: a claim checkpoint needs a lease, a resource and a sequence of at least 1", models.ErrExecutorInvalidConfig)
	}
	store := m.pluginExecutorClaimStore
	if store == nil || m.pluginExecutorProfileLoader == nil {
		return errors.New("plugin executor claim checkpoints are unavailable")
	}
	record, inventory, claim, err := m.activePluginExecutorClaim(ctx, store, request.GetContext(), phase, request.GetClaim())
	if err != nil {
		return err
	}
	provider, secrets, err := m.pluginExecutorClaimProvider(ctx, record, inventory)
	if err != nil {
		return err
	}
	if err := validatePluginExecutorResourceForProvider(provider, request.GetResource()); err != nil {
		return err
	}
	if err := rejectSecretInProviderState(secrets, request.GetResource()); err != nil {
		return err
	}
	next, err := nextPluginExecutorClaimInventory(inventory, provider, request.GetResource(), phase)
	if err != nil {
		return err
	}
	digest, err := pluginExecutorClaimDigest(phase, request.GetResource())
	if err != nil {
		return err
	}
	updated := *record
	updated.Metadata = map[string]interface{}{MetadataKeyPluginExecutor: next}
	updated.Runtime = agentruntime.RuntimePluginRemote
	updated.ExpectedPluginExecutorRevision = inventory.Revision
	_, err = store.CheckpointPluginExecutorClaimInventory(ctx, &updated, models.PluginExecutorClaimCheckpoint{
		Claim: claim, Kind: pluginExecutorClaimPhaseKind(phase), Sequence: request.GetSequence(), Digest: digest,
	})
	return err
}

// activePluginExecutorClaim resolves the record, envelope and active claim a
// checkpoint names, and refuses a checkpoint whose identity, capability or
// claim kind does not match.
func (m *Manager) activePluginExecutorClaim(
	ctx context.Context,
	store PluginExecutorClaimStore,
	callback *pluginsdk.ExecutorProviderRequestContext,
	phase, claimEcho string,
) (*models.ExecutorRunning, pluginExecutorInventory, *models.TaskEnvironmentRecoveryClaim, error) {
	notFound := fmt.Errorf("%w: no active claim matches this checkpoint", models.ErrExecutorClaimNotFound)
	record, err := store.GetExecutorRunningBySessionID(ctx, callback.GetSessionId())
	if err != nil || record == nil || record.Runtime != agentruntime.RuntimePluginRemote || record.AgentExecutionID != callback.GetExecutionId() {
		return nil, pluginExecutorInventory{}, nil, notFound
	}
	inventory, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil || !pluginExecutorClaimCallbackMatches(record, inventory, callback) {
		return nil, pluginExecutorInventory{}, nil, notFound
	}
	if !negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDestroyCheckpoints) {
		return nil, pluginExecutorInventory{}, nil, fmt.Errorf("%w: the provider did not declare destroy_checkpoints", models.ErrExecutorConflict)
	}
	claim, err := store.GetTaskEnvironmentRecoveryClaim(ctx, inventory.EnvironmentID)
	if err != nil || !pluginExecutorClaimBelongsTo(claim, record, inventory) {
		return nil, pluginExecutorInventory{}, nil, notFound
	}
	kind := pluginExecutorClaimKind(inventory.OperationID, claim)
	if kind == "" || callback == nil || claimEcho == "" || claimEcho != claim.OperationID {
		return nil, pluginExecutorInventory{}, nil, notFound
	}
	if kind != pluginExecutorClaimPhaseKind(phase) {
		return nil, pluginExecutorInventory{}, nil, fmt.Errorf("%w: a %s claim does not accept the %s phase", models.ErrExecutorInvalidConfig, kind, phase)
	}
	return record, inventory, claim, nil
}

func pluginExecutorClaimBelongsTo(claim *models.TaskEnvironmentRecoveryClaim, record *models.ExecutorRunning, inventory pluginExecutorInventory) bool {
	return claim != nil && claim.SessionID == record.SessionID && claim.OwnerTaskID == record.TaskID &&
		claim.OwnershipGeneration == inventory.EnvironmentGeneration
}

func pluginExecutorClaimCallbackMatches(record *models.ExecutorRunning, inventory pluginExecutorInventory, callback *pluginsdk.ExecutorProviderRequestContext) bool {
	return inventory.EnvironmentID == callback.GetEnvironmentId() && record.TaskID == callback.GetTaskId() &&
		inventory.OperationID == callback.GetOperationId() && inventory.PluginID == callback.GetPluginId() &&
		inventory.InstallationID == callback.GetInstallationId() && inventory.ProviderKey == callback.GetProviderKey() &&
		int32(inventory.ContractVersion) == callback.GetContractVersion() &&
		uint64(inventory.EnvironmentGeneration) == callback.GetEnvironmentGeneration()
}

func (m *Manager) pluginExecutorClaimProvider(ctx context.Context, record *models.ExecutorRunning, inventory pluginExecutorInventory) (models.ExecutorProvider, map[string]string, error) {
	profile, err := m.pluginExecutorProfileLoader.ExecutorProviderProfileForRecovery(
		ctx, inventory.ProfileID, record.TaskID, inventory.EnvironmentID, inventory.ProviderIdentity,
		inventory.EnvironmentGeneration, inventory.ProfileConfig, inventory.SecretReferences,
	)
	if err != nil || profile == nil || profile.Provider.Identity != inventory.ProviderIdentity || profile.Provider.InstallationID != inventory.InstallationID {
		return models.ExecutorProvider{}, nil, errors.New("recorded plugin executor provider is unavailable for this checkpoint")
	}
	return profile.Provider, profile.SecretValues, nil
}

func nextPluginExecutorClaimInventory(
	inventory pluginExecutorInventory,
	provider models.ExecutorProvider,
	resource *pluginsdk.ExecutorResourceDescriptor,
	phase string,
) (pluginExecutorInventory, error) {
	descriptor, err := resolvePluginExecutorEarlyWorkspace(inventory.NegotiatedCapabilities, inventory.WorkspaceDescriptor, resource)
	if err != nil {
		return pluginExecutorInventory{}, err
	}
	expiresAt, err := pluginExecutorEarlierExpiry(inventory.ExpiresAt, resource.GetExpiresAt())
	if err != nil {
		return pluginExecutorInventory{}, err
	}
	next := inventory
	next.Phase, next.Resource, next.WorkspaceDescriptor, next.ExpiresAt = phase, resource, descriptor, expiresAt
	next.StateVersion, next.Platform = resource.GetStateVersion(), resource.GetPlatform()
	next.Capabilities = intersectPluginExecutorCapabilities(provider.Capabilities, resource.GetCapabilities(), resource.GetRetention())
	if err := checkPluginExecutorEnvelopeBudget(next, pluginExecutorStateBudget(provider)); err != nil {
		return pluginExecutorInventory{}, err
	}
	return next, nil
}

func isPluginExecutorClaimPhase(phase string) bool {
	return strings.TrimSpace(phase) == pluginExecutorPhaseCleanupProgress || strings.TrimSpace(phase) == pluginExecutorPhaseRecoveryProgress
}
