package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
)

type SourceIssueTransition struct {
	TargetID   string
	TargetName string
}

type SourceIssueCapabilities struct {
	WorkspaceID         string
	TaskID              string
	TaskResourceVersion string
	Provider            string
	SourceID            string
	Identifier          string
	URL                 string
	Title               string
	StatusID            string
	StatusName          string
	Transitions         []SourceIssueTransition
	ResourceVersion     string
	ObservedAt          string
	Receipt             HostReadReceipt
}

type SourceIssueCapabilitiesQuery struct {
	RequestID   string
	WorkspaceID string
	TaskID      string
}

type SourceIssueWritebackCommand struct {
	RequestID                     string
	WorkspaceID                   string
	TaskID                        string
	IdempotencyKey                string
	ExpectedTaskResourceVersion   string
	ExpectedSourceResourceVersion string
	ApprovalRevision              uint64
	ManifestDigest                string
	Body                          string
	TargetID                      string
}

type SourceIssueWritebackReceipt struct {
	ReceiptID             string
	OperationID           string
	InstallationID        string
	WorkspaceID           string
	TaskID                string
	Provider              string
	SourceID              string
	Operation             string
	State                 string
	Status                CommandStatus
	Reason                string
	ProviderReceiptID     string
	ExpectedSourceVersion string
	CreatedAt             string
	UpdatedAt             string
}

type ExactSourceIssueWritebackManager interface {
	GetCapabilities(ctx context.Context, query SourceIssueCapabilitiesQuery) (*CommandResult, *SourceIssueCapabilities, *HostReadReceipt, error)
	Comment(ctx context.Context, command SourceIssueWritebackCommand) (*CommandResult, *SourceIssueWritebackReceipt, error)
	Transition(ctx context.Context, command SourceIssueWritebackCommand) (*CommandResult, *SourceIssueWritebackReceipt, error)
}

type ExactSourceIssueWritebackHost interface {
	SourceIssueWriteback() ExactSourceIssueWritebackManager
}

func HostSourceIssueWriteback(host Host) (ExactSourceIssueWritebackManager, bool) {
	extended, ok := host.(ExactSourceIssueWritebackHost)
	if !ok || extended.SourceIssueWriteback() == nil {
		return nil, false
	}
	return extended.SourceIssueWriteback(), true
}

type grpcExactSourceIssueWritebackManager struct{ client pluginv1.HostClient }

