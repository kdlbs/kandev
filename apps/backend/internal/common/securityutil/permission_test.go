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

func TestProjectPermissionActionStripsSchemelessURLQueryAndFragment(t *testing.T) {
	const canary = "s3cr3t-query-canary"
	projection := ProjectPermissionAction("network", "Connect", map[string]any{
		"destination": "example.com/private/path?X-Amz-Signature=" + canary + "#fragment",
	})

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), canary) {
		t.Fatalf("projection leaked schemeless URL query credentials: %s", encoded)
	}
	if projection.Destination != "example.com/private/path" || !projection.Redacted {
		t.Fatalf("unexpected schemeless URL projection: %+v", projection)
	}
}

func TestProjectPermissionActionMapsRealACPToolKinds(t *testing.T) {
	// These are the raw acp.ToolKind wire values agentctl forwards unchanged
	// as ActionType (see forwardPermissionRequest in
	// internal/agentctl/server/acp/client.go) — not the internal display
	// names ("command", "file_write", ...) this package also accepts.
	cases := []struct {
		acpKind    string
		wantType   string
		detailKey  string
		wantDetail string
	}{
		{acpKind: "execute", wantType: "command", detailKey: "command", wantDetail: "ls -la"},
		{acpKind: "edit", wantType: "file_write", detailKey: "path", wantDetail: "/workspace/file.go"},
		{acpKind: "read", wantType: "file_read", detailKey: "path", wantDetail: "/workspace/file.go"},
		{acpKind: "fetch", wantType: "network", detailKey: "destination", wantDetail: "example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.acpKind, func(t *testing.T) {
			projection := ProjectPermissionAction(tc.acpKind, "Approve action", map[string]any{
				tc.detailKey: tc.wantDetail,
			})
			if projection.Type != tc.wantType {
				t.Fatalf("acp kind %q: got Type %q, want %q", tc.acpKind, projection.Type, tc.wantType)
			}
			encoded, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), tc.wantDetail) {
				t.Fatalf("acp kind %q: projection dropped its display detail: %s", tc.acpKind, encoded)
			}
		})
	}
}

func TestProjectPermissionActionFailsClosedOnMalformedAbsoluteURL(t *testing.T) {
	const canary = "s3cr3t-canary-pass"
	// A space in the host is rejected by net/url.Parse ("invalid character
	// in host name"), so stripURLCredentials cannot confirm it stripped the
	// embedded userinfo. Before the fail-open fix, this returned the raw
	// value — including the credential — unchanged.
	projection := ProjectPermissionAction("network", "Connect", map[string]any{
		"destination": "https://user:" + canary + "@ex ample.com/",
	})

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), canary) {
		t.Fatalf("projection leaked credential from an unparseable absolute URL: %s", encoded)
	}
	if projection.Destination != "" || !projection.Redacted {
		t.Fatalf("unparseable absolute URL should fail closed: %+v", projection)
	}
}

func TestProjectPermissionActionFailsClosedOnSchemelessCredentialWithNoHost(t *testing.T) {
	const canary = "s3cr3t-canary-pass"
	// Looks credential-shaped (userinfo containing ':' before '@') but there
	// is nothing after '@' for net/url to resolve as a host. Before the
	// fail-open fix, this returned the raw value — including the credential
	// — unchanged.
	projection := ProjectPermissionAction("network", "Connect", map[string]any{
		"destination": "user:" + canary + "@",
	})

	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), canary) {
		t.Fatalf("projection leaked credential from a hostless schemeless value: %s", encoded)
	}
	if projection.Destination != "" || !projection.Redacted {
		t.Fatalf("hostless schemeless credential shape should fail closed: %+v", projection)
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
