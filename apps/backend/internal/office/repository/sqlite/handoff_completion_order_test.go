package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func TestEnsureHandoffCompletionOutboxReusesEarliestLegacyAnchor(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	completionKey := "handoff_completed:task:COMPLETED"
	comments := []*models.TaskComment{
		{
			ID: "older-report-uuid", TaskID: "source-task", AuthorType: "agent",
			AuthorID: "older-agent", Body: "Older report.\n<!-- handoff-completion:" + completionKey + " -->",
			Source: "handoff", CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID: "newer-report-uuid", TaskID: "source-task", AuthorType: "agent",
			AuthorID: "newer-agent", Body: "Newer report.\n<!-- handoff-completion:" + completionKey + " -->",
			Source: "handoff", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, comment := range comments {
		if err := repo.CreateTaskComment(ctx, comment); err != nil {
			t.Fatalf("seed comment %s: %v", comment.ID, err)
		}
	}

	item := &sqlite.HandoffCompletionOutbox{
		CompletionKey: completionKey, CommentID: completionKey,
		SourceTaskID: "source-task", SourceWorkspaceID: "source-workspace",
		SourceSessionID: "source-session", SourceAgentProfileID: "source-agent",
		DeliveryTaskID: "delivery-task", DeliveryWorkspaceID: "delivery-workspace",
		DeliveryTitle: "Delivery", DeliveryState: "COMPLETED", CommentEventPending: true,
	}
	incoming := &models.TaskComment{
		ID: completionKey, TaskID: "source-task", AuthorType: "agent",
		AuthorID: "current-agent", Body: "Replacement report.", Source: "handoff",
	}
	if err := repo.EnsureHandoffCompletionOutbox(ctx, item, incoming); err != nil {
		t.Fatalf("ensure completion receipt: %v", err)
	}

	receipt, err := repo.GetHandoffCompletionOutbox(ctx, completionKey)
	if err != nil {
		t.Fatalf("read completion receipt: %v", err)
	}
	if receipt == nil || receipt.CommentID != comments[0].ID {
		t.Fatalf("receipt comment ID = %+v, want earliest legacy ID %q", receipt, comments[0].ID)
	}
}
