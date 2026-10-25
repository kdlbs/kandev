// Guards fixture-package/manifest.yaml against drift: it must stay a valid,
// runtime-managed manifest that declares the id, webhook, and UI
// bundle path the e2e suite and `make e2e-plugin-package` depend on. See
// docs/plans/plugins/GRPC-CONTRACT.md §6.
package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
)

//go:embed fixture-package/manifest.yaml
var fixtureManifestYAML []byte

func TestFixtureManifest_ParsesAndValidates(t *testing.T) {
	m, err := manifest.Parse(fixtureManifestYAML)
	require.NoError(t, err)
	require.NoError(t, m.Validate())

	require.Equal(t, "kandev-plugin-e2e", m.ID)
	require.Equal(t, manifest.CurrentAPIVersion, m.APIVersion)
	require.Equal(t, "1.0.0", m.Version)
	require.True(t, m.IsManaged())
	require.Equal(t, "https://github.com/kdlbs/kandev-plugin-template", m.RepoURL)
	require.Equal(t, "/ui/bundle.js", m.UI.Bundle)
	require.True(t, m.HasEvent("task.created"))
	require.True(t, m.Capabilities.State)
	require.True(t, m.Capabilities.UserState)
	require.Equal(t, []string{"fixture-source-control"}, m.RepositoryProviders)
	require.Equal(t, "connection-status", m.Actions[0].Key)
	require.Equal(t, "workspace", m.Actions[0].ResourceScope)
	actions := make(map[string]manifest.Action, len(m.Actions))
	for _, action := range m.Actions {
		actions[action.Key] = action
	}
	require.Equal(t, "workspace", actions[repositoryInspectActionKey].ResourceScope)
	require.Equal(t, "workspace", actions[repositoryBranchesActionKey].ResourceScope)
	require.Equal(t, "workspace", actions[utilityDefaultAction].ResourceScope)
	require.Equal(t, "workspace", actions[utilityPreferenceAction].ResourceScope)
	require.Equal(t, "task", actions["link-pull-request"].ResourceScope)
	require.Equal(t, "workspace", actions[exactTaskCreateAction].ResourceScope)
	require.Equal(t, "workspace", actions[taskTreeDeleteAction].ResourceScope)
	require.Equal(t, "workspace", actions[managedConversationEnsureAction].ResourceScope)
	require.Contains(t, m.Capabilities.APIRead, "managed_agent_conversations")
	require.Contains(t, m.Capabilities.APIWrite, "managed_agent_conversations")
	require.Contains(t, m.Capabilities.APIRead, "tasks")
	require.Contains(t, m.Capabilities.APIRead, "sessions")
	require.Contains(t, m.Capabilities.APIRead, "interactions")
	require.Contains(t, m.Capabilities.APIWrite, "execution")
	require.Contains(t, m.Capabilities.APIWrite, "interactions")
	for _, key := range []string{
		"managed-conversation-status", "managed-conversation-inputs", "managed-conversation-enqueue",
		"managed-conversation-cancel", "managed-conversation-pause", "managed-conversation-resume",
		"managed-conversation-recover", "managed-conversation-permission-response",
		"managed-conversation-clarification-response",
	} {
		action, ok := actions[key]
		require.True(t, ok, "action %q must be declared", key)
		require.Equal(t, "workspace", action.ResourceScope)
		require.Greater(t, action.MaxBodyBytes, 0)
	}
	require.Len(t, m.ReferenceSources, 1)
	require.Equal(t, "fixture-pull-requests", m.ReferenceSources[0].Source)
	require.Equal(t, "fixture-source-control", m.ReferenceSources[0].Provider)
	require.Equal(t, "pull_request", m.ReferenceSources[0].Kind)
	require.Len(t, m.AgentTools, 1)
	require.Equal(t, "test_echo", m.AgentTools[0].Name)

	require.Len(t, m.Webhooks, 2)
	require.Equal(t, "test-hook", m.Webhooks[0].Key)
	require.Equal(t, "POST", m.Webhooks[0].Method)
	require.Equal(t, manifest.WebhookAccessAuthenticated, m.Webhooks[0].EffectiveAccess(m.APIVersion), "test-hook exercises the private (auth-gated) webhook path")
	require.Equal(t, "public-hook", m.Webhooks[1].Key)
	require.Equal(t, manifest.WebhookAccessPublic, m.Webhooks[1].EffectiveAccess(m.APIVersion), "public-hook exercises the anonymous auth-gate opt-in")
}

