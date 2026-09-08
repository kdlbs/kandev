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
	writeProbeWebhookKey        = "write"
	fixtureReferenceSource      = "fixture-pull-requests"
	fixturePullRequestID        = "pull-request-42"
	revokedPullRequestID        = "pull-request-revoked"
	fixtureProviderID           = "fixture-source-control"
	fixtureCredentialHost       = "bitbucket.example.test"
	fixtureCredentialPath       = "/scm/TEAM/fixture"
	connectionStatusAction      = "connection-status"
	repositoryInspectActionKey  = "repositories.inspect"
	repositoryBranchesActionKey = "repositories.branches"
	searchPurpose               = "search"
	submissionPurpose           = "submission"
	fixtureTaskIDKey            = "task_id"
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
	return &fixturePlugin{dataDir: resolveDataDir()}
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
	case "link-pull-request":
		response["linked"] = true
		response[fixtureTaskIDKey] = req.Context.TaskID
		response["pull_request_id"] = fixturePullRequestID
	case "watch-create-task":
		return p.createWatchTask(ctx, req.Context.WorkspaceID)
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
