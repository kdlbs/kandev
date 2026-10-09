package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	managedConversationCapability                   = "managed_agent_conversations"
	managedConversationOutcomeAlreadyApplied        = "already_applied"
	managedConversationOutcomeCreated               = "created"
	managedConversationOutcomeConfigurationRequired = "configuration_required"
)

type pluginHostManagedConversationManager struct{ host *pluginHost }

type managedConversationCall struct {
	service ManagedAgentConversationService
	store   *state.CommandStore
	unlock  func()
}

type managedMutationAdmission struct {
	call     *managedConversationCall
	record   state.CommandRecord
	target   string
	replayed bool
}

func (c *managedConversationCall) close() { c.unlock() }

func (h *pluginHost) ManagedAgentConversations() pluginsdk.ManagedAgentConversationManager {
	return &pluginHostManagedConversationManager{host: h}
}

func (m *pluginHostManagedConversationManager) begin(
	ctx context.Context, workspaceID, capabilityID, method string, approvalRevision uint64,
	manifestDigest, requestDigest string,
) (*managedConversationCall, *pluginsdk.CommandResult, error) {
	h := m.host
	if h == nil || h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}, nil
	}
	if h.managedConversations == nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_conversation_service_unavailable"}, nil
	}
	svc := h.managedConversations()
	if svc == nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_conversation_service_unavailable"}, nil
	}
	h.service.approvalEffectMu.Lock()
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil {
		h.service.approvalEffectMu.Unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}, nil
	}
	if installed.Status != StatusActive {
		h.service.approvalEffectMu.Unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: "installation_inactive"}, nil
	}
	currentDigest := ManifestCapabilityDigest(installed.Manifest)
	if manifestDigest != currentDigest {
		h.service.approvalEffectMu.Unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}, nil
	}
	decision := h.service.authorizePluginCapability(
		h.installationID, workspaceID, capabilityID, approvalRevision, requestDigest,
		CanonicalApprovalDigest("host-method", method, "v2"),
	)
	if !decision.Allowed {
		h.service.approvalEffectMu.Unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}, nil
	}
	commandStore := h.commandStore
	if commandStore == nil {
		commandStore = h.service.exactCommandStoreDep()
	}
	return &managedConversationCall{service: svc, store: commandStore, unlock: h.service.approvalEffectMu.Unlock}, nil, nil
}

func (m *pluginHostManagedConversationManager) Get(ctx context.Context, query pluginsdk.ManagedAgentConversationQuery) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	digest, err := managedConversationDigest(struct {
		WorkspaceID string `json:"workspace_id"`
		InstanceKey string `json:"instance_key"`
	}{query.WorkspaceID, query.InstanceKey})
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.InvalidArgument, "invalid managed conversation request")
	}
	call, denied, err := m.begin(ctx, query.WorkspaceID, "host.v2.read:"+managedConversationCapability,
		"GetManagedAgentConversationStatusExact", query.ApprovalRevision, query.ManifestDigest, digest)
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if denied != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(managedCommandGRPCCode(denied.Status), denied.Reason)
	}
	defer call.close()
	return call.service.GetManaged(ctx, m.host.installationID, query.WorkspaceID, query.InstanceKey)
}

func (m *pluginHostManagedConversationManager) List(ctx context.Context, query pluginsdk.ManagedAgentConversationListQuery) ([]pluginsdk.ManagedAgentConversationDescriptor, error) {
	digest, err := managedConversationDigest(struct {
		WorkspaceID string `json:"workspace_id"`
	}{query.WorkspaceID})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid managed conversation request")
	}
	call, denied, err := m.begin(ctx, query.WorkspaceID, "host.v2.read:"+managedConversationCapability,
		"ListManagedAgentConversationsExact", query.ApprovalRevision, query.ManifestDigest, digest)
	if err != nil {
		return nil, err
	}
	if denied != nil {
		return nil, status.Error(managedCommandGRPCCode(denied.Status), denied.Reason)
	}
	defer call.close()
	return call.service.ListManaged(ctx, m.host.installationID, query.WorkspaceID)
}