func TestManagedConversationEnsureReturnsOpaqueTaskAndSessionIDs(t *testing.T) {
	manager := &fixtureManagedConversationManager{
		descriptor: pluginsdk.ManagedAgentConversationDescriptor{
			TaskID: "opaque-task", SessionID: "opaque-session", WorkspaceID: "workspace-1",
			InstanceKey: "coordinator", Revision: 4,
		},
	}
	p := fixturePluginWithExactHost(manager, nil, nil, nil)

	response, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-ensure", `{"instance_key":"coordinator","agent_profile_id":"profile-1"}`))
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, "opaque-task", body["task_id"])
	require.Equal(t, "opaque-session", body["session_id"])
	require.Equal(t, float64(4), body["revision"])
	require.Equal(t, uint64(9), manager.ensureInput.ApprovalRevision)
	require.Equal(t, "manifest-v1", manager.ensureInput.ManifestDigest)
	require.Equal(t, "workspace-1", manager.ensureInput.WorkspaceID)
}

func TestManagedConversationStatusUsesCanonicalExactReadsAndReportsUnsupportedRecovery(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	queries := &fixtureExactQueries{
		task: pluginsdk.ExactTaskObservation{
			Task:            pluginsdk.Task{ID: "opaque-task", WorkspaceID: "workspace-1"},
			CanonicalStatus: "in_progress", SemanticActivity: "working", ExecutionState: "running",
			StatusKnown: true, ResourceVersion: "task-v7", ObservedAt: "task-time",
		},
		taskReceipt: pluginsdk.HostReadReceipt{WorkspaceID: "workspace-1", CapabilityRevision: 9, RequestID: "task-read", SnapshotVersion: "task-snapshot"},
		interactions: pluginsdk.ExactInteractionPage{Items: []pluginsdk.ExactInteractionObservation{{
			Interaction:     pluginsdk.Interaction{ID: "interaction-1", TaskID: "opaque-task", SessionID: "opaque-session", Kind: pluginsdk.InteractionKindPermission, Status: pluginsdk.InteractionStatusPending},
			ResourceVersion: "interaction-v3", ObservedAt: "interaction-time",
		}}, PageInfo: pluginsdk.ExactReadPageInfo{NextCursor: "cursor-2", HasMore: true, SnapshotVersion: "interaction-snapshot", Receipt: pluginsdk.HostReadReceipt{WorkspaceID: "workspace-1", CapabilityRevision: 9, SnapshotVersion: "interaction-snapshot"}}},
		sessions: pluginsdk.ExactSessionPage{Items: []pluginsdk.ExactSessionObservation{{
			Session:        pluginsdk.Session{ID: "opaque-session", TaskID: "opaque-task", State: "FAILED"},
			ExecutionState: "FAILED", ResourceVersion: "session-v5", ExecutionID: "execution-1",
		}}},
	}
	host := fixtureExactHost(manager, queries, nil, nil)
	host.capability.Operations[0] = pluginsdk.CapabilityOperation{
		Method: "RecoverSessionExact", Supported: false, Authorized: false, UnavailableReason: "execution_provider_unsupported",
	}
	p := &fixturePlugin{}
	p.SetHost(host)

	response, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-status", `{"instance_key":"coordinator"}`))
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, "in_progress", body["canonical_status"])
	require.Equal(t, "task-v7", body["task_resource_version"])
	require.Equal(t, "session-v5", body["session_resource_version"])
	require.Equal(t, "execution-1", body["execution_id"])
	require.Equal(t, "FAILED", body["session_state"])
	require.Equal(t, false, body["recovery_supported"])
	require.Equal(t, "execution_provider_unsupported", body["recovery_reason"])
	items, ok := body["pending_interactions"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	interaction := items[0].(map[string]any)
	require.Equal(t, "interaction-v3", interaction["resource_version"])
	require.Equal(t, "interaction-snapshot", body["interactions_snapshot_version"])
	require.Equal(t, true, body["interactions_has_more"])
	require.Equal(t, "opaque-task", queries.getTaskQuery.TaskID)
	require.Equal(t, "workspace-1", queries.getTaskQuery.WorkspaceID)
	require.Equal(t, []string{"opaque-task"}, queries.interactionQuery.Filter.TaskIDs)
	require.Equal(t, []string{"opaque-session"}, queries.interactionQuery.Filter.SessionIDs)
	require.Equal(t, []string{"opaque-task"}, queries.sessionQuery.Filter.TaskIDs)
	unsupported, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-recover", `{"instance_key":"coordinator","request_id":"recover-unsupported","idempotency_key":"recover-unsupported","expected_revision":6,"expected_session_resource_version":"session-v5","expected_execution_id":"execution-1"}`))
	require.NoError(t, err)
	var unsupportedBody map[string]any
	require.NoError(t, json.Unmarshal(unsupported.Body, &unsupportedBody))
	require.Equal(t, "UNSUPPORTED", unsupportedBody["status"])
	require.Equal(t, "execution_provider_unsupported", unsupportedBody["reason"])
}

