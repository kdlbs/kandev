package acp

import (
	"context"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestAntigravityPermissionOptionsPreserved pins that a native question keeps
// its exact option IDs, labels, order, and request identity through the ACP
// client. Several allow_once options are normal for these questions, so the
// selected answer must be the one the handler returned, not a positional
// default.
func TestAntigravityPermissionOptionsPreserved(t *testing.T) {
	var captured *types.PermissionRequest
	handler := func(_ context.Context, req *types.PermissionRequest) (*types.PermissionResponse, error) {
		captured = req
		return &types.PermissionResponse{OptionID: "option-b"}, nil
	}

	client := NewClient()
	response, err := client.forwardPermissionRequest(context.Background(), handler, acpsdk.RequestPermissionRequest{
		SessionId: "session-1",
		ToolCall: acpsdk.ToolCallUpdate{
			ToolCallId: acpcompat.AntigravityHumanInputPrefix + "question-1",
			Title:      stringPtr("Which approach?"),
			Kind:       toolKindPtr(acpsdk.ToolKindOther),
		},
		Options: []acpsdk.PermissionOption{
			{OptionId: "option-a", Name: "Option A", Kind: acpsdk.PermissionOptionKindAllowOnce},
			{OptionId: "option-b", Name: "Option B", Kind: acpsdk.PermissionOptionKindAllowOnce},
		},
	})
	if err != nil {
		t.Fatalf("forwardPermissionRequest returned error: %v", err)
	}
	if captured == nil {
		t.Fatal("permission handler was not called")
	}
	if captured.ToolCallID != acpcompat.AntigravityHumanInputPrefix+"question-1" {
		t.Fatalf("ToolCallID = %q, want the native request identity", captured.ToolCallID)
	}
	want := []types.PermissionOption{
		{OptionID: "option-a", Name: "Option A", Kind: streams.PermissionOptionKindAllowOnce},
		{OptionID: "option-b", Name: "Option B", Kind: streams.PermissionOptionKindAllowOnce},
	}
	if len(captured.Options) != len(want) {
		t.Fatalf("options = %+v, want %+v", captured.Options, want)
	}
	for i := range want {
		if !reflect.DeepEqual(captured.Options[i], want[i]) {
			t.Fatalf("option %d = %+v, want %+v", i, captured.Options[i], want[i])
		}
	}
	if response.Outcome.Selected == nil || string(response.Outcome.Selected.OptionId) != "option-b" {
		t.Fatalf("response outcome = %+v, want option-b selected exactly", response.Outcome)
	}
}
