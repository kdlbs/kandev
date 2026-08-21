package securityutil

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestProjectPermissionActionAllowlistAndRedaction(t *testing.T) {
	const secret = "sk-abcdefghijklmnopqrstuvwxyz123456"
	projection := ProjectPermissionAction("command", "Run command", map[string]any{
		"description": "Use token=" + secret,
		"raw_input": map[string]any{
			"command": "curl --api-key=" + secret + " https://example.com",
			"cwd":     "/workspace/project",
			"env":     map[string]any{"API_KEY": secret},
			"headers": map[string]any{"Authorization": "Bearer " + secret},
		},
	})

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("projection leaked a secret: %s", encoded)
	}
	if strings.Contains(string(encoded), "API_KEY") || strings.Contains(string(encoded), "headers") {
		t.Fatalf("projection leaked a hidden field: %s", encoded)
	}
	if projection.CWD != "/workspace/project" || !projection.Redacted {
		t.Fatalf("unexpected projection: %+v", projection)
	}
}

func TestSanitizePermissionTextTruncatesAtUTF8Boundary(t *testing.T) {
	value := strings.Repeat("a", maxPermissionPresentationBytes-1) + "€tail"
	sanitized, changed := SanitizePermissionText(value)

	if !changed {
		t.Fatal("expected oversized presentation to be truncated")
	}
	if !utf8.ValidString(sanitized) {
		t.Fatalf("truncated presentation is not valid UTF-8: %q", sanitized[len(sanitized)-4:])
	}
	if sanitized != strings.Repeat("a", maxPermissionPresentationBytes-1) {
		t.Fatalf("unexpected UTF-8 truncation length/content: %d bytes", len(sanitized))
	}
}

func TestProjectPermissionActionRedactsSchemelessURLCredentials(t *testing.T) {
	projection := ProjectPermissionAction("network", "Connect", map[string]any{
		"destination": "user:pass@example.com/private/path",
	})

	if projection.Destination != "example.com/private/path" || !projection.Redacted {
		t.Fatalf("unexpected schemeless credential projection: %+v", projection)
	}
}

func TestProjectPermissionActionDropsMCPArguments(t *testing.T) {
	projection := ProjectPermissionAction("mcp_tool", "Call tool", map[string]any{
		"server":    "github",
		"tool":      "create_issue",
		"arguments": map[string]any{"token": "hidden"},
	})

	if projection.Server != "github" || projection.Tool != "create_issue" {
		t.Fatalf("unexpected MCP identity: %+v", projection)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "arguments") || strings.Contains(string(encoded), "hidden") {
		t.Fatalf("projection leaked MCP arguments: %s", encoded)
	}
}
