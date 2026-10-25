// Package main implements fixturePlugin, the pluginsdk.Plugin backing the
// plugin-fixture binary (see the package doc comment in main.go).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	deliveriesFileName     = "deliveries.jsonl"
	webhooksFileName       = "webhooks.jsonl"
	configSnapshotFileName = "config.json"
	secretProbeFileName    = "secret-probe.json"
	writeProbeFileName     = "write-probe.json"

	// writeProbeWebhookKey triggers the Host data API write round-trip
	// (CreateTask + CreateComment). Gated on this key so unrelated webhook
	// deliveries don't attempt writes.
	writeProbeWebhookKey                           = "write"
	fixtureReferenceSource                         = "fixture-pull-requests"
	fixturePullRequestID                           = "pull-request-42"
	revokedPullRequestID                           = "pull-request-revoked"
	fixtureProviderID                              = "fixture-source-control"
	fixtureCredentialHost                          = "bitbucket.example.test"
	fixtureCredentialPath                          = "/scm/TEAM/fixture"
	connectionStatusAction                         = "connection-status"
	utilityDefaultAction                           = "utility-default"
	utilityPreferenceAction                        = "utility-preference"
	utilityProfilePrompt                           = "/e2e:utility-profile"
	repositoryInspectActionKey                     = "repositories.inspect"
	repositoryBranchesActionKey                    = "repositories.branches"
	searchPurpose                                  = "search"
	submissionPurpose                              = "submission"
	fixtureTaskIDKey                               = "task_id"
	exactTaskCreateAction                          = "exact-task-create"
	taskTreeDeleteAction                           = "task-tree-delete"
	managedConversationEnsureAction                = "managed-conversation-ensure"
	managedConversationStatusAction                = "managed-conversation-status"
	managedConversationInputsAction                = "managed-conversation-inputs"
	managedConversationEnqueueAction               = "managed-conversation-enqueue"
	managedConversationCancelAction                = "managed-conversation-cancel"
	managedConversationPauseAction                 = "managed-conversation-pause"
	managedConversationResumeAction                = "managed-conversation-resume"
	managedConversationRecoverAction               = "managed-conversation-recover"
	managedConversationPermissionResponseAction    = "managed-conversation-permission-response"
	managedConversationClarificationResponseAction = "managed-conversation-clarification-response"
)

// deliveryRecord is one recorded OnEvent delivery, appended as a JSON line
// to deliveries.jsonl. e2e tests poll this file as evidence that an event
// reached the plugin over the real gRPC transport.
type deliveryRecord struct {
	EventType string `json:"event_type"`
	EventID   string `json:"event_id"`
}

// webhookRecord is one recorded HandleWebhook delivery, appended as a JSON
// line to webhooks.jsonl.
type webhookRecord struct {
	WebhookKey string `json:"webhook_key"`
	Method     string `json:"method"`
}

// fixturePlugin implements pluginsdk.Plugin (via UnimplementedPlugin) for
// Go integration tests and Playwright e2e: it records every delivery to
// disk under dataDir so tests can poll for evidence without needing their
// own gRPC client.
type fixturePlugin struct {
	pluginsdk.UnimplementedPlugin

	dataDir string

	mu                   sync.Mutex
	sawFirstEvent        bool
	revokedByWorkspaceID map[string]bool
	executorMu           sync.Mutex
	executorState        fixtureExecutorState
	executorTransports   map[string]*fixtureExecutorTransport
	executorStateErr     error
}

var _ pluginsdk.Plugin = (*fixturePlugin)(nil)

var _ pluginsdk.AgentToolPlugin = (*fixturePlugin)(nil)

func (p *fixturePlugin) InvokeAgentTool(_ context.Context, req *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error) {
	value, _ := req.Arguments["value"].(string)
	return &pluginsdk.AgentToolResult{
		Text: fmt.Sprintf("fixture echo: %s", value),
		StructuredContent: map[string]any{
			"value": value, fixtureTaskIDKey: req.Context.TaskID, "surface": req.Context.Surface,
		},
	}, nil
}

// newFixturePlugin builds a fixturePlugin whose data directory is resolved
// from KANDEV_PLUGIN_DATA_DIR (falling back to the current working
// directory), per §2 of docs/plans/plugins/GRPC-CONTRACT.md.
func newFixturePlugin() *fixturePlugin {
	return newFixturePluginAt(resolveDataDir())
}

