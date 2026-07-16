package manifest

import (
	"strings"
	"testing"
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
  tools: "/tools/{tool_name}"
  webhooks: "/webhooks/{webhook_key}"

capabilities:
  events: ["task.created", "task.*"]
  api_read: ["tasks", "agents"]
  api_write: ["tasks"]
  state: true
  secrets: true

tools:
  - name: "slack_send_message"
    display_name: "Send Slack Message"
    description: "Posts a message to a Slack channel"
    input_schema:
      type: object
      properties:
        channel: { type: string }
        text: { type: string }
      required: ["channel", "text"]

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

func TestValidate_ValidManifestPasses(t *testing.T) {
	m, err := Parse([]byte(validManifestYAML))
	if err != nil {
		t.Fatalf("Parse() unexpected error: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
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
			name:    "missing tools endpoint",
			mutate:  func(m *Manifest) { m.Endpoints.Tools = "" },
			wantErr: "endpoints.tools",
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
			name: "duplicate tool keys",
			mutate: func(m *Manifest) {
				m.Tools = append(m.Tools, Tool{Name: "slack_send_message", DisplayName: "Dup"})
			},
			wantErr: "duplicate tool",
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
			mutate:  func(m *Manifest) { m.APIVersion = 2 },
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
