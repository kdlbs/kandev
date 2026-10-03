package service_test

import (
	"context"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// TestQueueRun_TaskCommentPrefixedKeysNeverCoalesce pins shouldCoalesceRun's
// task_comment guard (pre-existing behavior, not introduced by
// REQ-OFFICE-GATE-COMMENT-001): two task_comment requests for the same
// agent, addressed to different comments, must both land as independent
// rows even though they land inside the same coalescing window. The gate
// comment fan-out (AC-OFFICE-GATE-COMMENT-001) relies on this — a second
// comment waking an already-woken seat must never overwrite the first
// comment's queued run.
func TestQueueRun_TaskCommentPrefixedKeysNeverCoalesce(t *testing.T) {
	svc, _, repo := newTestServiceWithRepo(t)
	ctx := context.Background()

	first, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:c1:review:task-1:a1:aaaa",
		Payload:        agentInPayload("a1"),
	})
	if err != nil {
		t.Fatalf("queue first comment run: %v", err)
	}
	if first != runsservice.QueueOutcomeQueued {
		t.Fatalf("first outcome = %q, want %q", first, runsservice.QueueOutcomeQueued)
	}

	second, err := svc.QueueRun(ctx, runsservice.QueueRunRequest{
		Reason:         "task_comment",
		IdempotencyKey: "task_comment:c2:review:task-1:a1:bbbb",
		Payload:        agentInPayload("a1"),
	})
	if err != nil {
		t.Fatalf("queue second comment run: %v", err)
	}
	if second != runsservice.QueueOutcomeQueued {
		t.Fatalf("second outcome = %q, want %q (task_comment keys must not coalesce)", second, runsservice.QueueOutcomeQueued)
	}

	var count int
	if err := repo.Reader().GetContext(ctx, &count,
		`SELECT COUNT(*) FROM runs WHERE agent_profile_id = 'a1' AND reason = 'task_comment'`,
	); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if count != 2 {
		t.Fatalf("runs for a1 = %d, want 2 (no coalescing across distinct comments)", count)
	}
}
