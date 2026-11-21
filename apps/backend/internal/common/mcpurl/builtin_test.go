package mcpurl

import "testing"

func TestBuiltinMCPInstanceURL(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		port       int
		instanceID string
		scoped     bool
		endpoint   string
		want       string
	}{
		{name: "legacy empty host", port: 43120, endpoint: "/mcp", want: "http://localhost:43120/mcp"},
		{name: "scoped named host", host: "agentctl.internal", port: 43120, instanceID: "exec_A-1", scoped: true, endpoint: "/mcp", want: "http://agentctl.internal:43120/i/exec_A-1/mcp"},
		{name: "scoped wildcard host", host: "0.0.0.0", port: 43120, instanceID: "exec-1", scoped: true, endpoint: "/sse/packages/diagnostics", want: "http://127.0.0.1:43120/i/exec-1/sse/packages/diagnostics"},
		{name: "scoped IPv6 zone", host: "fe80::1%lo0", port: 43120, instanceID: "exec-1", scoped: true, endpoint: "/message", want: "http://[fe80::1%25lo0]:43120/i/exec-1/message"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuiltinURL(tt.host, tt.port, tt.instanceID, tt.scoped, tt.endpoint)
			if err != nil {
				t.Fatalf("BuiltinURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("BuiltinURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuiltinMCPInstancePathRejectsInvalidIdentityAndEndpoint(t *testing.T) {
	for _, id := range []string{"", ".", "..", "a/b", `a\\b`, "a b", "a?b", "a#b", "a%b", "é"} {
		t.Run("id_"+id, func(t *testing.T) {
			if _, err := BuiltinPath(id, true, "/mcp"); err == nil {
				t.Fatalf("BuiltinPath(%q, true, /mcp) succeeded", id)
			}
		})
	}
	for _, endpoint := range []string{"mcp", "//mcp", "/mcp/../message", "/mcp?x=1", "/mcp#fragment", "/other", "/mcp/packages/bad/id"} {
		t.Run("endpoint_"+endpoint, func(t *testing.T) {
			if _, err := BuiltinPath("exec-1", true, endpoint); err == nil {
				t.Fatalf("BuiltinPath(exec-1, true, %q) succeeded", endpoint)
			}
		})
	}
}

func TestBuiltinMCPInstanceLegacyAllowsEmptyIdentity(t *testing.T) {
	got, err := BuiltinPath("", false, "/mcp")
	if err != nil {
		t.Fatalf("BuiltinPath() error = %v", err)
	}
	if got != "/mcp" {
		t.Fatalf("BuiltinPath() = %q, want /mcp", got)
	}
}
