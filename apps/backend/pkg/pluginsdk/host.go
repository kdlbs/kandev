// host.go implements the Host side of the kandev.plugin.v1.Host service
// (§3 of docs/plans/plugins/GRPC-CONTRACT.md) in both directions:
//
//   - grpcHostClient: used inside the plugin subprocess. Wraps a
//     pluginv1.HostClient dialed over the go-plugin broker (see serve.go)
//     and satisfies the Go-native Host interface that Serve injects into
//     the author's Plugin.
//   - grpcHostServer: used inside kandev. Wraps kandev's own Go-native Host
//     implementation (state store, secrets, event bus) and satisfies the
//     generated pluginv1.HostServer interface so it can be registered on
//     the broker-served grpc.Server that GRPCPlugin.GRPCClient spins up
//     (see serve.go's "Host injection" section).
//
// Both directions share the same Go-native Host interface and the same
// proto conversion helpers in types.go, so kandev's runtime manager
// implements Host exactly once and gets both the client and server wiring
// for free via GRPCPlugin.
package pluginsdk

import (
	"context"

	pluginv1 "github.com/kandev/kandev/proto/kandev/plugin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Host is the set of operations kandev exposes back to a running plugin,
// per §3's Host service. On the plugin side, Serve injects an
// SDK-provided implementation that proxies these calls to kandev over the
// go-plugin broker. On the kandev side, the runtime manager provides its
// own Go-native implementation of this same interface (backed by the real
// state store / secrets / event bus) and hands it to GRPCPlugin.Host; the
// SDK wraps it into the generated pluginv1.HostServer for registration.
type Host interface {
	// GetState looks up a single state entry. found is false, err is nil
	// when the key does not exist.
	GetState(ctx context.Context, scope, scopeID, key string) (value map[string]any, found bool, err error)

	// SetState upserts a single state entry.
	SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error

	// DeleteState removes a single state entry. Deleting a missing key is
	// not an error.
	DeleteState(ctx context.Context, scope, scopeID, key string) error

	// ListState returns every state entry for a scope/scopeID pair.
	ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error)

	// GetConfig returns the plugin's own operator-editable config: the
	// values set in kandev's Settings > Plugins > <plugin> page against the
	// manifest's config_schema. Returns an empty (non-nil) map when no
	// config has been set yet. Ungated — a plugin can always read its own
	// config, secret values included. Plugins should re-read config at
	// startup: kandev restarts a running plugin when its config changes.
	GetConfig(ctx context.Context) (map[string]any, error)

	// RevealSecret resolves an operator-provided secret reference (e.g. a
	// ref placed in config pointing at a shared kandev secret) to its
	// cleartext value. For secrets the plugin itself owns, use
	// GetSecret/SetSecret/DeleteSecret instead.
	RevealSecret(ctx context.Context, ref string) (string, error)

	// GetSecret reads a plugin-owned secret previously stored with
	// SetSecret. found is false, err is nil when the key was never set.
	// Requires the `secrets` capability.
	GetSecret(ctx context.Context, key string) (value string, found bool, err error)

	// SetSecret upserts a plugin-owned secret into kandev's encrypted
	// vault. Keys are namespaced to this plugin server-side — a plugin can
	// never read or write another plugin's (or kandev's) secrets through
	// this API. Keys must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,127}. Requires
	// the `secrets` capability.
	SetSecret(ctx context.Context, key, value string) error

	// DeleteSecret removes a plugin-owned secret. Deleting a missing key
	// is not an error. Requires the `secrets` capability.
	DeleteSecret(ctx context.Context, key string) error

	// EmitEvent publishes a plugin-originated event onto kandev's bus.
	EmitEvent(ctx context.Context, name string, payload map[string]any) error

	// Tasks returns the reader for the Host data API's task RPCs
	// (capability api_read:tasks per ADR 0043).
	Tasks() TaskReader

	// Sessions returns the reader for the Host data API's session and
	// session-code-stats RPCs (capability api_read:sessions).
	Sessions() SessionReader

	// Workspaces returns the reader for the Host data API's workspace RPCs
	// (capability api_read:workspaces).
	Workspaces() WorkspaceReader

	// Workflows returns the reader for the Host data API's workflow and
	// workflow-step RPCs (capability api_read:workflows).
	Workflows() WorkflowReader

	// AgentProfiles returns the reader for the Host data API's agent
	// profile RPCs (capability api_read:agent_profiles).
	AgentProfiles() AgentProfileReader

	// Repositories returns the reader for the Host data API's repository
	// RPCs (capability api_read:repositories).
	Repositories() RepositoryReader

	// Messages returns the accessor for the Host data API's message RPCs.
	// List (capability api_read:messages) reads historical user/agent
	// conversation content; Send (capability api_write:messages) delivers a
	// prompt to a task session.
	Messages() MessageReader

	// InvokeUtilityAgent runs a one-shot, non-interactive completion. With no
	// options, or an empty ProfileID, it uses the platform default utility
	// profile. A non-empty ProfileID selects that profile for this call only.
	// Requires the `agent_invoke` capability and returns gRPC FailedPrecondition
	// for a missing or ineligible profile.
	InvokeUtilityAgent(ctx context.Context, prompt string, options ...UtilityAgentOptions) (string, error)
}

// UtilityAgentOptions contains per-call utility completion options. ProfileID
// is an agent-profile ID, not a utility-agent record ID. An empty value uses
// the platform default.
type UtilityAgentOptions struct {
	ProfileID string
}

// TaskReader is the accessor behind Host.Tasks(), mirroring the Host data
// API's ListTasks/GetTask/CreateTask/UpdateTask RPCs (ADR 0043). Reads
// (List/Get) require api_read:tasks; writes (Create/Update) require
// api_write:tasks — the two capabilities gate independently, so a plugin may
// declare one without the other. (The name is kept for source stability with
// already-shipped read-only plugins; the interface now also writes.)
type TaskReader interface {
	// List returns tasks matching filter, newest page first per page.
	List(ctx context.Context, filter TaskFilter, page Page) ([]Task, *PageInfo, error)

	// Get returns a single task by id.
	Get(ctx context.Context, id string) (*Task, error)

	// Create creates a task through kandev's task service (so task.* events
	// fire and WS clients update) and returns the created task. Requires
	// api_write:tasks.
	Create(ctx context.Context, in CreateTaskInput) (*Task, error)

	// Update mutates a conservative field surface of an existing task
	// (title/description/state) and returns the updated task. Requires
	// api_write:tasks. WorkflowStepID is rejected when present — use Move to
	// transition a task between workflow steps.
	Update(ctx context.Context, in UpdateTaskInput) (*Task, error)

	// Move transitions a task to a workflow step through the same path the
	// board's own move uses (validation, WIP admission, task.moved
	// publication, auto-start gates, queue reconciliation) — unlike Update,
	// which rejects a workflow step change. Requires api_write:tasks.
	Move(ctx context.Context, in MoveTaskInput) (*MoveTaskOutcome, error)
}

// SessionReader is the read-only accessor behind Host.Sessions(), mirroring
// the Host data API's ListSessions/ListSessionCodeStats RPCs.
type SessionReader interface {
	// List returns sessions matching filter.
	List(ctx context.Context, filter SessionFilter, page Page) ([]Session, *PageInfo, error)

	// CodeStats returns computed per-session code-change stats matching
	// filter. SessionCodeStats is a stable, computed shape — never raw
	// commit/snapshot rows.
	CodeStats(ctx context.Context, filter SessionFilter, page Page) ([]SessionCodeStats, *PageInfo, error)
}

// WorkspaceReader is the read-only accessor behind Host.Workspaces(),
// mirroring the Host data API's ListWorkspaces RPC.
type WorkspaceReader interface {
	List(ctx context.Context, page Page) ([]Workspace, *PageInfo, error)
}

// WorkflowReader is the read-only accessor behind Host.Workflows(),
// mirroring the Host data API's ListWorkflows/ListWorkflowSteps RPCs.
type WorkflowReader interface {
	// List returns workflows for workspaceID.
	List(ctx context.Context, workspaceID string, page Page) ([]Workflow, *PageInfo, error)

	// ListSteps returns the steps for workflowID, in position order.
	ListSteps(ctx context.Context, workflowID string) ([]WorkflowStep, error)
}

// AgentProfileReader is the read-only accessor behind Host.AgentProfiles(),
// mirroring the Host data API's ListAgentProfiles RPC.
type AgentProfileReader interface {
	List(ctx context.Context, page Page) ([]AgentProfile, *PageInfo, error)
}

// ExecutorProfileHost is an optional Host extension. It is kept separate from
// Host so existing host implementations remain source-compatible.
type ExecutorProfileHost interface {
	ExecutorProfiles() ExecutorProfileReader
}

type ExecutorProfileReader interface {
	List(ctx context.Context, page Page) ([]ExecutorProfile, *PageInfo, error)
}

// ExecutorProfiles returns the optional executor-profile reader.
func ExecutorProfiles(host Host) (ExecutorProfileReader, bool) {
	provider, ok := host.(ExecutorProfileHost)
	if !ok {
		return nil, false
	}
	return provider.ExecutorProfiles(), true
}

// RepositoryReader is the read-only accessor behind Host.Repositories(),
// mirroring the Host data API's ListRepositories RPC.
type RepositoryReader interface {
	// List returns repositories for workspaceID.
	List(ctx context.Context, workspaceID string, page Page) ([]Repository, *PageInfo, error)
}

// MessageReader is the accessor behind Host.Messages(), mirroring the Host
// data API's ListMessages/SendMessage RPCs. List (api_read:messages) reads
// historical conversation content filtered by session, task, and/or time
// range; Send (api_write:messages) delivers a prompt to a task session. The
// two capabilities gate independently. (The name is kept for source stability
// with already-shipped read-only plugins; the interface now also writes.)
type MessageReader interface {
	// List returns messages matching filter, oldest first within a page.
	List(ctx context.Context, filter MessageFilter, page Page) ([]Message, *PageInfo, error)

	// Send delivers text as a prompt to a task session through kandev's
	// orchestrator (the same delivery path message_task uses), recording a
	// user message stamped "plugin:<id>". sessionID may be empty to target the
	// task's primary session. Returns the target session and a dispatch status
	// ("queued" | "sent" | "started"). Requires api_write:messages.
	Send(ctx context.Context, taskID, sessionID, text string) (*MessageDispatch, error)
}

