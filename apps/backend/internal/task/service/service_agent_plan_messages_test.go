package service

import (
	"context"
	"strings"
	"testing"
)

// @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1
// @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.2
func TestUpsertAgentPlanMessageCoalescesSnapshotsAndSeparatesToolCalls(t *testing.T) {
	service, _, repo := newMessageTestService(t)
	ctx := context.Background()

	for _, snapshot := range []string{"# Plan\n\n1. Read", "# Plan\n\n1. Read\n2. Write"} {
		err := service.UpsertAgentPlanMessage(
			ctx, "sess-msg", "source-call-1", snapshot, "task-msg", "",
		)
		if err != nil {
			t.Fatalf("upsert correlated plan: %v", err)
		}
	}

	if err := service.UpsertAgentPlanMessage(
		ctx, "sess-msg", "source-call-2", "# Other plan", "task-msg", "",
	); err != nil {
		t.Fatalf("upsert separate plan: %v", err)
	}

	messages, err := repo.ListMessages(ctx, "sess-msg")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].Content != "# Plan\n\n1. Read\n2. Write" {
		t.Fatalf("first plan content = %q", messages[0].Content)
	}
	if messages[1].Content != "# Other plan" {
		t.Fatalf("second plan content = %q", messages[1].Content)
	}
	if correlation, _ := messages[0].Metadata["tool_call_id"].(string); !strings.HasPrefix(correlation, "agent-plan:") {
		t.Fatalf("plan correlation = %q, want agent-plan namespace", correlation)
	}
	if source := messages[0].Metadata["agent_plan_tool_call_id"]; source != "source-call-1" {
		t.Fatalf("source tool call id = %v, want source-call-1", source)
	}
}
