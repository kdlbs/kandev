package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/plugins/state"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type exactTaskManagementClaimWriter interface {
	ChangeTaskManagementClaim(context.Context, string, taskmodels.TaskManagementClaimChange) (*taskmodels.TaskManagementClaim, error)
}

func (h *pluginHost) TaskManagementClaims() pluginsdk.ExactTaskManagementClaimCommandManager {
	return exactTaskManagementClaimCommandAdapter{host: h}
}

type exactTaskManagementClaimCommandAdapter struct{ host *pluginHost }

func (a exactTaskManagementClaimCommandAdapter) Acquire(ctx context.Context, input pluginsdk.ExactTaskManagementClaimAcquire) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	return a.host.AcquireTaskManagementClaimExact(ctx, input)
}

func (a exactTaskManagementClaimCommandAdapter) Release(ctx context.Context, input pluginsdk.ExactTaskManagementClaimRelease) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	return a.host.ReleaseTaskManagementClaimExact(ctx, input)
}

func (a exactTaskManagementClaimCommandAdapter) Transfer(ctx context.Context, input pluginsdk.ExactTaskManagementClaimTransfer) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	return a.host.TransferTaskManagementClaimExact(ctx, input)
}

func validManagementClaimFence(instanceKey string, generation int64) bool {
	if generation < 0 {
		return false
	}
	if generation == 0 {
		return instanceKey == ""
	}
	return instanceKey == "" || isBoundedApprovalIdentifier(instanceKey)
}

func claimToSDK(value *taskmodels.TaskManagementClaim) *pluginsdk.TaskManagementClaim {
	if value == nil {
		return nil
	}
	claim := &pluginsdk.TaskManagementClaim{
		TaskID: value.TaskID, WorkspaceID: value.WorkspaceID, InstallationID: value.InstallationID,
		OwnerKind: value.OwnerKind, OwnerActorID: value.OwnerActorID,
		InstanceKey: value.InstanceKey, Generation: value.Generation, ResourceVersion: value.ResourceVersion,
		UpdatedAt: value.UpdatedAt.UTC().Format(time.RFC3339Nano), UpdatedByActor: value.UpdatedByActor,
	}
	if value.AcquiredAt != nil {
		claim.AcquiredAt = value.AcquiredAt.UTC().Format(time.RFC3339Nano)
	}
	return claim
}

func (h *pluginHost) AcquireTaskManagementClaimExact(ctx context.Context, input pluginsdk.ExactTaskManagementClaimAcquire) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	return h.changeTaskManagementClaimExact(ctx, input.ExactTaskManagementClaimCommand, taskmodels.TaskManagementClaimAcquire, input.InstanceKey, "", "")
}

func (h *pluginHost) ReleaseTaskManagementClaimExact(ctx context.Context, input pluginsdk.ExactTaskManagementClaimRelease) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	return h.changeTaskManagementClaimExact(ctx, input.ExactTaskManagementClaimCommand, taskmodels.TaskManagementClaimRelease, input.InstanceKey, "", "")
}

func (h *pluginHost) TransferTaskManagementClaimExact(ctx context.Context, input pluginsdk.ExactTaskManagementClaimTransfer) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	return h.changeTaskManagementClaimExact(ctx, input.ExactTaskManagementClaimCommand, taskmodels.TaskManagementClaimTransfer, input.InstanceKey, input.TargetInstallationID, input.TargetInstanceKey)
}

