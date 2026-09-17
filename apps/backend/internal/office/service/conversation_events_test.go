package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

func TestNativeConversationFirstMessageAndSelfReply(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "chief")
	insertTestTask(t, svc, "conversation", "ws-1")
	setTestTaskAssignee(t, svc, "conversation", "chief")
	svc.ExecSQL(t, `INSERT INTO office_channels
		(id, workspace_id, agent_profile_id, platform, config, webhook_secret, status, task_id, created_at, updated_at)
		VALUES ('channel', 'ws-1', 'chief', 'web', '{}', '', 'active', 'conversation', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	for _, author := range []string{"user", "agent"} {
		if err := svc.CreateComment(ctx, &models.TaskComment{
			TaskID: "conversation", AuthorType: author, AuthorID: "chief", Body: "status",
		}); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Reason != service.RunReasonTaskComment || runs[0].AgentProfileID != "chief" {
		t.Fatalf("expected one first-message run, no self-reply loop: %+v", runs)
	}
}

func TestNativeConversationMultipleDelegationBatches(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	createTestAgent(t, svc, "ws-1", "chief")
	insertTestTask(t, svc, "conversation", "ws-1")
	setTestTaskAssignee(t, svc, "conversation", "chief")
	svc.ExecSQL(t, `INSERT INTO office_channels
		(id, workspace_id, agent_profile_id, platform, config, webhook_secret, status, task_id, created_at, updated_at)
		VALUES ('channel', 'ws-1', 'chief', 'web', '{}', '', 'active', 'conversation', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	for i, id := range []string{"child-1", "child-1", "child-2"} {
		_, err := svc.QueueNativeConversation(ctx, "conversation", engine.TriggerOnChildrenCompleted,
			engine.OnChildrenCompletedPayload{ChildSummaries: []engine.ChildSummary{{TaskID: id, Status: "done", Summary: "verified result"}}},
			"children_completed:conversation")
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			rows, err := svc.ListRuns(ctx, "ws-1")
			if err != nil || len(rows) != 1 {
				t.Fatalf("first batch: %v", err)
			}
			if err := svc.FinishRun(ctx, rows[0].ID, service.RunOutcomeProcessed); err != nil {
				t.Fatal(err)
			}
		}
	}
	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("want two distinct batches, got %d", len(runs))
	}
	for _, run := range runs {
		if !strings.Contains(run.Payload, "verified result") {
			t.Fatal("worker result was dropped")
		}
	}
}
