package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

type planCommentMessageRepository interface {
	CreateMessageWithPlanComments(
		context.Context,
		*models.Message,
		[]models.TaskPlanCommentRef,
		bool,
	) (*models.TaskPlanCommentSnapshot, error)
}

func requirePlanCommentMessageRepository(t *testing.T, repo *Repository) planCommentMessageRepository {
	t.Helper()
	contract, ok := any(repo).(planCommentMessageRepository)
	if !ok {
		t.Fatal("Repository does not implement atomic plan-comment message creation")
	}
	return contract
}

func TestCreateMessageWithPlanCommentsConsumesAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "atomic")
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("atomic", "message-plan-comments")

	snapshot, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-atomic", Version: 1}}, true)
	if err != nil {
		t.Fatalf("CreateMessageWithPlanComments: %v", err)
	}
	if snapshot.Revision != 2 || len(snapshot.Comments) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	want := "### Plan Comments\n\n```\nselected\n```\n> stored atomic\n\n---\n\ntyped content"
	if message.Content != want {
		t.Fatalf("message content = %q, want %q", message.Content, want)
	}
	stored, err := repo.GetMessageWithPromptIndex(ctx, message.ID)
	if err != nil || stored.Content != want || stored.PromptIndex != 1 {
		t.Fatalf("stored message = %#v, err=%v", stored, err)
	}
}

func TestCreateMessageWithPlanCommentsRollsBackOnStaleReference(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "stale")
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("stale", "message-plan-comments-stale")

	snapshot, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-stale", Version: 9}}, false)
	var changed *plancommenttx.CommentsChangedError
	if !errors.As(err, &changed) || snapshot != nil || changed.Snapshot == nil {
		t.Fatalf("stale result snapshot=%#v err=%#v", snapshot, err)
	}
	if _, err := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rolled-back message lookup error = %v, want sql.ErrNoRows", err)
	}
	pending, err := repo.ListTaskPlanComments(ctx, "task-message-comments-stale")
	if err != nil || pending.Revision != 1 || len(pending.Comments) != 1 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, err)
	}
}

func TestCreateMessageWithPlanCommentsRejectsStalePrimary(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "primary")
	secondaryID := "session-message-comments-secondary"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: secondaryID, TaskID: "task-message-comments-primary", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("primary", "message-plan-comments-primary")
	message.TaskSessionID = secondaryID

	_, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-primary", Version: 1}}, true)
	var changed *plancommenttx.PrimarySessionChangedError
	if !errors.As(err, &changed) || changed.SessionID != "session-message-comments-primary" {
		t.Fatalf("primary guard error = %#v", err)
	}
	pending, listErr := repo.ListTaskPlanComments(ctx, "task-message-comments-primary")
	if listErr != nil || len(pending.Comments) != 1 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, listErr)
	}
}

func seedMessagePlanComment(t *testing.T, ctx context.Context, repo *Repository, suffix string) {
	t.Helper()
	taskID := "task-message-comments-" + suffix
	sessionID := "session-message-comments-" + suffix
	seedForMsgTest(t, repo, taskID, sessionID, "turn-message-comments-"+suffix)
	if err := repo.SetSessionPrimary(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-" + suffix, TaskID: taskID, Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-" + suffix, TaskID: taskID, PlanID: "plan-" + suffix,
		Body: "stored " + suffix, SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
}

func planCommentMessage(suffix, id string) *models.Message {
	return &models.Message{
		ID: id, TaskID: "task-message-comments-" + suffix,
		TaskSessionID: "session-message-comments-" + suffix,
		TurnID:        "turn-message-comments-" + suffix,
		AuthorType:    models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: plancomments.WithPlaceholder("typed content"),
	}
}