func (m *pluginHostManagedConversationManager) Ensure(ctx context.Context, input pluginsdk.ManagedAgentConversationSpec) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentConversationDescriptor, error) {
	input.AgentToolNames = slices.Clone(input.AgentToolNames)
	slices.Sort(input.AgentToolNames)
	if err := validateManagedConversationSpec(input); err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	digest, err := managedConversationDigest(struct {
		WorkspaceID        string   `json:"workspace_id"`
		InstanceKey        string   `json:"instance_key"`
		ExpectedRevision   uint64   `json:"expected_revision"`
		AgentProfileID     string   `json:"agent_profile_id"`
		ExecutorID         string   `json:"executor_id"`
		ExecutorProfileID  string   `json:"executor_profile_id"`
		BasePrompt         string   `json:"base_prompt"`
		InstructionVersion string   `json:"instruction_version"`
		AgentToolNames     []string `json:"agent_tool_names"`
	}{input.WorkspaceID, input.InstanceKey, input.ExpectedRevision, input.AgentProfileID,
		input.ExecutorID, input.ExecutorProfileID, input.BasePrompt, input.InstructionVersion, input.AgentToolNames})
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	admission, command, descriptor, err := m.beginMutation(ctx, managedMutationInput{
		requestID: input.RequestID, idempotencyKey: input.IdempotencyKey, workspaceID: input.WorkspaceID,
		instanceKey: input.InstanceKey, expectedRevision: input.ExpectedRevision, approvalRevision: input.ApprovalRevision,
		manifestDigest: input.ManifestDigest, method: "EnsureManagedAgentConversationExact", digest: digest,
	})
	if err != nil || command != nil {
		return command, descriptor, err
	}
	defer admission.call.close()
	if len(input.AgentToolNames) > 0 {
		if result := m.authorizeManagedToolSelection(input); result != nil {
			return m.finish(ctx, admission, result.Status, result.Reason, descriptor)
		}
	}
	descriptor, outcome, err := admission.call.service.EnsureManaged(
		ctx, m.host.pluginID, m.host.installationID, input, admission.record.Intent.OperationID, digest,
	)
	if err != nil {
		resultStatus := managedConversationErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "managed_conversation_unavailable"}, descriptor, nil
		}
		return m.finish(ctx, admission, resultStatus, string(resultStatus), descriptor)
	}
	resultStatus := pluginsdk.CommandNoChange
	switch {
	case outcome == managedConversationOutcomeAlreadyApplied && admission.replayed:
		resultStatus = pluginsdk.CommandAlreadyApplied
	case outcome == managedConversationOutcomeCreated || descriptor.Revision > input.ExpectedRevision:
		resultStatus = pluginsdk.CommandApplied
	case outcome == managedConversationOutcomeConfigurationRequired:
		resultStatus = pluginsdk.CommandConflict
	}
	reason := ""
	if outcome == managedConversationOutcomeConfigurationRequired {
		reason = managedConversationOutcomeConfigurationRequired
	}
	return m.finish(ctx, admission, resultStatus, reason, descriptor)
}

func (m *pluginHostManagedConversationManager) authorizeManagedToolSelection(input pluginsdk.ManagedAgentConversationSpec) *pluginsdk.CommandResult {
	installed := m.host.service.installedRecordByInstallationID(m.host.installationID)
	if installed == nil || installed.Status != StatusActive {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	for _, localName := range input.AgentToolNames {
		if _, err := findAgentTool(installed.AgentTools, localName, "managed-conversation"); err != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "tool_not_declared_for_managed_conversation"}
		}
	}
	declared, err := ManifestCapabilityIDs(installed.Manifest)
	if err != nil || !containsString(declared, "host.v2.write:managed_agent_tools") {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUndeclaredCapability)}
	}
	selectionDigest := CanonicalApprovalDigest("managed-agent-tools-selection", strings.Join(input.AgentToolNames, "\x00"), strconv.FormatUint(input.ExpectedRevision, 10))
	decision := m.host.service.authorizePluginCapability(
		m.host.installationID, input.WorkspaceID, "host.v2.write:managed_agent_tools", input.ApprovalRevision,
		selectionDigest, CanonicalApprovalDigest("host-method", "EnsureManagedAgentConversationAgentToolsExact", "v2"),
	)
	if !decision.Allowed {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	return nil
}

