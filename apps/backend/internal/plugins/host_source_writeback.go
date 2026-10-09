package plugins

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type SourceIssueErrorKind string

const (
	sourceWritebackStateSending   = "sending"
	sourceWritebackStateUncertain = "uncertain"
	sourceWritebackReasonUnknown  = "provider_outcome_unknown"
)

const (
	SourceIssueErrorNotFound           SourceIssueErrorKind = "not_found"
	SourceIssueErrorStale              SourceIssueErrorKind = "stale"
	SourceIssueErrorUnsupported        SourceIssueErrorKind = "unsupported"
	SourceIssueErrorMissingCredentials SourceIssueErrorKind = "missing_credentials"
	SourceIssueErrorDenied             SourceIssueErrorKind = "denied"
	SourceIssueErrorRateLimited        SourceIssueErrorKind = "rate_limited"
	SourceIssueErrorUnavailable        SourceIssueErrorKind = "unavailable"
	SourceIssueErrorUncertain          SourceIssueErrorKind = sourceWritebackStateUncertain
)

// SourceIssueError describes whether a provider operation was rejected,
// unavailable, or may have taken effect.
type SourceIssueError struct {
	Kind  SourceIssueErrorKind
	Cause error
}

func (e *SourceIssueError) Error() string {
	if e == nil {
		return "source issue error"
	}
	if e.Cause == nil {
		return string(e.Kind)
	}
	return fmt.Sprintf("source issue %s: %v", e.Kind, e.Cause)
}

func (e *SourceIssueError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type SourceIssueController interface {
	GetCapabilities(context.Context, string, string) (*pluginsdk.SourceIssueCapabilities, error)
	ApplyWriteback(context.Context, SourceIssueWritebackInput, func() error) (string, error)
}

type SourceIssueWritebackInput struct {
	WorkspaceID                   string
	TaskID                        string
	Provider                      string
	SourceID                      string
	Operation                     string
	ExpectedTaskResourceVersion   string
	ExpectedSourceResourceVersion string
	Body                          string
	TargetID                      string
}

type exactSourceIssueWritebackManager struct{ host *pluginHost }

func (h *pluginHost) SourceIssueWriteback() pluginsdk.ExactSourceIssueWritebackManager {
	return exactSourceIssueWritebackManager{host: h}
}

func (m exactSourceIssueWritebackManager) GetCapabilities(
	ctx context.Context,
	query pluginsdk.SourceIssueCapabilitiesQuery,
) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueCapabilities, *pluginsdk.HostReadReceipt, error) {
	if !isBoundedApprovalIdentifier(query.TaskID) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil, nil, nil
	}
	identity, unlock, err := m.host.exactReadAdmission(ctx, query.RequestID, query.WorkspaceID,
		exactReadSourceIssueCapabilities, "host.v2.read:source_issues", query.TaskID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer unlock()
	if m.host.sourceIssueController == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "source_issue_controller_unavailable"}, nil, nil, nil
	}
	item, readErr := m.host.sourceIssueController.GetCapabilities(ctx, query.WorkspaceID, query.TaskID)
	if item != nil && (item.TaskID != query.TaskID || item.WorkspaceID != query.WorkspaceID || item.ResourceVersion == "") {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "invalid_source_issue_observation"}, nil, nil, nil
	}
	receipt := singleReadReceipt(identity, query.RequestID)
	if readErr != nil {
		status, reason := sourceIssueErrorResult(readErr)
		return &pluginsdk.CommandResult{Status: status, Reason: reason}, item, &receipt, nil
	}
	if item == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandNotFound, Reason: "source_issue_not_found"}, nil, &receipt, nil
	}
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied}, item, &receipt, nil
}

func (m exactSourceIssueWritebackManager) Comment(
	ctx context.Context,
	command pluginsdk.SourceIssueWritebackCommand,
) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	if strings.TrimSpace(command.Body) == "" || len(command.Body) > 32768 || strings.ContainsRune(command.Body, '\x00') {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_comment"}, nil, nil
	}
	return m.host.applySourceIssueWriteback(ctx, command, "comment")
}

func (m exactSourceIssueWritebackManager) Transition(
	ctx context.Context,
	command pluginsdk.SourceIssueWritebackCommand,
) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	if !isBoundedApprovalIdentifier(command.TargetID) {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_transition"}, nil, nil
	}
	return m.host.applySourceIssueWriteback(ctx, command, "transition")
}

