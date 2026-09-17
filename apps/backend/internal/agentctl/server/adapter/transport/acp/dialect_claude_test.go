package acp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestClaudePermissionIdentity(t *testing.T) {
	a := newTestAdapterForAgent(claudeAgentID)
	a.activeToolCalls["tool-1"] = &streams.NormalizedPayload{}
	var captured *PermissionRequest
	a.permissionHandler = func(_ context.Context, req *PermissionRequest) (*PermissionResponse, error) {
		captured = req
		return &PermissionResponse{OptionID: "allow-once"}, nil
	}

	_, err := a.handlePermissionRequest(t.Context(), &PermissionRequest{
		ToolCallID: "tool-1",
		Title:      "mcp__kandev__update_task_plan_kandev",
		ActionType: string(streams.ActionTypeOther),
		ToolMeta: map[string]any{
			"claudeCode": map[string]any{"toolName": "mcp__kandev__update_task_plan_kandev"},
		},
		Options: []PermissionOption{{
			OptionID: "allow-once",
			Kind:     streams.PermissionOptionKindAllowOnce,
		}},
	})
	if err != nil {
		t.Fatalf("handlePermissionRequest returned error: %v", err)
	}
	if captured == nil || captured.ToolName == nil || *captured.ToolName != "mcp__kandev__update_task_plan_kandev" {
		t.Fatalf("normalized ToolName = %#v, want mcp__kandev__update_task_plan_kandev", captured)
	}
}

func TestClaudePermissionIdentityNormalization(t *testing.T) {
	qualified := "mcp__kandev__update_task_plan_kandev"
	other := "mcp__other__tool"
	empty := ""

	tests := []struct {
		name         string
		programmatic *string
		meta         map[string]any
		title        string
		actionType   string
		want         *string
	}{
		{
			name:         "programmatic name is authoritative when metadata agrees",
			programmatic: &qualified,
			meta:         map[string]any{"claudeCode": map[string]any{"toolName": qualified}},
			title:        qualified,
			actionType:   claudePermissionOtherActionType,
			want:         &qualified,
		},
		{
			name:         "conflicting metadata blocks approval identity",
			programmatic: &qualified,
			meta:         map[string]any{"claudeCode": map[string]any{"toolName": other}},
			want:         &empty,
		},
		{
			name: "Claude metadata supplies qualified identity",
			meta: map[string]any{"claudeCode": map[string]any{"toolName": qualified}},
			want: &qualified,
		},
		{
			name:       "legacy exact title is accepted only for other kind",
			title:      qualified,
			actionType: claudePermissionOtherActionType,
			want:       &qualified,
		},
		{
			name:       "legacy ambiguous title has no identity",
			title:      "mcp__kandev__external__execute",
			actionType: claudePermissionOtherActionType,
		},
		{
			name:         "programmatic ambiguous name remains authoritative",
			programmatic: stringPtr("mcp__kandev__external__execute"),
			want:         stringPtr("mcp__kandev__external__execute"),
		},
		{
			name:       "legacy title with execute kind is not an identity",
			title:      qualified,
			actionType: "execute",
		},
		{
			name:       "malformed Claude metadata blocks title fallback",
			meta:       map[string]any{"claudeCode": "invalid"},
			title:      qualified,
			actionType: claudePermissionOtherActionType,
			want:       &empty,
		},
		{
			name:         "malformed Claude metadata blocks programmatic identity",
			programmatic: &qualified,
			meta:         map[string]any{"claudeCode": "invalid"},
			want:         &empty,
		},
		{
			name:       "malformed tool name blocks title fallback",
			meta:       map[string]any{"claudeCode": map[string]any{"toolName": 42}},
			title:      qualified,
			actionType: claudePermissionOtherActionType,
			want:       &empty,
		},
		{
			name: "unqualified metadata remains ineligible",
			meta: map[string]any{"claudeCode": map[string]any{"toolName": "update_task_plan_kandev"}},
			want: stringPtr("update_task_plan_kandev"),
		},
		{
			name:         "explicit empty name remains authoritative",
			programmatic: &empty,
			title:        qualified,
			actionType:   claudePermissionOtherActionType,
			want:         &empty,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := newACPDialect(claudeAgentID).normalizePermissionToolName(
				test.programmatic, test.meta, test.title, test.actionType)
			if (got == nil) != (test.want == nil) {
				t.Fatalf("normalized name = %v, want %v", got, test.want)
			}
			if got != nil && *got != *test.want {
				t.Fatalf("normalized name = %q, want %q", *got, *test.want)
			}
		})
	}
}

func TestNonClaudePermissionIdentityDoesNotUseClaudeTitleFallback(t *testing.T) {
	qualified := "mcp__kandev__update_task_plan_kandev"
	got := newACPDialect("other-agent").normalizePermissionToolName(nil, nil, qualified, claudePermissionOtherActionType)
	if got != nil {
		t.Fatalf("non-Claude normalized name = %q, want nil", *got)
	}
}

func stringPtr(value string) *string { return &value }