func (m *pluginHostManagedConversationManager) beginMutation(ctx context.Context, input managedMutationInput) (*managedMutationAdmission, *pluginsdk.CommandResult, pluginsdk.ManagedAgentConversationDescriptor, error) {
	call, denied, err := m.begin(ctx, input.workspaceID, "host.v2.write:"+managedConversationCapability,
		input.method, input.approvalRevision, input.manifestDigest, input.digest)
	if err != nil || denied != nil {
		return nil, denied, pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if call.store == nil {
		call.close()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	target := CanonicalApprovalDigest("managed-agent-conversation", m.host.installationID, input.workspaceID, input.instanceKey)
	record, replayed, err := call.store.Admit(ctx, state.CommandIntent{
		InstallationID: m.host.installationID, WorkspaceID: input.workspaceID,
		RequestID: input.requestID, IdempotencyKey: input.idempotencyKey,
		Method: input.method, CapabilityID: "host.v2.write:" + managedConversationCapability,
		PayloadDigest: input.digest, TargetID: target,
		ExpectedResourceVersion: strconv.FormatUint(input.expectedRevision, 10),
		ApprovalRevision:        input.approvalRevision, ManifestDigest: input.manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		call.close()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	if err != nil {
		call.close()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	if replayed && record.Receipt.State == exactReceiptStateCompleted {
		var descriptor pluginsdk.ManagedAgentConversationDescriptor
		if input.method != "DeleteManagedAgentConversationExact" {
			descriptor, _ = call.service.GetManaged(ctx, m.host.installationID, input.workspaceID, input.instanceKey)
		}
		call.close()
		return nil, commandResultFromRecord(record), descriptor, nil
	}
	return &managedMutationAdmission{call: call, record: record, target: target, replayed: replayed}, nil, pluginsdk.ManagedAgentConversationDescriptor{}, nil
}

type managedMutationInput struct {
	requestID, idempotencyKey, workspaceID, instanceKey string
	expectedRevision, approvalRevision                  uint64
	manifestDigest, method, digest                      string
}

func (m *pluginHostManagedConversationManager) finish(ctx context.Context, admission *managedMutationAdmission, result pluginsdk.CommandStatus, reason string, descriptor pluginsdk.ManagedAgentConversationDescriptor) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentConversationDescriptor, error) {
	resourceVersion := ""
	if descriptor.Revision > 0 {
		resourceVersion = strconv.FormatUint(descriptor.Revision, 10)
	}
	completed, err := admission.call.store.Complete(ctx, admission.record.Intent.OperationID, string(result), reason, admission.target, resourceVersion)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, descriptor, nil
	}
	return commandResultFromRecord(completed), descriptor, nil
}

func (m *pluginHostManagedConversationManager) SetPaused(ctx context.Context, input pluginsdk.ManagedAgentConversationPause) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentConversationDescriptor, error) {
	if !validManagedMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.InstanceKey, input.ApprovalRevision, input.ManifestDigest) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	digest, err := managedConversationDigest(struct {
		WorkspaceID      string `json:"workspace_id"`
		InstanceKey      string `json:"instance_key"`
		ExpectedRevision uint64 `json:"expected_revision"`
		Paused           bool   `json:"paused"`
	}{input.WorkspaceID, input.InstanceKey, input.ExpectedRevision, input.Paused})
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, pluginsdk.ManagedAgentConversationDescriptor{}, nil
	}
	admission, command, descriptor, err := m.beginMutation(ctx, managedMutationInput{
		requestID: input.RequestID, idempotencyKey: input.IdempotencyKey, workspaceID: input.WorkspaceID,
		instanceKey: input.InstanceKey, expectedRevision: input.ExpectedRevision, approvalRevision: input.ApprovalRevision,
		manifestDigest: input.ManifestDigest, method: "SetManagedAgentConversationPausedExact", digest: digest,
	})
	if err != nil || command != nil {
		return command, descriptor, err
	}
	defer admission.call.close()
	descriptor, err = admission.call.service.SetManagedPaused(
		ctx, m.host.installationID, input.WorkspaceID, input.InstanceKey, input.ExpectedRevision, input.Paused,
		admission.record.Intent.OperationID, digest,
	)
	if err != nil {
		resultStatus := managedConversationErrorStatus(err)
		if resultStatus == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: resultStatus, Reason: "managed_conversation_unavailable"}, descriptor, nil
		}
		return m.finish(ctx, admission, resultStatus, string(resultStatus), descriptor)
	}
	result := pluginsdk.CommandApplied
	if descriptor.Revision == input.ExpectedRevision {
		result = pluginsdk.CommandNoChange
	}
	return m.finish(ctx, admission, result, "", descriptor)
}

