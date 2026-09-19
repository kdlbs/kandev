package manifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const validManifestYAML = `
id: "kandev-plugin-slack"
api_version: 1
version: "1.0.0"
display_name: "Slack Notifications"
description: "Post to Slack on task events"
author: "kandev"
categories: ["connector"]

base_url: "http://localhost:9100"

endpoints:
  health: "/health"
  events: "/events"
  webhooks: "/webhooks/{webhook_key}"

capabilities:
  events: ["task.created", "task.*"]
  api_read: ["tasks", "agents"]
  api_write: ["tasks"]
  state: true
  secrets: true

webhooks:
  - key: "slack-events"
    description: "Slack Events API webhook"
    method: "POST"

config_schema:
  type: object
  properties:
    bot_token_secret: { type: string, description: "Secret reference for Slack bot token" }
  required: ["bot_token_secret"]

ui:
  pages:
    - key: "slack-settings"
      title: "Slack Settings"
      path: "/settings"
      surface: "settings"
`

func TestParse_ValidManifestParsesID(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if m.ID != "kandev-plugin-slack" {
		t.Fatalf("m.ID = %q, want %q", m.ID, "kandev-plugin-slack")
	}
}

func TestValidate_StaticWebAppManifestPassesWithoutBackend(t *testing.T) {
	m, err := Parse([]byte(`
id: canvas-board
api_version: 2
version: 1.0.0
display_name: Canvas Board
description: A static board
author: test
ui:
  web_apps:
    - key: main
      title: Task board
      entry: ui/index.html
      placements: [task-canvas, workspace-canvas]
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if !m.IsStaticWebAppOnly() {
		t.Fatal("IsStaticWebAppOnly() = false, want true")
	}
}

func TestValidate_WebAppRejectsUnsafeEntryAndUnknownPlacement(t *testing.T) {
	m := validManifest(t)
	m.UI.WebApps = []WebApp{{Key: "main", Title: "Board", Entry: "../index.html", Placements: []string{"admin"}}}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "ui.web_apps") || !strings.Contains(err.Error(), "unsupported placement") {
		t.Fatalf("Validate() error = %v, want web-app entry and placement errors", err)
	}
}

func TestValidate_WebAppNormalizesNetworkOrigins(t *testing.T) {
	m := validManifest(t)
	m.UI.WebApps = []WebApp{{
		Key: "main", Title: "Board", Entry: "ui/index.html",
		Placements:     []string{WebAppPlacementTask},
		NetworkOrigins: []string{"https://API.example.com"},
	}}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if got := m.UI.WebApps[0].NetworkOrigins; len(got) != 1 || got[0] != "https://api.example.com" {
		t.Fatalf("network origins = %#v, want canonical HTTPS origin", got)
	}
}

func TestValidate_WebAppRejectsInvalidNetworkOrigin(t *testing.T) {
	m := validManifest(t)
	m.UI.WebApps = []WebApp{{
		Key: "main", Title: "Board", Entry: "ui/index.html",
		Placements:     []string{WebAppPlacementTask},
		NetworkOrigins: []string{"https://api.example.com/v1"},
	}}
	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "network_origins") {
		t.Fatalf("Validate() error = %v, want network origin error", err)
	}
}

// TestParse_UserStateCapabilityRoundTrips pins capabilities.user_state
// (Approach D1 / AC17): a plugin declaring host-provided per-user storage
// parses with Capabilities.UserState set.
func TestParse_UserStateCapabilityRoundTrips(t *testing.T) {
	yaml := strings.Replace(validManifestYAML, "secrets: true", "secrets: true\n  user_state: true", 1)
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if !m.Capabilities.UserState {
		t.Fatalf("Capabilities.UserState = false, want true")
	}
}

// TestParse_UserStateCapabilityDefaultsFalse pins that omitting
// capabilities.user_state defaults to false — a plugin must opt in before
// the per-user storage routes accept its writes (AC17).
func TestParse_UserStateCapabilityDefaultsFalse(t *testing.T) {
	m := validManifest(t)
	if m.Capabilities.UserState {
		t.Fatalf("Capabilities.UserState = true, want false (not declared in fixture)")
	}
}

// TestParse_WebhookAccessRoundTrips pins that webhooks[].access parses
// (AC10): the auth gate reads this field to decide whether a webhook is
// reachable without a caller identity.
func TestParse_WebhookAccessRoundTrips(t *testing.T) {
	input := strings.Replace(validManifestYAML, `method: "POST"`, "method: \"POST\"\n    access: public", 1)
	m, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	encoded, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal() unexpected error: %v", err)
	}
	m, err = Parse(encoded)
	if err != nil {
		t.Fatalf("Parse(round trip) unexpected error: %v", err)
	}
	if len(m.Webhooks) != 1 || m.Webhooks[0].EffectiveAccess(m.APIVersion) != WebhookAccessPublic {
		t.Fatalf("Webhooks[0].EffectiveAccess() = %+v, want a single public webhook", m.Webhooks)
	}
}

func TestWebhookAccessDefaultIsVersioned(t *testing.T) {
	webhook := Webhook{Key: "events"}
	if got := webhook.EffectiveAccess(LegacyAPIVersion); got != WebhookAccessPublic {
		t.Fatalf("v1 EffectiveAccess() = %q, want public", got)
	}
	if got := webhook.EffectiveAccess(CurrentAPIVersion); got != WebhookAccessAuthenticated {
		t.Fatalf("v2 EffectiveAccess() = %q, want authenticated", got)
	}
}

func TestValidateAcceptsSupportedAPIVersions(t *testing.T) {
	for _, version := range []int{LegacyAPIVersion, CurrentAPIVersion} {
		m := validManifest(t)
		m.APIVersion = version
		if err := m.Validate(); err != nil {
			t.Fatalf("api_version %d: Validate() unexpected error: %v", version, err)
		}
	}
}

func TestValidate_ValidManifestPasses(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_RejectsActionWithUnknownResourceScope(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: connection.get
    resource_scope: user
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for an action with an unknown resource scope, got nil")
	}
}

func TestValidate_RejectsDuplicateActionKeys(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: connection.get
    resource_scope: workspace
    max_body_bytes: 1024
  - key: connection.get
    resource_scope: workspace
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for duplicate action keys, got nil")
	}
}

func TestParseActionUsesDocumentedScopeField(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: connection.get
    scope: workspace
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error for documented action scope: %v", err)
	}
	if got := m.Actions[0].ResourceScope; got != "workspace" {
		t.Fatalf("action scope = %q, want workspace", got)
	}
}

func TestValidate_RejectsActionWithoutBodyLimit(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: connection.get
    resource_scope: workspace
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for an unbounded action body, got nil")
	}
}

func TestValidate_RejectsActionWithoutKey(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - resource_scope: workspace
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for action without a key, got nil")
	}
}

func TestValidate_RejectsRouteUnsafeActionKey(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
actions:
  - key: pullrequests/link
    scope: workspace
    max_body_bytes: 1024
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "action key") {
		t.Fatalf("Validate() error = %v, want route-safe action key rejection", err)
	}
}

func TestValidate_RejectsDuplicateRepositoryProvider(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
repository_providers: [bitbucket, bitbucket]
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for duplicate repository providers, got nil")
	}
}

func TestValidate_RejectsRepositoryProvidersThatCollideAfterNormalization(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
repository_providers: [Bitbucket, bitbucket]
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for case-insensitive duplicate repository providers, got nil")
	}
}

func TestValidate_RejectsNonCanonicalRepositoryProviderID(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
repository_providers: [Bitbucket]
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("Validate() error = %v, want canonical lowercase provider rejection", err)
	}
}

func TestValidate_RejectsRepositoryProviderWithSurroundingWhitespace(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
repository_providers: [" bitbucket "]
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for repository provider with surrounding whitespace, got nil")
	}
}

func TestValidate_RejectsDuplicateReferenceSource(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
reference_sources:
  - source: bitbucket
    provider: bitbucket
    kind: pull_request
    display_name: Bitbucket
    kind_label: Pull request
  - source: bitbucket
    provider: bitbucket
    kind: build
    display_name: Bitbucket
    kind_label: Build
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for duplicate reference sources, got nil")
	}
}

func TestValidate_RejectsDuplicateReferenceProviderKind(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
reference_sources:
  - source: bitbucket_pull_requests
    provider: bitbucket
    kind: pull_request
    display_name: Bitbucket
    kind_label: Pull request
  - source: bitbucket_prs
    provider: bitbucket
    kind: pull_request
    display_name: Bitbucket
    kind_label: Pull request
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for duplicate reference provider/kind, got nil")
	}
}

func TestValidate_RejectsReferenceIdentityThatMentionRegistryCannotNormalize(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
reference_sources:
  - source: bitbucket_pull_requests
    provider: Bitbucket
    kind: pull_request
    display_name: Bitbucket
    kind_label: Pull request
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("Validate() error = %v, want canonical reference identity rejection", err)
	}
}

func TestValidate_RejectsWhitespaceOnlyReferenceLabel(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML + `
reference_sources:
  - source: bitbucket_pull_requests
    provider: bitbucket
    kind: pull_request
    display_name: "   "
    kind_label: Pull request
`))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}

	if err := m.Validate(); err == nil || !strings.Contains(err.Error(), "display_name") {
		t.Fatalf("Validate() error = %v, want invalid reference label rejection", err)
	}
}

func TestParse_AgentToolDeclarationIsRetained(t *testing.T) {
	yaml := validManifestYAML + `
agent_tools:
  - name: add_tag
    description: Add a tag to the current task.
    surfaces: [kanban-task]
    input_schema:
      type: object
      properties:
        tag_id: {type: string}
      required: [tag_id]
`
	m, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if len(m.AgentTools) != 1 {
		t.Fatalf("AgentTools = %v, want one declaration", m.AgentTools)
	}
	tool := m.AgentTools[0]
	if tool.Name != "add_tag" {
		t.Fatalf("AgentTools[0].Name = %q, want add_tag", tool.Name)
	}
	if tool.Description != "Add a tag to the current task." {
		t.Fatalf("AgentTools[0].Description = %q", tool.Description)
	}
	if len(tool.Surfaces) != 1 || tool.Surfaces[0] != AgentToolSurfaceKanban {
		t.Fatalf("AgentTools[0].Surfaces = %v, want [%s]", tool.Surfaces, AgentToolSurfaceKanban)
	}
	if tool.InputSchema["type"] != "object" {
		t.Fatalf("AgentTools[0].InputSchema.type = %v, want object", tool.InputSchema["type"])
	}
	properties, ok := tool.InputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("AgentTools[0].InputSchema.properties = %T, want map[string]any", tool.InputSchema["properties"])
	}
	tagID, ok := properties["tag_id"].(map[string]any)
	if !ok || tagID["type"] != "string" {
		t.Fatalf("AgentTools[0].InputSchema.properties.tag_id = %v, want string schema", properties["tag_id"])
	}
	required, ok := tool.InputSchema["required"].([]any)
	if !ok || len(required) != 1 || required[0] != "tag_id" {
		t.Fatalf("AgentTools[0].InputSchema.required = %v, want [tag_id]", tool.InputSchema["required"])
	}
}

func TestValidate_AgentToolContract(t *testing.T) {
	m := managedManifest(t)
	readOnly := true
	destructive := false
	m.AgentTools = []AgentTool{{
		Name:        "read_tag",
		Description: "Read the current task tag.",
		Surfaces:    []string{AgentToolSurfaceKanban},
		InputSchema: map[string]any{"type": "object"},
		Annotations: AgentToolAnnotations{
			ReadOnlyHint: &readOnly, DestructiveHint: &destructive,
		},
	}}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_RejectsAgentToolsOnLegacyRemotePlugin(t *testing.T) {
	m := validManifest(t)
	m.AgentTools = []AgentTool{{
		Name: "echo", Description: "Echo input.",
		Surfaces:    []string{AgentToolSurfaceKanban},
		InputSchema: map[string]any{"type": "object"},
	}}
	err := m.Validate()
	if err == nil || !strings.Contains(err.Error(), "agent_tools require runtime.type binary") {
		t.Fatalf("Validate() error = %v, want managed-runtime requirement", err)
	}
}

func TestValidate_RejectsInvalidAgentTool(t *testing.T) {
	m := validManifest(t)
	m.AgentTools = []AgentTool{{
		Name:        "Bad-Name",
		Description: "",
		Surfaces:    []string{"configuration"},
		InputSchema: map[string]any{"type": "string"},
	}}
	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected agent tool errors, got nil")
	}
}

// validManifest returns a freshly parsed copy of the valid fixture so each
// test can mutate it independently without affecting others.
func validManifest(t *testing.T) *Manifest {
	t.Helper()
	m, err := Parse([]byte(validManifestYAML))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	return m
}

func TestParse_DecodesRepoURL(t *testing.T) {
	const withRepo = validManifestYAML + "\nrepo_url: \"https://github.com/kdlbs/kandev-plugin-slack\"\n"
	m, err := Parse([]byte(withRepo))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if m.RepoURL != "https://github.com/kdlbs/kandev-plugin-slack" {
		t.Fatalf("m.RepoURL = %q, want the manifest's repo_url", m.RepoURL)
	}
}

func TestValidate_AcceptsHTTPRepoURL(t *testing.T) {
	m := validManifest(t)
	m.RepoURL = "https://github.com/kdlbs/kandev-plugin-slack"
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error for a valid http(s) repo_url: %v", err)
	}
}

func TestValidate_TrimsRepoURLInPlace(t *testing.T) {
	m := validManifest(t)
	m.RepoURL = "  https://github.com/kdlbs/kandev-plugin-slack  "
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if m.RepoURL != "https://github.com/kdlbs/kandev-plugin-slack" {
		t.Fatalf("m.RepoURL = %q, want the surrounding whitespace trimmed", m.RepoURL)
	}
}

func TestValidate_RejectsBadIDPattern(t *testing.T) {
	m := validManifest(t)
	m.ID = "Kandev_Plugin!"

	if err := m.Validate(); err == nil {
		t.Fatal("Validate() expected error for bad id pattern, got nil")
	}
}

// TestValidate_RejectsIDEndingInDotConfig pins the fix for an id whose
// "<id>.yml" record filename would collide with FSStore's
// "<id>.config.yml" operator-config naming convention (see
// store.isRecordFile): registration must reject this shape up front rather
// than let it silently vanish from FSStore.List() after install.
func TestValidate_RejectsIDEndingInDotConfig(t *testing.T) {
	m := validManifest(t)
	m.ID = "foo.config"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for an id ending in \".config\", got nil")
	}
	if !strings.Contains(err.Error(), "id") {
		t.Fatalf("Validate() error = %q, want it to mention the id", err.Error())
	}
}

func TestValidate_RejectsInvalidManifests(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(m *Manifest)
		wantErr string // substring expected in the error message
	}{
		{
			name:    "missing base_url",
			mutate:  func(m *Manifest) { m.BaseURL = "" },
			wantErr: "base_url",
		},
		{
			name:    "missing health endpoint",
			mutate:  func(m *Manifest) { m.Endpoints.Health = "" },
			wantErr: "endpoints.health",
		},
		{
			name:    "missing events endpoint",
			mutate:  func(m *Manifest) { m.Endpoints.Events = "" },
			wantErr: "endpoints.events",
		},
		{
			name:    "missing webhooks endpoint",
			mutate:  func(m *Manifest) { m.Endpoints.Webhooks = "" },
			wantErr: "endpoints.webhooks",
		},
		{
			name:    "unknown category",
			mutate:  func(m *Manifest) { m.Categories = []string{"not-a-real-category"} },
			wantErr: "category",
		},
		{
			name: "unknown ui surface",
			mutate: func(m *Manifest) {
				m.UI.Pages[0].Surface = "not-a-real-surface"
			},
			wantErr: "surface",
		},
		{
			name: "duplicate webhook keys",
			mutate: func(m *Manifest) {
				m.Webhooks = append(m.Webhooks, Webhook{Key: "slack-events", Method: "POST"})
			},
			wantErr: "duplicate webhook",
		},
		{
			name:    "unsupported api_version",
			mutate:  func(m *Manifest) { m.APIVersion = CurrentAPIVersion + 1 },
			wantErr: "api_version",
		},
		{
			name:    "empty version",
			mutate:  func(m *Manifest) { m.Version = "" },
			wantErr: "version",
		},
		{
			name:    "version is a single dot",
			mutate:  func(m *Manifest) { m.Version = "." },
			wantErr: "version",
		},
		{
			name:    "version contains a path separator",
			mutate:  func(m *Manifest) { m.Version = "1.0/0" },
			wantErr: "version",
		},
		{
			name:    "version contains a traversal segment",
			mutate:  func(m *Manifest) { m.Version = "../../etc" },
			wantErr: "version",
		},
		{
			name:    "version contains whitespace",
			mutate:  func(m *Manifest) { m.Version = "1.0.0 " },
			wantErr: "version",
		},
		{
			name:    "repo_url with a javascript: scheme",
			mutate:  func(m *Manifest) { m.RepoURL = "javascript:alert(document.cookie)" },
			wantErr: "repo_url",
		},
		{
			name:    "repo_url without an http(s) scheme",
			mutate:  func(m *Manifest) { m.RepoURL = "ftp://example.com/repo" },
			wantErr: "repo_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest(t)
			tt.mutate(m)

			err := m.Validate()
			if err == nil {
				t.Fatalf("Validate() expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestHasEvent_ExactSubscriptionMatches(t *testing.T) {
	m := validManifest(t)
	if !m.HasEvent("task.created") {
		t.Fatal("HasEvent(\"task.created\") = false, want true (exact subscription)")
	}
}

func TestHasEvent_WildcardSubscriptionMatches(t *testing.T) {
	// validManifestYAML declares capabilities.events: ["task.created", "task.*"]
	m := validManifest(t)
	if !m.HasEvent("task.state_changed") {
		t.Fatal(`HasEvent("task.state_changed") = false, want true via "task.*" subscription`)
	}
}

func TestHasEvent_UndeclaredEventDoesNotMatch(t *testing.T) {
	m := validManifest(t)
	if m.HasEvent("office.comment.created") {
		t.Fatal(`HasEvent("office.comment.created") = true, want false (not declared)`)
	}
}

func TestCanRead_DeclaredResourceAllowed(t *testing.T) {
	m := validManifest(t)
	if !m.CanRead("tasks") {
		t.Fatal(`CanRead("tasks") = false, want true (declared in api_read)`)
	}
}

func TestCanRead_UndeclaredResourceDenied(t *testing.T) {
	m := validManifest(t)
	if m.CanRead("projects") {
		t.Fatal(`CanRead("projects") = true, want false (not declared in api_read)`)
	}
}

func TestCanWrite_DeclaredResourceAllowed(t *testing.T) {
	m := validManifest(t)
	if !m.CanWrite("tasks") {
		t.Fatal(`CanWrite("tasks") = false, want true (declared in api_write)`)
	}
}

func TestCanWrite_UndeclaredResourceDenied(t *testing.T) {
	m := validManifest(t)
	if m.CanWrite("comments") {
		t.Fatal(`CanWrite("comments") = true, want false (not declared in api_write)`)
	}
}

func TestValidate_RootRelativeBundleAndStylesPass(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"
	m.UI.Styles = []string{"/ui/bundle.css"}

	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_RejectsNonRootRelativeBundle(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "ui/bundle.js"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for non-root-relative ui.bundle, got nil")
	}
	if !strings.Contains(err.Error(), "bundle") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "bundle")
	}
}

func TestValidate_RejectsNonRootRelativeStyle(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"
	m.UI.Styles = []string{"ui/bundle.css"}

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for non-root-relative ui.styles entry, got nil")
	}
	if !strings.Contains(err.Error(), "styles") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "styles")
	}
}

func TestValidate_ManifestWithBundleAndNoPagesPasses(t *testing.T) {
	m := validManifest(t)
	m.UI.Pages = nil
	m.UI.Bundle = "/ui/bundle.js"

	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_ValidKeybindingPasses(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"
	m.UI.Keybindings = []UIKeybinding{
		{ID: "open-demo", Default: "mod+shift+k", Description: "Open the demo modal"},
	}

	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidate_RejectsDuplicateKeybindingID(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"
	m.UI.Keybindings = []UIKeybinding{
		{ID: "open-demo", Default: "mod+k", Description: "Open the demo modal"},
		{ID: "open-demo", Default: "mod+j", Description: "Open a different modal"},
	}

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for duplicate ui.keybindings id, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "duplicate")
	}
}

func TestValidate_RejectsEmptyKeybindingDescription(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"
	m.UI.Keybindings = []UIKeybinding{
		{ID: "open-demo", Default: "mod+k", Description: ""},
	}

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for empty ui.keybindings description, got nil")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "description")
	}
}

func TestValidate_RejectsKeybindingWithoutBundle(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = ""
	m.UI.Keybindings = []UIKeybinding{
		{ID: "open-demo", Default: "mod+k", Description: "Open the demo modal"},
	}

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for ui.keybindings without ui.bundle, got nil")
	}
	if !strings.Contains(err.Error(), "ui.bundle") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "ui.bundle")
	}
}

func TestParseKeybindingCombo_ValidCombos(t *testing.T) {
	valid := []string{"k", "mod+k", "mod+shift+k", "ctrl+alt+f1", "arrowup", "cmd+enter", " mod + K "}
	for _, combo := range valid {
		if err := parseKeybindingCombo(combo); err != nil {
			t.Errorf("parseKeybindingCombo(%q) unexpected error: %v", combo, err)
		}
	}
}

func TestParseKeybindingCombo_RejectsEmpty(t *testing.T) {
	if err := parseKeybindingCombo(""); err == nil {
		t.Fatal("parseKeybindingCombo(\"\") expected error, got nil")
	}
}

func TestParseKeybindingCombo_RejectsTwoNonModifierKeys(t *testing.T) {
	err := parseKeybindingCombo("mod+k+j")
	if err == nil {
		t.Fatal("parseKeybindingCombo(\"mod+k+j\") expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("parseKeybindingCombo() error = %q, want substring %q", err.Error(), "exactly one")
	}
}

func TestParseKeybindingCombo_RejectsNoNonModifierKey(t *testing.T) {
	err := parseKeybindingCombo("mod+shift")
	if err == nil {
		t.Fatal("parseKeybindingCombo(\"mod+shift\") expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("parseKeybindingCombo() error = %q, want substring %q", err.Error(), "exactly one")
	}
}

func TestWebhookAccessAndBodyLimitValidation(t *testing.T) {
	tests := []struct {
		name    string
		webhook Webhook
		wantErr string
	}{
		{name: "legacy defaults", webhook: Webhook{Key: "transcribe"}},
		{name: "authenticated 16 MiB", webhook: Webhook{Key: "transcribe", Access: "authenticated", MaxBodyBytes: 16 << 20}},
		{name: "public over default ceiling", webhook: Webhook{Key: "transcribe", Access: "public", MaxBodyBytes: (4 << 20) + 1}, wantErr: "authenticated"},
		{name: "unknown access", webhook: Webhook{Key: "transcribe", Access: "private"}, wantErr: "access"},
		{name: "over host ceiling", webhook: Webhook{Key: "transcribe", MaxBodyBytes: (16 << 20) + 1}, wantErr: "max_body_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest(t)
			m.Webhooks = []Webhook{tt.webhook}
			err := m.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseKeybindingCombo_RejectsUnknownModifier(t *testing.T) {
	err := parseKeybindingCombo("hyper+k")
	if err == nil {
		t.Fatal("parseKeybindingCombo(\"hyper+k\") expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown token") {
		t.Fatalf("parseKeybindingCombo() error = %q, want substring %q", err.Error(), "unknown token")
	}
}

// "control" and "super" are undocumented aliases for ctrl/cmd that the
// backend used to accept even though docs/plans/plugins/PLUGIN-API.md only
// documents mod|ctrl|cmd|meta|alt|option|shift. Dropped for consistency with
// the public contract and the frontend parser.
func TestParseKeybindingCombo_RejectsUndocumentedModifierAliases(t *testing.T) {
	for _, combo := range []string{"control+k", "super+k"} {
		err := parseKeybindingCombo(combo)
		if err == nil {
			t.Fatalf("parseKeybindingCombo(%q) expected error, got nil", combo)
		}
		if !strings.Contains(err.Error(), "unknown token") {
			t.Fatalf("parseKeybindingCombo(%q) error = %q, want substring %q", combo, err.Error(), "unknown token")
		}
	}
}

// A manifest default like "mod+shift+slash" or "shift+1" is stored/compared
// against KeyboardEvent.key, but a physically-held Shift means the browser
// reports the shifted character ("?", "!") instead of the token validated
// here — so the combo could never actually dispatch. Reject it at
// registration time instead.
func TestParseKeybindingCombo_RejectsShiftWithAlteredKey(t *testing.T) {
	for _, combo := range []string{"shift+1", "mod+shift+slash", "shift+9", "shift+minus"} {
		err := parseKeybindingCombo(combo)
		if err == nil {
			t.Fatalf("parseKeybindingCombo(%q) expected error, got nil", combo)
		}
		if !strings.Contains(err.Error(), "shift") {
			t.Fatalf("parseKeybindingCombo(%q) error = %q, want substring %q", combo, err.Error(), "shift")
		}
	}
}

// Shift is fine with letters (case is normalized before matching), named
// keys, and function keys, since Shift doesn't change their KeyboardEvent.key.
func TestParseKeybindingCombo_AllowsShiftWithUnaffectedKeys(t *testing.T) {
	for _, combo := range []string{"shift+k", "mod+shift+tab", "shift+arrowup", "ctrl+shift+f1"} {
		if err := parseKeybindingCombo(combo); err != nil {
			t.Errorf("parseKeybindingCombo(%q) unexpected error: %v", combo, err)
		}
	}
}

func TestHasUIBundle_TrueWhenBundleSet(t *testing.T) {
	m := validManifest(t)
	m.UI.Bundle = "/ui/bundle.js"

	if !m.HasUIBundle() {
		t.Fatal("HasUIBundle() = false, want true when ui.bundle is set")
	}
}

func TestHasUIBundle_FalseWhenBundleUnset(t *testing.T) {
	m := validManifest(t)

	if m.HasUIBundle() {
		t.Fatal("HasUIBundle() = true, want false when ui.bundle is unset")
	}
}

const managedManifestYAML = `
id: "kandev-plugin-hello"
api_version: 1
version: "1.0.0"
display_name: "Hello Plugin"
description: "A runtime-managed example plugin"
author: "kandev"
categories: ["tools"]

runtime:
  type: binary
  executables:
    linux-amd64: "server/plugin-linux-amd64"
    darwin-arm64: "server/plugin-darwin-arm64"
    windows-amd64: "server/plugin-windows-amd64.exe"

capabilities:
  state: true
`

func managedManifest(t *testing.T) *Manifest {
	t.Helper()
	m, err := Parse([]byte(managedManifestYAML))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	return m
}

func TestValidate_ManagedManifestPasses(t *testing.T) {
	m := managedManifest(t)
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestIsManaged_TrueForBinaryRuntime(t *testing.T) {
	m := managedManifest(t)
	if !m.IsManaged() {
		t.Fatal("IsManaged() = false, want true for runtime.type=binary")
	}
}

func TestIsManaged_FalseForLegacyRemote(t *testing.T) {
	m := validManifest(t)
	if m.IsManaged() {
		t.Fatal("IsManaged() = true, want false for a legacy base_url manifest")
	}
}

func TestExecutableFor_ReturnsMatchingPlatform(t *testing.T) {
	m := managedManifest(t)
	path, ok := m.ExecutableFor("linux", "amd64")
	if !ok {
		t.Fatal("ExecutableFor(\"linux\", \"amd64\") ok = false, want true")
	}
	if path != "server/plugin-linux-amd64" {
		t.Fatalf("ExecutableFor(\"linux\", \"amd64\") = %q, want %q", path, "server/plugin-linux-amd64")
	}
}

func TestExecutableFor_ReturnsFalseForMissingPlatform(t *testing.T) {
	m := managedManifest(t)
	if _, ok := m.ExecutableFor("freebsd", "arm64"); ok {
		t.Fatal("ExecutableFor(\"freebsd\", \"arm64\") ok = true, want false")
	}
}

func TestValidate_RejectsManagedManifestWithBaseURL(t *testing.T) {
	m := managedManifest(t)
	m.BaseURL = "http://localhost:9100"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for runtime-managed manifest with base_url, got nil")
	}
	if !strings.Contains(err.Error(), "base_url") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "base_url")
	}
}

func TestValidate_RejectsManagedManifestWithEndpoints(t *testing.T) {
	m := managedManifest(t)
	m.Endpoints.Health = "/health"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for runtime-managed manifest with endpoints, got nil")
	}
	if !strings.Contains(err.Error(), "endpoints") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "endpoints")
	}
}

func TestValidate_RejectsManagedManifestWithNoExecutables(t *testing.T) {
	m := managedManifest(t)
	m.Runtime.Executables = nil

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for runtime-managed manifest with no executables, got nil")
	}
	if !strings.Contains(err.Error(), "executables") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "executables")
	}
}

func TestValidate_RejectsManagedManifestWithTraversalExecutablePath(t *testing.T) {
	m := managedManifest(t)
	m.Runtime.Executables["linux-amd64"] = "../evil"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for traversal executable path, got nil")
	}
	if !strings.Contains(err.Error(), "runtime.executables") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "runtime.executables")
	}
}

func TestValidate_RejectsManagedManifestWithAbsoluteExecutablePath(t *testing.T) {
	m := managedManifest(t)
	m.Runtime.Executables["linux-amd64"] = "/etc/passwd"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for absolute executable path, got nil")
	}
	if !strings.Contains(err.Error(), "runtime.executables") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "runtime.executables")
	}
}

func TestValidate_RejectsUnknownRuntimeType(t *testing.T) {
	m := managedManifest(t)
	m.Runtime.Type = "docker"

	err := m.Validate()
	if err == nil {
		t.Fatal("Validate() expected error for unknown runtime.type, got nil")
	}
	if !strings.Contains(err.Error(), "runtime.type") {
		t.Fatalf("Validate() error = %q, want substring %q", err.Error(), "runtime.type")
	}
}
