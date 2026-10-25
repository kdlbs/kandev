package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestListMessagesResolvesRunningNoticeOutsidePage(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	tool := seedMessage(t, repo, &models.Message{
		ID: "compaction", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolCall,
		Content: "Compacting", CreatedAt: base, UpdatedAt: base,
	})
	seedMessage(t, repo, &models.Message{
		ID: "notice", AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeStatus,
		Content: "Still waiting", CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute),
		Metadata: map[string]interface{}{"action_visibility": "running"},
	})
	assertNotice := func(wantResolved bool) {
		t.Helper()
		messages, hasMore, err := svc.ListMessagesPaginated(ctx, ListMessagesRequest{TaskSessionID: "sess-msg", Limit: 1, Sort: "desc"})
		if err != nil || !hasMore || len(messages) != 1 || messages[0].ID != "notice" {
			t.Fatalf("paginated messages = %v, more=%v, err=%v", messages, hasMore, err)
		}
		if resolved := messages[0].Metadata["running_notice_resolved"] == true; resolved != wantResolved {
			t.Fatalf("notice resolved=%v, want %v", resolved, wantResolved)
		}
	}
	assertNotice(false)
	tool.Content = "Compacted"
	if err := repo.UpdateMessage(ctx, tool); err != nil {
		t.Fatal(err)
	}
	assertNotice(true)
	persisted, err := repo.GetMessage(ctx, "notice")
	if err != nil || persisted.Metadata["running_notice_resolved"] != nil {
		t.Fatalf("projection changed persisted notice: %v, err=%v", persisted, err)
	}
	messages, err := svc.ListMessages(ctx, "sess-msg")
	if err != nil || len(messages) != 2 || messages[1].Metadata["running_notice_resolved"] != true {
		t.Fatalf("full messages = %v, err=%v", messages, err)
	}
}