func TestManagedConversationStatusContinuesInteractionSnapshot(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	queries := &fixtureExactQueries{
		task: pluginsdk.ExactTaskObservation{Task: pluginsdk.Task{ID: "opaque-task", WorkspaceID: "workspace-1"}, StatusKnown: true},
		interactions: pluginsdk.ExactInteractionPage{PageInfo: pluginsdk.ExactReadPageInfo{
			NextCursor: "cursor-next", HasMore: true, SnapshotVersion: "snapshot-1",
		}},
	}
	p := fixturePluginWithExactHost(manager, queries, nil, nil)

	first, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-status", `{"instance_key":"coordinator"}`))
	require.NoError(t, err)
	var firstBody map[string]any
	require.NoError(t, json.Unmarshal(first.Body, &firstBody))
	receipt := firstBody["interactions_read_receipt"].(map[string]any)
	requestID := receipt["request_id"].(string)

	continuation, err := json.Marshal(map[string]any{
		"instance_key": "coordinator", "interactions_request_id": requestID,
		"interactions_snapshot_version": "snapshot-1", "interactions_cursor": "cursor-next",
	})
	require.NoError(t, err)
	_, err = p.HandleAction(context.Background(), &pluginsdk.PluginActionRequest{
		ActionKey: "managed-conversation-status", Context: pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-1"}, Body: continuation,
	})
	require.NoError(t, err)
	require.Len(t, queries.interactionQueries, 2)
	require.Equal(t, requestID, queries.interactionQueries[1].RequestID)
	require.Equal(t, "snapshot-1", queries.interactionQueries[1].Page.SnapshotVersion)
	require.Equal(t, "cursor-next", queries.interactionQueries[1].Page.Cursor)
}

func TestManagedConversationStatusReportsUnsupportedManagedProvider(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	host := fixtureExactHost(manager, nil, nil, nil)
	host.capability.Operations[1] = pluginsdk.CapabilityOperation{
		Method: "GetManagedAgentConversationStatusExact", Supported: false,
		UnavailableReason: "managed_conversation_service_unavailable",
	}
	p := &fixturePlugin{}
	p.SetHost(host)

	response, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-status", `{"instance_key":"coordinator"}`))
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, false, body["managed_conversation_supported"])
	require.Equal(t, "managed_conversation_service_unavailable", body["managed_conversation_reason"])
	require.Equal(t, "managed_conversation_unsupported", body["task_status_reason"])
	require.Empty(t, manager.getCalls)
}

