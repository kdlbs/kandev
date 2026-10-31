package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"syscall"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"google.golang.org/protobuf/proto"
)

const (
	pluginExecutorCapabilityDrainReceipt = "drain_receipt"

	// The closed set of drain outcomes the destroy request carries.
	pluginDrainOutcomeDrained      = "drained"
	pluginDrainOutcomeUnreachable  = "unreachable"
	pluginDrainOutcomeUncertain    = "uncertain"
	pluginDrainOutcomeNotAttempted = "not_attempted"

	pluginExecutorDrainDeadline = 60 * time.Second
	// pluginExecutorCleanupClaimWait bounds how long a failed launch waits for a
	// concurrent cleanup to release the environment's cleanup claim.
	pluginExecutorCleanupClaimWait  = 20 * time.Second
	pluginExecutorCleanupClaimRetry = 250 * time.Millisecond
	// pluginExecutorPersistGrace is the time left for inventory writes after
	// the provider calls of a cleanup have used their deadlines.
	pluginExecutorPersistGrace = 30 * time.Second

	pluginDrainReceiptDigestDomain = "kandev.plugin.drain-receipt.v1"

	// pluginDrainReceiptNeverHeldCredential is the first drain_receipt version
	// whose not_attempted means the host never held an agentctl credential.
	pluginDrainReceiptNeverHeldCredential = 2
)

// pluginRuntimeDrainer asks the exact recorded agentctl to seal and drain
// under one cleanup claim. It returns a receipt only when every managed
// process group stopped.
type pluginRuntimeDrainer func(
	ctx context.Context,
	resolver agentctl.ConnectionLeaseResolver,
	log *logger.Logger,
	token, claimOperationID string,
	deadline time.Duration,
) (*agentctl.RuntimeDrainReceipt, error)

// defaultPluginRuntimeDrainer reads the runtime's identity and binds the seal
// to it, so a request can never seal a different process than the one whose
// credential the host holds.
func defaultPluginRuntimeDrainer(
	ctx context.Context,
	resolver agentctl.ConnectionLeaseResolver,
	log *logger.Logger,
	token, claimOperationID string,
	deadline time.Duration,
) (*agentctl.RuntimeDrainReceipt, error) {
	control, err := agentctl.NewEndpointControlClient(ctx, resolver, log, agentctl.WithControlAuthToken(token))
	if err != nil {
		return nil, err
	}
	defer control.Close()
	identity, err := control.GetIdentity(ctx)
	if err != nil {
		return nil, err
	}
	return control.SealAndDrainRuntime(ctx, agentctl.RuntimeDrainRequest{
		RequestID:              claimOperationID,
		ExpectedServerIdentity: identity.ServerIdentity,
		DeadlineMS:             deadline.Milliseconds(),
	})
}

// pluginExecutorDestroyTimeout is the provider's destroy deadline.
func pluginExecutorDestroyTimeout(provider models.ExecutorProvider) time.Duration {
	if provider.DestroyTimeoutSeconds > 0 {
		return time.Duration(provider.DestroyTimeoutSeconds) * time.Second
	}
	return 30 * time.Second
}

