package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func TestServiceCreateMessageWithPlanCommentsConsumesAndReplays(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	sessionID, turnID := seedServicePlanComment(t, ctx, repo, "accepted")
	eventBus.ClearEvents()
	request := &CreateMessageRequest{
		TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID,
		Content: plancomments.WithPlaceholder("typed body"), AuthorType: "user",
		PlanCommentRefs:       []models.TaskPlanCommentRef{{ID: "comment-service-accepted", Version: 1}},
		RequirePrimarySession: true,
	}

	first, err := svc.CreateMessageIdempotent(ctx, "message-service-plan-comment", request)
	if err != nil {
		t.Fatalf("CreateMessageIdempotent: %v", err)
	}
	if first.Content != "### Plan Comments\n\n```\nselected\n```\n> accepted feedback\n\n---\n\ntyped body" {
		t.Fatalf("stored content = %q", first.Content)
	}
	if got := eventTypesForPlanCommentTest(eventBus); len(got) != 3 ||
		got[0] != events.MessageAdded || got[1] != events.SessionPendingActionChanged ||
		got[2] != events.TaskPlanCommentsChanged {
		t.Fatalf("published event types = %v", got)
	}

	replayed, err := svc.CreateMessageIdempotent(ctx, "message-service-plan-comment", request)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.ID != first.ID || replayed.Content != first.Content {
		t.Fatalf("replay = %#v, want %#v", replayed, first)
	}
	if got := eventTypesForPlanCommentTest(eventBus); len(got) != 3 {
		t.Fatalf("replay published events: %v", got)
	}

	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-service-later", TaskID: "task-123", PlanID: "plan-service-accepted",
		Body: "later feedback", SelectedText: "later", AnchorFrom: 2, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
	different := *request
	different.PlanCommentRefs = []models.TaskPlanCommentRef{{ID: "comment-service-later", Version: 1}}
	if _, err := svc.CreateMessageIdempotent(ctx, "message-service-plan-comment", &different); !errors.Is(err, ErrMessageIDConflict) {
		t.Fatalf("different replay error = %v, want ErrMessageIDConflict", err)
	}
	pending, err := repo.ListTaskPlanComments(ctx, "task-123")
	if err != nil || len(pending.Comments) != 1 || pending.Comments[0].ID != "comment-service-later" {
		t.Fatalf("later comment snapshot = %#v, err=%v", pending, err)
	}
}

func TestServiceCreateMessageWithPlanCommentsRollsBackStaleReference(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	sessionID, turnID := seedServicePlanComment(t, ctx, repo, "stale")
	eventBus.ClearEvents()

	_, err := svc.CreateMessageIdempotent(ctx, "message-service-plan-comment-stale", &CreateMessageRequest{
		TaskSessionID: sessionID, TaskID: "task-123", TurnID: turnID,
		Content: plancomments.WithPlaceholder("typed body"), AuthorType: "user",
		PlanCommentRefs: []models.TaskPlanCommentRef{{ID: "comment-service-stale", Version: 8}},
	})
	if !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) {
		t.Fatalf("stale create error = %v", err)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("failed admission events = %#v", eventBus.GetPublishedEvents())
	}
	messages, err := repo.ListMessages(ctx, sessionID)
	if err != nil || len(messages) != 0 {
		t.Fatalf("messages after rollback = %#v, err=%v", messages, err)
	}
}

func seedServicePlanComment(
	t *testing.T,
	ctx context.Context,
	repo *sqliterepo.Repository,
	suffix string,
) (string, string) {
	t.Helper()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turnID := setupTestTurn(t, repo, sessionID, "task-123", "turn-service-comment-"+suffix)
	if err := repo.SetSessionPrimary(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-service-" + suffix, TaskID: "task-123", Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-service-" + suffix, TaskID: "task-123", PlanID: "plan-service-" + suffix,
		Body: suffix + " feedback", SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
	return sessionID, turnID
}

func eventTypesForPlanCommentTest(eventBus *MockEventBus) []string {
	events := eventBus.GetPublishedEvents()
	types := make([]string, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}