func TestManagedConversationInputActionsPreserveOrderingAndExactVersions(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	p := fixturePluginWithExactHost(manager, nil, nil, nil)

	list, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-inputs", `{"instance_key":"coordinator","sequence_cursor":3,"limit":2}`))
	require.NoError(t, err)
	var listed struct {
		Inputs []pluginsdk.ManagedAgentInputReceipt `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal(list.Body, &listed))
	require.Len(t, listed.Inputs, 2)
	require.Equal(t, uint64(4), listed.Inputs[0].Sequence)
	require.Equal(t, uint64(5), listed.Inputs[1].Sequence)
	require.Equal(t, uint64(3), manager.listQuery.SequenceCursor)
	require.Equal(t, uint32(2), manager.listQuery.Limit)
	require.Equal(t, uint64(9), manager.listQuery.ApprovalRevision)
	require.Equal(t, "manifest-v1", manager.listQuery.ManifestDigest)

	enqueued, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-enqueue", `{"instance_key":"coordinator","request_id":"request-1","idempotency_key":"idem-1","expected_revision":6,"occurrence_key":"occurrence-1","payload":"hello"}`))
	require.NoError(t, err)
	var enqueueBody map[string]any
	require.NoError(t, json.Unmarshal(enqueued.Body, &enqueueBody))
	require.Equal(t, "APPLIED", enqueueBody["status"])
	require.Equal(t, "input-1", enqueueBody["host_input_id"])
	require.Equal(t, uint64(6), manager.enqueueInput.ExpectedConversationRevision)
	require.Equal(t, uint64(9), manager.enqueueInput.ApprovalRevision)
	require.Equal(t, "manifest-v1", manager.enqueueInput.ManifestDigest)
	require.Equal(t, "occurrence-1", manager.enqueueInput.OccurrenceKey)
}

func TestManagedConversationLifecycleAndHumanResponsesUseExactManagers(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	execution := &fixtureExecutionCommands{}
	interactions := &fixtureInteractionCommands{}
	p := fixturePluginWithExactHost(manager, &fixtureExactQueries{}, execution, interactions)

	_, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-pause", `{"instance_key":"coordinator","request_id":"pause-1","idempotency_key":"pause-1","expected_revision":6}`))
	require.NoError(t, err)
	require.True(t, manager.pauseInput.Paused)
	require.Equal(t, uint64(6), manager.pauseInput.ExpectedRevision)
	require.Equal(t, uint64(9), manager.pauseInput.ApprovalRevision)
	require.Equal(t, "manifest-v1", manager.pauseInput.ManifestDigest)
	_, err = p.HandleAction(context.Background(), fixtureAction("managed-conversation-resume", `{"instance_key":"coordinator","request_id":"resume-1","idempotency_key":"resume-1","expected_revision":6}`))
	require.NoError(t, err)
	require.False(t, manager.pauseInput.Paused)

	_, err = p.HandleAction(context.Background(), fixtureAction("managed-conversation-cancel", `{"instance_key":"coordinator","request_id":"cancel-1","idempotency_key":"cancel-1","expected_revision":6,"host_input_id":"input-1","expected_execution_id":"execution-1"}`))
	require.NoError(t, err)
	require.Equal(t, "input-1", manager.cancelInput.HostInputID)
	require.Equal(t, "execution-1", manager.cancelInput.ExpectedExecutionID)
	require.Equal(t, uint64(6), manager.cancelInput.ExpectedConversationRevision)
	require.Equal(t, uint64(9), manager.cancelInput.ApprovalRevision)

	_, err = p.HandleAction(context.Background(), fixtureAction("managed-conversation-recover", `{"instance_key":"coordinator","request_id":"recover-1","idempotency_key":"recover-1","expected_revision":6,"expected_session_resource_version":"session-v5","expected_execution_id":"execution-1"}`))
	require.NoError(t, err)
	require.Equal(t, "resume", execution.recovery.Action)
	require.Equal(t, "opaque-task", execution.recovery.TaskID)
	require.Equal(t, "opaque-session", execution.recovery.SessionID)
	require.Equal(t, "session-v5", execution.recovery.ExpectedSessionResourceVersion)
	require.Equal(t, "execution-1", execution.recovery.ExpectedExecutionID)
	require.Equal(t, uint64(9), execution.recovery.ApprovalRevision)

	_, err = p.HandleAction(context.Background(), fixtureAction("managed-conversation-permission-response", `{"instance_key":"coordinator","request_id":"permission-1","interaction_id":"interaction-1","expected_resource_version":"interaction-v3","option_id":"allow-once","human_response_receipt_id":"receipt-1"}`))
	require.NoError(t, err)
	require.Equal(t, "receipt-1", interactions.permission.HumanResponseReceiptID)
	require.Equal(t, "interaction-v3", interactions.permission.ExpectedResourceVersion)
	require.Equal(t, "allow-once", interactions.permission.OptionID)
	require.Equal(t, uint64(9), interactions.permission.ApprovalRevision)
	require.Equal(t, "manifest-v1", interactions.permission.ManifestDigest)

	_, err = p.HandleAction(context.Background(), fixtureAction("managed-conversation-clarification-response", `{"instance_key":"coordinator","request_id":"clarification-1","interaction_id":"interaction-1","expected_resource_version":"interaction-v3","human_response_receipt_id":"receipt-2","answers":[{"question_id":"q1","selected_options":["option-1"]}]}`))
	require.NoError(t, err)
	require.Equal(t, "receipt-2", interactions.clarification.HumanResponseReceiptID)
	require.Equal(t, "q1", interactions.clarification.Answers[0].QuestionID)
	require.Equal(t, "interaction-v3", interactions.clarification.ExpectedResourceVersion)
	require.Equal(t, uint64(9), interactions.clarification.ApprovalRevision)
	require.Equal(t, "manifest-v1", interactions.clarification.ManifestDigest)
}

func TestManagedConversationRecoveryRejectsStaleConversationRevision(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	execution := &fixtureExecutionCommands{}
	p := fixturePluginWithExactHost(manager, &fixtureExactQueries{}, execution, &fixtureInteractionCommands{})

	response, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-recover", `{"instance_key":"coordinator","request_id":"recover-stale","idempotency_key":"recover-stale","expected_revision":5,"expected_session_resource_version":"session-v5","expected_execution_id":"execution-1"}`))
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, "CONFLICT", body["status"])
	require.Equal(t, "conversation_revision_changed", body["reason"])
	require.Empty(t, execution.recovery.Action)
}

func TestManagedConversationActionsRejectUnknownFieldsAndMissingExactAPIs(t *testing.T) {
	manager := &fixtureManagedConversationManager{descriptor: fixtureConversationDescriptor()}
	p := fixturePluginWithExactHost(manager, nil, nil, nil)
	_, err := p.HandleAction(context.Background(), fixtureAction("managed-conversation-pause", `{"instance_key":"coordinator","request_id":"r1","idempotency_key":"i1","expected_revision":6,"force":true}`))
	require.Error(t, err)
	require.Empty(t, manager.pauseInput.RequestID)

	noQueries := fixturePluginWithExactHost(manager, nil, nil, nil)
	response, err := noQueries.HandleAction(context.Background(), fixtureAction("managed-conversation-status", `{"instance_key":"coordinator"}`))
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &body))
	require.Equal(t, "exact_query_manager_unavailable", body["task_status_reason"])
	require.Equal(t, "exact_query_manager_unavailable", body["interactions_reason"])

	commands := &fixtureInteractionCommands{}
	staleResponsePlugin := fixturePluginWithExactHost(manager, &fixtureExactQueries{}, nil, commands)
	_, err = staleResponsePlugin.HandleAction(context.Background(), fixtureAction("managed-conversation-permission-response", `{"instance_key":"coordinator","request_id":"permission-stale","interaction_id":"interaction-1","expected_resource_version":"interaction-v2","option_id":"allow-once","human_response_receipt_id":"receipt-stale"}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "resource version changed")
	require.Empty(t, commands.permission.InteractionID)
}