//nolint:cyclop,funlen // The claim command binds caller, observed claim generation, approval, and idempotency.
func (h *pluginHost) changeTaskManagementClaimExact(
	ctx context.Context,
	input pluginsdk.ExactTaskManagementClaimCommand,
	action, instanceKey, targetInstallationID, targetInstanceKey string,
) (*pluginsdk.CommandResult, *pluginsdk.TaskManagementClaim, error) {
	if !validExactManagementClaimCommand(input, instanceKey, action, targetInstallationID, targetInstanceKey) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	payloadDigest, err := taskManagementClaimDigest(input, action, instanceKey, targetInstallationID, targetInstanceKey)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil
	}
	if h.service == nil || h.installationID == "" {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}, nil, nil
	}
	h.service.approvalEffectMu.Lock()
	defer h.service.approvalEffectMu.Unlock()
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}, nil, nil
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if input.ManifestDigest != manifestDigest {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}, nil, nil
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, input.WorkspaceID, "host.v2.write:tasks", input.ApprovalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", "TaskManagementClaim."+action, "v2"),
	)
	if !decision.Allowed {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}, nil, nil
	}
	writer, ok := h.taskWriter.(exactTaskManagementClaimWriter)
	if !ok {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "task_management_claims_unsupported"}, nil, nil
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}, nil, nil
	}
	record, replayed, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: "TaskManagementClaim." + action, CapabilityID: "host.v2.write:tasks",
		PayloadDigest: payloadDigest, TargetID: input.TaskID,
		ExpectedResourceVersion: input.ExpectedTaskResourceVersion + "/" + input.ExpectedClaimResourceVersion,
		ApprovalRevision:        input.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}, nil, nil
	}
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}, nil, nil
	}
	if replayed && record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(record), nil, nil
	}
	if action == taskmodels.TaskManagementClaimAcquire {
		// Acquire establishes ownership for the calling installation and
		// instance. These are the owner fields, while target fields are only
		// supplied by transfer requests.
		targetInstallationID = h.installationID
		targetInstanceKey = instanceKey
	}
	claim, err := writer.ChangeTaskManagementClaim(ctx, input.TaskID, taskmodels.TaskManagementClaimChange{
		Action: action, TaskID: input.TaskID, WorkspaceID: input.WorkspaceID,
		ExpectedTaskResourceVersion:  input.ExpectedTaskResourceVersion,
		ExpectedClaimResourceVersion: input.ExpectedClaimResourceVersion,
		InstallationID:               targetInstallationID, InstanceKey: targetInstanceKey,
		ActorID: "plugin:" + h.installationID, ActorInstallationID: h.installationID,
		ActorInstanceKey: instanceKey, Reason: input.Reason,
	})
	if err != nil {
		return h.completeTaskManagementClaimError(ctx, store, record, input.TaskID, err), nil, nil
	}
	sdkClaim := claimToSDK(claim)
	version := ""
	if sdkClaim != nil {
		version = sdkClaim.ResourceVersion
	}
	completed, err := store.Complete(ctx, record.Intent.OperationID, string(pluginsdk.CommandApplied), "", input.TaskID, version)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, sdkClaim, nil
	}
	return commandResultFromRecord(completed), sdkClaim, nil
}

func validExactManagementClaimCommand(input pluginsdk.ExactTaskManagementClaimCommand, instanceKey, action, targetInstallationID, targetInstanceKey string) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.TaskID) || !isBoundedApprovalIdentifier(input.IdempotencyKey) ||
		!isBoundedApprovalIdentifier(instanceKey) || input.ApprovalRevision == 0 || len(input.ManifestDigest) != 64 ||
		len(input.ExpectedClaimResourceVersion) > 128 || len(input.Reason) > 500 {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, input.ExpectedTaskResourceVersion); err != nil {
		return false
	}
	if action == taskmodels.TaskManagementClaimTransfer {
		return isBoundedApprovalIdentifier(targetInstallationID) && isBoundedApprovalIdentifier(targetInstanceKey)
	}
	return targetInstallationID == "" && targetInstanceKey == ""
}

func taskManagementClaimDigest(input pluginsdk.ExactTaskManagementClaimCommand, action, instanceKey, targetInstallationID, targetInstanceKey string) (string, error) {
	payload := struct {
		Action                       string `json:"action"`
		WorkspaceID                  string `json:"workspace_id"`
		TaskID                       string `json:"task_id"`
		InstanceKey                  string `json:"instance_key"`
		ExpectedTaskResourceVersion  string `json:"expected_task_resource_version"`
		ExpectedClaimResourceVersion string `json:"expected_claim_resource_version"`
		TargetInstallationID         string `json:"target_installation_id"`
		TargetInstanceKey            string `json:"target_instance_key"`
		Reason                       string `json:"reason"`
	}{action, input.WorkspaceID, input.TaskID, instanceKey, input.ExpectedTaskResourceVersion,
		input.ExpectedClaimResourceVersion, targetInstallationID, targetInstanceKey, input.Reason}
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > 65536 {
		return "", fmt.Errorf("invalid task management claim payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (h *pluginHost) completeTaskManagementClaimError(ctx context.Context, store *state.CommandStore, record state.CommandRecord, taskID string, err error) *pluginsdk.CommandResult {
	status := pluginsdk.CommandUnavailable
	switch {
	case errors.Is(err, repoerrors.ErrTaskManagementClaimConflict), errors.Is(err, repoerrors.ErrTaskManagementClaimOwned):
		status = pluginsdk.CommandConflict
	case errors.Is(err, repoerrors.ErrTaskNotFound):
		status = pluginsdk.CommandNotFound
	case errors.Is(err, repoerrors.ErrTaskVersionConflict):
		status = pluginsdk.CommandConflict
	default:
		return &pluginsdk.CommandResult{Status: status, Reason: "task_management_claim_unavailable"}
	}
	completed, completeErr := store.Complete(ctx, record.Intent.OperationID, string(status), string(status), taskID, "")
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}
	}
	return commandResultFromRecord(completed)
}
