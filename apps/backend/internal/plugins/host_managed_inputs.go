package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	managedInputPageDefault = 50
	managedInputPageMaximum = 200
	managedInputBytesMax    = 65536
)

type managedInputMutationAdmission struct {
	call     *managedConversationCall
	record   state.CommandRecord
	target   string
	replayed bool
}

func (m *pluginHostManagedConversationManager) EnqueueInput(
	ctx context.Context,
	input pluginsdk.ManagedAgentInputEnqueue,
) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentInputReceipt, error) {
	if !validManagedInputEnqueue(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	digest, err := managedInputEnqueueDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	inputID := managedInputID(m.host.installationID, input.WorkspaceID, input.InstanceKey, input.OccurrenceKey)
	admission, command, err := m.beginInputMutation(ctx, input.WorkspaceID, input.InstanceKey,
		input.RequestID, input.IdempotencyKey, input.ExpectedConversationRevision,
		input.ApprovalRevision, input.ManifestDigest, "EnqueueManagedAgentInputExact", digest)
	if err != nil || command != nil {
		return command, pluginsdk.ManagedAgentInputReceipt{}, err
	}
	defer admission.call.close()
	service, ok := admission.call.service.(ManagedAgentInputService)
	if !ok {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_input_service_unavailable"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	if admission.replayed && admission.record.Receipt.State == exactReceiptStateCompleted {
		receipt, getErr := service.GetManagedInput(ctx, m.host.installationID, pluginsdk.ManagedAgentInputQuery{
			WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey, HostInputID: inputID,
		})
		if getErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_input_receipt_unavailable"}, pluginsdk.ManagedAgentInputReceipt{}, nil
		}
		return commandResultFromRecord(admission.record), receipt, nil
	}
	receipt, occurrenceReplayed, callErr := service.EnqueueManagedInput(
		ctx, m.host.installationID, inputID, input, admission.record.Intent.OperationID, digest,
	)
	if callErr != nil {
		return m.finishInputError(ctx, admission, callErr)
	}
	if receipt.HostInputID != inputID {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_input_identity_mismatch"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	result := pluginsdk.CommandApplied
	if occurrenceReplayed {
		result = pluginsdk.CommandAlreadyApplied
	}
	completed, completeErr := m.finishInput(ctx, admission, result, "", receipt.Sequence)
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, receipt, nil
	}
	return completed, receipt, nil
}

func (m *pluginHostManagedConversationManager) GetInput(
	ctx context.Context,
	query pluginsdk.ManagedAgentInputQuery,
) (pluginsdk.ManagedAgentInputReceipt, error) {
	if !validManagedInputQuery(query) {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.InvalidArgument, "invalid managed input query")
	}
	digest, err := managedConversationDigest(struct {
		WorkspaceID string `json:"workspace_id"`
		InstanceKey string `json:"instance_key"`
		HostInputID string `json:"host_input_id"`
	}{query.WorkspaceID, query.InstanceKey, query.HostInputID})
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.InvalidArgument, "invalid managed input query")
	}
	call, denied, err := m.begin(ctx, query.WorkspaceID, "host.v2.read:"+managedConversationCapability,
		"GetManagedAgentInputExact", query.ApprovalRevision, query.ManifestDigest, digest)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, err
	}
	if denied != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(managedCommandGRPCCode(denied.Status), denied.Reason)
	}
	defer call.close()
	service, ok := call.service.(ManagedAgentInputService)
	if !ok {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.Unavailable, "managed input service is unavailable")
	}
	return service.GetManagedInput(ctx, m.host.installationID, query)
}

func (m *pluginHostManagedConversationManager) ListInputs(
	ctx context.Context,
	query pluginsdk.ManagedAgentInputListQuery,
) (pluginsdk.ManagedAgentInputPage, error) {
	if !validManagedInputListQuery(query) {
		return pluginsdk.ManagedAgentInputPage{}, status.Error(codes.InvalidArgument, "invalid managed input list query")
	}
	if query.Limit == 0 {
		query.Limit = managedInputPageDefault
	}
	digest, err := managedConversationDigest(struct {
		WorkspaceID    string `json:"workspace_id"`
		InstanceKey    string `json:"instance_key"`
		SequenceCursor uint64 `json:"sequence_cursor"`
		Limit          uint32 `json:"limit"`
	}{query.WorkspaceID, query.InstanceKey, query.SequenceCursor, query.Limit})
	if err != nil {
		return pluginsdk.ManagedAgentInputPage{}, status.Error(codes.InvalidArgument, "invalid managed input list query")
	}
	call, denied, err := m.begin(ctx, query.WorkspaceID, "host.v2.read:"+managedConversationCapability,
		"ListManagedAgentInputsExact", query.ApprovalRevision, query.ManifestDigest, digest)
	if err != nil {
		return pluginsdk.ManagedAgentInputPage{}, err
	}
	if denied != nil {
		return pluginsdk.ManagedAgentInputPage{}, status.Error(managedCommandGRPCCode(denied.Status), denied.Reason)
	}
	defer call.close()
	service, ok := call.service.(ManagedAgentInputService)
	if !ok {
		return pluginsdk.ManagedAgentInputPage{}, status.Error(codes.Unavailable, "managed input service is unavailable")
	}
	return service.ListManagedInputs(ctx, m.host.installationID, query)
}