func fixtureAction(action, body string) *pluginsdk.PluginActionRequest {
	return &pluginsdk.PluginActionRequest{ActionKey: action, Context: pluginsdk.VerifiedActionContext{WorkspaceID: "workspace-1"}, Body: []byte(body)}
}

func fixtureConversationDescriptor() pluginsdk.ManagedAgentConversationDescriptor {
	return pluginsdk.ManagedAgentConversationDescriptor{
		TaskID: "opaque-task", SessionID: "opaque-session", WorkspaceID: "workspace-1",
		InstanceKey: "coordinator", Revision: 6, AgentProfileID: "profile-1",
	}
}

func fixturePluginWithExactHost(manager *fixtureManagedConversationManager, queries *fixtureExactQueries, execution *fixtureExecutionCommands, interactions *fixtureInteractionCommands) *fixturePlugin {
	p := &fixturePlugin{}
	p.SetHost(fixtureExactHost(manager, queries, execution, interactions))
	return p
}

func fixtureExactHost(manager *fixtureManagedConversationManager, queries *fixtureExactQueries, execution *fixtureExecutionCommands, interactions *fixtureInteractionCommands) *fixtureHost {
	host := &fixtureHost{manager: manager, queries: queries, execution: execution, interactions: interactions}
	host.capability = &pluginsdk.CapabilityContext{
		WorkspaceID: "workspace-1", ApprovalRevision: 9, ManifestDigest: "manifest-v1",
		Operations: []pluginsdk.CapabilityOperation{
			{Method: "RecoverSessionExact", Supported: true, Authorized: true},
			{Method: "GetManagedAgentConversationStatusExact", Supported: true, Authorized: true},
			{Method: "GetTaskExact", Supported: true, Authorized: true},
			{Method: "ListSessionsExact", Supported: true, Authorized: true},
			{Method: "ListPendingInteractionsExact", Supported: true, Authorized: true},
			{Method: "GetInteractionExact", Supported: true, Authorized: true},
		},
	}
	return host
}