// pluginDrainReceiptID binds a receipt to the environment, generation, runtime
// identity and cleanup claim it was issued for, so a receipt of another claim
// can never be mistaken for this one.
func pluginDrainReceiptID(inventory pluginExecutorInventory, claimOperationID string) string {
	sum := sha256.New()
	for _, part := range []string{
		pluginDrainReceiptDigestDomain, inventory.EnvironmentID,
		strconv.FormatInt(inventory.EnvironmentGeneration, 10), inventory.RuntimeIdentity, claimOperationID,
	} {
		sum.Write([]byte(part))
		sum.Write([]byte{0})
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// pluginDrainDecision is what a destroy request carries about the drain.
type pluginDrainDecision struct {
	outcome string
	receipt *pluginsdk.ExecutorDrainReceipt
}

func (d pluginDrainDecision) applyTo(req *pluginsdk.DestroyExecutorEnvironmentRequest) {
	req.DrainOutcome = d.outcome
	req.DrainReceipt = d.receipt
}

// drainBeforePluginDestroy runs, for a provider that requires drain_receipt,
// the seal-and-drain of the recorded runtime and persists its outcome in the
// envelope before the destroy call. An outcome already recorded under this same
// claim is reused; an outcome recorded under any other claim is replaced. The
// returned record and inventory carry the persisted revision.
func (r *PluginRemoteExecutor) drainBeforePluginDestroy(
	ctx context.Context,
	record *models.ExecutorRunning,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	provider models.ExecutorProvider,
	snapshot *pluginsdk.ExecutorProfileSnapshot,
	claim *models.TaskEnvironmentRecoveryClaim,
	reason string,
) (*models.ExecutorRunning, pluginExecutorInventory, pluginDrainDecision, error) {
	if !negotiatedDeclares(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDrainReceipt) {
		return record, inventory, pluginDrainDecision{}, nil
	}
	if decision, ok := storedPluginDrainDecision(inventory, claim.OperationID); ok {
		return record, inventory, decision, nil
	}
	operationContext := pluginExecutorRequestContextForRecord(record, inventory, models.ExecutorProviderLaunchProfile{
		Provider:            models.ExecutorProvider{PluginID: inventory.PluginID, InstallationID: inventory.InstallationID, Key: inventory.ProviderKey, ContractVersion: inventory.ContractVersion},
		OwnershipGeneration: inventory.EnvironmentGeneration,
	})
	decision := r.drainPluginRuntime(ctx, record.TransientAuthToken, operationContext, inventory, resource, snapshot, claim.OperationID, reason)
	inventory.DrainClaim = claim.OperationID
	inventory.DrainOutcome = decision.outcome
	inventory.DrainReceiptID = ""
	if decision.receipt != nil {
		inventory.DrainReceiptID = decision.receipt.GetReceiptId()
	}
	if err := checkPluginExecutorEnvelopeBudget(inventory, pluginExecutorStateBudget(provider)); err != nil {
		return record, inventory, decision, fmt.Errorf("plugin executor drain outcome cannot be recorded: %w", err)
	}
	if err := r.checkpointPluginExecutorRecord(ctx, record, inventory); err != nil {
		return record, inventory, decision, fmt.Errorf("plugin executor drain outcome was not persisted: %w", err)
	}
	persisted, err := decodePluginExecutorInventory(record.Metadata)
	if err != nil {
		return record, inventory, decision, err
	}
	r.reachCrashBarrier(ctx, BarrierAfterDrain)
	return record, persisted, decision, nil
}

// storedPluginDrainDecision returns the receipt recorded for this claim. Only
// a drained outcome whose receipt id matches the one this claim would have
// produced is reused; any other outcome is drained again, which a sealed
// runtime answers idempotently.
func storedPluginDrainDecision(inventory pluginExecutorInventory, claimOperationID string) (pluginDrainDecision, bool) {
	if inventory.DrainClaim != claimOperationID || inventory.DrainOutcome != pluginDrainOutcomeDrained {
		return pluginDrainDecision{}, false
	}
	if inventory.DrainReceiptID != pluginDrainReceiptID(inventory, claimOperationID) {
		return pluginDrainDecision{}, false
	}
	return pluginDrainDecision{outcome: pluginDrainOutcomeDrained, receipt: &pluginsdk.ExecutorDrainReceipt{
		ReceiptId: inventory.DrainReceiptID, CleanupClaim: claimOperationID, RuntimeIdentity: inventory.RuntimeIdentity,
	}}, true
}

// drainPluginRuntime seals and drains the recorded runtime and returns the
// drain evidence a destroy carries. The outcome follows one table:
//
//	credential known, drain answered     -> drained (with a receipt)
//	credential known, drain not answered -> unreachable or uncertain
//	no credential, drain_receipt@1:
//	  no handshake ever, launch_failed   -> not_attempted
//	  otherwise                          -> unreachable
//	no credential, drain_receipt@2:
//	  the host never held a credential   -> not_attempted, whatever the reason
//	  otherwise                          -> unreachable
//
// Under @1 not_attempted is the provider's proof that no runtime ever existed.
// Under @2 it states only that the host never held an agentctl credential, so
// no instance, agent or shell can exist through agentctl; the provider verifies
// the rest. The version is the one stored with the record.
func (r *PluginRemoteExecutor) drainPluginRuntime(
	ctx context.Context,
	token string,
	operationContext *pluginsdk.ExecutorProviderRequestContext,
	inventory pluginExecutorInventory,
	resource *pluginsdk.ExecutorResourceDescriptor,
	snapshot *pluginsdk.ExecutorProfileSnapshot,
	claimOperationID string,
	reason string,
) pluginDrainDecision {
	if token == "" || r.drainRuntime == nil {
		return pluginDrainDecision{outcome: pluginNoCredentialDrainOutcome(inventory, token, reason)}
	}
	operationContext = proto.Clone(operationContext).(*pluginsdk.ExecutorProviderRequestContext)
	drainCtx, cancel := context.WithTimeout(ctx, pluginExecutorDrainDeadline)
	defer cancel()
	operationContext.Deadline = time.Now().Add(pluginExecutorDrainDeadline).UTC().Format(time.RFC3339Nano)
	receipt, err := r.drainRuntime(
		drainCtx, r.connectionResolver(operationContext, resource, pluginExecutorRuntimePort, snapshot), r.logger,
		token, claimOperationID, pluginExecutorDrainDeadline,
	)
	if err != nil || receipt == nil {
		return pluginDrainDecision{outcome: classifyPluginDrainFailure(err)}
	}
	return pluginDrainDecision{outcome: pluginDrainOutcomeDrained, receipt: &pluginsdk.ExecutorDrainReceipt{
		ReceiptId: pluginDrainReceiptID(inventory, claimOperationID), CleanupClaim: claimOperationID,
		RuntimeIdentity: inventory.RuntimeIdentity, CompletedAt: receipt.CompletedAt.UTC().Format(time.RFC3339Nano),
	}}
}

// pluginNoCredentialDrainOutcome is the outcome of a destroy for which no drain
// could be attempted.
func pluginNoCredentialDrainOutcome(inventory pluginExecutorInventory, token, reason string) string {
	if token == "" && pluginDrainNeverHeldCredential(inventory) {
		return pluginDrainOutcomeNotAttempted
	}
	if reason == pluginExecutorCleanupReasonLaunch && !inventory.HandshakeStarted {
		return pluginDrainOutcomeNotAttempted
	}
	return pluginDrainOutcomeUnreachable
}

// pluginDrainNeverHeldCredential reports whether the record proves the host
// never held an agentctl credential. It requires a recorded drain_receipt
// version that defines not_attempted that way, and no sign of a credential:
//
//	no handshake was ever started                       -> true
//	a handshake started, durable_bootstrap negotiated,
//	  credential_recorded explicitly false, none rejected -> true
//	anything else (rejected handshake, a recorded credential, a provider
//	  without durable_bootstrap, a record that does not carry
//	  credential_recorded)                              -> false
//
// The durable_bootstrap requirement is the host's ordering guarantee that the
// token reference is checkpointed before an instance is created with the token.
func pluginDrainNeverHeldCredential(inventory pluginExecutorInventory) bool {
	if negotiatedVersion(inventory.NegotiatedCapabilities, pluginExecutorCapabilityDrainReceipt) < pluginDrainReceiptNeverHeldCredential ||
		inventory.HandshakeRejected || inventory.credentialRecorded() {
		return false
	}
	if !inventory.HandshakeStarted {
		return true
	}
	return pluginExecutorDurableBootstrap(inventory) && inventory.CredentialRecorded != nil
}

// classifyPluginDrainFailure maps a failed drain to the outcome it proves. Only
// a refused connection or a runtime that no longer answers to its identity is
// `unreachable`; a lost reply, a deadline, or anything ambiguous is `uncertain`.
func classifyPluginDrainFailure(err error) string {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return pluginDrainOutcomeUnreachable
	}
	var rejected *agentctl.RuntimeDrainRejectedError
	if errors.As(err, &rejected) {
		if rejected.StatusCode == http.StatusPreconditionFailed {
			return pluginDrainOutcomeUnreachable
		}
	}
	return pluginDrainOutcomeUncertain
}
