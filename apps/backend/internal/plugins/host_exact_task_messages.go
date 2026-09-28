package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/plugins/state"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exactTaskMessageAdmission struct {
	writer exactTaskMessageMessenger
	store  *state.CommandStore
	record state.CommandRecord
	unlock func()
}

func validExactTaskMessage(input pluginsdk.ExactTaskMessage) bool {
	if !isBoundedApprovalIdentifier(input.RequestID) || !isBoundedApprovalIdentifier(input.WorkspaceID) ||
		!isBoundedApprovalIdentifier(input.TaskID) || !isBoundedApprovalIdentifier(input.SessionID) ||
		!isBoundedApprovalIdentifier(input.IdempotencyKey) || input.ApprovalRevision == 0 || len(input.ManifestDigest) != 64 ||
		!validManagementClaimFence(input.ManagementInstanceKey, input.ExpectedClaimGeneration) ||
		!utf8.ValidString(input.Content) || len(input.Content) == 0 || len(input.Content) > 32768 || strings.TrimSpace(input.Content) == "" {
		return false
	}
	_, taskErr := time.Parse(time.RFC3339Nano, input.ExpectedTaskResourceVersion)
	_, sessionErr := time.Parse(time.RFC3339Nano, input.ExpectedSessionResourceVersion)
	return taskErr == nil && sessionErr == nil
}

func exactTaskMessageDigest(input pluginsdk.ExactTaskMessage) (string, error) {
	canonical := struct {
		WorkspaceID             string `json:"workspace_id"`
		TaskID                  string `json:"task_id"`
		SessionID               string `json:"session_id"`
		TaskVersion             string `json:"task_version"`
		SessionVersion          string `json:"session_version"`
		Content                 string `json:"content"`
		ManagementInstanceKey   string `json:"management_instance_key"`
		ExpectedClaimGeneration int64  `json:"expected_claim_generation"`
	}{input.WorkspaceID, input.TaskID, input.SessionID, input.ExpectedTaskResourceVersion,
		input.ExpectedSessionResourceVersion, input.Content,
		input.ManagementInstanceKey, input.ExpectedClaimGeneration}
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 65536 {
		return "", fmt.Errorf("invalid exact task message payload")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (h *pluginHost) sendTaskMessageExact(ctx context.Context, input pluginsdk.ExactTaskMessage) (*pluginsdk.CommandResult, error) {
	if !validExactTaskMessage(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	digest, err := exactTaskMessageDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	admission, result := h.admitExactTaskMessage(ctx, input, digest)
	if result != nil {
		return result, nil
	}
	defer admission.unlock()
	if admission.record.Receipt.State == exactReceiptStateCompleted {
		return commandResultFromRecord(admission.record), nil
	}
	queueID, alreadyApplied, err := admission.writer.SendMessageExact(ctx, ExactTaskMessageInput{
		WorkspaceID: input.WorkspaceID, TaskID: input.TaskID, SessionID: input.SessionID,
		ExpectedTaskResourceVersion:    input.ExpectedTaskResourceVersion,
		ExpectedSessionResourceVersion: input.ExpectedSessionResourceVersion,
		Content:                        input.Content, Source: h.pluginSource(), OperationID: admission.record.Intent.OperationID,
		PayloadDigest: digest,
		ClaimFence: taskmodels.TaskManagementClaimFence{
			InstallationID: h.installationID, InstanceKey: input.ManagementInstanceKey,
			Generation: input.ExpectedClaimGeneration,
		},
	})
	if err != nil {
		resultStatus := exactTaskMessageErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "task_message_unavailable"}, nil
		}
		completed, completeErr := admission.store.Complete(ctx, admission.record.Intent.OperationID,
			string(resultStatus), string(resultStatus), input.TaskID, input.ExpectedSessionResourceVersion)
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
		}
		return commandResultFromRecord(completed), nil
	}
	if queueID == "" {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "task_message_receipt_unavailable"}, nil
	}
	resultStatus := pluginsdk.CommandApplied
	if alreadyApplied {
		resultStatus = pluginsdk.CommandAlreadyApplied
	}
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID,
		string(resultStatus), "", queueID, input.ExpectedSessionResourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
	}
	return commandResultFromRecord(completed), nil
}

func (h *pluginHost) admitExactTaskMessage(
	ctx context.Context,
	input pluginsdk.ExactTaskMessage,
	payloadDigest string,
) (*exactTaskMessageAdmission, *pluginsdk.CommandResult) {
	if h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if input.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, input.WorkspaceID, "host.v2.write:messages", input.ApprovalRevision,
		payloadDigest, CanonicalApprovalDigest("host-method", exactTaskMessageMethod, "v2"),
	)
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	messenger, _ := h.writeDependencies()
	writer, ok := messenger.(exactTaskMessageMessenger)
	if !ok {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "exact_task_messages_unsupported"}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
	}
	version := input.ExpectedTaskResourceVersion + "/" + input.ExpectedSessionResourceVersion
	record, _, err := store.Admit(ctx, state.CommandIntent{
		InstallationID: h.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: exactTaskMessageMethod, CapabilityID: "host.v2.write:messages", PayloadDigest: payloadDigest,
		TargetID: input.TaskID, ExpectedResourceVersion: version,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}
	}
	if err != nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}
	}
	return &exactTaskMessageAdmission{writer: writer, store: store, record: record, unlock: unlock}, nil
}

func exactTaskMessageErrorStatus(err error) pluginsdk.CommandStatus {
	switch status.Code(err) {
	case codes.Aborted, codes.AlreadyExists, codes.FailedPrecondition:
		return pluginsdk.CommandConflict
	case codes.ResourceExhausted:
		return pluginsdk.CommandRateLimited
	case codes.NotFound:
		return pluginsdk.CommandNotFound
	case codes.InvalidArgument:
		return pluginsdk.CommandInvalid
	default:
		return pluginsdk.CommandUnavailable
	}
}
