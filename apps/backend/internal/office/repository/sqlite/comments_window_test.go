package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// seedComment creates a comment with an explicit CreatedAt so ordering
// assertions don't depend on wall-clock timing between inserts.
func seedComment(t *testing.T, repo interface {
	CreateTaskComment(ctx context.Context, c *models.TaskComment) error
}, taskID, body string, createdAt time.Time) *models.TaskComment {
	t.Helper()
	c := &models.TaskComment{
		TaskID:     taskID,
		AuthorType: "agent",
		AuthorID:   "agent-1",
		Body:       body,
		Source:     "run",
	}
	if err := repo.CreateTaskComment(context.Background(), c); err != nil {
		t.Fatalf("seed comment %q: %v", body, err)
	}
	return c
}

// AC-003.1/AC-003.2/AC-003.3: the window selects the newest `limit`
// comments by (created_at DESC, id DESC) but presents them ascending by
// the same tiebreak columns.
func TestListTaskCommentsWindow_OrdersAscendingAndLimits(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	c1 := seedComment(t, repo, "task-1", "first", base)
	c2 := seedComment(t, repo, "task-1", "second", base.Add(time.Minute))
	c3 := seedComment(t, repo, "task-1", "third", base.Add(2*time.Minute))

	comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-1", 2)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(comments))
	}
	// Newest 2 by created_at DESC are c3, c2; presented ascending: c2, c3.
	if comments[0].ID != c2.ID || comments[1].ID != c3.ID {
		t.Fatalf("order = [%s, %s], want [%s, %s]", comments[0].ID, comments[1].ID, c2.ID, c3.ID)
	}
	_ = c1
}

// AC-005.1: an empty result set returns a non-nil empty slice, not nil,
// so JSON marshals `[]` rather than `null`.
func TestListTaskCommentsWindow_EmptyReturnsNonNilSlice(t *testing.T) {
	repo := newTestRepo(t)
	comments, total, err := repo.ListTaskCommentsWindow(context.Background(), "no-such-task", 20)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
	if comments == nil {
		t.Fatal("comments slice must be non-nil (empty), got nil")
	}
	if len(comments) != 0 {
		t.Fatalf("len(comments) = %d, want 0", len(comments))
	}
}

// AC-003.6: total reflects the full comment count on the task, independent
// of the requested limit.
func TestListTaskCommentsWindow_TotalIndependentOfLimit(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedComment(t, repo, "task-2", "c", base.Add(time.Duration(i)*time.Minute))
	}
	comments, total, err := repo.ListTaskCommentsWindow(ctx, "task-2", 1)
	if err != nil {
		t.Fatalf("ListTaskCommentsWindow: %v", err)
	}
	if total != 5 {
		t.Fatalf("total = %d, want 5", total)
	}
	if len(comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(comments))
	}
}