func (m *pluginHostManagedConversationManager) CancelInput(
	ctx context.Context,
	input pluginsdk.ManagedAgentInputCancel,
) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentInputReceipt, error) {
	if !validManagedInputCancel(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	digest, err := managedInputCancelDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	admission, command, err := m.beginInputMutation(ctx, input.WorkspaceID, input.InstanceKey,
		input.RequestID, input.IdempotencyKey, input.ExpectedConversationRevision,
		input.ApprovalRevision, input.ManifestDigest, "CancelManagedAgentInputExact", digest)
	if err != nil || command != nil {
		return command, pluginsdk.ManagedAgentInputReceipt{}, err
	}
	defer admission.call.close()
	service, ok := admission.call.service.(ManagedAgentInputService)
	if !ok {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_input_service_unavailable"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	if admission.replayed && admission.record.Receipt.State == exactReceiptStateCompleted {
		receipt, getErr := service.GetManagedInput(ctx, m.host.installationID, pluginsdk.ManagedAgentInputQuery{
			WorkspaceID: input.WorkspaceID, InstanceKey: input.InstanceKey, HostInputID: input.HostInputID,
		})
		if getErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_input_receipt_unavailable"}, pluginsdk.ManagedAgentInputReceipt{}, nil
		}
		return commandResultFromRecord(admission.record), receipt, nil
	}
	receipt, changed, callErr := service.CancelManagedInput(
		ctx, m.host.installationID, input, admission.record.Intent.OperationID, digest,
	)
	if callErr != nil {
		return m.finishInputError(ctx, admission, callErr)
	}
	result := pluginsdk.CommandNoChange
	if changed {
		result = pluginsdk.CommandApplied
	}
	completed, completeErr := m.finishInput(ctx, admission, result, "", receipt.Sequence)
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, receipt, nil
	}
	return completed, receipt, nil
}

func (m *pluginHostManagedConversationManager) Dispatch(
	ctx context.Context,
	input pluginsdk.ManagedAgentConversationDispatch,
) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentDispatchStatus, pluginsdk.ManagedAgentConversationDescriptor, error) {
	if !validManagedAgentDispatch(input) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, "", pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	digest, err := managedAgentDispatchDigest(input)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, "", pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	admission, command, err := m.beginInputMutation(ctx, input.WorkspaceID, input.InstanceKey,
		input.RequestID, input.IdempotencyKey, input.ExpectedConversationRevision,
		input.ApprovalRevision, input.ManifestDigest, "DispatchManagedAgentConversationExact", digest)
	if err != nil || command != nil {
		return command, "", pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	defer admission.call.close()
	service, ok := admission.call.service.(ManagedAgentInputService)
	if !ok {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_input_service_unavailable"}, "", pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	if admission.replayed && admission.record.Receipt.State == exactReceiptStateCompleted {
		result := commandResultFromRecord(admission.record)
		return result, dispatchStatusFromCommandReason(result.Reason), pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	dispatchStatus, descriptor, callErr := service.DispatchManagedInput(
		ctx, m.host.installationID, input, admission.record.Intent.OperationID, digest,
	)
	if callErr != nil {
		result, _, callErr := m.finishInputError(ctx, admission, callErr)
		return result, "", descriptor, callErr
	}
	var result pluginsdk.CommandStatus
	reason := string(dispatchStatus)
	switch dispatchStatus {
	case pluginsdk.ManagedAgentDispatchBusy:
		result = pluginsdk.CommandNoChange
	case pluginsdk.ManagedAgentDispatchStarted, pluginsdk.ManagedAgentDispatchSent:
		result = pluginsdk.CommandApplied
	default:
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_dispatch_outcome_unknown"}, dispatchStatus, descriptor, nil
	}
	completed, completeErr := m.finishInput(ctx, admission, result, reason, descriptor.Revision)
	if completeErr != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, dispatchStatus, descriptor, nil
	}
	return completed, dispatchStatus, descriptor, nil
}

func (m *pluginHostManagedConversationManager) beginInputMutation(
	ctx context.Context,
	workspaceID, instanceKey, requestID, idempotencyKey string,
	expectedRevision, approvalRevision uint64,
	manifestDigest, method, digest string,
) (*managedInputMutationAdmission, *pluginsdk.CommandResult, error) {
	call, denied, err := m.begin(ctx, workspaceID, "host.v2.write:"+managedConversationCapability,
		method, approvalRevision, manifestDigest, digest)
	if err != nil || denied != nil {
		return nil, denied, err
	}
	if call.store == nil {
		call.close()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}, nil
	}
	target := CanonicalApprovalDigest("managed-agent-conversation", m.host.installationID, workspaceID, instanceKey)
	record, replayed, err := call.store.Admit(ctx, state.CommandIntent{
		InstallationID: m.host.installationID, WorkspaceID: workspaceID,
		RequestID: requestID, IdempotencyKey: idempotencyKey,
		Method: method, CapabilityID: "host.v2.write:" + managedConversationCapability,
		PayloadDigest: digest, TargetID: target,
		ExpectedResourceVersion: strconv.FormatUint(expectedRevision, 10),
		ApprovalRevision:        approvalRevision, ManifestDigest: manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		call.close()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}, nil
	}
	if err != nil {
		call.close()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}, nil
	}
	return &managedInputMutationAdmission{call: call, record: record, target: target, replayed: replayed}, nil, nil
}

