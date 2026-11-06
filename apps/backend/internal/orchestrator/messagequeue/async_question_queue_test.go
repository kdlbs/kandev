package messagequeue

import (
	"context"
	"errors"
	"testing"
)

func TestAsyncQuestionsQueueAdmissionIsNotMergedOrEditable(t *testing.T) {
	svc := newAutoMergeTestService(t, 10)
	svc.SetAutoMergeEnabled(true)
	ctx := context.Background()

	first, err := svc.QueueMessage(ctx, "session", "task", "user draft", "", QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue user draft: %v", err)
	}
	notification, err := svc.QueueMessageWithMetadata(ctx, "session", "task",
		"async answer", "", "async-question", false, nil,
		map[string]interface{}{MetadataAsyncQuestion: "question-1"})
	if err != nil {
		t.Fatalf("queue async notification: %v", err)
	}
	status := svc.GetStatus(ctx, "session")
	if status.Count != 2 {
		t.Fatalf("queue count = %d, want 2: an async continuation must not auto-merge into a draft", status.Count)
	}
	if notification.ID == first.ID {
		t.Fatal("async notification was folded into the user draft")
	}
	if _, err := svc.BeginEdit(ctx, "session", notification.ID, "connection-1"); !errors.Is(err, ErrEditLeaseNotFound) {
		t.Fatalf("edit = %v, want ErrEditLeaseNotFound for a generated notification", err)
	}
	if _, err := svc.ClaimSendNow(ctx, "session", []QueuedMessage{*notification}); !errors.Is(err, ErrAsyncQuestionNotSendable) {
		t.Fatalf("send now = %v, want ErrAsyncQuestionNotSendable", err)
	}
}
