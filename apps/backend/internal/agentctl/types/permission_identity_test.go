package types

import "testing"

func TestParseQualifiedMCPToolName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantServer string
		wantTool   string
		wantOK     bool
	}{
		{name: "qualified tool", input: "mcp__kandev__update_task_plan_kandev", wantServer: "kandev", wantTool: "update_task_plan_kandev", wantOK: true},
		{name: "hyphenated identifiers", input: "mcp__third-party__tool-name", wantServer: "third-party", wantTool: "tool-name", wantOK: true},
		{name: "single underscores in tool", input: "mcp__kandev__tool_with_underscores", wantServer: "kandev", wantTool: "tool_with_underscores", wantOK: true},
		{name: "delimiter in tool suffix is ambiguous", input: "mcp__kandev__tool__with__underscores"},
		{name: "delimiter in server name is ambiguous", input: "mcp__kandev__external__execute"},
		{name: "trailing underscore server is ambiguous", input: "mcp__kandev___execute"},
		{name: "missing prefix", input: "kandev__update_task_plan_kandev"},
		{name: "missing server", input: "mcp____update_task_plan_kandev"},
		{name: "missing tool", input: "mcp__kandev__"},
		{name: "whitespace", input: "mcp__kandev__tool name"},
		{name: "slash", input: "mcp__kandev__tool/name"},
		{name: "unicode identifier", input: "mcp__kandev__工具"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, tool, ok := ParseQualifiedMCPToolName(test.input)
			if server != test.wantServer || tool != test.wantTool || ok != test.wantOK {
				t.Fatalf("ParseQualifiedMCPToolName(%q) = (%q, %q, %v), want (%q, %q, %v)",
					test.input, server, tool, ok, test.wantServer, test.wantTool, test.wantOK)
			}
		})
	}
}
