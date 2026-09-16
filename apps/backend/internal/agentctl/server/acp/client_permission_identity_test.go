package acp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestPermissionProgrammaticName(t *testing.T) {
	programmaticName := "mcp__kandev__update_task_plan_kandev"
	meta := map[string]any{
		"claudeCode": map[string]any{"toolName": "update_task_plan_kandev"},
	}
	var captured *types.PermissionRequest
	handler := func(_ context.Context, req *types.PermissionRequest) (*types.PermissionResponse, error) {
		captured = req
		return &types.PermissionResponse{OptionID: "allow-once"}, nil
	}

	client := NewClient()
	_, err := client.forwardPermissionRequest(context.Background(), handler, acpsdk.RequestPermissionRequest{
		SessionId: "session-1",
		ToolCall: acpsdk.ToolCallUpdate{
			ToolCallId: "tool-1",
			Title:      stringPtr("Update task plan"),
			Kind:       toolKindPtr(acpsdk.ToolKindOther),
			Name:       &programmaticName,
			Meta:       meta,
		},
		Options: []acpsdk.PermissionOption{{
			OptionId: "allow-once",
			Kind:     acpsdk.PermissionOptionKindAllowOnce,
		}},
	})
	if err != nil {
		t.Fatalf("forwardPermissionRequest returned error: %v", err)
	}
	if captured == nil {
		t.Fatal("permission handler was not called")
	}
	if captured.ToolName == nil || *captured.ToolName != programmaticName {
		t.Fatalf("ToolName = %v, want %q", captured.ToolName, programmaticName)
	}
	// claudeCode.toolName uses the short form intentionally. The client passes
	// both fields through as-is, and dialect normalization happens later in the
	// adapter. The mismatch proves that the client is not synthesizing a name.
	if captured.ToolMeta["claudeCode"].(map[string]any)["toolName"] != "update_task_plan_kandev" {
		t.Fatalf("ToolMeta = %#v, want Claude metadata", captured.ToolMeta)
	}

	encoded, err := json.Marshal(captured)
	if err != nil {
		t.Fatalf("marshal permission request: %v", err)
	}
	if strings.Contains(string(encoded), programmaticName) || strings.Contains(string(encoded), "claudeCode") {
		t.Fatalf("internal identity fields leaked into serialized request: %s", encoded)
	}
}

func TestRequestPermissionWithoutOptionsCancels(t *testing.T) {
	client := NewClient()
	response, err := client.RequestPermission(context.Background(), acpsdk.RequestPermissionRequest{
		SessionId: "session-1",
		ToolCall:  acpsdk.ToolCallUpdate{ToolCallId: "tool-1"},
	})
	if err != nil {
		t.Fatalf("RequestPermission returned error: %v", err)
	}
	if response.Outcome.Cancelled == nil || response.Outcome.Selected != nil {
		t.Fatalf("empty permission response = %+v, want cancelled", response.Outcome)
	}
}

func stringPtr(value string) *string { return &value }

func toolKindPtr(value acpsdk.ToolKind) *acpsdk.ToolKind { return &value }