func (h *pluginHost) applySourceIssueWriteback(
	ctx context.Context,
	command pluginsdk.SourceIssueWritebackCommand,
	operation string,
) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	authorization, result := h.authorizeSourceWriteback(command, operation)
	if result != nil {
		return result, nil, nil
	}
	defer authorization.unlock()
	admission, result, receipt := authorization.admit(ctx, command, operation)
	if result != nil {
		return result, receipt, nil
	}
	return h.executeSourceWriteback(ctx, admission)
}

type sourceWritebackAuthorization struct {
	store          *state.CommandStore
	installationID string
	manifestDigest string
	controller     SourceIssueController
	unlock         func()
}

func (h *pluginHost) authorizeSourceWriteback(command pluginsdk.SourceIssueWritebackCommand, operation string) (*sourceWritebackAuthorization, *pluginsdk.CommandResult) {
	if !validSourceIssueWritebackCommand(command) {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}
	}
	if h.service == nil || h.installationID == "" {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "authorization_unavailable"}
	}
	h.service.approvalEffectMu.Lock()
	unlock := h.service.approvalEffectMu.Unlock
	installed := h.service.installedRecordByInstallationID(h.installationID)
	if installed == nil || (h.pluginID != "" && installed.ID != h.pluginID) {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyForeignInstallation)}
	}
	manifestDigest := ManifestCapabilityDigest(installed.Manifest)
	if command.ManifestDigest != manifestDigest {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(ApprovalDenyUnavailableCapability)}
	}
	method := sourceIssueMethod(operation)
	decision := h.service.authorizePluginCapability(h.installationID, command.WorkspaceID, "host.v2.write:source_issues",
		command.ApprovalRevision, CanonicalApprovalDigest("capability-context", command.WorkspaceID, method),
		CanonicalApprovalDigest("host-method", method))
	if !decision.Allowed {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: string(decision.Reason)}
	}
	if h.sourceIssueController == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "source_issue_controller_unavailable"}
	}
	store := h.commandStore
	if store == nil {
		store = h.service.exactCommandStoreDep()
	}
	if store == nil {
		unlock()
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_store_unavailable"}
	}
	return &sourceWritebackAuthorization{store: store, installationID: h.installationID,
		manifestDigest: manifestDigest, controller: h.sourceIssueController, unlock: unlock}, nil
}

func (a *sourceWritebackAuthorization) admit(ctx context.Context, command pluginsdk.SourceIssueWritebackCommand, operation string) (*sourceWritebackAdmission, *pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt) {
	digest, err := sourceIssueWritebackDigest(operation, command)
	if err != nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandInvalid, Reason: "invalid_request"}, nil
	}
	method := sourceIssueMethod(operation)
	record, _, err := a.store.Admit(ctx, state.CommandIntent{
		InstallationID: a.installationID, WorkspaceID: command.WorkspaceID, RequestID: command.RequestID,
		IdempotencyKey: command.IdempotencyKey, Method: method, CapabilityID: "host.v2.write:source_issues",
		PayloadDigest: digest, TargetID: command.TaskID,
		ExpectedResourceVersion: command.ExpectedTaskResourceVersion + "/" + command.ExpectedSourceResourceVersion,
		ApprovalRevision:        command.ApprovalRevision, ManifestDigest: a.manifestDigest,
	})
	if errors.Is(err, state.ErrCommandPayloadConflict) {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandConflict, Reason: "idempotency_payload_mismatch"}, nil
	}
	if err != nil {
		return nil, &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_admission_unavailable"}, nil
	}
	if record.Receipt.State == exactReceiptStateCompleted {
		return nil, replaySourceWriteback(record), storedSourceWritebackReceipt(ctx, a.store, record.Intent.OperationID)
	}
	return &sourceWritebackAdmission{store: a.store, record: record, command: command, operation: operation,
		payloadDigest: digest, installationID: a.installationID, controller: a.controller}, nil, nil
}

type sourceWritebackAdmission struct {
	store          *state.CommandStore
	record         state.CommandRecord
	command        pluginsdk.SourceIssueWritebackCommand
	operation      string
	payloadDigest  string
	installationID string
	controller     SourceIssueController
}