// PluginOwnedTaskTreeHost is an optional Host extension for previewing task
// trees whose source provenance matches the caller. Delete remains in the
// compatibility interface, but the current Kandev Host denies it because the
// RPC cannot carry a native Human's one-use task-deletion confirmation.
type PluginOwnedTaskTreeHost interface {
	PluginOwnedTaskTrees() PluginOwnedTaskTreeManager
}

type PluginOwnedTaskTreeManager interface {
	Preview(ctx context.Context, rootTaskID string) ([]Task, error)
	// Delete is retained for wire compatibility. The current Kandev Host returns
	// PermissionDenied with native_human_confirmation_required; direct plugins
	// to the native task UI when a person requests deletion.
	Delete(ctx context.Context, rootTaskID string) ([]string, error)
}

// The third optional Host extension, InteractionHost (pending agent
// interactions and their responses), lives in interactions.go.

// PluginOwnedTaskTrees returns the optional provenance-safe task-tree manager.
func PluginOwnedTaskTrees(host Host) (PluginOwnedTaskTreeManager, bool) {
	manager, ok := host.(PluginOwnedTaskTreeHost)
	if !ok {
		return nil, false
	}
	return manager.PluginOwnedTaskTrees(), true
}

// ── Agent conversation host extension ────────────────────────────────────

// AgentConversationHost is an optional Host extension for managing workspace
// agent conversations. It is kept separate from Host so existing host
// implementations remain source-compatible.
type AgentConversationHost interface {
	AgentConversations() AgentConversationManager
}

// AgentConversationManager is the interface for managing workspace agent
// conversations. It lets a plugin create (or find), dispatch to, and
// delete a hidden workflowless ephemeral task/session per
// (plugin_id, workspace_id, conversation_key).
type AgentConversationManager interface {
	// Ensure creates or repairs one conversation per (workspace_id, conversation_key).
	// Returns the existing descriptor when one already exists for this
	// plugin/workspace/key. Returns a typed configuration-required result
	// (status="configuration_required") when the referenced agent profile is
	// missing, disabled, or incompatible — the conversation is neither created
	// nor dispatched until the operator resolves the profile.
	Ensure(ctx context.Context, spec AgentConversationSpec) (AgentConversationDescriptor, string, error)

	// Dispatch sends text to an ensured conversation. OccurrenceKey provides
	// stable idempotency: a key that was already claimed returns the prior
	// dispatch result. Returns status "duplicate_occurrence" when
	// occurrence_key matches a previously dispatched occurrence (same session
	// for in-flight turns, skipped for busy-session coalesced drops).
	// Returns "skipped_busy" when the session is mid-turn and the dispatch
	// was coalesced rather than queued.
	Dispatch(ctx context.Context, workspaceID, conversationKey, text, occurrenceKey string) (AgentConversationDispatch, error)

	// Delete removes all conversations matching the workspace and key owned
	// by this plugin. Returns the count of deleted conversations.
	Delete(ctx context.Context, workspaceID, conversationKey string) (int32, error)
}

// AgentConversations returns the optional agent conversation manager.
func AgentConversations(host Host) (AgentConversationManager, bool) {
	manager, ok := host.(AgentConversationHost)
	if !ok {
		return nil, false
	}
	return manager.AgentConversations(), true
}

// newHostClient wraps a *grpc.ClientConn (dialed over the go-plugin broker)
// as a Go-native Host implementation.
func newHostClient(conn *grpc.ClientConn) Host {
	return &grpcHostClient{client: pluginv1.NewHostClient(conn)}
}

type grpcHostClient struct {
	client pluginv1.HostClient
}

func (h *grpcHostClient) ExecutorProfiles() ExecutorProfileReader {
	return grpcExecutorProfileReader{client: h.client}
}

func (h *grpcHostClient) PluginOwnedTaskTrees() PluginOwnedTaskTreeManager {
	return grpcPluginOwnedTaskTreeManager{client: h.client}
}

func (h *grpcHostClient) AgentConversations() AgentConversationManager {
	return grpcAgentConversationManager{client: h.client}
}

func (h *grpcHostClient) ManagedAgentConversations() ManagedAgentConversationManager {
	return grpcManagedAgentConversationManager{client: h.client}
}

func (h *grpcHostClient) GetState(ctx context.Context, scope, scopeID, key string) (map[string]any, bool, error) {
	resp, err := h.client.GetState(ctx, &pluginv1.GetStateRequest{Scope: scope, ScopeId: scopeID, Key: key})
	if err != nil {
		return nil, false, err
	}
	if !resp.GetFound() {
		return nil, false, nil
	}
	value, err := structToMap(resp.GetValue())
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (h *grpcHostClient) SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error {
	protoValue, err := mapToStruct(value)
	if err != nil {
		return err
	}
	_, err = h.client.SetState(ctx, &pluginv1.SetStateRequest{Scope: scope, ScopeId: scopeID, Key: key, Value: protoValue})
	return err
}

func (h *grpcHostClient) DeleteState(ctx context.Context, scope, scopeID, key string) error {
	_, err := h.client.DeleteState(ctx, &pluginv1.DeleteStateRequest{Scope: scope, ScopeId: scopeID, Key: key})
	return err
}

func (h *grpcHostClient) ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error) {
	resp, err := h.client.ListState(ctx, &pluginv1.ListStateRequest{Scope: scope, ScopeId: scopeID})
	if err != nil {
		return nil, err
	}
	return stateEntriesFromProto(resp.GetEntries())
}

func (h *grpcHostClient) GetConfig(ctx context.Context) (map[string]any, error) {
	resp, err := h.client.GetConfig(ctx, &pluginv1.GetConfigRequest{})
	if err != nil {
		return nil, err
	}
	config, err := structToMap(resp.GetConfig())
	if err != nil {
		return nil, err
	}
	if config == nil {
		config = map[string]any{}
	}
	return config, nil
}

func (h *grpcHostClient) RevealSecret(ctx context.Context, ref string) (string, error) {
	resp, err := h.client.RevealSecret(ctx, &pluginv1.RevealSecretRequest{Ref: ref})
	if err != nil {
		return "", err
	}
	return resp.GetValue(), nil
}

func (h *grpcHostClient) GetSecret(ctx context.Context, key string) (string, bool, error) {
	resp, err := h.client.GetSecret(ctx, &pluginv1.GetSecretRequest{Key: key})
	if err != nil {
		return "", false, err
	}
	if !resp.GetFound() {
		return "", false, nil
	}
	return resp.GetValue(), true, nil
}

func (h *grpcHostClient) SetSecret(ctx context.Context, key, value string) error {
	_, err := h.client.SetSecret(ctx, &pluginv1.SetSecretRequest{Key: key, Value: value})
	return err
}

func (h *grpcHostClient) DeleteSecret(ctx context.Context, key string) error {
	_, err := h.client.DeleteSecret(ctx, &pluginv1.DeleteSecretRequest{Key: key})
	return err
}

func (h *grpcHostClient) EmitEvent(ctx context.Context, name string, payload map[string]any) error {
	protoPayload, err := mapToStruct(payload)
	if err != nil {
		return err
	}
	_, err = h.client.EmitEvent(ctx, &pluginv1.EmitEventRequest{EventName: name, Payload: protoPayload})
	return err
}

func (h *grpcHostClient) GetCapabilityContext(ctx context.Context, workspaceID string) (*CapabilityContext, error) {
	response, err := h.client.GetCapabilityContext(ctx, &pluginv1.GetCapabilityContextRequest{WorkspaceId: workspaceID})
	if err != nil {
		return nil, err
	}
	return capabilityContextFromProto(response.GetContext()), nil
}

func (h *grpcHostClient) UpdateTaskExact(ctx context.Context, input ExactTaskUpdate) (*CommandResult, *Task, error) {
	response, err := h.client.UpdateTaskExact(ctx, exactTaskUpdateToProto(input))
	if err != nil {
		return nil, nil, err
	}
	result := commandResultFromProto(response.GetResult())
	if response.GetTask() == nil {
		return result, nil, nil
	}
	task, err := taskFromProto(response.GetTask())
	if err != nil {
		return nil, nil, err
	}
	return result, &task, nil
}

func (h *grpcHostClient) TaskCommands() ExactTaskCommandManager {
	return grpcExactTaskCommandManager{client: h.client}
}

func (h *grpcHostClient) TaskManagementClaims() ExactTaskManagementClaimCommandManager {
	return grpcExactTaskManagementClaimCommandManager{client: h.client}
}

func (h *grpcHostClient) TaskCompletionGates() ExactTaskCompletionGateCommandManager {
	return grpcExactTaskCompletionGateCommandManager{client: h.client}
}

func (h *grpcHostClient) WorkspaceAdministration() ExactWorkspaceAdministrationManager {
	return grpcExactWorkspaceAdministrationManager{client: h.client}
}

func (h *grpcHostClient) SourceIssueWriteback() ExactSourceIssueWritebackManager {
	return grpcExactSourceIssueWritebackManager{client: h.client}
}

func (h *grpcHostClient) Tasks() TaskReader { return grpcTaskReader{client: h.client} }

func (h *grpcHostClient) Sessions() SessionReader { return grpcSessionReader{client: h.client} }

func (h *grpcHostClient) Workspaces() WorkspaceReader { return grpcWorkspaceReader{client: h.client} }

func (h *grpcHostClient) Workflows() WorkflowReader { return grpcWorkflowReader{client: h.client} }

func (h *grpcHostClient) AgentProfiles() AgentProfileReader {
	return grpcAgentProfileReader{client: h.client}
}

func (h *grpcHostClient) Repositories() RepositoryReader {
	return grpcRepositoryReader{client: h.client}
}

func (h *grpcHostClient) Messages() MessageReader { return grpcMessageReader{client: h.client} }

