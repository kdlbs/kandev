package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestServiceCreateMessageWithPreviewFeedbackConsumesAndReplays(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	sessionID, turnID := seedServicePlanComment(t, ctx, repo, "preview")
	preview := seedServicePreviewFeedback(t, ctx, repo, "preview")
	eventBus.ClearEvents()
	request := &CreateMessageRequest{
		TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID,
		Content: "typed body", AuthorType: "user",
		PreviewFeedbackRefs:   []models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}},
		RequirePrimarySession: true,
	}

	first, err := svc.CreateMessageIdempotent(ctx, "message-service-preview-feedback", request)
	if err != nil {
		t.Fatalf("CreateMessageIdempotent: %v", err)
	}
	for _, want := range []string{"typed body", "### Web Preview Feedback", "Runtime total", "node_path"} {
		if !strings.Contains(first.Content, want) {
			t.Fatalf("stored content missing %q:\n%s", want, first.Content)
		}
	}
	eventTypes := eventTypesForPlanCommentTest(eventBus)
	if len(eventTypes) != 3 || eventTypes[0] != events.MessageAdded ||
		eventTypes[1] != events.SessionPendingActionChanged ||
		eventTypes[2] != events.TaskPreviewFeedbackChanged {
		t.Fatalf("published event types = %v", eventTypes)
	}

	replayed, err := svc.CreateMessageIdempotent(ctx, "message-service-preview-feedback", request)
	if err != nil || replayed.ID != first.ID || replayed.Content != first.Content {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	pending, err := repo.ListTaskPreviewFeedback(ctx, "task-123")
	if err != nil || len(pending.Items) != 0 || pending.Revision != 2 {
		t.Fatalf("pending snapshot=%#v err=%v", pending, err)
	}
}

func seedServicePreviewFeedback(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	suffix string,
) *models.TaskPreviewFeedback {
	t.Helper()
	item := &models.TaskPreviewFeedback{
		ID: "preview-service-" + suffix, TaskID: "task-123",
		Kind: models.TaskPreviewFeedbackText, Comment: "Runtime total",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/totals", SelectedText: "$42",
		TextAnchor: json.RawMessage(`{
			"start":{"node_path":[0],"offset":0},
			"end":{"node_path":[0],"offset":3},
			"containing_element":{"tag":"strong","outer_html":"<strong>$42</strong>"}
		}`),
	}
	if _, err := repo.CreateTaskPreviewFeedback(ctx, item, "", ""); err != nil {
		t.Fatal(err)
	}
	return item
}
