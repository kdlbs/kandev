package sqlite

import (
	"testing"
)

func TestCodexMCPStartupWarningUsesMetadataRetention(t *testing.T) {
	if !retentionEventPayloadSupported("mcp_startup_warning", false) {
		t.Fatal("Codex MCP startup warning is not retained as session metadata")
	}
	if retentionEventPayloadSupported("mcp_startup_warning", true) {
		t.Fatal("Codex MCP startup warning was classified as a terminal delivery event")
	}
}