func newFixturePluginAt(dataDir string) *fixturePlugin {
	plugin := &fixturePlugin{
		dataDir: dataDir, revokedByWorkspaceID: make(map[string]bool),
		executorTransports: make(map[string]*fixtureExecutorTransport),
		executorState:      fixtureExecutorState{Environments: make(map[string]fixtureExecutorResource)},
	}
	data, err := os.ReadFile(filepath.Join(dataDir, fixtureExecutorStateFileName))
	if err == nil {
		if err := json.Unmarshal(data, &plugin.executorState); err != nil {
			plugin.executorStateErr = fmt.Errorf("plugin-fixture: decode provider inventory: %w", err)
		}
	} else if !os.IsNotExist(err) {
		plugin.executorStateErr = fmt.Errorf("plugin-fixture: read provider inventory: %w", err)
	}
	if plugin.executorState.Environments == nil {
		plugin.executorState.Environments = make(map[string]fixtureExecutorResource)
	}
	return plugin
}

// resolveDataDir returns KANDEV_PLUGIN_DATA_DIR if set, otherwise the
// current working directory.
func resolveDataDir() string {
	if dir := os.Getenv("KANDEV_PLUGIN_DATA_DIR"); dir != "" {
		return dir
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// OnEvent appends a deliveries.jsonl line recording the event, then — only
// for the first event this process instance has seen — best-effort
// exercises the Host.SetState round trip (errors are ignored; this is
// coverage, not a critical path).
func (p *fixturePlugin) OnEvent(ctx context.Context, e *pluginsdk.Event) error {
	rec := deliveryRecord{EventType: e.EventType, EventID: e.EventID}
	if err := appendJSONLine(filepath.Join(p.dataDir, deliveriesFileName), rec); err != nil {
		return err
	}

	if p.markFirstEvent() {
		p.recordLastEventBestEffort(ctx, e)
	}
	return nil
}

// markFirstEvent returns true exactly once (on the first call), false on
// every subsequent call.
func (p *fixturePlugin) markFirstEvent() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sawFirstEvent {
		return false
	}
	p.sawFirstEvent = true
	return true
}

// recordLastEventBestEffort calls Host.SetState("instance", "",
// "last_event", ...) if a Host has been injected. Errors (including "no
// Host yet") are silently ignored — this exists purely to exercise the
// Host round trip for e2e coverage, not to guarantee delivery.
func (p *fixturePlugin) recordLastEventBestEffort(ctx context.Context, e *pluginsdk.Event) {
	host := p.Host()
	if host == nil {
		return
	}
	_ = host.SetState(ctx, "instance", "", "last_event", map[string]any{
		"event_type": e.EventType,
		"event_id":   e.EventID,
	})
}

// HandleWebhook appends a webhooks.jsonl line recording the delivery,
// best-effort snapshots the plugin's current operator config to
// config.json (evidence for e2e that the Host GetConfig RPC delivers the
// values set in Settings > Plugins, secrets in cleartext), and responds
// 200 "ok".
func (p *fixturePlugin) HandleWebhook(ctx context.Context, req *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	rec := webhookRecord{WebhookKey: req.WebhookKey, Method: req.Method}
	if err := appendJSONLine(filepath.Join(p.dataDir, webhooksFileName), rec); err != nil {
		return nil, err
	}
	p.snapshotConfigBestEffort(ctx)
	p.snapshotSecretProbeBestEffort(ctx)
	if req.WebhookKey == writeProbeWebhookKey {
		p.snapshotWriteProbeBestEffort(ctx)
	}
	return &pluginsdk.WebhookResponse{Status: 200, Body: []byte("ok")}, nil
}

// HandleAction provides fixture-only authenticated actions. The response is
// deliberately free of operator credentials: a browser can prove its action
// was authorized without learning the plugin's config or secret values.
func (p *fixturePlugin) HandleAction(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("plugin-fixture: missing action request")
	}
	response := map[string]any{"connected": true, "workspace_id": req.Context.WorkspaceID}
	switch req.ActionKey {
	case connectionStatusAction:
		if requestedRevocation(req.Body) {
			p.setCredentialRevoked(req.Context.WorkspaceID)
			response["connected"] = false
			response["error"] = "connection unavailable"
		}
	case utilityDefaultAction:
		return p.invokeUtilityAgent(ctx, false)
	case utilityPreferenceAction:
		return p.invokeUtilityAgent(ctx, true)
	case "link-pull-request":
		response["linked"] = true
		response[fixtureTaskIDKey] = req.Context.TaskID
		response["pull_request_id"] = fixturePullRequestID
	case "watch-create-task":
		return p.createWatchTask(ctx, req.Context.WorkspaceID)
	case exactTaskCreateAction:
		return p.createExactTask(ctx, req)
	case taskTreeDeleteAction:
		return p.deleteTaskTree(ctx, req)
	case managedConversationEnsureAction:
		return p.ensureManagedConversation(ctx, req)
	case managedConversationStatusAction:
		return p.managedConversationStatus(ctx, req)
	case managedConversationInputsAction:
		return p.managedConversationInputs(ctx, req)
	case managedConversationEnqueueAction:
		return p.enqueueManagedConversationInput(ctx, req)
	case managedConversationCancelAction:
		return p.cancelManagedConversationInput(ctx, req)
	case managedConversationPauseAction:
		return p.setManagedConversationPaused(ctx, req, true)
	case managedConversationResumeAction:
		return p.setManagedConversationPaused(ctx, req, false)
	case managedConversationRecoverAction:
		return p.recoverManagedConversationSession(ctx, req)
	case managedConversationPermissionResponseAction:
		return p.respondManagedConversationPermission(ctx, req)
	case managedConversationClarificationResponseAction:
		return p.answerManagedConversationClarification(ctx, req)
	case repositoryInspectActionKey:
		return p.inspectRepository(req.Body)
	case repositoryBranchesActionKey:
		return p.listRepositoryBranches()
	default:
		return nil, fmt.Errorf("plugin-fixture: unknown action %q", req.ActionKey)
	}
	body, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: marshaling action response: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func (p *fixturePlugin) createExactTask(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		IdempotencyKey string `json:"idempotency_key"`
		ExternalID     string `json:"external_id"`
		Title          string `json:"title"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
	}
	if err := json.Unmarshal(req.Body, &input); err != nil {
		return nil, fmt.Errorf("plugin-fixture: decode exact task create: %w", err)
	}
	host := p.Host()
	if host == nil {
		return nil, fmt.Errorf("plugin-fixture: host unavailable")
	}
	exact, ok := pluginsdk.HostV2(host)
	if !ok {
		return nil, fmt.Errorf("plugin-fixture: exact Host v2 unavailable")
	}
	capability, err := exact.GetCapabilityContext(ctx, req.Context.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: read task command capability: %w", err)
	}
	commands, ok := pluginsdk.HostTaskCommands(host)
	if !ok {
		return nil, fmt.Errorf("plugin-fixture: exact task commands unavailable")
	}
	result, task, err := commands.CreateTask(ctx, pluginsdk.ExactTaskCreate{
		RequestID: input.IdempotencyKey, WorkspaceID: req.Context.WorkspaceID,
		IdempotencyKey: input.IdempotencyKey, ExternalID: input.ExternalID,
		ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
		Task: pluginsdk.CreateTaskInput{
			WorkspaceID: req.Context.WorkspaceID, WorkflowID: input.WorkflowID,
			WorkflowStepID: &input.WorkflowStepID, Title: input.Title,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: create exact task: %w", err)
	}
	response := map[string]any{}
	if result != nil {
		response["status"] = result.Status
		response["reason"] = result.Reason
	}
	if task != nil {
		response[fixtureTaskIDKey] = task.ID
		response["resource_version"] = task.ResourceVersion
	}
	body, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: encode exact task create result: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func (p *fixturePlugin) ensureManagedConversation(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		InstanceKey    string `json:"instance_key"`
		AgentProfileID string `json:"agent_profile_id"`
	}
	if err := decodeFixtureActionBody(req.Body, &input); err != nil || input.InstanceKey == "" {
		return nil, fmt.Errorf("plugin-fixture: managed conversation instance_key is required")
	}
	_, capability, manager, err := p.managedConversationActionContext(ctx, req)
	if err != nil {
		return nil, err
	}
	result, descriptor, err := manager.Ensure(ctx, pluginsdk.ManagedAgentConversationSpec{
		RequestID: "fixture-managed-" + input.InstanceKey, IdempotencyKey: "fixture-managed-" + input.InstanceKey,
		WorkspaceID: req.Context.WorkspaceID, InstanceKey: input.InstanceKey,
		ApprovalRevision: capability.ApprovalRevision, ManifestDigest: capability.ManifestDigest,
		AgentProfileID: input.AgentProfileID, BasePrompt: "Fixture managed conversation",
		InstructionVersion: "1",
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: ensure managed conversation: %w", err)
	}
	if result == nil || (result.Status != pluginsdk.CommandApplied && result.Status != pluginsdk.CommandAlreadyApplied && result.Status != pluginsdk.CommandNoChange) {
		return nil, fmt.Errorf("plugin-fixture: ensure managed conversation returned %v", result)
	}
	if descriptor.WorkspaceID != req.Context.WorkspaceID || descriptor.InstanceKey != input.InstanceKey || descriptor.TaskID == "" || descriptor.SessionID == "" {
		return nil, fmt.Errorf("plugin-fixture: ensure returned an incomplete or out-of-scope managed conversation")
	}
	body, err := json.Marshal(map[string]any{
		"plugin_id": "kandev-plugin-e2e", "instance_key": descriptor.InstanceKey,
		"task_id": descriptor.TaskID, "session_id": descriptor.SessionID,
		"revision": descriptor.Revision, "created": result.Status == pluginsdk.CommandApplied,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: marshal managed conversation result: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func (p *fixturePlugin) deleteTaskTree(ctx context.Context, req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	var input struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(req.Body, &input); err != nil {
		return nil, fmt.Errorf("plugin-fixture: decode task tree delete: %w", err)
	}
	host := p.Host()
	if host == nil {
		return nil, fmt.Errorf("plugin-fixture: host unavailable")
	}
	trees, ok := pluginsdk.PluginOwnedTaskTrees(host)
	if !ok {
		return nil, fmt.Errorf("plugin-fixture: task tree manager unavailable")
	}
	if _, err := trees.Delete(ctx, input.TaskID); err != nil {
		return nil, fmt.Errorf("plugin-fixture: task tree delete: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: []byte(`{"deleted":true}`)}, nil
}

func (p *fixturePlugin) invokeUtilityAgent(ctx context.Context, usePreference bool) (*pluginsdk.PluginActionResponse, error) {
	host := p.Host()
	if host == nil {
		return nil, fmt.Errorf("plugin-fixture: host unavailable")
	}

	options, err := p.utilityAgentOptions(ctx, usePreference)
	if err != nil {
		return nil, err
	}
	response, err := host.InvokeUtilityAgent(ctx, utilityProfilePrompt, options...)
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: invoke utility agent: %w", err)
	}
	body, err := json.Marshal(map[string]string{"response": response})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: marshaling utility response: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func (p *fixturePlugin) utilityAgentOptions(ctx context.Context, usePreference bool) ([]pluginsdk.UtilityAgentOptions, error) {
	if !usePreference {
		return nil, nil
	}
	config, err := p.Host().GetConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: read config: %w", err)
	}
	profileID := ""
	if raw, ok := config["agent_profile"]; ok {
		var valid bool
		profileID, valid = raw.(string)
		if !valid {
			return nil, fmt.Errorf("plugin-fixture: config %q must be a string", "agent_profile")
		}
	}
	return []pluginsdk.UtilityAgentOptions{{ProfileID: profileID}}, nil
}

func (p *fixturePlugin) inspectRepository(body []byte) (*pluginsdk.PluginActionResponse, error) {
	var request struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &request); err != nil || !fixtureRepositoryURL(request.URL) {
		return &pluginsdk.PluginActionResponse{Body: []byte(`{"matched":false}`)}, nil
	}
	return &pluginsdk.PluginActionResponse{Body: []byte(`{"repository":{"provider_id":"fixture-source-control","provider_host":"bitbucket.example.test","provider_scope":"","provider_repository_id":"fixture-repository","owner_or_project":"TEAM","name":"fixture","clone_url":"https://bitbucket.example.test/scm/TEAM/fixture.git","default_branch":"main"}}`)}, nil
}

func fixtureRepositoryURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.EqualFold(parsed.Hostname(), fixtureCredentialHost)
}

func (p *fixturePlugin) listRepositoryBranches() (*pluginsdk.PluginActionResponse, error) {
	return &pluginsdk.PluginActionResponse{Body: []byte(`{"branches":[{"name":"main","is_default":true},{"name":"feature/provider-contract"}]}`)}, nil
}

func (p *fixturePlugin) createWatchTask(ctx context.Context, workspaceID string) (*pluginsdk.PluginActionResponse, error) {
	host := p.Host()
	if host == nil {
		return nil, fmt.Errorf("plugin-fixture: host unavailable")
	}
	task, err := host.Tasks().Create(ctx, pluginsdk.CreateTaskInput{
		WorkspaceID: workspaceID,
		Title:       "Bitbucket watch task",
		Description: "created by the provider-neutral fixture watch",
		Metadata:    map[string]any{"watch": "fixture"},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: creating watch task: %w", err)
	}
	body, err := json.Marshal(map[string]any{"watch_created": true, fixtureTaskIDKey: task.ID})
	if err != nil {
		return nil, fmt.Errorf("plugin-fixture: marshaling watch response: %w", err)
	}
	return &pluginsdk.PluginActionResponse{Body: body}, nil
}

func requestedRevocation(body []byte) bool {
	var request struct {
		Revoke bool `json:"revoke"`
	}
	return json.Unmarshal(body, &request) == nil && request.Revoke
}

func (p *fixturePlugin) setCredentialRevoked(workspaceID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.revokedByWorkspaceID == nil {
		p.revokedByWorkspaceID = make(map[string]bool)
	}
	p.revokedByWorkspaceID[workspaceID] = true
}

func (p *fixturePlugin) isCredentialRevoked(workspaceID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.revokedByWorkspaceID[workspaceID]
}

// SearchEntityReferences returns a deterministic, Bitbucket-shaped pull
// request. The source descriptor in the manifest supplies the provider and
// kind; this backend returns only untrusted candidate data.
func (*fixturePlugin) SearchEntityReferences(_ context.Context, req *pluginsdk.SearchEntityReferencesRequest) (*pluginsdk.SearchEntityReferencesResponse, error) {
	if req == nil || req.Source != fixtureReferenceSource {
		return &pluginsdk.SearchEntityReferencesResponse{}, nil
	}
	if strings.Contains(strings.ToLower(req.Query), "revoked") {
		return &pluginsdk.SearchEntityReferencesResponse{Candidates: []pluginsdk.EntityReferenceCandidate{{
			ProviderLocalID: revokedPullRequestID,
			Title:           "Pull request #99: Revoked before submission",
			URL:             "https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/99",
		}}}, nil
	}
	return &pluginsdk.SearchEntityReferencesResponse{Candidates: []pluginsdk.EntityReferenceCandidate{{
		ProviderLocalID: fixturePullRequestID,
		Title:           "Pull request #42: Provider-neutral contract",
		URL:             "https://bitbucket.example.test/projects/TEAM/repos/fixture/pull-requests/42",
		Attributes:      map[string]any{"repository": "TEAM/fixture"},
	}}}, nil
}

// AuthorizeEntityReference models a reference that disappears between search
// and send. This lets browser E2E prove the host checks a live plugin at
// submission time instead of trusting the previously selected suggestion.
func (*fixturePlugin) AuthorizeEntityReference(_ context.Context, req *pluginsdk.AuthorizeEntityReferenceRequest) (*pluginsdk.AuthorizeEntityReferenceResponse, error) {
	if req == nil || req.Source != fixtureReferenceSource {
		return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false, Reason: "reference source unavailable"}, nil
	}
	// Search authorization determines whether a candidate may be shown; the
	// fixture must allow the candidate at that point so the host can exercise
	// the separate, submit-time reauthorization boundary.
	id, _ := req.Reference["id"].(string)
	if id != fixturePullRequestID && id != revokedPullRequestID {
		return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false, Reason: "pull request is not owned by this source"}, nil
	}
	if req.Purpose != searchPurpose && req.Purpose != submissionPurpose {
		return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false, Reason: "reference purpose is unsupported"}, nil
	}
	if id == revokedPullRequestID && req.Purpose == submissionPurpose {
		return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: false, Reason: "pull request is no longer available"}, nil
	}
	return &pluginsdk.AuthorizeEntityReferenceResponse{Allowed: true}, nil
}

// ResolveGitCredential supplies deterministic transient material only for the
// fixture provider's exact host/path. Production plugins must resolve their
// own short-lived credential without exposing it through browser actions.
func (p *fixturePlugin) ResolveGitCredential(_ context.Context, req *pluginsdk.ResolveGitCredentialRequest) (*pluginsdk.ResolveGitCredentialResponse, error) {
	if !isFixtureCredentialScope(req) || p.isCredentialRevoked(req.WorkspaceID) {
		return nil, fmt.Errorf("plugin-fixture: connection unavailable")
	}
	return &pluginsdk.ResolveGitCredentialResponse{
		Username: "fixture-user", Secret: "fixture-credential-secret", ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339),
	}, nil
}

// GetGitCredentialBinding returns a non-secret revision for the exact fixture
// connection. An empty binding is the documented revocation signal.
func (p *fixturePlugin) GetGitCredentialBinding(_ context.Context, req *pluginsdk.GitCredentialBindingRequest) (*pluginsdk.GitCredentialBindingResponse, error) {
	if !isFixtureBindingScope(req) || p.isCredentialRevoked(req.WorkspaceID) {
		return &pluginsdk.GitCredentialBindingResponse{}, nil
	}
	return &pluginsdk.GitCredentialBindingResponse{Binding: "fixture-connection-v1"}, nil
}

func isFixtureCredentialScope(req *pluginsdk.ResolveGitCredentialRequest) bool {
	return req != nil && req.ProviderID == fixtureProviderID && req.Host == fixtureCredentialHost && req.Path == fixtureCredentialPath
}

func isFixtureBindingScope(req *pluginsdk.GitCredentialBindingRequest) bool {
	return req != nil && req.ProviderID == fixtureProviderID && req.Host == fixtureCredentialHost && req.Path == fixtureCredentialPath
}

// writeProbeRecord captures the outcome of the Host data API write round-trip
// so tests can poll write-probe.json as evidence a plugin created a task and
// sent a message to its session over the real gRPC transport (or was denied —
// the error is recorded).
type writeProbeRecord struct {
	TaskID        string `json:"task_id,omitempty"`
	TaskError     string `json:"task_error,omitempty"`
	MessageStatus string `json:"message_status,omitempty"`
	MessageError  string `json:"message_error,omitempty"`
}

// snapshotWriteProbeBestEffort exercises the write RPCs end to end: Host
// CreateTask then, on success, Host SendMessage to the new task, writing the
// result (task id, dispatch status, or the error) to write-probe.json.
// Best-effort — the fixture manifest exercises the current api_write contract;
// any permission or service error is still recorded as useful evidence.
func (p *fixturePlugin) snapshotWriteProbeBestEffort(ctx context.Context) {
	host := p.Host()
	if host == nil {
		return
	}
	rec := writeProbeRecord{}
	task, err := host.Tasks().Create(ctx, pluginsdk.CreateTaskInput{
		WorkspaceID: "ws-probe",
		WorkflowID:  "wf-probe",
		Title:       "fixture write probe",
		Description: "created by plugin-fixture over the Host data API",
	})
	if err != nil {
		rec.TaskError = err.Error()
	} else if task != nil {
		rec.TaskID = task.ID
		if dispatch, merr := host.Messages().Send(ctx, task.ID, "", "fixture probe message"); merr != nil {
			rec.MessageError = merr.Error()
		} else if dispatch != nil {
			rec.MessageStatus = dispatch.Status
		}
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(p.dataDir, writeProbeFileName), data, 0o600)
}

// snapshotSecretProbeBestEffort exercises the plugin-scoped secret
// primitives end to end: SetSecret then GetSecret through the Host, writing
// the read-back value to secret-probe.json as evidence for e2e that a
// plugin-owned secret survives a vault round trip over the real transport.
func (p *fixturePlugin) snapshotSecretProbeBestEffort(ctx context.Context) {
	host := p.Host()
	if host == nil {
		return
	}
	if err := host.SetSecret(ctx, "probe", "s3cret-roundtrip"); err != nil {
		return
	}
	value, found, err := host.GetSecret(ctx, "probe")
	if err != nil || !found {
		return
	}
	data, err := json.Marshal(map[string]string{"probe": value})
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(p.dataDir, secretProbeFileName), data, 0o600)
}

// snapshotConfigBestEffort writes the current Host.GetConfig result to
// config.json (overwriting any previous snapshot). Errors — including "no
// Host injected yet" — are silently ignored: like recordLastEventBestEffort,
// this exists purely as e2e coverage of the Host round trip.
func (p *fixturePlugin) snapshotConfigBestEffort(ctx context.Context) {
	host := p.Host()
	if host == nil {
		return
	}
	config, err := host.GetConfig(ctx)
	if err != nil {
		return
	}
	data, err := json.Marshal(config)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(p.dataDir, configSnapshotFileName), data, 0o600)
}

// appendJSONLine marshals v to a single JSON line and appends it to path,
// creating path's parent directory and the file itself as needed.
func appendJSONLine(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("plugin-fixture: creating data dir for %s: %w", path, err)
	}

	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("plugin-fixture: marshaling record: %w", err)
	}
	data = append(bytes.TrimRight(data, "\n"), '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("plugin-fixture: opening %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("plugin-fixture: writing %s: %w", path, err)
	}
	return f.Close()
}