type fixtureHost struct {
	pluginsdk.Host
	manager      *fixtureManagedConversationManager
	queries      *fixtureExactQueries
	execution    *fixtureExecutionCommands
	interactions *fixtureInteractionCommands
	capability   *pluginsdk.CapabilityContext
}

func (h *fixtureHost) GetCapabilityContext(context.Context, string) (*pluginsdk.CapabilityContext, error) {
	return h.capability, nil
}

func (h *fixtureHost) UpdateTaskExact(context.Context, pluginsdk.ExactTaskUpdate) (*pluginsdk.CommandResult, *pluginsdk.Task, error) {
	return nil, nil, nil
}

func (h *fixtureHost) ManagedAgentConversations() pluginsdk.ManagedAgentConversationManager {
	return h.manager
}
func (h *fixtureHost) ExactQueries() pluginsdk.ExactQueryManager {
	if h.queries == nil {
		return nil
	}
	return h.queries
}
func (h *fixtureHost) ExecutionCommands() pluginsdk.ExactExecutionCommandManager {
	if h.execution == nil {
		return nil
	}
	return h.execution
}
func (h *fixtureHost) ExactInteractionCommands() pluginsdk.ExactInteractionCommandManager {
	if h.interactions == nil {
		return nil
	}
	return h.interactions
}

type fixtureManagedConversationManager struct {
	pluginsdk.ManagedAgentConversationManager
	descriptor   pluginsdk.ManagedAgentConversationDescriptor
	ensureInput  pluginsdk.ManagedAgentConversationSpec
	pauseInput   pluginsdk.ManagedAgentConversationPause
	listQuery    pluginsdk.ManagedAgentInputListQuery
	enqueueInput pluginsdk.ManagedAgentInputEnqueue
	cancelInput  pluginsdk.ManagedAgentInputCancel
	getCalls     []pluginsdk.ManagedAgentConversationQuery
}