func (h *pluginHost) executeSourceWriteback(ctx context.Context, admission *sourceWritebackAdmission) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	commandRecord := admission.record
	if result, receipt, handled := resumeSourceWritebackReceipt(ctx, admission); handled {
		return result, receipt, nil
	}
	capabilities, readErr := admission.controller.GetCapabilities(ctx, admission.command.WorkspaceID, admission.command.TaskID)
	if capabilities == nil {
		status, reason := sourceIssueErrorResult(readErr)
		if readErr == nil {
			status, reason = pluginsdk.CommandNotFound, "source_issue_not_found"
		}
		return completeSourceWritebackCommand(ctx, admission, status, reason, admission.command.TaskID)
	}
	if !validSourceIssueCapabilities(capabilities, admission.command) {
		return completeSourceWritebackCommand(ctx, admission, pluginsdk.CommandUnavailable,
			"invalid_source_issue_observation", admission.command.TaskID)
	}
	sourceRecord, _, err := admission.store.PrepareSourceWriteback(ctx, state.SourceWritebackIntent{
		OperationID: commandRecord.Intent.OperationID, InstallationID: admission.installationID,
		WorkspaceID: admission.command.WorkspaceID, TaskID: admission.command.TaskID,
		Provider: capabilities.Provider, SourceID: capabilities.SourceID, Operation: admission.operation,
		ExpectedSourceVersion: admission.command.ExpectedSourceResourceVersion, PayloadDigest: admission.payloadDigest,
	})
	if err != nil {
		return completeSourceWritebackCommand(ctx, admission, pluginsdk.CommandUnavailable,
			"source_writeback_receipt_unavailable", capabilities.SourceID)
	}
	if sourceRecord.State == exactReceiptStateCompleted || sourceRecord.State == sourceWritebackStateUncertain {
		return completeSourceWritebackReplay(ctx, admission.store, commandRecord, sourceRecord)
	}
	if sourceRecord.State == sourceWritebackStateSending {
		return h.finishSourceWritebackKnown(ctx, admission, pluginsdk.CommandUncertain, sourceWritebackReasonUnknown, "")
	}
	if readErr != nil {
		status, reason := sourceIssueErrorResult(readErr)
		return h.finishSourceWritebackKnown(ctx, admission, status, reason, "")
	}
	if capabilities.TaskResourceVersion != admission.command.ExpectedTaskResourceVersion ||
		capabilities.ResourceVersion != admission.command.ExpectedSourceResourceVersion {
		return h.finishSourceWritebackKnown(ctx, admission, pluginsdk.CommandConflict, "source_version_changed", "")
	}
	return h.sendSourceWriteback(ctx, admission, capabilities)
}

func resumeSourceWritebackReceipt(ctx context.Context, admission *sourceWritebackAdmission) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, bool) {
	receipt, err := admission.store.GetSourceWriteback(ctx, admission.record.Intent.OperationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false
	}
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "source_writeback_receipt_unavailable"}, nil, true
	}
	switch receipt.State {
	case exactReceiptStateCompleted, sourceWritebackStateUncertain:
		result, storedReceipt, _ := completeSourceWriteback(ctx, admission.store, admission.record, receipt)
		return result, storedReceipt, true
	case sourceWritebackStateSending:
		uncertain, finishErr := admission.store.FinishSourceWriteback(ctx, receipt.OperationID,
			string(pluginsdk.CommandUncertain), sourceWritebackReasonUnknown, "")
		if finishErr != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "source_writeback_receipt_unavailable"},
				sourceWritebackReceiptFromState(receipt), true
		}
		result, storedReceipt, _ := completeSourceWriteback(ctx, admission.store, admission.record, uncertain)
		return result, storedReceipt, true
	default:
		return nil, nil, false
	}
}

func validSourceIssueCapabilities(value *pluginsdk.SourceIssueCapabilities, command pluginsdk.SourceIssueWritebackCommand) bool {
	return value != nil && value.WorkspaceID == command.WorkspaceID && value.TaskID == command.TaskID &&
		value.Provider != "" && value.SourceID != "" && value.ResourceVersion != ""
}

