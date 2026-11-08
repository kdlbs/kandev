package sqlite_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresEnsureHandoffCompletionOutboxReusesLegacyMarker races two
// receipt writers against a legacy report. It verifies PostgreSQL LIKE
// escaping, receipt idempotency, and preservation of the stored report. It
// skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresEnsureHandoffCompletionOutboxReusesLegacyMarker(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}

	ctx := context.Background()
	completionKey := "handoff_completed:task%_!:COMPLETED"
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	body := "Historical report body.\n\n<!-- handoff-completion:" + completionKey + " -->"
	legacy := &models.TaskComment{
		ID:         "legacy-report-uuid",
		TaskID:     "source-task",
		AuthorType: "agent",
		AuthorID:   "historical-agent",
		Body:       body,
		Source:     "handoff",
		CreatedAt:  createdAt,
	}
	if err := repo.CreateTaskComment(ctx, legacy); err != nil {
		t.Fatalf("seed legacy comment: %v", err)
	}

	item := sqlite.HandoffCompletionOutbox{
		CompletionKey:        completionKey,
		CommentID:            completionKey,
		SourceTaskID:         "source-task",
		SourceWorkspaceID:    "source-workspace",
		SourceSessionID:      "source-session",
		SourceAgentProfileID: "source-agent",
		DeliveryTaskID:       "delivery-task",
		DeliveryWorkspaceID:  "delivery-workspace",
		DeliveryTitle:        "Delivery",
		DeliveryState:        "COMPLETED",
		CommentEventPending:  true,
	}
	incoming := models.TaskComment{
		ID:         completionKey,
		TaskID:     "source-task",
		AuthorType: "agent",
		AuthorID:   "current-agent",
		Body:       "New report body.",
		Source:     "handoff",
	}
	start := make(chan struct{})
	errs := make([]error, 2)
	var workers sync.WaitGroup
	for i := range errs {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			itemCopy := item
			commentCopy := incoming
			errs[i] = repo.EnsureHandoffCompletionOutbox(ctx, &itemCopy, &commentCopy)
		}(i)
	}
	close(start)
	workers.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent ensure %d: %v", i, err)
		}
	}

	receipt, err := repo.GetHandoffCompletionOutbox(ctx, completionKey)
	if err != nil {
		t.Fatalf("read completion receipt: %v", err)
	}
	if receipt == nil {
		t.Fatal("completion receipt is missing")
	}
	if receipt.CommentID != legacy.ID {
		t.Errorf("receipt comment ID = %q, want legacy UUID %q", receipt.CommentID, legacy.ID)
	}
	storedComments, err := repo.ListTaskComments(ctx, "source-task")
	if err != nil {
		t.Fatalf("list source comments: %v", err)
	}
	if len(storedComments) != 1 || storedComments[0].ID != receipt.CommentID {
		t.Fatalf("source comments = %+v, want one comment referenced by the receipt", storedComments)
	}
	stored, err := repo.GetTaskComment(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("read legacy comment: %v", err)
	}
	if stored.Body != body || stored.AuthorID != "historical-agent" || !stored.CreatedAt.Equal(createdAt) {
		t.Errorf("legacy comment changed: body=%q author=%q created_at=%v", stored.Body, stored.AuthorID, stored.CreatedAt)
	}
}
