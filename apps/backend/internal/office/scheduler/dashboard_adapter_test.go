package scheduler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
)

func TestConvertChangeToMutation_PropagatesSkipAssigneeCommentWake(t *testing.T) {
	got := convertChangeToMutation(dashboard.TaskReactivityChange{
		Comment: &dashboard.TaskReactivityComment{
			ID:         "comment-1",
			Body:       "@Reviewer please look",
			AuthorType: "user",
			AuthorID:   "user-1",
		},
		SkipAssigneeCommentWake: true,
	})

	if got.Comment == nil {
		t.Fatal("Comment = nil, want converted comment")
	}
	if !got.Comment.SkipAssigneeWake {
		t.Fatal("SkipAssigneeWake = false, want true")
	}
}

func TestDashboardApprovalAdapter_PropagatesDecisionWakeContext(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "assignee-1")
	adapter := NewDashboardApprovalAdapter(ss)

	err := adapter.QueueApprovalRuns(context.Background(), []dashboard.ApprovalRun{{
		AgentID:         "assignee-1",
		Reason:          RunReasonTaskChangesRequested,
		TaskID:          "task-1",
		WorkspaceID:     "ws-1",
		ActorID:         "reviewer-1",
		ActorType:       "agent",
		DecisionComment: "The retry path still drops the error.",
		IdempotencyKey:  "decision:decision-1",
	}})
	if err != nil {
		t.Fatalf("QueueApprovalRuns: %v", err)
	}

	runs, err := repo.ListRuns(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(runs))
	}
	if runs[0].IdempotencyKey == nil || *runs[0].IdempotencyKey != "decision:decision-1" {
		t.Fatalf("idempotency key = %v, want decision:decision-1", runs[0].IdempotencyKey)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(runs[0].Payload), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["decision_comment"] != "The retry path still drops the error." {
		t.Fatalf("decision_comment = %q, want rejection reason", payload["decision_comment"])
	}
}

func TestDashboardApprovalAdapter_AllowsDistinctDecisionWakes(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "assignee-1")
	adapter := NewDashboardApprovalAdapter(ss)
	ctx := context.Background()

	keys := []string{"decision:decision-1", "decision:decision-2"}
	comments := []string{"round one", "round two"}
	for i, key := range keys {
		if err := adapter.QueueApprovalRuns(ctx, []dashboard.ApprovalRun{{
			AgentID:         "assignee-1",
			Reason:          RunReasonTaskChangesRequested,
			TaskID:          "task-1",
			WorkspaceID:     "ws-1",
			DecisionComment: comments[i],
			IdempotencyKey:  key,
		}}); err != nil {
			t.Fatalf("QueueApprovalRuns round %d: %v", i+1, err)
		}
		if i == 0 {
			// A real second review round occurs after the first run has been
			// delivered. Age the row past the coalescing window so this test
			// isolates idempotency-key behavior.
			ageRunsRequestedAt(t, ss, 10*time.Second)
		}
	}

	runs, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("runs = %d, want 2 for two decision rounds", len(runs))
	}
	if runs[0].IdempotencyKey == nil || runs[1].IdempotencyKey == nil || *runs[0].IdempotencyKey == *runs[1].IdempotencyKey {
		t.Fatalf("decision wake keys = %v, %v, want distinct keys", runs[0].IdempotencyKey, runs[1].IdempotencyKey)
	}
}