func (h *pluginHost) sendSourceWriteback(ctx context.Context, admission *sourceWritebackAdmission, capabilities *pluginsdk.SourceIssueCapabilities) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	command := admission.command
	input := SourceIssueWritebackInput{
		WorkspaceID: command.WorkspaceID, TaskID: command.TaskID, Provider: capabilities.Provider,
		SourceID: capabilities.SourceID, Operation: admission.operation,
		ExpectedTaskResourceVersion:   command.ExpectedTaskResourceVersion,
		ExpectedSourceResourceVersion: command.ExpectedSourceResourceVersion,
		Body:                          command.Body, TargetID: command.TargetID,
	}
	started := false
	providerReceiptID, writeErr := admission.controller.ApplyWriteback(ctx, input, func() error {
		_, didStart, startErr := admission.store.StartSourceWriteback(ctx, admission.record.Intent.OperationID)
		if startErr != nil {
			return &SourceIssueError{Kind: SourceIssueErrorUnavailable, Cause: startErr}
		}
		if !didStart {
			return &SourceIssueError{Kind: SourceIssueErrorUncertain, Cause: errors.New("writeback send was already reserved")}
		}
		started = true
		return nil
	})
	status, reason := sourceWritebackOutcome(writeErr, started)
	finished, err := admission.store.FinishSourceWriteback(ctx, admission.record.Intent.OperationID,
		string(status), reason, providerReceiptID)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "source_writeback_receipt_unavailable"}, nil, nil
	}
	return completeSourceWriteback(ctx, admission.store, admission.record, finished)
}

func sourceWritebackOutcome(writeErr error, started bool) (pluginsdk.CommandStatus, string) {
	if writeErr == nil && started {
		return pluginsdk.CommandApplied, ""
	}
	if writeErr == nil {
		return pluginsdk.CommandUnavailable, "provider_write_not_started"
	}
	status, reason := sourceIssueErrorResult(writeErr)
	if started {
		var sourceErr *SourceIssueError
		if !errors.As(writeErr, &sourceErr) {
			return pluginsdk.CommandUncertain, sourceWritebackReasonUnknown
		}
	} else if status == pluginsdk.CommandUncertain {
		return pluginsdk.CommandUnavailable, "provider_write_not_started"
	}
	return status, reason
}

func (h *pluginHost) finishSourceWritebackKnown(ctx context.Context, admission *sourceWritebackAdmission, status pluginsdk.CommandStatus, reason, providerReceiptID string) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	finished, err := admission.store.FinishSourceWriteback(ctx, admission.record.Intent.OperationID,
		string(status), reason, providerReceiptID)
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "source_writeback_receipt_unavailable"}, nil, nil
	}
	return completeSourceWriteback(ctx, admission.store, admission.record, finished)
}

func completeSourceWritebackCommand(ctx context.Context, admission *sourceWritebackAdmission, status pluginsdk.CommandStatus, reason, targetID string) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	completed, err := admission.store.Complete(ctx, admission.record.Intent.OperationID, string(status), reason, targetID, "")
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"}, nil, nil
	}
	return commandResultFromRecord(completed), nil, nil
}

func replaySourceWriteback(record state.CommandRecord) *pluginsdk.CommandResult {
	result := commandResultFromRecord(record)
	if result.Status == pluginsdk.CommandApplied {
		result.Status = pluginsdk.CommandAlreadyApplied
		if result.Receipt != nil {
			result.Receipt.Status = pluginsdk.CommandAlreadyApplied
		}
	}
	return result
}

func storedSourceWritebackReceipt(ctx context.Context, store *state.CommandStore, operationID string) *pluginsdk.SourceIssueWritebackReceipt {
	stored, err := store.GetSourceWriteback(ctx, operationID)
	if err != nil {
		return nil
	}
	return sourceWritebackReceiptFromState(stored)
}

func sourceIssueMethod(operation string) string {
	if operation == "transition" {
		return exactSourceIssueTransitionMethod
	}
	return exactSourceIssueCommentMethod
}

func validSourceIssueWritebackCommand(command pluginsdk.SourceIssueWritebackCommand) bool {
	if !isBoundedApprovalIdentifier(command.RequestID) || !isBoundedApprovalIdentifier(command.WorkspaceID) ||
		!isBoundedApprovalIdentifier(command.TaskID) || !isBoundedApprovalIdentifier(command.IdempotencyKey) ||
		!isBoundedApprovalIdentifier(command.ExpectedTaskResourceVersion) ||
		!isBoundedApprovalIdentifier(command.ExpectedSourceResourceVersion) || command.ApprovalRevision == 0 ||
		!isBoundedApprovalIdentifier(command.ManifestDigest) {
		return false
	}
	return true
}

