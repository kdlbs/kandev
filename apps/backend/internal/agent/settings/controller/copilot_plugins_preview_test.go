package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestCopilotPluginPreview(t *testing.T) {
	ctrl, _, _ := newCopilotDiscoveryController(t)
	resp, err := ctrl.PreviewAgentCommand(context.Background(), agents.CopilotACPAgentID, CommandPreviewRequest{
		CLIPassthrough:         true,
		CopilotSelectedPlugins: []string{"live:fixture-market/harness", "installed:copilot-plugins/spark"},
	})
	if err != nil {
		t.Fatalf("PreviewAgentCommand: %v", err)
	}
	if resp.DeferredPluginCount != 2 || !resp.CommandIncomplete {
		t.Fatalf("preview = %#v", resp)
	}
	if len(resp.DeferredPluginIDs) != 2 || resp.DeferredPluginIDs[0] != "installed:copilot-plugins/spark" {
		t.Fatalf("deferred IDs = %#v", resp.DeferredPluginIDs)
	}
	if !strings.Contains(resp.CommandString, "@github/copilot@1.0.88") {
		t.Fatalf("passthrough preview = %q, want selected managed runtime", resp.CommandString)
	}
	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "--plugin-dir") {
		t.Fatalf("preview exposed a resolved plugin mount: %s", encoded)
	}

	acp, err := ctrl.PreviewAgentCommand(context.Background(), agents.CopilotACPAgentID, CommandPreviewRequest{
		CopilotSelectedPlugins: []string{"installed:copilot-plugins/spark"},
	})
	if err != nil {
		t.Fatalf("ACP PreviewAgentCommand: %v", err)
	}
	if !strings.Contains(acp.CommandString, "@github/copilot@1.0.88") {
		t.Fatalf("ACP preview = %q, want the selected managed runtime", acp.CommandString)
	}

	empty, err := ctrl.PreviewAgentCommand(context.Background(), agents.CopilotACPAgentID, CommandPreviewRequest{CLIPassthrough: true})
	if err != nil {
		t.Fatalf("empty PreviewAgentCommand: %v", err)
	}
	if empty.DeferredPluginCount != 0 || empty.CommandIncomplete {
		t.Fatalf("empty preview = %#v", empty)
	}
	if strings.Contains(empty.CommandString, "@github/copilot@") {
		t.Fatalf("empty selection changed the passthrough base: %q", empty.CommandString)
	}

	if _, err := ctrl.PreviewAgentCommand(context.Background(), "claude-acp", CommandPreviewRequest{
		CopilotSelectedPlugins: []string{"installed:m/p"},
	}); err == nil {
		t.Fatal("non-Copilot preview with plugins unexpectedly succeeded")
	}
}