func (m *pluginHostManagedConversationManager) Delete(ctx context.Context, input pluginsdk.ManagedAgentConversationDelete) (*pluginsdk.CommandResult, error) {
	if !validManagedMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.InstanceKey, input.ApprovalRevision, input.ManifestDigest) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	digest, err := managedConversationDigest(struct {
		WorkspaceID      string `json:"workspace_id"`
		InstanceKey      string `json:"instance_key"`
		ExpectedRevision uint64 `json:"expected_revision"`
	}{input.WorkspaceID, input.InstanceKey, input.ExpectedRevision})
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	admission, command, _, err := m.beginMutation(ctx, managedMutationInput{
		requestID: input.RequestID, idempotencyKey: input.IdempotencyKey, workspaceID: input.WorkspaceID,
		instanceKey: input.InstanceKey, expectedRevision: input.ExpectedRevision, approvalRevision: input.ApprovalRevision,
		manifestDigest: input.ManifestDigest, method: "DeleteManagedAgentConversationExact", digest: digest,
	})
	if err != nil || command != nil {
		return command, err
	}
	defer admission.call.close()
	err = admission.call.service.DeleteManaged(
		ctx, m.host.installationID, input.WorkspaceID, input.InstanceKey, input.ExpectedRevision,
		admission.record.Intent.OperationID, digest,
	)
	if err != nil {
		var outcome interface{ ManagedDeletionOutcome() string }
		if errors.As(err, &outcome) {
			switch outcome.ManagedDeletionOutcome() {
			case "admitted_failed", "outcome_uncertain":
				return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "managed_conversation_delete_" + outcome.ManagedDeletionOutcome()}, nil
			case "committed":
				completed, completeErr := admission.call.store.Complete(ctx, admission.record.Intent.OperationID, string(pluginsdk.CommandAlreadyApplied), "", admission.target, "")
				if completeErr != nil {
					return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
				}
				return commandResultFromRecord(completed), nil
			}
		}
		result := managedConversationErrorStatus(err)
		if result == pluginsdk.CommandUnavailable {
			return &pluginsdk.CommandResult{Status: result, Reason: "managed_conversation_unavailable"}, nil
		}
		completed, completeErr := admission.call.store.Complete(ctx, admission.record.Intent.OperationID, string(result), string(result), admission.target, "")
		if completeErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
		}
		return commandResultFromRecord(completed), nil
	}
	completed, err := admission.call.store.Complete(ctx, admission.record.Intent.OperationID, string(pluginsdk.CommandApplied), "", admission.target, "")
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil
	}
	return commandResultFromRecord(completed), nil
}

func validManagedMutation(requestID, idemKey, workspaceID, instanceKey string, approvalRevision uint64, manifestDigest string) bool {
	return isBoundedApprovalIdentifier(requestID) && isBoundedApprovalIdentifier(idemKey) &&
		isBoundedApprovalIdentifier(workspaceID) && instanceKey != "" && len(instanceKey) <= 128 &&
		strings.TrimSpace(instanceKey) == instanceKey && approvalRevision > 0 && len(manifestDigest) == 64
}

func validateManagedConversationSpec(input pluginsdk.ManagedAgentConversationSpec) error {
	if !validManagedMutation(input.RequestID, input.IdempotencyKey, input.WorkspaceID, input.InstanceKey, input.ApprovalRevision, input.ManifestDigest) {
		return errors.New("managed conversation identity is incomplete")
	}
	if len(input.BasePrompt) > 65536 || len(input.InstructionVersion) > 256 || len(input.AgentProfileID) > 256 ||
		len(input.ExecutorID) > 256 || len(input.ExecutorProfileID) > 256 {
		return errors.New("managed conversation configuration exceeds its limit")
	}
	if err := mcpprofile.ValidateManagedToolNames(input.AgentToolNames); err != nil {
		return err
	}
	return nil
}

func managedConversationDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(encoded) > 65536 {
		return "", errors.New("managed conversation payload exceeds its limit")
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func managedConversationErrorStatus(err error) pluginsdk.CommandStatus {
	switch status.Code(err) {
	case codes.Aborted, codes.AlreadyExists, codes.FailedPrecondition:
		return pluginsdk.CommandConflict
	case codes.NotFound:
		return pluginsdk.CommandNotFound
	case codes.InvalidArgument:
		return pluginsdk.CommandInvalid
	case codes.PermissionDenied, codes.Unauthenticated:
		return pluginsdk.CommandDenied
	case codes.ResourceExhausted:
		return pluginsdk.CommandRateLimited
	default:
		return pluginsdk.CommandUnavailable
	}
}

func managedCommandGRPCCode(command pluginsdk.CommandStatus) codes.Code {
	switch command {
	case pluginsdk.CommandNotFound:
		return codes.NotFound
	case pluginsdk.CommandInvalid:
		return codes.InvalidArgument
	case pluginsdk.CommandUnavailable:
		return codes.Unavailable
	case pluginsdk.CommandConflict:
		return codes.Aborted
	case pluginsdk.CommandRateLimited:
		return codes.ResourceExhausted
	default:
		return codes.PermissionDenied
	}
}

var _ pluginsdk.ManagedAgentConversationManager = (*pluginHostManagedConversationManager)(nil)
