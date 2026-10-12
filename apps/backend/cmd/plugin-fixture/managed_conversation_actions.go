//nolint:revive // Keep the end-to-end fixture action matrix together for its generated plugin harness.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const managedConversationUnsupportedReason = "managed_conversation_unsupported"

type managedConversationActionInput struct {
	InstanceKey                 string `json:"instance_key"`
	InteractionsRequestID       string `json:"interactions_request_id,omitempty"`
	InteractionsSnapshotVersion string `json:"interactions_snapshot_version,omitempty"`
	InteractionsCursor          string `json:"interactions_cursor,omitempty"`
}

type managedConversationCommandInput struct {
	InstanceKey      string `json:"instance_key"`
	RequestID        string `json:"request_id"`
	IdempotencyKey   string `json:"idempotency_key"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

type managedConversationStatusResponse struct {
	PluginID                     string                             `json:"plugin_id"`
	InstanceKey                  string                             `json:"instance_key"`
	TaskID                       string                             `json:"task_id"`
	SessionID                    string                             `json:"session_id"`
	Revision                     uint64                             `json:"revision"`
	DesiredPaused                bool                               `json:"desired_paused"`
	AgentProfileID               string                             `json:"agent_profile_id,omitempty"`
	ExecutorID                   string                             `json:"executor_id,omitempty"`
	ExecutorProfileID            string                             `json:"executor_profile_id,omitempty"`
	ManagedConversationSupported bool                               `json:"managed_conversation_supported"`
	ManagedConversationReason    string                             `json:"managed_conversation_reason,omitempty"`
	TaskStatusSupported          bool                               `json:"task_status_supported"`
	TaskStatusReason             string                             `json:"task_status_reason,omitempty"`
	CanonicalStatus              string                             `json:"canonical_status,omitempty"`
	SemanticActivity             string                             `json:"semantic_activity,omitempty"`
	ExecutionState               string                             `json:"execution_state,omitempty"`
	TaskStatusKnown              bool                               `json:"task_status_known"`
	TaskBlockingReasons          []string                           `json:"task_blocking_reasons,omitempty"`
	LastMeaningfulActivityAt     *string                            `json:"last_meaningful_activity_at,omitempty"`
	TaskResourceVersion          string                             `json:"task_resource_version,omitempty"`
	TaskObservedAt               string                             `json:"task_observed_at,omitempty"`
	TaskReadReceipt              fixtureHostReadReceiptDTO          `json:"task_read_receipt,omitempty"`
	SessionSupported             bool                               `json:"session_supported"`
	SessionReason                string                             `json:"session_reason,omitempty"`
	SessionState                 string                             `json:"session_state,omitempty"`
	SessionResourceVersion       string                             `json:"session_resource_version,omitempty"`
	ExecutionID                  string                             `json:"execution_id,omitempty"`
	InteractionsSupported        bool                               `json:"interactions_supported"`
	InteractionsReason           string                             `json:"interactions_reason,omitempty"`
	PendingInteractions          []fixtureInteractionObservationDTO `json:"pending_interactions"`
	InteractionsSnapshotVersion  string                             `json:"interactions_snapshot_version,omitempty"`
	InteractionsReadReceipt      fixtureHostReadReceiptDTO          `json:"interactions_read_receipt,omitempty"`
	InteractionsNextCursor       string                             `json:"interactions_next_cursor,omitempty"`
	InteractionsHasMore          bool                               `json:"interactions_has_more"`
	RecoverySupported            bool                               `json:"recovery_supported"`
	RecoveryReason               string                             `json:"recovery_reason,omitempty"`
}

type fixtureHostReadReceiptDTO struct {
	InstallationID     string `json:"installation_id,omitempty"`
	WorkspaceID        string `json:"workspace_id,omitempty"`
	CapabilityRevision uint64 `json:"capability_revision,omitempty"`
	RequestID          string `json:"request_id,omitempty"`
	SnapshotVersion    string `json:"snapshot_version,omitempty"`
	ObservedAt         string `json:"observed_at,omitempty"`
}

type fixtureInteractionObservationDTO struct {
	ID                string                          `json:"id"`
	Kind              string                          `json:"kind"`
	Status            string                          `json:"status"`
	TaskID            string                          `json:"task_id"`
	SessionID         string                          `json:"session_id"`
	TurnID            string                          `json:"turn_id,omitempty"`
	Title             string                          `json:"title,omitempty"`
	Context           string                          `json:"context,omitempty"`
	ToolCallID        string                          `json:"tool_call_id,omitempty"`
	ActionType        string                          `json:"action_type,omitempty"`
	AgentDisconnected bool                            `json:"agent_disconnected"`
	CreatedAt         string                          `json:"created_at,omitempty"`
	UpdatedAt         string                          `json:"updated_at,omitempty"`
	Options           []fixtureInteractionOptionDTO   `json:"options,omitempty"`
	Questions         []fixtureInteractionQuestionDTO `json:"questions,omitempty"`
	ResourceVersion   string                          `json:"resource_version"`
	ObservedAt        string                          `json:"observed_at,omitempty"`
}

type fixtureInteractionOptionDTO struct {
	OptionID    string `json:"option_id"`
	Label       string `json:"label"`
	Kind        string `json:"kind,omitempty"`
	Description string `json:"description,omitempty"`
}

type fixtureInteractionQuestionDTO struct {
	ID      string                        `json:"id"`
	Title   string                        `json:"title,omitempty"`
	Prompt  string                        `json:"prompt,omitempty"`
	Options []fixtureInteractionOptionDTO `json:"options,omitempty"`
}

type fixtureManagedInputReceiptDTO struct {
	HostInputID          string                            `json:"host_input_id"`
	OccurrenceKey        string                            `json:"occurrence_key,omitempty"`
	Sequence             uint64                            `json:"sequence"`
	Origin               pluginsdk.ManagedAgentInputOrigin `json:"origin"`
	Payload              string                            `json:"payload,omitempty"`
	CoalesceKey          string                            `json:"coalesce_key,omitempty"`
	ConversationRevision uint64                            `json:"conversation_revision"`
	State                pluginsdk.ManagedAgentInputState  `json:"state"`
	CreatedAt            string                            `json:"created_at,omitempty"`
	UpdatedAt            string                            `json:"updated_at,omitempty"`
	QueueEntryID         string                            `json:"queue_entry_id,omitempty"`
	ExecutionID          string                            `json:"execution_id,omitempty"`
	TurnID               string                            `json:"turn_id,omitempty"`
	SupersededBy         string                            `json:"superseded_by,omitempty"`
}

type fixtureCommandReceiptDTO struct {
	ID               string                  `json:"id"`
	OperationID      string                  `json:"operation_id,omitempty"`
	Status           pluginsdk.CommandStatus `json:"status"`
	TargetID         string                  `json:"target_id,omitempty"`
	ResourceVersion  string                  `json:"resource_version,omitempty"`
	ApprovalRevision uint64                  `json:"approval_revision"`
	CreatedAt        string                  `json:"created_at,omitempty"`
	UpdatedAt        string                  `json:"updated_at,omitempty"`
}

func decodeFixtureActionBody(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func (p *fixturePlugin) managedConversationActionContext(ctx context.Context, req *pluginsdk.PluginActionRequest) (pluginsdk.ExactHost, *pluginsdk.CapabilityContext, pluginsdk.ManagedAgentConversationManager, error) {
	if req.Context.WorkspaceID == "" {
		return nil, nil, nil, fmt.Errorf("plugin-fixture: workspace is required")
	}
	host := p.Host()
	if host == nil {
		return nil, nil, nil, fmt.Errorf("plugin-fixture: host unavailable")
	}
	exact, ok := pluginsdk.HostV2(host)
	if !ok {
		return nil, nil, nil, fmt.Errorf("plugin-fixture: exact Host v2 unavailable")
	}
	capability, err := exact.GetCapabilityContext(ctx, req.Context.WorkspaceID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("plugin-fixture: read managed conversation capability context: %w", err)
	}
	if capability == nil || capability.WorkspaceID != req.Context.WorkspaceID || capability.ManifestDigest == "" {
		return nil, nil, nil, fmt.Errorf("plugin-fixture: invalid managed conversation capability context")
	}
	manager := exact.ManagedAgentConversations()
	if manager == nil {
		return nil, nil, nil, fmt.Errorf("plugin-fixture: managed conversation manager unavailable")
	}
	return exact, capability, manager, nil
}

func getManagedConversation(ctx context.Context, manager pluginsdk.ManagedAgentConversationManager, capability *pluginsdk.CapabilityContext, workspaceID, instanceKey string) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	if instanceKey == "" {
		return pluginsdk.ManagedAgentConversationDescriptor{}, fmt.Errorf("plugin-fixture: instance_key is required")
	}
	descriptor, err := manager.Get(ctx, pluginsdk.ManagedAgentConversationQuery{
		WorkspaceID: workspaceID, InstanceKey: instanceKey,
		ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
	})
	if err != nil {
		return pluginsdk.ManagedAgentConversationDescriptor{}, fmt.Errorf("plugin-fixture: get managed conversation: %w", err)
	}
	if descriptor.WorkspaceID != workspaceID || descriptor.InstanceKey != instanceKey || descriptor.TaskID == "" || descriptor.SessionID == "" {
		return pluginsdk.ManagedAgentConversationDescriptor{}, fmt.Errorf("plugin-fixture: managed conversation identity is incomplete or outside workspace")
	}
	return descriptor, nil
}

func capabilityOperation(capability *pluginsdk.CapabilityContext, method string) (pluginsdk.CapabilityOperation, bool) {
	for _, operation := range capability.Operations {
		if operation.Method == method {
			return operation, true
		}
	}
	return pluginsdk.CapabilityOperation{Method: method, UnavailableReason: "host_operation_not_advertised"}, false
}

func operationAvailable(operation pluginsdk.CapabilityOperation, found bool) (bool, string) {
	if !found {
		return false, "host_operation_not_advertised"
	}
	if !operation.Supported {
		if operation.UnavailableReason != "" {
			return false, operation.UnavailableReason
		}
		return false, "host_operation_unsupported"
	}
	if !operation.Authorized {
		if operation.UnavailableReason != "" {
			return false, operation.UnavailableReason
		}
		return false, "capability_not_approved"
	}
	return true, ""
}

func (p *fixturePlugin) managedConversationStatus(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input managedConversationActionInput
	if err := decodeFixtureActionBody(req.Body, &input); err != nil {
		return nil, fmt.Errorf("plugin-fixture: decode managed conversation status: %w", err)
	}
	if hasInteractionContinuation(input) && (input.InteractionsRequestID == "" || input.InteractionsSnapshotVersion == "" || input.InteractionsCursor == "") {
		return nil, fmt.Errorf("plugin-fixture: interaction continuation requires its request, snapshot, and cursor")
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	conversationOp, conversationOpFound := capabilityOperation(capability, "GetManagedAgentConversationStatusExact")
	conversationSupported, conversationReason := operationAvailable(conversationOp, conversationOpFound)
	recoveryOp, recoveryOpFound := capabilityOperation(capability, "RecoverSessionExact")
	recoverySupported, recoveryReason := operationAvailable(recoveryOp, recoveryOpFound)
	response := managedConversationStatusResponse{
		PluginID: "kandev-plugin-e2e", InstanceKey: input.InstanceKey,
		ManagedConversationSupported: conversationSupported, ManagedConversationReason: conversationReason,
		RecoverySupported: recoverySupported, RecoveryReason: recoveryReason,
		PendingInteractions: []fixtureInteractionObservationDTO{},
	}
	if !conversationSupported {
		response.ManagedConversationSupported = false
		if response.ManagedConversationReason == "" {
			response.ManagedConversationReason = managedConversationUnsupportedReason
		}
		response.TaskStatusReason = managedConversationUnsupportedReason
		response.InteractionsReason = managedConversationUnsupportedReason
		return encodeFixtureActionResponse(response)
	}
	descriptor, err := getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, input.InstanceKey)
	if err != nil {
		return nil, err
	}
	response.InstanceKey = descriptor.InstanceKey
	response.TaskID = descriptor.TaskID
	response.SessionID = descriptor.SessionID
	response.Revision = descriptor.Revision
	response.DesiredPaused = descriptor.DesiredPaused
	response.AgentProfileID = descriptor.AgentProfileID
	response.ExecutorID = descriptor.ExecutorID
	response.ExecutorProfileID = descriptor.ExecutorProfileID
	queries, hasQueries := pluginsdk.HostExactQueries(p.Host())
	if !hasQueries {
		response.TaskStatusReason = "exact_query_manager_unavailable"
		response.InteractionsReason = "exact_query_manager_unavailable"
		return encodeFixtureActionResponse(response)
	}
	if err := readManagedTaskStatus(ctx, queries, capability, req.Context.WorkspaceID, descriptor, &response); err != nil {
		return nil, err
	}
	if err := readManagedSessionFence(ctx, queries, capability, req.Context.WorkspaceID, descriptor, &response); err != nil {
		return nil, err
	}
	if err := readManagedPendingInteractions(ctx, queries, capability, req.Context.WorkspaceID, descriptor, input, &response); err != nil {
		return nil, err
	}
	return encodeFixtureActionResponse(response)
}

func readManagedSessionFence(ctx context.Context, queries pluginsdk.ExactQueryManager, capability *pluginsdk.CapabilityContext, workspaceID string, descriptor pluginsdk.ManagedAgentConversationDescriptor, response *managedConversationStatusResponse) error {
	operation, found := capabilityOperation(capability, "ListSessionsExact")
	response.SessionSupported, response.SessionReason = operationAvailable(operation, found)
	if !response.SessionSupported {
		response.RecoverySupported = false
		if response.RecoveryReason == "" {
			response.RecoveryReason = response.SessionReason
		}
		return nil
	}
	requestID := fixtureExactReadRequestID("managed-session-fence")
	page := pluginsdk.ExactReadPage{Limit: 200}
	for pageNumber := 0; pageNumber < 10; pageNumber++ {
		result, err := queries.ListSessions(ctx, pluginsdk.ExactSessionQuery{
			RequestID: requestID, WorkspaceID: workspaceID,
			Filter: pluginsdk.SessionFilter{TaskIDs: []string{descriptor.TaskID}}, Page: page,
		})
		if err != nil {
			response.SessionSupported = false
			response.SessionReason = "session_read_unavailable"
			response.RecoverySupported = false
			response.RecoveryReason = response.SessionReason
			return nil
		}
		for _, observation := range result.Items {
			if observation.Session.ID != descriptor.SessionID {
				continue
			}
			if observation.Session.TaskID != descriptor.TaskID || observation.ResourceVersion == "" {
				return fmt.Errorf("plugin-fixture: exact session read escaped managed conversation scope")
			}
			response.SessionState = observation.ExecutionState
			response.SessionResourceVersion = observation.ResourceVersion
			response.ExecutionID = observation.ExecutionID
			if response.RecoverySupported {
				switch {
				case observation.ExecutionID == "":
					response.RecoverySupported = false
					response.RecoveryReason = "session_execution_unavailable"
				case observation.ExecutionState == "RUNNING" || observation.ExecutionState == "STARTING":
					response.RecoverySupported = false
					response.RecoveryReason = "session_execution_active"
				}
			}
			return nil
		}
		if !result.PageInfo.HasMore || result.PageInfo.NextCursor == "" {
			break
		}
		page.Cursor = result.PageInfo.NextCursor
		page.SnapshotVersion = result.PageInfo.SnapshotVersion
	}
	response.SessionSupported = false
	response.SessionReason = "managed_session_not_observed"
	response.RecoverySupported = false
	response.RecoveryReason = response.SessionReason
	return nil
}

func readManagedTaskStatus(ctx context.Context, queries pluginsdk.ExactQueryManager, capability *pluginsdk.CapabilityContext, workspaceID string, descriptor pluginsdk.ManagedAgentConversationDescriptor, response *managedConversationStatusResponse) error {
	operation, found := capabilityOperation(capability, "GetTaskExact")
	response.TaskStatusSupported, response.TaskStatusReason = operationAvailable(operation, found)
	if !response.TaskStatusSupported {
		return nil
	}
	task, receipt, err := queries.GetTask(ctx, pluginsdk.ExactTaskGetQuery{
		RequestID: fixtureExactReadRequestID("status-task"), WorkspaceID: workspaceID, TaskID: descriptor.TaskID,
	})
	if err != nil {
		response.TaskStatusSupported = false
		response.TaskStatusReason = "task_status_read_unavailable"
		return nil
	}
	if task.Task.ID != descriptor.TaskID || task.Task.WorkspaceID != workspaceID {
		return fmt.Errorf("plugin-fixture: exact task status read escaped managed conversation scope")
	}
	response.CanonicalStatus = task.CanonicalStatus
	response.SemanticActivity = task.SemanticActivity
	response.ExecutionState = task.ExecutionState
	response.TaskStatusKnown = task.StatusKnown
	response.TaskBlockingReasons = append([]string(nil), task.BlockingReasons...)
	response.LastMeaningfulActivityAt = task.LastMeaningfulActivityAt
	response.TaskResourceVersion = task.ResourceVersion
	response.TaskObservedAt = task.ObservedAt
	response.TaskReadReceipt = fixtureReadReceiptDTO(receipt)
	return nil
}

func hasInteractionContinuation(input managedConversationActionInput) bool {
	return input.InteractionsRequestID != "" || input.InteractionsSnapshotVersion != "" || input.InteractionsCursor != ""
}

func readManagedPendingInteractions(ctx context.Context, queries pluginsdk.ExactQueryManager, capability *pluginsdk.CapabilityContext, workspaceID string, descriptor pluginsdk.ManagedAgentConversationDescriptor, input managedConversationActionInput, response *managedConversationStatusResponse) error {
	operation, found := capabilityOperation(capability, "ListPendingInteractionsExact")
	response.InteractionsSupported, response.InteractionsReason = operationAvailable(operation, found)
	if !response.InteractionsSupported {
		return nil
	}
	requestID := input.InteractionsRequestID
	if requestID == "" {
		requestID = fixtureExactReadRequestID("status-interactions")
	}
	page, err := queries.ListPendingInteractions(ctx, pluginsdk.ExactInteractionQuery{
		RequestID: requestID, WorkspaceID: workspaceID,
		Filter: pluginsdk.InteractionFilter{TaskIDs: []string{descriptor.TaskID}, SessionIDs: []string{descriptor.SessionID}},
		Page: pluginsdk.ExactReadPage{
			Limit: 200, Cursor: input.InteractionsCursor, SnapshotVersion: input.InteractionsSnapshotVersion,
		},
	})
	if err != nil {
		response.InteractionsSupported = false
		response.InteractionsReason = "pending_interactions_read_unavailable"
		response.PendingInteractions = nil
		return nil
	}
	response.InteractionsSnapshotVersion = page.PageInfo.SnapshotVersion
	response.InteractionsReadReceipt = fixtureReadReceiptDTO(page.PageInfo.Receipt)
	response.InteractionsNextCursor = page.PageInfo.NextCursor
	response.InteractionsHasMore = page.PageInfo.HasMore
	response.PendingInteractions = make([]fixtureInteractionObservationDTO, 0, len(page.Items))
	for _, observation := range page.Items {
		interaction := observation.Interaction
		if interaction.TaskID != descriptor.TaskID || interaction.SessionID != descriptor.SessionID || interaction.Status != pluginsdk.InteractionStatusPending {
			return fmt.Errorf("plugin-fixture: pending interaction read escaped managed conversation scope")
		}
		response.PendingInteractions = append(response.PendingInteractions, fixtureInteractionObservationDTO{
			ID: interaction.ID, Kind: interaction.Kind, Status: interaction.Status,
			TaskID: interaction.TaskID, SessionID: interaction.SessionID, TurnID: interaction.TurnID,
			Title: interaction.Title, Context: interaction.Context, ToolCallID: interaction.ToolCallID,
			ActionType: interaction.ActionType, AgentDisconnected: interaction.AgentDisconnected,
			CreatedAt: interaction.CreatedAt, UpdatedAt: interaction.UpdatedAt,
			Options: fixtureInteractionOptions(interaction.Options), Questions: fixtureInteractionQuestions(interaction.Questions),
			ResourceVersion: observation.ResourceVersion, ObservedAt: observation.ObservedAt,
		})
	}
	return nil
}

func fixtureReadReceiptDTO(receipt pluginsdk.HostReadReceipt) fixtureHostReadReceiptDTO {
	return fixtureHostReadReceiptDTO{
		InstallationID: receipt.InstallationID, WorkspaceID: receipt.WorkspaceID,
		CapabilityRevision: receipt.CapabilityRevision, RequestID: receipt.RequestID,
		SnapshotVersion: receipt.SnapshotVersion, ObservedAt: receipt.ObservedAt,
	}
}

func fixtureExactReadRequestID(operation string) string {
	return "fixture-" + operation + "-" + uuid.NewString()
}

func fixtureInteractionOptions(options []pluginsdk.InteractionOption) []fixtureInteractionOptionDTO {
	if len(options) == 0 {
		return nil
	}
	out := make([]fixtureInteractionOptionDTO, len(options))
	for index, option := range options {
		out[index] = fixtureInteractionOptionDTO{OptionID: option.OptionID, Label: option.Label, Kind: option.Kind, Description: option.Description}
	}
	return out
}

func fixtureInteractionQuestions(questions []pluginsdk.InteractionQuestion) []fixtureInteractionQuestionDTO {
	if len(questions) == 0 {
		return nil
	}
	out := make([]fixtureInteractionQuestionDTO, len(questions))
	for index, question := range questions {
		out[index] = fixtureInteractionQuestionDTO{ID: question.ID, Title: question.Title, Prompt: question.Prompt, Options: fixtureInteractionOptions(question.Options)}
	}
	return out
}

func encodeFixtureActionResponse(value any) (*pluginsdk.PluginActionResponse, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: marshal exact action response: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func managedInputReceiptDTO(receipt pluginsdk.ManagedAgentInputReceipt) fixtureManagedInputReceiptDTO {
	return fixtureManagedInputReceiptDTO{
		HostInputID: receipt.HostInputID, OccurrenceKey: receipt.OccurrenceKey, Sequence: receipt.Sequence,
		Origin: receipt.Origin, Payload: receipt.Payload, CoalesceKey: receipt.CoalesceKey,
		ConversationRevision: receipt.ConversationRevision, State: receipt.State,
		CreatedAt: receipt.CreatedAt, UpdatedAt: receipt.UpdatedAt, QueueEntryID: receipt.QueueEntryID,
		ExecutionID: receipt.ExecutionID, TurnID: receipt.TurnID, SupersededBy: receipt.SupersededBy,
	}
}

func commandReceiptDTO(receipt *pluginsdk.CommandReceipt) *fixtureCommandReceiptDTO {
	if receipt == nil {
		return nil
	}
	return &fixtureCommandReceiptDTO{
		ID: receipt.ID, OperationID: receipt.OperationID, Status: receipt.Status, TargetID: receipt.TargetID,
		ResourceVersion: receipt.ResourceVersion, ApprovalRevision: receipt.ApprovalRevision,
		CreatedAt: receipt.CreatedAt, UpdatedAt: receipt.UpdatedAt,
	}
}

func managedCommandResponse(result *pluginsdk.CommandResult, descriptor pluginsdk.ManagedAgentConversationDescriptor) map[string]any {
	response := map[string]any{
		"instance_key": descriptor.InstanceKey, "revision": descriptor.Revision,
		"task_id": descriptor.TaskID, "session_id": descriptor.SessionID,
	}
	if result == nil {
		response["status"] = pluginsdk.CommandUnavailable
		response["reason"] = "host_command_returned_no_result"
		return response
	}
	response["status"] = result.Status
	if result.Reason != "" {
		response["reason"] = result.Reason
	}
	if receipt := commandReceiptDTO(result.Receipt); receipt != nil {
		response["command_receipt"] = receipt
	}
	return response
}

func (p *fixturePlugin) managedConversationInputs(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		InstanceKey    string `json:"instance_key"`
		SequenceCursor uint64 `json:"sequence_cursor"`
		Limit          uint32 `json:"limit"`
	}
	if err := decodeFixtureActionBody(req.Body, &input); err != nil || input.InstanceKey == "" || input.Limit > 200 {
		return nil, fmt.Errorf("plugin-fixture: invalid managed conversation input list")
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	descriptor, err := getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, input.InstanceKey)
	if err != nil {
		return nil, err
	}
	page, err := manager.ListInputs(ctx, pluginsdk.ManagedAgentInputListQuery{
		WorkspaceID: req.Context.WorkspaceID, InstanceKey: input.InstanceKey,
		SequenceCursor: input.SequenceCursor, Limit: input.Limit,
		ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: list managed conversation inputs: %w", err)
	}
	inputs := make([]fixtureManagedInputReceiptDTO, len(page.Inputs))
	for index, receipt := range page.Inputs {
		inputs[index] = managedInputReceiptDTO(receipt)
	}
	return encodeFixtureActionResponse(map[string]any{
		"instance_key": descriptor.InstanceKey, "revision": descriptor.Revision,
		"inputs": inputs, "next_sequence_cursor": page.NextSequenceCursor, "has_more": page.HasMore,
	})
}

func (p *fixturePlugin) enqueueManagedConversationInput(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		InstanceKey      string `json:"instance_key"`
		RequestID        string `json:"request_id"`
		IdempotencyKey   string `json:"idempotency_key"`
		ExpectedRevision uint64 `json:"expected_revision"`
		OccurrenceKey    string `json:"occurrence_key"`
		Payload          string `json:"payload"`
	}
	if err := decodeFixtureActionBody(req.Body, &input); err != nil || input.InstanceKey == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.ExpectedRevision == 0 || input.OccurrenceKey == "" || input.Payload == "" {
		return nil, fmt.Errorf("plugin-fixture: invalid managed conversation enqueue")
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	descriptor, err := getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, input.InstanceKey)
	if err != nil {
		return nil, err
	}
	result, receipt, err := manager.EnqueueInput(ctx, pluginsdk.ManagedAgentInputEnqueue{
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		WorkspaceID: req.Context.WorkspaceID, InstanceKey: input.InstanceKey,
		ExpectedConversationRevision: input.ExpectedRevision,
		ApprovalRevision:             capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
		OccurrenceKey: input.OccurrenceKey, Origin: pluginsdk.ManagedAgentInputHuman, Payload: input.Payload,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: enqueue managed conversation input: %w", err)
	}
	response := managedCommandResponse(result, descriptor)
	response["input"] = managedInputReceiptDTO(receipt)
	response["host_input_id"] = receipt.HostInputID
	response["sequence"] = receipt.Sequence
	response["input_state"] = receipt.State
	return encodeFixtureActionResponse(response)
}

func (p *fixturePlugin) cancelManagedConversationInput(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		InstanceKey         string `json:"instance_key"`
		RequestID           string `json:"request_id"`
		IdempotencyKey      string `json:"idempotency_key"`
		ExpectedRevision    uint64 `json:"expected_revision"`
		HostInputID         string `json:"host_input_id"`
		ExpectedExecutionID string `json:"expected_execution_id"`
	}
	if err := decodeFixtureActionBody(req.Body, &input); err != nil || input.InstanceKey == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.ExpectedRevision == 0 || input.HostInputID == "" {
		return nil, fmt.Errorf("plugin-fixture: invalid managed conversation cancel")
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	descriptor, err := getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, input.InstanceKey)
	if err != nil {
		return nil, err
	}
	result, receipt, err := manager.CancelInput(ctx, pluginsdk.ManagedAgentInputCancel{
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		WorkspaceID: req.Context.WorkspaceID, InstanceKey: input.InstanceKey, HostInputID: input.HostInputID,
		ExpectedConversationRevision: input.ExpectedRevision, ExpectedExecutionID: input.ExpectedExecutionID,
		ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: cancel managed conversation input: %w", err)
	}
	response := managedCommandResponse(result, descriptor)
	response["input"] = managedInputReceiptDTO(receipt)
	response["host_input_id"] = receipt.HostInputID
	response["sequence"] = receipt.Sequence
	response["input_state"] = receipt.State
	return encodeFixtureActionResponse(response)
}

func (p *fixturePlugin) setManagedConversationPaused(ctx context.Context, req *pluginsdk.PluginActionRequest, paused bool) (*pluginsdk.PluginActionResponse, error) {
	var input managedConversationCommandInput
	if err := decodeFixtureActionBody(req.Body, &input); err != nil || input.InstanceKey == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.ExpectedRevision == 0 {
		return nil, fmt.Errorf("plugin-fixture: invalid managed conversation pause command")
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	_, err = getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, input.InstanceKey)
	if err != nil {
		return nil, err
	}
	result, updated, err := manager.SetPaused(ctx, pluginsdk.ManagedAgentConversationPause{
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		WorkspaceID: req.Context.WorkspaceID, InstanceKey: input.InstanceKey,
		ExpectedRevision: input.ExpectedRevision, ApprovalRevision: capability.ApprovalRevision,
		ManifestDigest: capability.ManifestDigest, Paused: paused,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: set managed conversation pause state: %w", err)
	}
	response := managedCommandResponse(result, updated)
	response["desired_paused"] = updated.DesiredPaused
	return encodeFixtureActionResponse(response)
}

func (p *fixturePlugin) recoverManagedConversationSession(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	input, err := decodeManagedConversationRecoveryInput(req.Body)
	if err != nil {
		return nil, err
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	descriptor, err := getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, input.InstanceKey)
	if err != nil {
		return nil, err
	}
	response := managedCommandResponse(nil, descriptor)
	if descriptor.Revision != input.ExpectedRevision {
		response["status"] = pluginsdk.CommandConflict
		response["reason"] = "conversation_revision_changed"
		return encodeFixtureActionResponse(response)
	}
	if unsupported := managedRecoveryUnavailable(capability, p.Host()); unsupported != nil {
		response["status"] = unsupported.Status
		response["reason"] = unsupported.Reason
		return encodeFixtureActionResponse(response)
	}
	execution, _ := pluginsdk.HostExecutionCommands(p.Host())
	result, run, err := execution.RecoverSession(ctx, pluginsdk.ExactSessionRecoveryCommand{
		ExactTaskExecutionCommand: pluginsdk.ExactTaskExecutionCommand{
			RequestID: input.RequestID, WorkspaceID: req.Context.WorkspaceID,
			TaskID: descriptor.TaskID, SessionID: descriptor.SessionID,
			ExpectedSessionResourceVersion: input.ExpectedSessionResourceVersion,
			ExpectedExecutionID:            input.ExpectedExecutionID, IdempotencyKey: input.IdempotencyKey,
			ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
		},
		Action: "resume",
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: recover managed conversation session: %w", err)
	}
	response = managedCommandResponse(result, descriptor)
	response["action"] = "resume"
	appendRecoveredRun(response, run)
	return encodeFixtureActionResponse(response)
}

type managedConversationRecoveryInput struct {
	InstanceKey                    string `json:"instance_key"`
	RequestID                      string `json:"request_id"`
	IdempotencyKey                 string `json:"idempotency_key"`
	ExpectedRevision               uint64 `json:"expected_revision"`
	ExpectedSessionResourceVersion string `json:"expected_session_resource_version"`
	ExpectedExecutionID            string `json:"expected_execution_id"`
}

func decodeManagedConversationRecoveryInput(body []byte) (managedConversationRecoveryInput, error) {
	var input managedConversationRecoveryInput
	if err := decodeFixtureActionBody(body, &input); err != nil {
		return input, fmt.Errorf("plugin-fixture: invalid managed conversation recovery: %w", err)
	}
	if input.InstanceKey == "" || input.RequestID == "" || input.IdempotencyKey == "" || input.ExpectedRevision == 0 || input.ExpectedSessionResourceVersion == "" || input.ExpectedExecutionID == "" {
		return input, fmt.Errorf("plugin-fixture: invalid managed conversation recovery")
	}
	return input, nil
}

func managedRecoveryUnavailable(capability *pluginsdk.CapabilityContext, host pluginsdk.Host) *pluginsdk.CommandResult {
	operation, found := capabilityOperation(capability, "RecoverSessionExact")
	if !found || !operation.Supported {
		reason := operation.UnavailableReason
		if reason == "" {
			reason = "execution_control_unsupported"
		}
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: reason}
	}
	if !operation.Authorized {
		reason := operation.UnavailableReason
		if reason == "" {
			reason = "capability_not_approved"
		}
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandDenied, Reason: reason}
	}
	manager, ok := pluginsdk.HostExecutionCommands(host)
	if !ok || manager == nil {
		return &pluginsdk.CommandResult{Status: pluginsdk.CommandUnsupported, Reason: "execution_control_unavailable"}
	}
	return nil
}

func appendRecoveredRun(response map[string]any, run pluginsdk.ExactTaskRunResult) {
	if run.SessionID != "" {
		response["session_id"] = run.SessionID
	}
	if run.ExecutionID != "" {
		response["execution_id"] = run.ExecutionID
	}
	if run.SessionState != "" {
		response["session_state"] = run.SessionState
	}
}

func (p *fixturePlugin) managedInteractionTarget(ctx context.Context, req *pluginsdk.PluginActionRequest, instanceKey, interactionID, expectedVersion string) (pluginsdk.ExactHost, *pluginsdk.CapabilityContext, pluginsdk.ManagedAgentConversationDescriptor, pluginsdk.ExactInteractionObservation, error) {
	exact, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, nil, pluginsdk.ManagedAgentConversationDescriptor{}, pluginsdk.ExactInteractionObservation{}, err
	}
	descriptor, err := getManagedConversation(ctx, manager, capability, req.Context.WorkspaceID, instanceKey)
	if err != nil {
		return nil, nil, pluginsdk.ManagedAgentConversationDescriptor{}, pluginsdk.ExactInteractionObservation{}, err
	}
	queries, ok := pluginsdk.HostExactQueries(p.Host())
	if !ok {
		return nil, nil, pluginsdk.ManagedAgentConversationDescriptor{}, pluginsdk.ExactInteractionObservation{}, fmt.Errorf("plugin-fixture: exact query manager unavailable")
	}
	observation, _, err := queries.GetInteraction(ctx, pluginsdk.ExactInteractionGetQuery{
		RequestID:   fixtureExactReadRequestID("interaction-read"),
		WorkspaceID: req.Context.WorkspaceID, InteractionID: interactionID,
	})
	if err != nil {
		return nil, nil, pluginsdk.ManagedAgentConversationDescriptor{}, pluginsdk.ExactInteractionObservation{}, fmt.Errorf("plugin-fixture: read exact human interaction: %w", err)
	}
	interaction := observation.Interaction
	if interaction.ID != interactionID || interaction.TaskID != descriptor.TaskID || interaction.SessionID != descriptor.SessionID || interaction.Status != pluginsdk.InteractionStatusPending {
		return nil, nil, pluginsdk.ManagedAgentConversationDescriptor{}, pluginsdk.ExactInteractionObservation{}, fmt.Errorf("plugin-fixture: interaction is not pending in this managed conversation")
	}
	if observation.ResourceVersion == "" || observation.ResourceVersion != expectedVersion {
		return nil, nil, pluginsdk.ManagedAgentConversationDescriptor{}, pluginsdk.ExactInteractionObservation{}, fmt.Errorf("plugin-fixture: interaction resource version changed")
	}
	return exact, capability, descriptor, observation, nil
}

func (p *fixturePlugin) respondManagedConversationPermission(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	input, err := decodeManagedPermissionResponseInput(req.Body)
	if err != nil {
		return nil, err
	}
	_, capability, descriptor, _, err := p.managedInteractionTarget(ctx, req, input.InstanceKey, input.InteractionID, input.ExpectedResourceVersion)
	if err != nil {
		return nil, err
	}
	commands, ok := pluginsdk.HostExactInteractionCommands(p.Host())
	if !ok || commands == nil {
		return nil, fmt.Errorf("plugin-fixture: exact interaction command manager unavailable")
	}
	result, interaction, err := commands.RespondPermission(ctx, pluginsdk.ExactPermissionResponse{
		RequestID: input.RequestID, WorkspaceID: req.Context.WorkspaceID, InteractionID: input.InteractionID,
		ExpectedResourceVersion: input.ExpectedResourceVersion, OptionID: input.OptionID,
		HumanResponseReceiptID: input.HumanResponseReceiptID, Cancelled: input.Cancelled,
		ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: respond to managed conversation permission: %w", err)
	}
	response := managedCommandResponse(result, descriptor)
	if interaction != nil {
		response["interaction_id"] = interaction.ID
		response["interaction_status"] = interaction.Status
	}
	return encodeFixtureActionResponse(response)
}

type managedPermissionResponseInput struct {
	InstanceKey             string `json:"instance_key"`
	RequestID               string `json:"request_id"`
	InteractionID           string `json:"interaction_id"`
	ExpectedResourceVersion string `json:"expected_resource_version"`
	OptionID                string `json:"option_id"`
	HumanResponseReceiptID  string `json:"human_response_receipt_id"`
	Cancelled               bool   `json:"cancelled,omitempty"`
}

func decodeManagedPermissionResponseInput(body []byte) (managedPermissionResponseInput, error) {
	var input managedPermissionResponseInput
	if err := decodeFixtureActionBody(body, &input); err != nil {
		return input, fmt.Errorf("plugin-fixture: invalid managed conversation permission response: %w", err)
	}
	if input.InstanceKey == "" || input.RequestID == "" || input.InteractionID == "" || input.ExpectedResourceVersion == "" || input.HumanResponseReceiptID == "" || (!input.Cancelled && input.OptionID == "") || (input.Cancelled && input.OptionID != "") {
		return input, fmt.Errorf("plugin-fixture: invalid managed conversation permission response")
	}
	return input, nil
}

func (p *fixturePlugin) answerManagedConversationClarification(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		InstanceKey             string                            `json:"instance_key"`
		RequestID               string                            `json:"request_id"`
		InteractionID           string                            `json:"interaction_id"`
		ExpectedResourceVersion string                            `json:"expected_resource_version"`
		Answers                 []fixtureClarificationAnswerInput `json:"answers"`
		HumanResponseReceiptID  string                            `json:"human_response_receipt_id"`
	}
	if err := decodeFixtureActionBody(req.Body, &input); err != nil || input.InstanceKey == "" || input.RequestID == "" || input.InteractionID == "" || input.ExpectedResourceVersion == "" || input.HumanResponseReceiptID == "" || len(input.Answers) == 0 {
		return nil, fmt.Errorf("plugin-fixture: invalid managed conversation clarification response")
	}
	_, capability, descriptor, _, err := p.managedInteractionTarget(ctx, req, input.InstanceKey, input.InteractionID, input.ExpectedResourceVersion)
	if err != nil {
		return nil, err
	}
	commands, ok := pluginsdk.HostExactInteractionCommands(p.Host())
	if !ok || commands == nil {
		return nil, fmt.Errorf("plugin-fixture: exact interaction command manager unavailable")
	}
	answers := make([]pluginsdk.ClarificationAnswer, len(input.Answers))
	for index, answer := range input.Answers {
		answers[index] = pluginsdk.ClarificationAnswer{
			QuestionID: answer.QuestionID, SelectedOptions: answer.SelectedOptions, CustomText: answer.CustomText,
		}
	}
	result, interaction, err := commands.AnswerClarification(ctx, pluginsdk.ExactClarificationResponse{
		RequestID: input.RequestID, WorkspaceID: req.Context.WorkspaceID, InteractionID: input.InteractionID,
		ExpectedResourceVersion: input.ExpectedResourceVersion, Answers: answers,
		HumanResponseReceiptID: input.HumanResponseReceiptID,
		ApprovalRevision:       capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: answer managed conversation clarification: %w", err)
	}
	response := managedCommandResponse(result, descriptor)
	if interaction != nil {
		response["interaction_id"] = interaction.ID
		response["interaction_status"] = interaction.Status
	}
	return encodeFixtureActionResponse(response)
}

type fixtureClarificationAnswerInput struct {
	QuestionID      string   `json:"question_id"`
	SelectedOptions []string `json:"selected_options,omitempty"`
	CustomText      string   `json:"custom_text,omitempty"`
}