func (m *pluginHostManagedConversationManager) finishInput(
	ctx context.Context,
	admission *managedInputMutationAdmission,
	result pluginsdk.CommandStatus,
	reason string,
	resourceVersion uint64,
) (*pluginsdk.CommandResult, error) {
	completed, err := admission.call.store.Complete(ctx, admission.record.Intent.OperationID,
		string(result), reason, admission.target, strconv.FormatUint(resourceVersion, 10))
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, err
	}
	return commandResultFromRecord(completed), nil
}

func (m *pluginHostManagedConversationManager) finishInputError(
	ctx context.Context,
	admission *managedInputMutationAdmission,
	callErr error,
) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentInputReceipt, error) {
	result := managedConversationErrorStatus(callErr)
	if result == pluginsdk.CommandUnavailable {
		return &pluginsdk.CommandResult{Status: result, Reason: "managed_input_unavailable"}, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	completed, err := m.finishInput(ctx, admission, result, string(result), 0)
	if err != nil {
		return completed, pluginsdk.ManagedAgentInputReceipt{}, nil
	}
	return completed, pluginsdk.ManagedAgentInputReceipt{}, nil
}

func validManagedInputEnqueue(input pluginsdk.ManagedAgentInputEnqueue) bool {
	if !validManagedMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.InstanceKey, input.ApprovalRevision, input.ManifestDigest) ||
		input.ExpectedConversationRevision == 0 || !validManagedInputOrigin(input.Origin) || !validOccurrenceKey(input.OccurrenceKey) ||
		input.Payload == "" || len(input.Payload) > managedInputBytesMax || strings.ContainsRune(input.Payload, '\x00') ||
		!validManagedCoalesce(input.Origin, input.CoalesceKey) {
		return false
	}
	return true
}

func validManagedInputCancel(input pluginsdk.ManagedAgentInputCancel) bool {
	return validManagedMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.InstanceKey, input.ApprovalRevision, input.ManifestDigest) &&
		input.ExpectedConversationRevision > 0 && isBoundedApprovalIdentifier(input.HostInputID) &&
		(len(input.ExpectedExecutionID) <= 256 && strings.TrimSpace(input.ExpectedExecutionID) == input.ExpectedExecutionID)
}

func validManagedInputQuery(query pluginsdk.ManagedAgentInputQuery) bool {
	return validManagedConversationQuery(query.WorkspaceID, query.InstanceKey, query.ApprovalRevision, query.ManifestDigest) &&
		isBoundedApprovalIdentifier(query.HostInputID)
}

func validManagedInputListQuery(query pluginsdk.ManagedAgentInputListQuery) bool {
	return validManagedConversationQuery(query.WorkspaceID, query.InstanceKey, query.ApprovalRevision, query.ManifestDigest) &&
		query.Limit <= managedInputPageMaximum
}

func validManagedConversationQuery(workspaceID, instanceKey string, approvalRevision uint64, manifestDigest string) bool {
	return isBoundedApprovalIdentifier(workspaceID) && instanceKey != "" && len(instanceKey) <= 128 &&
		strings.TrimSpace(instanceKey) == instanceKey && approvalRevision > 0 && len(manifestDigest) == 64
}