func sourceIssueWritebackDigest(operation string, command pluginsdk.SourceIssueWritebackCommand) (string, error) {
	payload := struct {
		Operation                     string `json:"operation"`
		WorkspaceID                   string `json:"workspace_id"`
		TaskID                        string `json:"task_id"`
		ExpectedTaskResourceVersion   string `json:"expected_task_resource_version"`
		ExpectedSourceResourceVersion string `json:"expected_source_resource_version"`
		Body                          string `json:"body"`
		TargetID                      string `json:"target_id"`
	}{operation, command.WorkspaceID, command.TaskID, command.ExpectedTaskResourceVersion,
		command.ExpectedSourceResourceVersion, command.Body, command.TargetID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func sourceIssueErrorResult(err error) (pluginsdk.CommandStatus, string) {
	if err == nil {
		return pluginsdk.CommandUnavailable, "source_issue_unavailable"
	}
	var sourceErr *SourceIssueError
	if !errors.As(err, &sourceErr) {
		return pluginsdk.CommandUncertain, sourceWritebackReasonUnknown
	}
	switch sourceErr.Kind {
	case SourceIssueErrorNotFound:
		return pluginsdk.CommandNotFound, "source_issue_not_found"
	case SourceIssueErrorStale:
		return pluginsdk.CommandConflict, "source_link_changed"
	case SourceIssueErrorUnsupported:
		return pluginsdk.CommandUnsupported, "source_operation_unsupported"
	case SourceIssueErrorMissingCredentials:
		return pluginsdk.CommandUnavailable, "missing_credentials"
	case SourceIssueErrorDenied:
		return pluginsdk.CommandDenied, "provider_denied"
	case SourceIssueErrorRateLimited:
		return pluginsdk.CommandRateLimited, "provider_rate_limited"
	case SourceIssueErrorUncertain:
		return pluginsdk.CommandUncertain, sourceWritebackReasonUnknown
	default:
		return pluginsdk.CommandUnavailable, "provider_unavailable"
	}
}

func sourceWritebackReceiptFromState(value state.SourceWritebackReceipt) *pluginsdk.SourceIssueWritebackReceipt {
	if value.OperationID == "" {
		return nil
	}
	return &pluginsdk.SourceIssueWritebackReceipt{
		ReceiptID: value.ReceiptID, OperationID: value.OperationID, InstallationID: value.InstallationID,
		WorkspaceID: value.WorkspaceID, TaskID: value.TaskID, Provider: value.Provider,
		SourceID: value.SourceID, Operation: value.Operation, State: value.State,
		Status: pluginsdk.CommandStatus(value.ResultStatus), Reason: value.Reason,
		ProviderReceiptID: value.ProviderReceiptID, ExpectedSourceVersion: value.ExpectedSourceVersion,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}

func completeSourceWritebackReplay(
	ctx context.Context,
	store *state.CommandStore,
	commandRecord state.CommandRecord,
	sourceRecord state.SourceWritebackReceipt,
) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	if sourceRecord.State == sourceWritebackStateSending {
		var err error
		sourceRecord, err = store.FinishSourceWriteback(ctx, commandRecord.Intent.OperationID,
			string(pluginsdk.CommandUncertain), sourceWritebackReasonUnknown, "")
		if err != nil {
			return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "source_writeback_receipt_unavailable"}, nil, nil
		}
	}
	return completeSourceWriteback(ctx, store, commandRecord, sourceRecord)
}

func completeSourceWriteback(
	ctx context.Context,
	store *state.CommandStore,
	commandRecord state.CommandRecord,
	sourceRecord state.SourceWritebackReceipt,
) (*pluginsdk.CommandResult, *pluginsdk.SourceIssueWritebackReceipt, error) {
	completed, err := store.Complete(ctx, commandRecord.Intent.OperationID, sourceRecord.ResultStatus,
		sourceRecord.Reason, sourceRecord.SourceID, "")
	if err != nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnavailable, Reason: "command_receipt_unavailable"},
			sourceWritebackReceiptFromState(sourceRecord), nil
	}
	return commandResultFromRecord(completed), sourceWritebackReceiptFromState(sourceRecord), nil
}