func (m grpcExactSourceIssueWritebackManager) GetCapabilities(ctx context.Context, query SourceIssueCapabilitiesQuery) (*CommandResult, *SourceIssueCapabilities, *HostReadReceipt, error) {
	response, err := m.client.GetSourceIssueCapabilitiesExact(ctx, &pluginv1.GetSourceIssueCapabilitiesExactRequest{
		RequestId: query.RequestID, WorkspaceId: query.WorkspaceID, TaskId: query.TaskID,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	receipt := hostReadReceiptFromProto(response.GetReceipt())
	return commandResultFromProto(response.GetResult()), sourceIssueCapabilitiesFromProto(response.GetItem()), &receipt, nil
}

func (m grpcExactSourceIssueWritebackManager) Comment(ctx context.Context, command SourceIssueWritebackCommand) (*CommandResult, *SourceIssueWritebackReceipt, error) {
	response, err := m.client.CommentSourceIssueExact(ctx, &pluginv1.CommentSourceIssueExactRequest{
		RequestId: command.RequestID, WorkspaceId: command.WorkspaceID, TaskId: command.TaskID,
		IdempotencyKey: command.IdempotencyKey, ExpectedTaskResourceVersion: command.ExpectedTaskResourceVersion,
		ExpectedSourceResourceVersion: command.ExpectedSourceResourceVersion, Body: command.Body,
		ApprovalRevision: command.ApprovalRevision, ManifestDigest: command.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), sourceIssueWritebackReceiptFromProto(response.GetReceipt()), nil
}

func (m grpcExactSourceIssueWritebackManager) Transition(ctx context.Context, command SourceIssueWritebackCommand) (*CommandResult, *SourceIssueWritebackReceipt, error) {
	response, err := m.client.TransitionSourceIssueExact(ctx, &pluginv1.TransitionSourceIssueExactRequest{
		RequestId: command.RequestID, WorkspaceId: command.WorkspaceID, TaskId: command.TaskID,
		IdempotencyKey: command.IdempotencyKey, ExpectedTaskResourceVersion: command.ExpectedTaskResourceVersion,
		ExpectedSourceResourceVersion: command.ExpectedSourceResourceVersion, TargetId: command.TargetID,
		ApprovalRevision: command.ApprovalRevision, ManifestDigest: command.ManifestDigest,
	})
	if err != nil {
		return nil, nil, err
	}
	return commandResultFromProto(response.GetResult()), sourceIssueWritebackReceiptFromProto(response.GetReceipt()), nil
}

func sourceIssueCapabilitiesFromProto(value *pluginv1.ExactSourceIssueCapabilities) *SourceIssueCapabilities {
	if value == nil {
		return nil
	}
	result := &SourceIssueCapabilities{
		WorkspaceID: value.GetWorkspaceId(), TaskID: value.GetTaskId(), TaskResourceVersion: value.GetTaskResourceVersion(),
		Provider: value.GetProvider(), SourceID: value.GetSourceId(), Identifier: value.GetIdentifier(),
		URL: value.GetUrl(), Title: value.GetTitle(), StatusID: value.GetStatusId(), StatusName: value.GetStatusName(),
		ResourceVersion: value.GetResourceVersion(), ObservedAt: value.GetObservedAt(),
	}
	for _, transition := range value.GetTransitions() {
		result.Transitions = append(result.Transitions, SourceIssueTransition{TargetID: transition.GetTargetId(), TargetName: transition.GetTargetName()})
	}
	return result
}

func sourceIssueCapabilitiesToProto(value *SourceIssueCapabilities) *pluginv1.ExactSourceIssueCapabilities {
	if value == nil {
		return nil
	}
	result := &pluginv1.ExactSourceIssueCapabilities{
		WorkspaceId: value.WorkspaceID, TaskId: value.TaskID, TaskResourceVersion: value.TaskResourceVersion,
		Provider: value.Provider, SourceId: value.SourceID, Identifier: value.Identifier,
		Url: value.URL, Title: value.Title, StatusId: value.StatusID, StatusName: value.StatusName,
		ResourceVersion: value.ResourceVersion, ObservedAt: value.ObservedAt,
	}
	for _, transition := range value.Transitions {
		result.Transitions = append(result.Transitions, &pluginv1.SourceIssueTransition{TargetId: transition.TargetID, TargetName: transition.TargetName})
	}
	return result
}

func sourceIssueWritebackReceiptFromProto(value *pluginv1.SourceIssueWritebackReceipt) *SourceIssueWritebackReceipt {
	if value == nil {
		return nil
	}
	return &SourceIssueWritebackReceipt{
		ReceiptID: value.GetReceiptId(), OperationID: value.GetOperationId(), InstallationID: value.GetInstallationId(),
		WorkspaceID: value.GetWorkspaceId(), TaskID: value.GetTaskId(), Provider: value.GetProvider(),
		SourceID: value.GetSourceId(), Operation: value.GetOperation(), State: value.GetState(),
		Status: commandStatusFromProto(value.GetStatus()), Reason: value.GetReason(),
		ProviderReceiptID: value.GetProviderReceiptId(), ExpectedSourceVersion: value.GetExpectedSourceVersion(),
		CreatedAt: value.GetCreatedAt(), UpdatedAt: value.GetUpdatedAt(),
	}
}

func sourceIssueWritebackReceiptToProto(value *SourceIssueWritebackReceipt) *pluginv1.SourceIssueWritebackReceipt {
	if value == nil {
		return nil
	}
	return &pluginv1.SourceIssueWritebackReceipt{
		ReceiptId: value.ReceiptID, OperationId: value.OperationID, InstallationId: value.InstallationID,
		WorkspaceId: value.WorkspaceID, TaskId: value.TaskID, Provider: value.Provider, SourceId: value.SourceID,
		Operation: value.Operation, State: value.State, Status: commandStatusToProto(value.Status),
		Reason: value.Reason, ProviderReceiptId: value.ProviderReceiptID,
		ExpectedSourceVersion: value.ExpectedSourceVersion, CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
}
