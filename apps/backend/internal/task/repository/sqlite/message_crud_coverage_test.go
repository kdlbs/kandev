package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestMessageCRUDPaginationSearchAndMetadataLookups(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "task-message-crud", "session-message-crud", "turn-message-crud")
	seedForMsgTest(t, repo, "task-message-other", "session-message-other", "turn-message-other")

	base := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	messages := []*models.Message{
		{ID: "message-a", TaskSessionID: "session-message-crud", TaskID: "task-message-crud", TurnID: "turn-message-crud", Content: "literal 100% ready", CreatedAt: base, Metadata: map[string]any{"pending_id": "pending-shared", "question_id": "question-a", "status": "pending"}, Type: models.MessageTypeClarificationRequest, RequestsInput: true},
		{ID: "message-b", TaskSessionID: "session-message-crud", TaskID: "task-message-crud", TurnID: "turn-message-crud", Content: "literal under_score", CreatedAt: base.Add(time.Second), Metadata: map[string]any{"pending_id": "pending-shared", "question_id": "question-b", "tool_call_id": "tool-1", "status": "running"}, Type: models.MessageTypeToolCall},
		{ID: "message-c", TaskSessionID: "session-message-crud", TaskID: "task-message-crud", TurnID: "turn-message-crud", Content: "ordinary tail", CreatedAt: base.Add(2 * time.Second), Metadata: map[string]any{"tool_call_id": "tool-1", "status": "pending"}, Type: models.MessageTypePermissionRequest},
	}
	for _, message := range messages {
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatalf("CreateMessage(%s): %v", message.ID, err)
		}
	}

	listed, err := repo.ListMessages(ctx, "session-message-crud")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if got := messageIDs(listed); strings.Join(got, ",") != "message-a,message-b,message-c" {
		t.Fatalf("ListMessages IDs = %v, want stable created_at order", got)
	}

	page, more, err := repo.ListMessagesPaginated(ctx, "session-message-crud", models.ListMessagesOptions{Limit: 1})
	if err != nil {
		t.Fatalf("ListMessagesPaginated(first): %v", err)
	}
	if !more || len(page) != 1 || page[0].ID != "message-a" {
		t.Fatalf("first page = %v more=%v, want [message-a] and more", messageIDs(page), more)
	}
	page, more, err = repo.ListMessagesPaginated(ctx, "session-message-crud", models.ListMessagesOptions{After: "message-a", Limit: 5})
	if err != nil {
		t.Fatalf("ListMessagesPaginated(after): %v", err)
	}
	if more || strings.Join(messageIDs(page), ",") != "message-b,message-c" {
		t.Fatalf("after page = %v more=%v", messageIDs(page), more)
	}
	page, _, err = repo.ListMessagesPaginated(ctx, "session-message-crud", models.ListMessagesOptions{Before: "message-c", Sort: "desc", Limit: 5})
	if err != nil {
		t.Fatalf("ListMessagesPaginated(before desc): %v", err)
	}
	if strings.Join(messageIDs(page), ",") != "message-b,message-a" {
		t.Fatalf("before descending page = %v", messageIDs(page))
	}
	if _, _, err := repo.ListMessagesPaginated(ctx, "session-message-other", models.ListMessagesOptions{After: "message-a"}); err == nil {
		t.Fatal("cross-session cursor was accepted")
	}

	for query, wantID := range map[string]string{"100%": "message-a", "under_": "message-b"} {
		found, err := repo.SearchMessages(ctx, "session-message-crud", models.SearchMessagesOptions{Query: query})
		if err != nil {
			t.Fatalf("SearchMessages(%q): %v", query, err)
		}
		if len(found) != 1 || found[0].ID != wantID {
			t.Errorf("SearchMessages(%q) = %v, want [%s]", query, messageIDs(found), wantID)
		}
	}
	empty, err := repo.SearchMessages(ctx, "session-message-crud", models.SearchMessagesOptions{Query: "   "})
	if err != nil || empty != nil {
		t.Fatalf("blank SearchMessages = %v, %v; want nil, nil", empty, err)
	}

	pending, err := repo.FindMessagesByPendingID(ctx, "pending-shared")
	if err != nil || strings.Join(messageIDs(pending), ",") != "message-a,message-b" {
		t.Fatalf("FindMessagesByPendingID = %v, %v", messageIDs(pending), err)
	}
	bySessionPending, err := repo.GetMessageByPendingID(ctx, "session-message-crud", "pending-shared")
	if err != nil || bySessionPending.ID != "message-a" {
		t.Fatalf("GetMessageByPendingID = %+v, %v", bySessionPending, err)
	}
	latestPending, err := repo.FindMessageByPendingID(ctx, "pending-shared")
	if err != nil || latestPending.ID != "message-b" {
		t.Fatalf("FindMessageByPendingID = %+v, %v", latestPending, err)
	}
	pendingClarifications, err := repo.FindActiveClarificationMessagesBySessionID(ctx, "session-message-crud")
	if err != nil || strings.Join(messageIDs(pendingClarifications), ",") != "message-a" {
		t.Fatalf("FindActiveClarificationMessagesBySessionID = %v, %v", messageIDs(pendingClarifications), err)
	}
	question, err := repo.FindMessageByPendingIDAndQuestion(ctx, "session-message-crud", "pending-shared", "question-b")
	if err != nil || question.ID != "message-b" {
		t.Fatalf("FindMessageByPendingIDAndQuestion = %+v, %v", question, err)
	}
	tool, err := repo.GetMessageByToolCallID(ctx, "session-message-crud", "tool-1")
	if err != nil || tool.ID != "message-b" {
		t.Fatalf("GetMessageByToolCallID = %+v, %v; permission row must be excluded", tool, err)
	}

	updated, err := repo.CompletePendingToolCallsForTurn(ctx, "turn-message-crud")
	if err != nil || updated != 1 {
		t.Fatalf("CompletePendingToolCallsForTurn = %d, %v; want 1", updated, err)
	}
	tool, err = repo.GetMessage(ctx, "message-b")
	if err != nil || tool.Metadata["status"] != "complete" {
		t.Fatalf("completed tool metadata = %v, %v", tool, err)
	}
	permission, err := repo.GetMessage(ctx, "message-c")
	if err != nil || permission.Metadata["status"] != "pending" {
		t.Fatalf("permission metadata changed = %v, %v", permission, err)
	}

	if err := repo.DeleteMessage(ctx, "message-c"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if err := repo.DeleteMessage(ctx, "message-c"); err == nil {
		t.Fatal("second DeleteMessage returned nil")
	}
}

func messageIDs(messages []*models.Message) []string {
	ids := make([]string, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}
	return ids
}