func validManagedAgentDispatch(input pluginsdk.ManagedAgentConversationDispatch) bool {
	return validManagedMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.InstanceKey, input.ApprovalRevision, input.ManifestDigest) &&
		input.ExpectedConversationRevision > 0 && validManagedInputOrigin(input.Origin) && validOccurrenceKey(input.OccurrenceKey) &&
		input.Payload != "" && len(input.Payload) <= managedInputBytesMax && !strings.ContainsRune(input.Payload, '\x00') &&
		input.CoalesceKey == ""
}

func validManagedInputOrigin(origin pluginsdk.ManagedAgentInputOrigin) bool {
	switch origin {
	case pluginsdk.ManagedAgentInputHuman, pluginsdk.ManagedAgentInputAutomation,
		pluginsdk.ManagedAgentInputPeriodic, pluginsdk.ManagedAgentInputInteraction:
		return true
	default:
		return false
	}
}

func validManagedCoalesce(origin pluginsdk.ManagedAgentInputOrigin, key string) bool {
	if key == "" {
		return true
	}
	return origin == pluginsdk.ManagedAgentInputPeriodic && len(key) <= 128 && strings.TrimSpace(key) == key
}

func validOccurrenceKey(key string) bool {
	return key != "" && len(key) <= 512 && strings.TrimSpace(key) == key && !strings.ContainsRune(key, '\x00')
}

func managedInputEnqueueDigest(input pluginsdk.ManagedAgentInputEnqueue) (string, error) {
	payloadDigest := sha256.Sum256([]byte(input.Payload))
	return managedConversationDigest(struct {
		WorkspaceID                  string                            `json:"workspace_id"`
		InstanceKey                  string                            `json:"instance_key"`
		ExpectedConversationRevision uint64                            `json:"expected_conversation_revision"`
		OccurrenceKey                string                            `json:"occurrence_key"`
		Origin                       pluginsdk.ManagedAgentInputOrigin `json:"origin"`
		PayloadDigest                string                            `json:"payload_digest"`
		CoalesceKey                  string                            `json:"coalesce_key"`
	}{input.WorkspaceID, input.InstanceKey, input.ExpectedConversationRevision, input.OccurrenceKey,
		input.Origin, hex.EncodeToString(payloadDigest[:]), input.CoalesceKey})
}

func managedInputCancelDigest(input pluginsdk.ManagedAgentInputCancel) (string, error) {
	return managedConversationDigest(struct {
		WorkspaceID                  string `json:"workspace_id"`
		InstanceKey                  string `json:"instance_key"`
		HostInputID                  string `json:"host_input_id"`
		ExpectedConversationRevision uint64 `json:"expected_conversation_revision"`
		ExpectedExecutionID          string `json:"expected_execution_id"`
	}{input.WorkspaceID, input.InstanceKey, input.HostInputID, input.ExpectedConversationRevision, input.ExpectedExecutionID})
}

func managedAgentDispatchDigest(input pluginsdk.ManagedAgentConversationDispatch) (string, error) {
	payloadDigest := sha256.Sum256([]byte(input.Payload))
	return managedConversationDigest(struct {
		WorkspaceID                  string                            `json:"workspace_id"`
		InstanceKey                  string                            `json:"instance_key"`
		ExpectedConversationRevision uint64                            `json:"expected_conversation_revision"`
		OccurrenceKey                string                            `json:"occurrence_key"`
		Origin                       pluginsdk.ManagedAgentInputOrigin `json:"origin"`
		PayloadDigest                string                            `json:"payload_digest"`
	}{input.WorkspaceID, input.InstanceKey, input.ExpectedConversationRevision, input.OccurrenceKey,
		input.Origin, hex.EncodeToString(payloadDigest[:])})
}

func managedInputID(installationID, workspaceID, instanceKey, occurrenceKey string) string {
	return CanonicalApprovalDigest("managed-agent-input", installationID, workspaceID, instanceKey, occurrenceKey)
}

func dispatchStatusFromCommandReason(reason string) pluginsdk.ManagedAgentDispatchStatus {
	switch pluginsdk.ManagedAgentDispatchStatus(reason) {
	case pluginsdk.ManagedAgentDispatchBusy, pluginsdk.ManagedAgentDispatchStarted, pluginsdk.ManagedAgentDispatchSent:
		return pluginsdk.ManagedAgentDispatchStatus(reason)
	default:
		return ""
	}
}

var _ pluginsdk.ManagedAgentConversationManager = (*pluginHostManagedConversationManager)(nil)