func (m *fixtureManagedConversationManager) Ensure(_ context.Context, input pluginsdk.ManagedAgentConversationSpec) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentConversationDescriptor, error) {
	m.ensureInput = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied, Receipt: &pluginsdk.CommandReceipt{ID: "ensure-receipt", Status: pluginsdk.CommandApplied}}, m.descriptor, nil
}
func (m *fixtureManagedConversationManager) Get(_ context.Context, query pluginsdk.ManagedAgentConversationQuery) (pluginsdk.ManagedAgentConversationDescriptor, error) {
	m.getCalls = append(m.getCalls, query)
	return m.descriptor, nil
}
func (m *fixtureManagedConversationManager) SetPaused(_ context.Context, input pluginsdk.ManagedAgentConversationPause) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentConversationDescriptor, error) {
	m.pauseInput = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied}, m.descriptor, nil
}
func (m *fixtureManagedConversationManager) ListInputs(_ context.Context, query pluginsdk.ManagedAgentInputListQuery) (pluginsdk.ManagedAgentInputPage, error) {
	m.listQuery = query
	return pluginsdk.ManagedAgentInputPage{Inputs: []pluginsdk.ManagedAgentInputReceipt{{HostInputID: "input-4", Sequence: 4}, {HostInputID: "input-5", Sequence: 5}}, NextSequenceCursor: 5}, nil
}
func (m *fixtureManagedConversationManager) EnqueueInput(_ context.Context, input pluginsdk.ManagedAgentInputEnqueue) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentInputReceipt, error) {
	m.enqueueInput = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied, Receipt: &pluginsdk.CommandReceipt{ID: "enqueue-receipt", Status: pluginsdk.CommandApplied}}, pluginsdk.ManagedAgentInputReceipt{HostInputID: "input-1", Sequence: 7, State: pluginsdk.ManagedAgentInputAccepted}, nil
}
func (m *fixtureManagedConversationManager) CancelInput(_ context.Context, input pluginsdk.ManagedAgentInputCancel) (*pluginsdk.CommandResult, pluginsdk.ManagedAgentInputReceipt, error) {
	m.cancelInput = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied, Receipt: &pluginsdk.CommandReceipt{ID: "cancel-receipt", Status: pluginsdk.CommandApplied}}, pluginsdk.ManagedAgentInputReceipt{HostInputID: input.HostInputID, Sequence: 7, State: pluginsdk.ManagedAgentInputCancelled}, nil
}

type fixtureExactQueries struct {
	pluginsdk.ExactQueryManager
	task               pluginsdk.ExactTaskObservation
	taskReceipt        pluginsdk.HostReadReceipt
	interactions       pluginsdk.ExactInteractionPage
	sessions           pluginsdk.ExactSessionPage
	getTaskQuery       pluginsdk.ExactTaskGetQuery
	interactionQuery   pluginsdk.ExactInteractionQuery
	interactionQueries []pluginsdk.ExactInteractionQuery
	sessionQuery       pluginsdk.ExactSessionQuery
	sessionQueries     []pluginsdk.ExactSessionQuery
}