func (h *grpcHostClient) InvokeUtilityAgent(ctx context.Context, prompt string, options ...UtilityAgentOptions) (string, error) {
	if len(options) > 1 {
		return "", status.Error(codes.InvalidArgument, "InvokeUtilityAgent accepts at most one options value")
	}
	req := &pluginv1.InvokeUtilityAgentWithOptionsRequest{Prompt: prompt}
	if len(options) == 1 {
		req.ProfileId = options[0].ProfileID
	}
	resp, err := h.client.InvokeUtilityAgentWithOptions(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

var _ Host = (*grpcHostClient)(nil)

// grpcTaskReader implements TaskReader on the plugin side, calling the
// generated pluginv1.HostClient and converting proto<->Go-native.
type grpcTaskReader struct {
	client pluginv1.HostClient
}

func (r grpcTaskReader) List(ctx context.Context, filter TaskFilter, page Page) ([]Task, *PageInfo, error) {
	resp, err := r.client.ListTasks(ctx, &pluginv1.ListTasksRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	tasks, err := tasksFromProto(resp.GetTasks())
	if err != nil {
		return nil, nil, err
	}
	return tasks, pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcTaskReader) Get(ctx context.Context, id string) (*Task, error) {
	resp, err := r.client.GetTask(ctx, &pluginv1.GetTaskRequest{Id: id})
	if err != nil {
		return nil, err
	}
	task, err := taskFromProto(resp.GetTask())
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r grpcTaskReader) Create(ctx context.Context, in CreateTaskInput) (*Task, error) {
	request, err := in.toProto()
	if err != nil {
		return nil, err
	}
	resp, err := r.client.CreateTask(ctx, request)
	if err != nil {
		return nil, err
	}
	task, err := taskFromProto(resp.GetTask())
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r grpcTaskReader) Update(ctx context.Context, in UpdateTaskInput) (*Task, error) {
	resp, err := r.client.UpdateTask(ctx, in.toProto())
	if err != nil {
		return nil, err
	}
	task, err := taskFromProto(resp.GetTask())
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r grpcTaskReader) Move(ctx context.Context, in MoveTaskInput) (*MoveTaskOutcome, error) {
	resp, err := r.client.MoveTask(ctx, in.toProto())
	if err != nil {
		return nil, err
	}
	return moveTaskOutcomeFromProto(resp)
}

// grpcSessionReader implements SessionReader on the plugin side.
type grpcSessionReader struct {
	client pluginv1.HostClient
}

func (r grpcSessionReader) List(ctx context.Context, filter SessionFilter, page Page) ([]Session, *PageInfo, error) {
	resp, err := r.client.ListSessions(ctx, &pluginv1.ListSessionsRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return sessionsFromProto(resp.GetSessions()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcSessionReader) CodeStats(ctx context.Context, filter SessionFilter, page Page) ([]SessionCodeStats, *PageInfo, error) {
	resp, err := r.client.ListSessionCodeStats(ctx, &pluginv1.ListSessionCodeStatsRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return sessionCodeStatsSliceFromProto(resp.GetStats()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcWorkspaceReader implements WorkspaceReader on the plugin side.
type grpcWorkspaceReader struct {
	client pluginv1.HostClient
}

func (r grpcWorkspaceReader) List(ctx context.Context, page Page) ([]Workspace, *PageInfo, error) {
	resp, err := r.client.ListWorkspaces(ctx, &pluginv1.ListWorkspacesRequest{Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return workspacesFromProto(resp.GetWorkspaces()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcWorkflowReader implements WorkflowReader on the plugin side.
type grpcWorkflowReader struct {
	client pluginv1.HostClient
}

func (r grpcWorkflowReader) List(ctx context.Context, workspaceID string, page Page) ([]Workflow, *PageInfo, error) {
	resp, err := r.client.ListWorkflows(ctx, &pluginv1.ListWorkflowsRequest{WorkspaceId: workspaceID, Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return workflowsFromProto(resp.GetWorkflows()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcWorkflowReader) ListSteps(ctx context.Context, workflowID string) ([]WorkflowStep, error) {
	resp, err := r.client.ListWorkflowSteps(ctx, &pluginv1.ListWorkflowStepsRequest{WorkflowId: workflowID})
	if err != nil {
		return nil, err
	}
	return workflowStepsFromProto(resp.GetSteps()), nil
}

// grpcAgentProfileReader implements AgentProfileReader on the plugin side.
type grpcAgentProfileReader struct {
	client pluginv1.HostClient
}

type grpcExecutorProfileReader struct {
	client pluginv1.HostClient
}

func (r grpcExecutorProfileReader) List(ctx context.Context, page Page) ([]ExecutorProfile, *PageInfo, error) {
	resp, err := r.client.ListExecutorProfiles(ctx, &pluginv1.ListExecutorProfilesRequest{Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return executorProfilesFromProto(resp.GetProfiles()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcAgentProfileReader) List(ctx context.Context, page Page) ([]AgentProfile, *PageInfo, error) {
	resp, err := r.client.ListAgentProfiles(ctx, &pluginv1.ListAgentProfilesRequest{Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return agentProfilesFromProto(resp.GetProfiles()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcRepositoryReader implements RepositoryReader on the plugin side.
type grpcRepositoryReader struct {
	client pluginv1.HostClient
}

func (r grpcRepositoryReader) List(ctx context.Context, workspaceID string, page Page) ([]Repository, *PageInfo, error) {
	resp, err := r.client.ListRepositories(ctx, &pluginv1.ListRepositoriesRequest{WorkspaceId: workspaceID, Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return repositoriesFromProto(resp.GetRepositories()), pageInfoFromProto(resp.GetPageInfo()), nil
}

// grpcMessageReader implements MessageReader on the plugin side.
type grpcMessageReader struct {
	client pluginv1.HostClient
}

type grpcPluginOwnedTaskTreeManager struct {
	client pluginv1.HostClient
}

func (m grpcPluginOwnedTaskTreeManager) Preview(ctx context.Context, rootTaskID string) ([]Task, error) {
	resp, err := m.client.PreviewPluginOwnedTaskTree(ctx, &pluginv1.PreviewPluginOwnedTaskTreeRequest{RootTaskId: rootTaskID})
	if err != nil {
		return nil, err
	}
	return tasksFromProto(resp.GetTasks())
}

func (m grpcPluginOwnedTaskTreeManager) Delete(ctx context.Context, rootTaskID string) ([]string, error) {
	resp, err := m.client.DeletePluginOwnedTaskTree(ctx, &pluginv1.DeletePluginOwnedTaskTreeRequest{RootTaskId: rootTaskID})
	if err != nil {
		return deletedTaskIDsFromStatus(err), err
	}
	return resp.GetDeletedTaskIds(), nil
}

func deletedTaskIDsFromStatus(err error) []string {
	for _, detail := range status.Convert(err).Details() {
		if progress, ok := detail.(*pluginv1.DeletePluginOwnedTaskTreeProgress); ok {
			return append([]string(nil), progress.GetDeletedTaskIds()...)
		}
	}
	return nil
}

type grpcAgentConversationManager struct {
	client pluginv1.HostClient
}

type grpcManagedAgentConversationManager struct {
	client pluginv1.HostClient
}

func (m grpcManagedAgentConversationManager) Ensure(ctx context.Context, spec ManagedAgentConversationSpec) (*CommandResult, ManagedAgentConversationDescriptor, error) {
	resp, err := m.client.EnsureManagedAgentConversationExact(ctx, &pluginv1.EnsureManagedAgentConversationExactRequest{
		Spec: managedConversationSpecToProto(spec),
	})
	if err != nil {
		return nil, ManagedAgentConversationDescriptor{}, err
	}
	return commandResultFromProto(resp.GetResult()), managedConversationFromProto(resp.GetConversation()), nil
}

func (m grpcManagedAgentConversationManager) Get(ctx context.Context, query ManagedAgentConversationQuery) (ManagedAgentConversationDescriptor, error) {
	resp, err := m.client.GetManagedAgentConversationStatusExact(ctx, &pluginv1.GetManagedAgentConversationStatusExactRequest{
		WorkspaceId: query.WorkspaceID, InstanceKey: query.InstanceKey,
		ApprovalRevision: query.ApprovalRevision, ManifestDigest: query.ManifestDigest,
	})
	if err != nil {
		return ManagedAgentConversationDescriptor{}, err
	}
	return managedConversationFromProto(resp.GetConversation()), nil
}

func (m grpcManagedAgentConversationManager) List(ctx context.Context, query ManagedAgentConversationListQuery) ([]ManagedAgentConversationDescriptor, error) {
	resp, err := m.client.ListManagedAgentConversationsExact(ctx, &pluginv1.ListManagedAgentConversationsExactRequest{
		WorkspaceId: query.WorkspaceID, ApprovalRevision: query.ApprovalRevision, ManifestDigest: query.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ManagedAgentConversationDescriptor, len(resp.GetConversations()))
	for i, conversation := range resp.GetConversations() {
		out[i] = managedConversationFromProto(conversation)
	}
	return out, nil
}

func (m grpcManagedAgentConversationManager) SetPaused(ctx context.Context, input ManagedAgentConversationPause) (*CommandResult, ManagedAgentConversationDescriptor, error) {
	resp, err := m.client.SetManagedAgentConversationPausedExact(ctx, &pluginv1.SetManagedAgentConversationPausedExactRequest{
		RequestId: input.RequestID, IdempotencyKey: input.IdempotencyKey, WorkspaceId: input.WorkspaceID,
		InstanceKey: input.InstanceKey, ExpectedRevision: input.ExpectedRevision,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest, Paused: input.Paused,
	})
	if err != nil {
		return nil, ManagedAgentConversationDescriptor{}, err
	}
	return commandResultFromProto(resp.GetResult()), managedConversationFromProto(resp.GetConversation()), nil
}

func (m grpcManagedAgentConversationManager) Delete(ctx context.Context, input ManagedAgentConversationDelete) (*CommandResult, error) {
	resp, err := m.client.DeleteManagedAgentConversationExact(ctx, &pluginv1.DeleteManagedAgentConversationExactRequest{
		RequestId: input.RequestID, IdempotencyKey: input.IdempotencyKey, WorkspaceId: input.WorkspaceID,
		InstanceKey: input.InstanceKey, ExpectedRevision: input.ExpectedRevision,
		ApprovalRevision: input.ApprovalRevision, ManifestDigest: input.ManifestDigest,
	})
	if err != nil {
		return nil, err
	}
	return commandResultFromProto(resp.GetResult()), nil
}

func (m grpcManagedAgentConversationManager) EnqueueInput(ctx context.Context, input ManagedAgentInputEnqueue) (*CommandResult, ManagedAgentInputReceipt, error) {
	resp, err := m.client.EnqueueManagedAgentInputExact(ctx, managedAgentInputEnqueueToProto(input))
	if err != nil {
		return nil, ManagedAgentInputReceipt{}, err
	}
	return commandResultFromProto(resp.GetResult()), managedAgentInputFromProto(resp.GetInput()), nil
}

func (m grpcManagedAgentConversationManager) GetInput(ctx context.Context, query ManagedAgentInputQuery) (ManagedAgentInputReceipt, error) {
	resp, err := m.client.GetManagedAgentInputExact(ctx, managedAgentInputQueryToProto(query))
	if err != nil {
		return ManagedAgentInputReceipt{}, err
	}
	return managedAgentInputFromProto(resp.GetInput()), nil
}

func (m grpcManagedAgentConversationManager) ListInputs(ctx context.Context, query ManagedAgentInputListQuery) (ManagedAgentInputPage, error) {
	resp, err := m.client.ListManagedAgentInputsExact(ctx, managedAgentInputListQueryToProto(query))
	if err != nil {
		return ManagedAgentInputPage{}, err
	}
	page := ManagedAgentInputPage{
		Inputs:             make([]ManagedAgentInputReceipt, len(resp.GetInputs())),
		NextSequenceCursor: resp.GetNextSequenceCursor(), HasMore: resp.GetHasMore(),
	}
	for i, input := range resp.GetInputs() {
		page.Inputs[i] = managedAgentInputFromProto(input)
	}
	return page, nil
}

func (m grpcManagedAgentConversationManager) CancelInput(ctx context.Context, input ManagedAgentInputCancel) (*CommandResult, ManagedAgentInputReceipt, error) {
	resp, err := m.client.CancelManagedAgentInputExact(ctx, managedAgentInputCancelToProto(input))
	if err != nil {
		return nil, ManagedAgentInputReceipt{}, err
	}
	return commandResultFromProto(resp.GetResult()), managedAgentInputFromProto(resp.GetInput()), nil
}

func (m grpcManagedAgentConversationManager) Dispatch(ctx context.Context, input ManagedAgentConversationDispatch) (*CommandResult, ManagedAgentDispatchStatus, ManagedAgentConversationDescriptor, error) {
	resp, err := m.client.DispatchManagedAgentConversationExact(ctx, managedAgentConversationDispatchToProto(input))
	if err != nil {
		return nil, "", ManagedAgentConversationDescriptor{}, err
	}
	return commandResultFromProto(resp.GetResult()), managedAgentDispatchStatusFromProto(resp.GetDispatchStatus()), managedConversationFromProto(resp.GetConversation()), nil
}

func (m grpcAgentConversationManager) Ensure(ctx context.Context, spec AgentConversationSpec) (AgentConversationDescriptor, string, error) {
	resp, err := m.client.EnsureAgentConversation(ctx, &pluginv1.EnsureAgentConversationRequest{Spec: spec.toProto()})
	if err != nil {
		return AgentConversationDescriptor{}, "", err
	}
	return agentConversationDescriptorFromProto(resp.GetConvDescriptor()), resp.GetStatus(), nil
}

func (m grpcAgentConversationManager) Dispatch(ctx context.Context, workspaceID, conversationKey, text, occurrenceKey string) (AgentConversationDispatch, error) {
	resp, err := m.client.DispatchAgentConversation(ctx, &pluginv1.DispatchAgentConversationRequest{
		WorkspaceId:     workspaceID,
		ConversationKey: conversationKey,
		Text:            text,
		OccurrenceKey:   occurrenceKey,
	})
	if err != nil {
		return AgentConversationDispatch{}, err
	}
	return agentConversationDispatchFromProto(resp), nil
}

func (m grpcAgentConversationManager) Delete(ctx context.Context, workspaceID, conversationKey string) (int32, error) {
	resp, err := m.client.DeleteAgentConversation(ctx, &pluginv1.DeleteAgentConversationRequest{
		WorkspaceId:     workspaceID,
		ConversationKey: conversationKey,
	})
	if err != nil {
		return 0, err
	}
	return resp.GetDeletedCount(), nil
}

func (r grpcMessageReader) List(ctx context.Context, filter MessageFilter, page Page) ([]Message, *PageInfo, error) {
	resp, err := r.client.ListMessages(ctx, &pluginv1.ListMessagesRequest{Filter: filter.toProto(), Page: page.toProto()})
	if err != nil {
		return nil, nil, err
	}
	return messagesFromProto(resp.GetMessages()), pageInfoFromProto(resp.GetPageInfo()), nil
}

func (r grpcMessageReader) Send(ctx context.Context, taskID, sessionID, text string) (*MessageDispatch, error) {
	resp, err := r.client.SendMessage(ctx, &pluginv1.SendMessageRequest{TaskId: taskID, SessionId: sessionID, Text: text})
	if err != nil {
		return nil, err
	}
	return messageDispatchFromProto(resp), nil
}

// registerHostServer registers a grpc server that dispatches
// kandev.plugin.v1.Host RPCs to impl (kandev's Go-native Host
// implementation), converting proto<->Go-native types at the boundary.
func registerHostServer(s grpc.ServiceRegistrar, impl Host) {
	exact, _ := impl.(ExactHost)
	exactQueries, _ := impl.(ExactQueryHost)
	taskCommands, _ := impl.(ExactTaskCommandHost)
	claimCommands, _ := impl.(ExactTaskManagementClaimCommandHost)
	completionGateCommands, _ := impl.(ExactTaskCompletionGateCommandHost)
	workspaceAdminCommands, _ := impl.(ExactWorkspaceAdministrationHost)
	sourceIssueWritebackCommands, _ := impl.(ExactSourceIssueWritebackHost)
	executionCommands, _ := impl.(ExactExecutionCommandHost)
	interactionCommands, _ := impl.(ExactInteractionCommandHost)
	pluginv1.RegisterHostServer(s, &grpcHostServer{
		impl: impl, exact: exact, exactQueries: exactQueries, taskCommands: taskCommands,
		claimCommands: claimCommands, completionGateCommands: completionGateCommands,
		workspaceAdminCommands:       workspaceAdminCommands,
		sourceIssueWritebackCommands: sourceIssueWritebackCommands,
		executionCommands:            executionCommands, interactionCommands: interactionCommands,
	})
}

type grpcHostServer struct {
	pluginv1.UnimplementedHostServer
	impl                         Host
	exact                        ExactHost
	exactQueries                 ExactQueryHost
	taskCommands                 ExactTaskCommandHost
	claimCommands                ExactTaskManagementClaimCommandHost
	completionGateCommands       ExactTaskCompletionGateCommandHost
	workspaceAdminCommands       ExactWorkspaceAdministrationHost
	sourceIssueWritebackCommands ExactSourceIssueWritebackHost
	executionCommands            ExactExecutionCommandHost
	interactionCommands          ExactInteractionCommandHost
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) AcquireTaskManagementClaimExact(ctx context.Context, req *pluginv1.AcquireTaskManagementClaimExactRequest) (*pluginv1.TaskManagementClaimExactResponse, error) {
	if s.claimCommands == nil || s.claimCommands.TaskManagementClaims() == nil {
		return nil, status.Error(codes.Unimplemented, "task management claims are unavailable")
	}
	input := ExactTaskManagementClaimAcquire{
		ExactTaskManagementClaimCommand: ExactTaskManagementClaimCommand{
			RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
			ExpectedTaskResourceVersion:  req.GetExpectedTaskResourceVersion(),
			ExpectedClaimResourceVersion: req.GetExpectedClaimResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
			Reason: req.GetReason(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		},
		InstanceKey: req.GetInstanceKey(),
	}
	result, claim, err := s.claimCommands.TaskManagementClaims().Acquire(ctx, input)
	if err != nil {
		return nil, err
	}
	return &pluginv1.TaskManagementClaimExactResponse{Result: commandResultToProto(result), Claim: taskManagementClaimToProto(claim)}, nil
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) ReleaseTaskManagementClaimExact(ctx context.Context, req *pluginv1.ReleaseTaskManagementClaimExactRequest) (*pluginv1.TaskManagementClaimExactResponse, error) {
	if s.claimCommands == nil || s.claimCommands.TaskManagementClaims() == nil {
		return nil, status.Error(codes.Unimplemented, "task management claims are unavailable")
	}
	input := ExactTaskManagementClaimRelease{
		ExactTaskManagementClaimCommand: ExactTaskManagementClaimCommand{
			RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
			ExpectedTaskResourceVersion:  req.GetExpectedTaskResourceVersion(),
			ExpectedClaimResourceVersion: req.GetExpectedClaimResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
			Reason: req.GetReason(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		},
		InstanceKey: req.GetInstanceKey(),
	}
	result, claim, err := s.claimCommands.TaskManagementClaims().Release(ctx, input)
	if err != nil {
		return nil, err
	}
	return &pluginv1.TaskManagementClaimExactResponse{Result: commandResultToProto(result), Claim: taskManagementClaimToProto(claim)}, nil
}

func (s *grpcHostServer) TransferTaskManagementClaimExact(ctx context.Context, req *pluginv1.TransferTaskManagementClaimExactRequest) (*pluginv1.TaskManagementClaimExactResponse, error) {
	if s.claimCommands == nil || s.claimCommands.TaskManagementClaims() == nil {
		return nil, status.Error(codes.Unimplemented, "task management claims are unavailable")
	}
	input := ExactTaskManagementClaimTransfer{
		ExactTaskManagementClaimCommand: ExactTaskManagementClaimCommand{
			RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
			ExpectedTaskResourceVersion:  req.GetExpectedTaskResourceVersion(),
			ExpectedClaimResourceVersion: req.GetExpectedClaimResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
			Reason: req.GetReason(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		},
		InstanceKey: req.GetInstanceKey(), TargetInstallationID: req.GetTargetInstallationId(), TargetInstanceKey: req.GetTargetInstanceKey(),
	}
	result, claim, err := s.claimCommands.TaskManagementClaims().Transfer(ctx, input)
	if err != nil {
		return nil, err
	}
	return &pluginv1.TaskManagementClaimExactResponse{Result: commandResultToProto(result), Claim: taskManagementClaimToProto(claim)}, nil
}

func (s *grpcHostServer) SetTaskCompletionCriteriaExact(ctx context.Context, req *pluginv1.SetTaskCompletionCriteriaExactRequest) (*pluginv1.TaskCompletionGateExactResponse, error) {
	if s.completionGateCommands == nil || s.completionGateCommands.TaskCompletionGates() == nil {
		return nil, status.Error(codes.Unimplemented, "task completion gates are unavailable")
	}
	criteria := make([]TaskCompletionCriterionInput, 0, len(req.GetCriteria()))
	for _, item := range req.GetCriteria() {
		criteria = append(criteria, TaskCompletionCriterionInput{
			ID: item.GetId(), Description: item.GetDescription(),
			EvidenceSubject: completionEvidenceSubjectFromProto(item.GetEvidenceSubject()),
		})
	}
	result, gate, err := s.completionGateCommands.TaskCompletionGates().SetCriteria(ctx, ExactTaskCompletionCriteria{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(),
		ExpectedRevision: req.GetExpectedRevision(), Criteria: criteria,
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.TaskCompletionGateExactResponse{Result: commandResultToProto(result), Snapshot: completionGateToProto(gate)}, nil
}

func (s *grpcHostServer) VerifyTaskCompletionCriterionExact(ctx context.Context, req *pluginv1.VerifyTaskCompletionCriterionExactRequest) (*pluginv1.TaskCompletionGateExactResponse, error) {
	if s.completionGateCommands == nil || s.completionGateCommands.TaskCompletionGates() == nil {
		return nil, status.Error(codes.Unimplemented, "task completion gates are unavailable")
	}
	evidence := TaskCompletionEvidence{}
	if req.GetEvidence() != nil {
		evidence.Subject = completionEvidenceSubjectFromProto(req.GetEvidence().GetSubject())
		evidence.Summary = req.GetEvidence().GetSummary()
		evidence.Reference = req.GetEvidence().GetReference()
	}
	result, gate, err := s.completionGateCommands.TaskCompletionGates().Verify(ctx, ExactTaskCompletionEvidence{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		CriterionID: req.GetCriterionId(), IdempotencyKey: req.GetIdempotencyKey(),
		ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(), ExpectedRevision: req.GetExpectedRevision(),
		Evidence: evidence, ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.TaskCompletionGateExactResponse{Result: commandResultToProto(result), Snapshot: completionGateToProto(gate)}, nil
}

func (s *grpcHostServer) ApplyWorkspaceAdministrationExact(ctx context.Context, req *pluginv1.ApplyWorkspaceAdministrationExactRequest) (*pluginv1.WorkspaceAdministrationExactResponse, error) {
	if s.workspaceAdminCommands == nil || s.workspaceAdminCommands.WorkspaceAdministration() == nil {
		return nil, status.Error(codes.Unimplemented, "workspace administration is unavailable")
	}
	command, err := workspaceAdminCommandFromProto(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid workspace administration operation")
	}
	result, err := s.workspaceAdminCommands.WorkspaceAdministration().Apply(ctx, command)
	if err != nil {
		return nil, err
	}
	return &pluginv1.WorkspaceAdministrationExactResponse{Result: commandResultToProto(result)}, nil
}

func (s *grpcHostServer) GetSourceIssueCapabilitiesExact(ctx context.Context, req *pluginv1.GetSourceIssueCapabilitiesExactRequest) (*pluginv1.GetSourceIssueCapabilitiesExactResponse, error) {
	if s.sourceIssueWritebackCommands == nil || s.sourceIssueWritebackCommands.SourceIssueWriteback() == nil {
		return nil, status.Error(codes.Unimplemented, "source issue operations are unavailable")
	}
	result, item, receipt, err := s.sourceIssueWritebackCommands.SourceIssueWriteback().GetCapabilities(ctx, SourceIssueCapabilitiesQuery{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.GetSourceIssueCapabilitiesExactResponse{
		Result: commandResultToProto(result), Item: sourceIssueCapabilitiesToProto(item),
	}
	if receipt != nil {
		response.Receipt = hostReadReceiptToProto(*receipt)
	}
	return response, nil
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) CommentSourceIssueExact(ctx context.Context, req *pluginv1.CommentSourceIssueExactRequest) (*pluginv1.SourceIssueWritebackExactResponse, error) {
	if s.sourceIssueWritebackCommands == nil || s.sourceIssueWritebackCommands.SourceIssueWriteback() == nil {
		return nil, status.Error(codes.Unimplemented, "source issue operations are unavailable")
	}
	result, receipt, err := s.sourceIssueWritebackCommands.SourceIssueWriteback().Comment(ctx, SourceIssueWritebackCommand{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(),
		ExpectedSourceResourceVersion: req.GetExpectedSourceResourceVersion(), Body: req.GetBody(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.SourceIssueWritebackExactResponse{Result: commandResultToProto(result), Receipt: sourceIssueWritebackReceiptToProto(receipt)}, nil
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) TransitionSourceIssueExact(ctx context.Context, req *pluginv1.TransitionSourceIssueExactRequest) (*pluginv1.SourceIssueWritebackExactResponse, error) {
	if s.sourceIssueWritebackCommands == nil || s.sourceIssueWritebackCommands.SourceIssueWriteback() == nil {
		return nil, status.Error(codes.Unimplemented, "source issue operations are unavailable")
	}
	result, receipt, err := s.sourceIssueWritebackCommands.SourceIssueWriteback().Transition(ctx, SourceIssueWritebackCommand{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(),
		ExpectedSourceResourceVersion: req.GetExpectedSourceResourceVersion(), TargetID: req.GetTargetId(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.SourceIssueWritebackExactResponse{Result: commandResultToProto(result), Receipt: sourceIssueWritebackReceiptToProto(receipt)}, nil
}

func (s *grpcHostServer) GetCapabilityContext(ctx context.Context, req *pluginv1.GetCapabilityContextRequest) (*pluginv1.GetCapabilityContextResponse, error) {
	if s.exact == nil {
		return nil, status.Error(codes.Unimplemented, "exact Host operations are unavailable")
	}
	result, err := s.exact.GetCapabilityContext(ctx, req.GetWorkspaceId())
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetCapabilityContextResponse{Context: capabilityContextToProto(result)}, nil
}

func (s *grpcHostServer) UpdateTaskExact(ctx context.Context, req *pluginv1.UpdateTaskExactRequest) (*pluginv1.UpdateTaskExactResponse, error) {
	if s.exact == nil {
		return nil, status.Error(codes.Unimplemented, "exact Host operations are unavailable")
	}
	result, task, err := s.exact.UpdateTaskExact(ctx, exactTaskUpdateFromProto(req))
	if err != nil {
		return nil, err
	}
	response := &pluginv1.UpdateTaskExactResponse{Result: commandResultToProto(result)}
	if task != nil {
		response.Task, err = task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *grpcHostServer) CreateTaskExact(ctx context.Context, req *pluginv1.CreateTaskExactRequest) (*pluginv1.CreateTaskExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	input, err := createTaskInputFromProto(req.GetTask())
	if err != nil {
		return nil, err
	}
	result, task, err := s.taskCommands.TaskCommands().CreateTask(ctx, ExactTaskCreate{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExternalID: req.GetExternalId(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		Task: input,
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.CreateTaskExactResponse{Result: commandResultToProto(result)}
	if task != nil {
		response.Task, err = task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *grpcHostServer) SetTaskLabelsExact(ctx context.Context, req *pluginv1.SetTaskLabelsExactRequest) (*pluginv1.SetTaskLabelsExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, task, err := s.taskCommands.TaskCommands().SetLabels(ctx, ExactTaskLabels{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedResourceVersion: req.GetExpectedResourceVersion(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		Labels: append([]string(nil), req.GetLabels()...),
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.SetTaskLabelsExactResponse{Result: commandResultToProto(result)}
	if task != nil {
		response.Task, err = task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *grpcHostServer) AssignTaskExact(ctx context.Context, req *pluginv1.AssignTaskExactRequest) (*pluginv1.AssignTaskExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, task, err := s.taskCommands.TaskCommands().Assign(ctx, ExactTaskAssignment{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedResourceVersion: req.GetExpectedResourceVersion(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
		AssigneeUserID: req.GetAssigneeUserId(),
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.AssignTaskExactResponse{Result: commandResultToProto(result)}
	if task != nil {
		response.Task, err = task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *grpcHostServer) MoveTaskExact(ctx context.Context, req *pluginv1.MoveTaskExactRequest) (*pluginv1.MoveTaskExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, task, err := s.taskCommands.TaskCommands().Move(ctx, ExactTaskMove{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedResourceVersion: req.GetExpectedResourceVersion(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		WorkflowID: req.GetWorkflowId(), WorkflowStepID: req.GetWorkflowStepId(), Position: req.GetPosition(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.MoveTaskExactResponse{Result: commandResultToProto(result)}
	if task != nil {
		response.Task, err = task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func (s *grpcHostServer) ArchiveTaskExact(ctx context.Context, req *pluginv1.ArchiveTaskExactRequest) (*pluginv1.ArchiveTaskExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, task, err := s.taskCommands.TaskCommands().Archive(ctx, ExactTaskArchive{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		IdempotencyKey: req.GetIdempotencyKey(), ExpectedResourceVersion: req.GetExpectedResourceVersion(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	response := &pluginv1.ArchiveTaskExactResponse{Result: commandResultToProto(result)}
	if task != nil {
		response.Task, err = task.toProto()
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) AddTaskRelationExact(ctx context.Context, req *pluginv1.AddTaskRelationExactRequest) (*pluginv1.AddTaskRelationExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, err := s.taskCommands.TaskCommands().AddRelation(ctx, ExactTaskRelation{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		RelatedTaskID: req.GetRelatedTaskId(), ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(),
		ExpectedRelatedResourceVersion: req.GetExpectedRelatedResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.AddTaskRelationExactResponse{Result: commandResultToProto(result)}, nil
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) RemoveTaskRelationExact(ctx context.Context, req *pluginv1.RemoveTaskRelationExactRequest) (*pluginv1.RemoveTaskRelationExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, err := s.taskCommands.TaskCommands().RemoveRelation(ctx, ExactTaskRelation{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		RelatedTaskID: req.GetRelatedTaskId(), ExpectedTaskResourceVersion: req.GetExpectedTaskResourceVersion(),
		ExpectedRelatedResourceVersion: req.GetExpectedRelatedResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.RemoveTaskRelationExactResponse{Result: commandResultToProto(result)}, nil
}

func (s *grpcHostServer) SendTaskMessageExact(ctx context.Context, req *pluginv1.SendTaskMessageExactRequest) (*pluginv1.SendTaskMessageExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, err := s.taskCommands.TaskCommands().SendMessage(ctx, ExactTaskMessage{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		SessionID: req.GetSessionId(), IdempotencyKey: req.GetIdempotencyKey(),
		ExpectedTaskResourceVersion:    req.GetExpectedTaskResourceVersion(),
		ExpectedSessionResourceVersion: req.GetExpectedSessionResourceVersion(), Content: req.GetContent(),
		ManagementInstanceKey: req.GetManagementInstanceKey(), ExpectedClaimGeneration: req.GetExpectedClaimGeneration(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.SendTaskMessageExactResponse{Result: commandResultToProto(result)}, nil
}

func (s *grpcHostServer) IssueTaskDirectiveExact(ctx context.Context, req *pluginv1.IssueTaskDirectiveExactRequest) (*pluginv1.IssueTaskDirectiveExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, directive, err := s.taskCommands.TaskCommands().IssueDirective(ctx, ExactTaskDirectiveIssue{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), TaskID: req.GetTaskId(),
		SessionID: req.GetSessionId(), CapabilityClass: req.GetCapabilityClass(),
		InstructionDigest:              req.GetInstructionDigest(),
		ExpectedTaskResourceVersion:    req.GetExpectedTaskResourceVersion(),
		ExpectedSessionResourceVersion: req.GetExpectedSessionResourceVersion(),
		ExpiresAt:                      req.GetExpiresAt(), IdempotencyKey: req.GetIdempotencyKey(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.IssueTaskDirectiveExactResponse{Result: commandResultToProto(result), Directive: taskDirectiveToProto(directive)}, nil
}

//nolint:dupl // Keep each generated request and response mapping explicit at the gRPC boundary.
func (s *grpcHostServer) ResolveTaskDirectiveExact(ctx context.Context, req *pluginv1.ResolveTaskDirectiveExactRequest) (*pluginv1.ResolveTaskDirectiveExactResponse, error) {
	if s.taskCommands == nil || s.taskCommands.TaskCommands() == nil {
		return nil, status.Error(codes.Unimplemented, "exact task commands are unavailable")
	}
	result, directive, err := s.taskCommands.TaskCommands().ResolveDirective(ctx, ExactTaskDirectiveResolve{
		RequestID: req.GetRequestId(), WorkspaceID: req.GetWorkspaceId(), DirectiveID: req.GetDirectiveId(),
		ExpectedResourceVersion: req.GetExpectedResourceVersion(), IdempotencyKey: req.GetIdempotencyKey(),
		Resolution: req.GetResolution(), ResolutionDigest: req.GetResolutionDigest(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.ResolveTaskDirectiveExactResponse{Result: commandResultToProto(result), Directive: taskDirectiveToProto(directive)}, nil
}

func (s *grpcHostServer) managedAgentConversations() (ManagedAgentConversationManager, error) {
	if s.exact == nil || s.exact.ManagedAgentConversations() == nil {
		return nil, status.Error(codes.Unimplemented, "managed conversations are unavailable")
	}
	return s.exact.ManagedAgentConversations(), nil
}

func (s *grpcHostServer) EnsureManagedAgentConversationExact(ctx context.Context, req *pluginv1.EnsureManagedAgentConversationExactRequest) (*pluginv1.EnsureManagedAgentConversationExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	result, conversation, err := manager.Ensure(ctx, managedConversationSpecFromProto(req.GetSpec()))
	if err != nil {
		return nil, err
	}
	return &pluginv1.EnsureManagedAgentConversationExactResponse{
		Result: commandResultToProto(result), Conversation: managedConversationToProto(conversation),
	}, nil
}

func (s *grpcHostServer) GetManagedAgentConversationStatusExact(ctx context.Context, req *pluginv1.GetManagedAgentConversationStatusExactRequest) (*pluginv1.GetManagedAgentConversationStatusExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	conversation, err := manager.Get(ctx, ManagedAgentConversationQuery{
		WorkspaceID: req.GetWorkspaceId(), InstanceKey: req.GetInstanceKey(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetManagedAgentConversationStatusExactResponse{Conversation: managedConversationToProto(conversation)}, nil
}

func (s *grpcHostServer) ListManagedAgentConversationsExact(ctx context.Context, req *pluginv1.ListManagedAgentConversationsExactRequest) (*pluginv1.ListManagedAgentConversationsExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	conversations, err := manager.List(ctx, ManagedAgentConversationListQuery{
		WorkspaceID: req.GetWorkspaceId(), ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*pluginv1.ManagedAgentConversationDescriptor, len(conversations))
	for i, conversation := range conversations {
		out[i] = managedConversationToProto(conversation)
	}
	return &pluginv1.ListManagedAgentConversationsExactResponse{Conversations: out}, nil
}

func (s *grpcHostServer) SetManagedAgentConversationPausedExact(ctx context.Context, req *pluginv1.SetManagedAgentConversationPausedExactRequest) (*pluginv1.SetManagedAgentConversationPausedExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	result, conversation, err := manager.SetPaused(ctx, ManagedAgentConversationPause{
		RequestID: req.GetRequestId(), IdempotencyKey: req.GetIdempotencyKey(), WorkspaceID: req.GetWorkspaceId(),
		InstanceKey: req.GetInstanceKey(), ExpectedRevision: req.GetExpectedRevision(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(), Paused: req.GetPaused(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.SetManagedAgentConversationPausedExactResponse{
		Result: commandResultToProto(result), Conversation: managedConversationToProto(conversation),
	}, nil
}

func (s *grpcHostServer) DeleteManagedAgentConversationExact(ctx context.Context, req *pluginv1.DeleteManagedAgentConversationExactRequest) (*pluginv1.DeleteManagedAgentConversationExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	result, err := manager.Delete(ctx, ManagedAgentConversationDelete{
		RequestID: req.GetRequestId(), IdempotencyKey: req.GetIdempotencyKey(), WorkspaceID: req.GetWorkspaceId(),
		InstanceKey: req.GetInstanceKey(), ExpectedRevision: req.GetExpectedRevision(),
		ApprovalRevision: req.GetApprovalRevision(), ManifestDigest: req.GetManifestDigest(),
	})
	if err != nil {
		return nil, err
	}
	return &pluginv1.DeleteManagedAgentConversationExactResponse{Result: commandResultToProto(result)}, nil
}

func (s *grpcHostServer) EnqueueManagedAgentInputExact(ctx context.Context, req *pluginv1.EnqueueManagedAgentInputExactRequest) (*pluginv1.EnqueueManagedAgentInputExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	result, input, err := manager.EnqueueInput(ctx, managedAgentInputEnqueueFromProto(req))
	if err != nil {
		return nil, err
	}
	return &pluginv1.EnqueueManagedAgentInputExactResponse{
		Result: commandResultToProto(result), Input: managedAgentInputToProto(input),
	}, nil
}

func (s *grpcHostServer) GetManagedAgentInputExact(ctx context.Context, req *pluginv1.GetManagedAgentInputExactRequest) (*pluginv1.GetManagedAgentInputExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	input, err := manager.GetInput(ctx, managedAgentInputQueryFromProto(req))
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetManagedAgentInputExactResponse{Input: managedAgentInputToProto(input)}, nil
}

func (s *grpcHostServer) ListManagedAgentInputsExact(ctx context.Context, req *pluginv1.ListManagedAgentInputsExactRequest) (*pluginv1.ListManagedAgentInputsExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	page, err := manager.ListInputs(ctx, managedAgentInputListQueryFromProto(req))
	if err != nil {
		return nil, err
	}
	inputs := make([]*pluginv1.ManagedAgentInputReceipt, len(page.Inputs))
	for i, input := range page.Inputs {
		inputs[i] = managedAgentInputToProto(input)
	}
	return &pluginv1.ListManagedAgentInputsExactResponse{
		Inputs: inputs, NextSequenceCursor: page.NextSequenceCursor, HasMore: page.HasMore,
	}, nil
}

func (s *grpcHostServer) CancelManagedAgentInputExact(ctx context.Context, req *pluginv1.CancelManagedAgentInputExactRequest) (*pluginv1.CancelManagedAgentInputExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	result, input, err := manager.CancelInput(ctx, managedAgentInputCancelFromProto(req))
	if err != nil {
		return nil, err
	}
	return &pluginv1.CancelManagedAgentInputExactResponse{
		Result: commandResultToProto(result), Input: managedAgentInputToProto(input),
	}, nil
}

func (s *grpcHostServer) DispatchManagedAgentConversationExact(ctx context.Context, req *pluginv1.DispatchManagedAgentConversationExactRequest) (*pluginv1.DispatchManagedAgentConversationExactResponse, error) {
	manager, err := s.managedAgentConversations()
	if err != nil {
		return nil, err
	}
	result, dispatchStatus, conversation, err := manager.Dispatch(ctx, managedAgentConversationDispatchFromProto(req))
	if err != nil {
		return nil, err
	}
	return &pluginv1.DispatchManagedAgentConversationExactResponse{
		Result: commandResultToProto(result), DispatchStatus: managedAgentDispatchStatusToProto(dispatchStatus),
		Conversation: managedConversationToProto(conversation),
	}, nil
}

func (s *grpcHostServer) GetState(ctx context.Context, req *pluginv1.GetStateRequest) (*pluginv1.GetStateResponse, error) {
	value, found, err := s.impl.GetState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey())
	if err != nil {
		return nil, err
	}
	if !found {
		return &pluginv1.GetStateResponse{Found: false}, nil
	}
	protoValue, err := mapToStruct(value)
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetStateResponse{Found: true, Value: protoValue}, nil
}

func (s *grpcHostServer) SetState(ctx context.Context, req *pluginv1.SetStateRequest) (*pluginv1.SetStateResponse, error) {
	value, err := structToMap(req.GetValue())
	if err != nil {
		return nil, err
	}
	if err := s.impl.SetState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey(), value); err != nil {
		return nil, err
	}
	return &pluginv1.SetStateResponse{}, nil
}

func (s *grpcHostServer) DeleteState(ctx context.Context, req *pluginv1.DeleteStateRequest) (*pluginv1.DeleteStateResponse, error) {
	if err := s.impl.DeleteState(ctx, req.GetScope(), req.GetScopeId(), req.GetKey()); err != nil {
		return nil, err
	}
	return &pluginv1.DeleteStateResponse{}, nil
}

func (s *grpcHostServer) ListState(ctx context.Context, req *pluginv1.ListStateRequest) (*pluginv1.ListStateResponse, error) {
	entries, err := s.impl.ListState(ctx, req.GetScope(), req.GetScopeId())
	if err != nil {
		return nil, err
	}
	protoEntries := make([]*pluginv1.StateEntry, len(entries))
	for i := range entries {
		converted, err := entries[i].toProto()
		if err != nil {
			return nil, err
		}
		protoEntries[i] = converted
	}
	return &pluginv1.ListStateResponse{Entries: protoEntries}, nil
}

func (s *grpcHostServer) GetConfig(ctx context.Context, req *pluginv1.GetConfigRequest) (*pluginv1.GetConfigResponse, error) {
	config, err := s.impl.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	protoConfig, err := mapToStruct(config)
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetConfigResponse{Config: protoConfig}, nil
}

func (s *grpcHostServer) RevealSecret(ctx context.Context, req *pluginv1.RevealSecretRequest) (*pluginv1.RevealSecretResponse, error) {
	value, err := s.impl.RevealSecret(ctx, req.GetRef())
	if err != nil {
		return nil, err
	}
	return &pluginv1.RevealSecretResponse{Value: value}, nil
}

func (s *grpcHostServer) GetSecret(ctx context.Context, req *pluginv1.GetSecretRequest) (*pluginv1.GetSecretResponse, error) {
	value, found, err := s.impl.GetSecret(ctx, req.GetKey())
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetSecretResponse{Found: found, Value: value}, nil
}

func (s *grpcHostServer) SetSecret(ctx context.Context, req *pluginv1.SetSecretRequest) (*pluginv1.SetSecretResponse, error) {
	if err := s.impl.SetSecret(ctx, req.GetKey(), req.GetValue()); err != nil {
		return nil, err
	}
	return &pluginv1.SetSecretResponse{}, nil
}

func (s *grpcHostServer) DeleteSecret(ctx context.Context, req *pluginv1.DeleteSecretRequest) (*pluginv1.DeleteSecretResponse, error) {
	if err := s.impl.DeleteSecret(ctx, req.GetKey()); err != nil {
		return nil, err
	}
	return &pluginv1.DeleteSecretResponse{}, nil
}

func (s *grpcHostServer) EmitEvent(ctx context.Context, req *pluginv1.EmitEventRequest) (*pluginv1.EmitEventResponse, error) {
	payload, err := structToMap(req.GetPayload())
	if err != nil {
		return nil, err
	}
	if err := s.impl.EmitEvent(ctx, req.GetEventName(), payload); err != nil {
		return nil, err
	}
	return &pluginv1.EmitEventResponse{}, nil
}

func (s *grpcHostServer) InvokeUtilityAgent(ctx context.Context, req *pluginv1.InvokeUtilityAgentRequest) (*pluginv1.InvokeUtilityAgentResponse, error) {
	return s.invokeUtilityAgent(ctx, req.GetPrompt())
}

func (s *grpcHostServer) InvokeUtilityAgentWithOptions(ctx context.Context, req *pluginv1.InvokeUtilityAgentWithOptionsRequest) (*pluginv1.InvokeUtilityAgentResponse, error) {
	return s.invokeUtilityAgent(ctx, req.GetPrompt(), UtilityAgentOptions{ProfileID: req.GetProfileId()})
}

func (s *grpcHostServer) invokeUtilityAgent(ctx context.Context, prompt string, options ...UtilityAgentOptions) (*pluginv1.InvokeUtilityAgentResponse, error) {
	text, err := s.impl.InvokeUtilityAgent(ctx, prompt, options...)
	if err != nil {
		return nil, err
	}
	return &pluginv1.InvokeUtilityAgentResponse{Text: text}, nil
}

// ── Host data API reads (ADR 0043) ──────────────────────────────────────
//
// Each method below dispatches to the injected Go-native impl's resource
// accessor (impl.Tasks(), impl.Sessions(), ...) and converts proto<->native
// at the boundary, exactly like GetState/ListState above. Capability
// gating and the real service-layer calls live in the impl kandev's
// runtime manager provides (internal/plugins), not here — this adapter
// only wires the RPC to whatever impl.Tasks()/impl.Sessions()/... does, so
// it compiles against any Host, including one that embeds
// UnimplementedHostData and returns Unimplemented for every accessor.

func (s *grpcHostServer) ListTasks(ctx context.Context, req *pluginv1.ListTasksRequest) (*pluginv1.ListTasksResponse, error) {
	filter := taskFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	tasks, pageInfo, err := s.impl.Tasks().List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	protoTasks, err := tasksToProto(tasks)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListTasksResponse{Tasks: protoTasks, PageInfo: pageInfo.toProto()}, nil
}

// GetTask dispatches to the impl's TaskReader.Get and wraps the result in
// GetTaskResponse (Buf RPC_RESPONSE_STANDARD_NAME/RPC_REQUEST_RESPONSE_UNIQUE:
// no bare-DTO RPC responses). The Host contract is that a missing task is a
// gRPC NotFound *error* from Get, never a (nil, nil) success — the nil check
// below is defense-in-depth for a Host implementation that doesn't follow
// that contract, so a plugin still gets NotFound instead of a zero-value
// GetTaskResponse.
func (s *grpcHostServer) GetTask(ctx context.Context, req *pluginv1.GetTaskRequest) (*pluginv1.GetTaskResponse, error) {
	task, err := s.impl.Tasks().Get(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, status.Error(codes.NotFound, "task not found")
	}
	protoTask, err := task.toProto()
	if err != nil {
		return nil, err
	}
	return &pluginv1.GetTaskResponse{Task: protoTask}, nil
}

func (s *grpcHostServer) ListWorkspaces(ctx context.Context, req *pluginv1.ListWorkspacesRequest) (*pluginv1.ListWorkspacesResponse, error) {
	page := pageFromProto(req.GetPage())
	workspaces, pageInfo, err := s.impl.Workspaces().List(ctx, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListWorkspacesResponse{Workspaces: workspacesToProto(workspaces), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListWorkflows(ctx context.Context, req *pluginv1.ListWorkflowsRequest) (*pluginv1.ListWorkflowsResponse, error) {
	page := pageFromProto(req.GetPage())
	workflows, pageInfo, err := s.impl.Workflows().List(ctx, req.GetWorkspaceId(), page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListWorkflowsResponse{Workflows: workflowsToProto(workflows), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListWorkflowSteps(ctx context.Context, req *pluginv1.ListWorkflowStepsRequest) (*pluginv1.ListWorkflowStepsResponse, error) {
	steps, err := s.impl.Workflows().ListSteps(ctx, req.GetWorkflowId())
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListWorkflowStepsResponse{Steps: workflowStepsToProto(steps)}, nil
}

func (s *grpcHostServer) ListAgentProfiles(ctx context.Context, req *pluginv1.ListAgentProfilesRequest) (*pluginv1.ListAgentProfilesResponse, error) {
	page := pageFromProto(req.GetPage())
	profiles, pageInfo, err := s.impl.AgentProfiles().List(ctx, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListAgentProfilesResponse{Profiles: agentProfilesToProto(profiles), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListExecutorProfiles(ctx context.Context, req *pluginv1.ListExecutorProfilesRequest) (*pluginv1.ListExecutorProfilesResponse, error) {
	provider, ok := s.impl.(ExecutorProfileHost)
	if !ok {
		return nil, errUnimplementedHostData("executor_profiles")
	}
	profiles, pageInfo, err := provider.ExecutorProfiles().List(ctx, pageFromProto(req.GetPage()))
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListExecutorProfilesResponse{Profiles: executorProfilesToProto(profiles), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListRepositories(ctx context.Context, req *pluginv1.ListRepositoriesRequest) (*pluginv1.ListRepositoriesResponse, error) {
	page := pageFromProto(req.GetPage())
	repos, pageInfo, err := s.impl.Repositories().List(ctx, req.GetWorkspaceId(), page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListRepositoriesResponse{Repositories: repositoriesToProto(repos), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListSessions(ctx context.Context, req *pluginv1.ListSessionsRequest) (*pluginv1.ListSessionsResponse, error) {
	filter := sessionFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	sessions, pageInfo, err := s.impl.Sessions().List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListSessionsResponse{Sessions: sessionsToProto(sessions), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListSessionCodeStats(ctx context.Context, req *pluginv1.ListSessionCodeStatsRequest) (*pluginv1.ListSessionCodeStatsResponse, error) {
	filter := sessionFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	stats, pageInfo, err := s.impl.Sessions().CodeStats(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListSessionCodeStatsResponse{Stats: sessionCodeStatsSliceToProto(stats), PageInfo: pageInfo.toProto()}, nil
}

func (s *grpcHostServer) ListMessages(ctx context.Context, req *pluginv1.ListMessagesRequest) (*pluginv1.ListMessagesResponse, error) {
	filter := messageFilterFromProto(req.GetFilter())
	page := pageFromProto(req.GetPage())
	messages, pageInfo, err := s.impl.Messages().List(ctx, filter, page)
	if err != nil {
		return nil, err
	}
	return &pluginv1.ListMessagesResponse{Messages: messagesToProto(messages), PageInfo: pageInfo.toProto()}, nil
}

// ── Host data API writes (ADR 0043) ─────────────────────────────────────
//
// Each dispatches to the injected impl's writer accessor (impl.Tasks() for
// Create/Update, impl.Messages() for SendMessage) and converts native<->proto
// at the boundary. Capability gating (api_write:<resource>) and the real
// service-layer calls live in the kandev-side impl, exactly like the reads
// above. The nil checks are defense-in-depth for the trust boundary: the
// in-tree Host never returns (nil, nil) on success, but a custom or test Host
// might, and a plugin should get a gRPC error rather than a server panic.

func (s *grpcHostServer) CreateTask(ctx context.Context, req *pluginv1.CreateTaskRequest) (*pluginv1.CreateTaskResponse, error) {
	input, err := createTaskInputFromProto(req)
	if err != nil {
		return nil, err
	}
	task, err := s.impl.Tasks().Create(ctx, input)
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, status.Error(codes.Internal, "CreateTask returned nil task")
	}
	protoTask, err := task.toProto()
	if err != nil {
		return nil, err
	}
	return &pluginv1.CreateTaskResponse{Task: protoTask}, nil
}

func (s *grpcHostServer) UpdateTask(ctx context.Context, req *pluginv1.UpdateTaskRequest) (*pluginv1.UpdateTaskResponse, error) {
	task, err := s.impl.Tasks().Update(ctx, updateTaskInputFromProto(req))
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, status.Error(codes.Internal, "UpdateTask returned nil task")
	}
	protoTask, err := task.toProto()
	if err != nil {
		return nil, err
	}
	return &pluginv1.UpdateTaskResponse{Task: protoTask}, nil
}

func (s *grpcHostServer) MoveTask(ctx context.Context, req *pluginv1.MoveTaskRequest) (*pluginv1.MoveTaskResponse, error) {
	outcome, err := s.impl.Tasks().Move(ctx, moveTaskInputFromProto(req))
	if err != nil {
		return nil, err
	}
	if outcome == nil {
		return nil, status.Error(codes.Internal, "MoveTask returned nil outcome")
	}
	return outcome.toProto()
}

func (s *grpcHostServer) SendMessage(ctx context.Context, req *pluginv1.SendMessageRequest) (*pluginv1.SendMessageResponse, error) {
	dispatch, err := s.impl.Messages().Send(ctx, req.GetTaskId(), req.GetSessionId(), req.GetText())
	if err != nil {
		return nil, err
	}
	if dispatch == nil {
		return nil, status.Error(codes.Internal, "SendMessage returned nil dispatch")
	}
	return dispatch.toProto(), nil
}

func (s *grpcHostServer) PreviewPluginOwnedTaskTree(ctx context.Context, req *pluginv1.PreviewPluginOwnedTaskTreeRequest) (*pluginv1.PreviewPluginOwnedTaskTreeResponse, error) {
	provider, ok := s.impl.(PluginOwnedTaskTreeHost)
	if !ok {
		return nil, errUnimplementedHostData("plugin_owned_task_trees")
	}
	tasks, err := provider.PluginOwnedTaskTrees().Preview(ctx, req.GetRootTaskId())
	if err != nil {
		return nil, err
	}
	protoTasks, err := tasksToProto(tasks)
	if err != nil {
		return nil, err
	}
	return &pluginv1.PreviewPluginOwnedTaskTreeResponse{Tasks: protoTasks}, nil
}

func (s *grpcHostServer) DeletePluginOwnedTaskTree(ctx context.Context, req *pluginv1.DeletePluginOwnedTaskTreeRequest) (*pluginv1.DeletePluginOwnedTaskTreeResponse, error) {
	provider, ok := s.impl.(PluginOwnedTaskTreeHost)
	if !ok {
		return nil, errUnimplementedHostData("plugin_owned_task_trees")
	}
	deletedTaskIDs, err := provider.PluginOwnedTaskTrees().Delete(ctx, req.GetRootTaskId())
	if err != nil {
		if len(deletedTaskIDs) == 0 {
			return nil, err
		}
		detailed, detailErr := status.Convert(err).WithDetails(
			&pluginv1.DeletePluginOwnedTaskTreeProgress{DeletedTaskIds: deletedTaskIDs},
		)
		if detailErr != nil {
			return nil, err
		}
		return nil, detailed.Err()
	}
	return &pluginv1.DeletePluginOwnedTaskTreeResponse{DeletedTaskIds: deletedTaskIDs}, nil
}

// agentConversationManagerFor resolves the impl's agent conversation manager.
// The type assertion alone is not enough: AgentConversationHost is a public
// interface, and an implementation signals "this caller may not use agent
// conversations" (undeclared capability) or "not wired yet" by returning a nil
// manager. Calling through that nil interface panics the whole gRPC server, so
// the adapter answers Unimplemented instead.
func (s *grpcHostServer) agentConversationManagerFor() (AgentConversationManager, error) {
	host, ok := s.impl.(AgentConversationHost)
	if !ok {
		return nil, status.Error(codes.Unimplemented, "agent_conversation capability not implemented on this host")
	}
	manager := host.AgentConversations()
	if manager == nil {
		return nil, status.Error(codes.Unimplemented, "agent_conversation capability not implemented on this host")
	}
	return manager, nil
}

func (s *grpcHostServer) EnsureAgentConversation(ctx context.Context, req *pluginv1.EnsureAgentConversationRequest) (*pluginv1.EnsureAgentConversationResponse, error) {
	manager, err := s.agentConversationManagerFor()
	if err != nil {
		return nil, err
	}
	spec := agentConversationSpecFromProto(req.GetSpec())
	descriptor, statusStr, err := manager.Ensure(ctx, spec)
	if err != nil {
		return nil, err
	}
	return &pluginv1.EnsureAgentConversationResponse{
		ConvDescriptor: descriptor.toProto(),
		Status:         statusStr,
	}, nil
}

func (s *grpcHostServer) DispatchAgentConversation(ctx context.Context, req *pluginv1.DispatchAgentConversationRequest) (*pluginv1.DispatchAgentConversationResponse, error) {
	manager, err := s.agentConversationManagerFor()
	if err != nil {
		return nil, err
	}
	dispatch, err := manager.Dispatch(ctx, req.GetWorkspaceId(), req.GetConversationKey(), req.GetText(), req.GetOccurrenceKey())
	if err != nil {
		return nil, err
	}
	return dispatch.toProto(), nil
}

func (s *grpcHostServer) DeleteAgentConversation(ctx context.Context, req *pluginv1.DeleteAgentConversationRequest) (*pluginv1.DeleteAgentConversationResponse, error) {
	manager, err := s.agentConversationManagerFor()
	if err != nil {
		return nil, err
	}
	deletedCount, err := manager.Delete(ctx, req.GetWorkspaceId(), req.GetConversationKey())
	if err != nil {
		return nil, err
	}
	return &pluginv1.DeleteAgentConversationResponse{DeletedCount: deletedCount}, nil
}

var _ pluginv1.HostServer = (*grpcHostServer)(nil)

// UnimplementedHostData is an embeddable default for the Host data API
// (ADR 0043) sub-accessors: Tasks/Sessions/Workspaces/Workflows/
// AgentProfiles/Repositories. Embed it in a Go-native Host implementation
// to satisfy the interface before wiring real data access — every method
// on every returned reader returns a gRPC Unimplemented error. Override
// individual accessor methods (e.g. define your own Tasks() on the
// embedding type) as real capability-gated, service-backed logic lands;
// unlike UnimplementedPlugin's per-RPC methods, these are per-resource
// accessors because each one fans out to multiple reader methods.
type UnimplementedHostData struct{}

func (UnimplementedHostData) Tasks() TaskReader           { return unimplementedTaskReader{} }
func (UnimplementedHostData) Sessions() SessionReader     { return unimplementedSessionReader{} }
func (UnimplementedHostData) Workspaces() WorkspaceReader { return unimplementedWorkspaceReader{} }
func (UnimplementedHostData) Workflows() WorkflowReader   { return unimplementedWorkflowReader{} }
func (UnimplementedHostData) AgentProfiles() AgentProfileReader {
	return unimplementedAgentProfileReader{}
}
func (UnimplementedHostData) ExecutorProfiles() ExecutorProfileReader {
	return unimplementedExecutorProfileReader{}
}
func (UnimplementedHostData) Repositories() RepositoryReader {
	return unimplementedRepositoryReader{}
}
func (UnimplementedHostData) Messages() MessageReader { return unimplementedMessageReader{} }

func (UnimplementedHostData) PluginOwnedTaskTrees() PluginOwnedTaskTreeManager {
	return unimplementedPluginOwnedTaskTreeManager{}
}

// AgentConversations is the embeddable default for the agent_conversation
// Host extension: a Host that hasn't wired a conversation manager still
// satisfies AgentConversationHost, returning gRPC Unimplemented until
// overridden.
func (UnimplementedHostData) AgentConversations() AgentConversationManager {
	return unimplementedAgentConversationManager{}
}

// InvokeUtilityAgent is the embeddable default for the agent_invoke Host
// method (ADR 0048). It lives on UnimplementedHostData — the shared
// "unimplemented Host extensions" embed both real Host implementations use —
// so a Host that hasn't wired a utility agent (e.g. a test double) still
// satisfies the interface, returning gRPC Unimplemented until overridden.
func (UnimplementedHostData) InvokeUtilityAgent(context.Context, string, ...UtilityAgentOptions) (string, error) {
	return "", errUnimplementedHostData("utility_agent")
}

func errUnimplementedHostData(resource string) error {
	return status.Errorf(codes.Unimplemented, "pluginsdk: Host data API %q not implemented", resource)
}

type unimplementedTaskReader struct{}

func (unimplementedTaskReader) List(context.Context, TaskFilter, Page) ([]Task, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("tasks")
}

func (unimplementedTaskReader) Get(context.Context, string) (*Task, error) {
	return nil, errUnimplementedHostData("tasks")
}

func (unimplementedTaskReader) Create(context.Context, CreateTaskInput) (*Task, error) {
	return nil, errUnimplementedHostData("tasks")
}

func (unimplementedTaskReader) Update(context.Context, UpdateTaskInput) (*Task, error) {
	return nil, errUnimplementedHostData("tasks")
}

func (unimplementedTaskReader) Move(context.Context, MoveTaskInput) (*MoveTaskOutcome, error) {
	return nil, errUnimplementedHostData("tasks")
}

type unimplementedSessionReader struct{}

func (unimplementedSessionReader) List(context.Context, SessionFilter, Page) ([]Session, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("sessions")
}

func (unimplementedSessionReader) CodeStats(context.Context, SessionFilter, Page) ([]SessionCodeStats, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("sessions")
}

type unimplementedWorkspaceReader struct{}

func (unimplementedWorkspaceReader) List(context.Context, Page) ([]Workspace, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("workspaces")
}

type unimplementedWorkflowReader struct{}

func (unimplementedWorkflowReader) List(context.Context, string, Page) ([]Workflow, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("workflows")
}

func (unimplementedWorkflowReader) ListSteps(context.Context, string) ([]WorkflowStep, error) {
	return nil, errUnimplementedHostData("workflows")
}

type unimplementedAgentProfileReader struct{}

func (unimplementedAgentProfileReader) List(context.Context, Page) ([]AgentProfile, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("agent_profiles")
}

type unimplementedExecutorProfileReader struct{}

func (unimplementedExecutorProfileReader) List(context.Context, Page) ([]ExecutorProfile, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("executor_profiles")
}

type unimplementedRepositoryReader struct{}

func (unimplementedRepositoryReader) List(context.Context, string, Page) ([]Repository, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("repositories")
}

type unimplementedMessageReader struct{}

func (unimplementedMessageReader) List(context.Context, MessageFilter, Page) ([]Message, *PageInfo, error) {
	return nil, nil, errUnimplementedHostData("messages")
}

func (unimplementedMessageReader) Send(context.Context, string, string, string) (*MessageDispatch, error) {
	return nil, errUnimplementedHostData("messages")
}

type unimplementedPluginOwnedTaskTreeManager struct{}

func (unimplementedPluginOwnedTaskTreeManager) Preview(context.Context, string) ([]Task, error) {
	return nil, errUnimplementedHostData("plugin_owned_task_trees")
}

func (unimplementedPluginOwnedTaskTreeManager) Delete(context.Context, string) ([]string, error) {
	return nil, errUnimplementedHostData("plugin_owned_task_trees")
}

type unimplementedAgentConversationManager struct{}

func (unimplementedAgentConversationManager) Ensure(context.Context, AgentConversationSpec) (AgentConversationDescriptor, string, error) {
	return AgentConversationDescriptor{}, "", errUnimplementedHostData("agent_conversation")
}

func (unimplementedAgentConversationManager) Dispatch(context.Context, string, string, string, string) (AgentConversationDispatch, error) {
	return AgentConversationDispatch{}, errUnimplementedHostData("agent_conversation")
}

func (unimplementedAgentConversationManager) Delete(context.Context, string, string) (int32, error) {
	return 0, errUnimplementedHostData("agent_conversation")
}
