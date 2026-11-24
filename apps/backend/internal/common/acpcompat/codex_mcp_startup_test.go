package acpcompat

import "testing"

func TestIsCodexMCPStartupReport(t *testing.T) {
	tests := []struct {
		name       string
		agentID    string
		toolCallID string
		title      string
		kind       string
		status     string
		want       bool
	}{
		{
			name:       "Codex startup report",
			agentID:    "codex-acp",
			toolCallID: "mcp_startup.server%20one.550e8400-e29b-41d4-a716-446655440000",
			title:      "mcp__server one__startup",
			kind:       "other",
			status:     "failed",
			want:       true,
		},
		{
			name:       "ordinary failed tool with a similar title",
			agentID:    "codex-acp",
			toolCallID: "mcp_startup.server.550e8400-e29b-41d4-a716-446655440000",
			title:      "mcp__server__startup check",
			kind:       "other",
			status:     "failed",
		},
		{
			name:       "startup identity from another provider",
			agentID:    "other-acp",
			toolCallID: "mcp_startup.server.550e8400-e29b-41d4-a716-446655440000",
			title:      "mcp__server__startup",
			kind:       "other",
			status:     "failed",
		},
		{
			name:       "invalid report UUID",
			agentID:    "codex-acp",
			toolCallID: "mcp_startup.server.not-a-uuid",
			title:      "mcp__server__startup",
			kind:       "other",
			status:     "failed",
		},
		{
			name:       "server identity mismatch",
			agentID:    "codex-acp",
			toolCallID: "mcp_startup.other.550e8400-e29b-41d4-a716-446655440000",
			title:      "mcp__server__startup",
			kind:       "other",
			status:     "failed",
		},
		{
			name:       "wrong kind",
			agentID:    "codex-acp",
			toolCallID: "mcp_startup.server.550e8400-e29b-41d4-a716-446655440000",
			title:      "mcp__server__startup",
			kind:       "execute",
			status:     "failed",
		},
		{
			name:       "non-failed status",
			agentID:    "codex-acp",
			toolCallID: "mcp_startup.server.550e8400-e29b-41d4-a716-446655440000",
			title:      "mcp__server__startup",
			kind:       "other",
			status:     "completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCodexMCPStartupReport(tt.agentID, tt.toolCallID, tt.title, tt.kind, tt.status); got != tt.want {
				t.Fatalf("IsCodexMCPStartupReport() = %v, want %v", got, tt.want)
			}
		})
	}
}