func (q *fixtureExactQueries) GetTask(_ context.Context, query pluginsdk.ExactTaskGetQuery) (pluginsdk.ExactTaskObservation, pluginsdk.HostReadReceipt, error) {
	q.getTaskQuery = query
	return q.task, q.taskReceipt, nil
}
func (q *fixtureExactQueries) ListSessions(_ context.Context, query pluginsdk.ExactSessionQuery) (pluginsdk.ExactSessionPage, error) {
	q.sessionQuery = query
	q.sessionQueries = append(q.sessionQueries, query)
	page := q.sessions
	page.PageInfo.Receipt.RequestID = query.RequestID
	page.PageInfo.Receipt.WorkspaceID = query.WorkspaceID
	return page, nil
}
func (q *fixtureExactQueries) ListPendingInteractions(_ context.Context, query pluginsdk.ExactInteractionQuery) (pluginsdk.ExactInteractionPage, error) {
	q.interactionQuery = query
	q.interactionQueries = append(q.interactionQueries, query)
	page := q.interactions
	page.PageInfo.Receipt.RequestID = query.RequestID
	page.PageInfo.Receipt.WorkspaceID = query.WorkspaceID
	page.PageInfo.Receipt.SnapshotVersion = page.PageInfo.SnapshotVersion
	return page, nil
}
func (q *fixtureExactQueries) GetInteraction(_ context.Context, query pluginsdk.ExactInteractionGetQuery) (pluginsdk.ExactInteractionObservation, pluginsdk.HostReadReceipt, error) {
	return pluginsdk.ExactInteractionObservation{
		Interaction:     pluginsdk.Interaction{ID: query.InteractionID, TaskID: "opaque-task", SessionID: "opaque-session", Kind: pluginsdk.InteractionKindPermission, Status: pluginsdk.InteractionStatusPending},
		ResourceVersion: "interaction-v3",
	}, pluginsdk.HostReadReceipt{WorkspaceID: query.WorkspaceID, CapabilityRevision: 9, RequestID: query.RequestID, SnapshotVersion: "interaction-read"}, nil
}

type fixtureExecutionCommands struct {
	pluginsdk.ExactExecutionCommandManager
	recovery pluginsdk.ExactSessionRecoveryCommand
}

func (m *fixtureExecutionCommands) RecoverSession(_ context.Context, input pluginsdk.ExactSessionRecoveryCommand) (*pluginsdk.CommandResult, pluginsdk.ExactTaskRunResult, error) {
	m.recovery = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied, Receipt: &pluginsdk.CommandReceipt{ID: "recovery-receipt", Status: pluginsdk.CommandApplied}}, pluginsdk.ExactTaskRunResult{SessionID: input.SessionID, ExecutionID: "execution-2"}, nil
}

type fixtureInteractionCommands struct {
	pluginsdk.ExactInteractionCommandManager
	permission    pluginsdk.ExactPermissionResponse
	clarification pluginsdk.ExactClarificationResponse
}

func (m *fixtureInteractionCommands) RespondPermission(_ context.Context, input pluginsdk.ExactPermissionResponse) (*pluginsdk.CommandResult, *pluginsdk.Interaction, error) {
	m.permission = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied, Receipt: &pluginsdk.CommandReceipt{ID: "permission-receipt", Status: pluginsdk.CommandApplied}}, &pluginsdk.Interaction{ID: input.InteractionID, Status: pluginsdk.InteractionStatusApproved}, nil
}
func (m *fixtureInteractionCommands) AnswerClarification(_ context.Context, input pluginsdk.ExactClarificationResponse) (*pluginsdk.CommandResult, *pluginsdk.Interaction, error) {
	m.clarification = input
	return &pluginsdk.CommandResult{Status: pluginsdk.CommandApplied, Receipt: &pluginsdk.CommandReceipt{ID: "clarification-receipt", Status: pluginsdk.CommandApplied}}, &pluginsdk.Interaction{ID: input.InteractionID, Status: pluginsdk.InteractionStatusAnswered}, nil
}

func TestFixtureManifest_DeclaresHostPlatformExecutable(t *testing.T) {
	m, err := manifest.Parse(fixtureManifestYAML)
	require.NoError(t, err)

	// The Makefile's `e2e-plugin-package` target only ever builds/packs for
	// the host platform, but the committed manifest lists every platform
	// the fixture might run on in CI (linux/darwin/windows, amd64/arm64).
	for platformKey, execPath := range map[string]string{
		"linux-amd64":   "server/plugin-linux-amd64",
		"linux-arm64":   "server/plugin-linux-arm64",
		"darwin-amd64":  "server/plugin-darwin-amd64",
		"darwin-arm64":  "server/plugin-darwin-arm64",
		"windows-amd64": "server/plugin-windows-amd64.exe",
	} {
		require.Equal(t, execPath, m.Runtime.Executables[platformKey], "platform %s", platformKey)
	}
}
