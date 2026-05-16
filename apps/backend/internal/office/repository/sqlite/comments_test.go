package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestTaskComment_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	comment := &models.TaskComment{
		TaskID:     "task-1",
		AuthorType: "user",
		AuthorID:   "user-1",
		Body:       "This needs attention.",
		Source:     "user",
	}
	if err := repo.CreateTaskComment(ctx, comment); err != nil {
		t.Fatalf("create: %v", err)
	}

	comments, err := repo.ListTaskComments(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("count = %d, want 1", len(comments))
	}
	if comments[0].Body != "This needs attention." {
		t.Errorf("body = %q", comments[0].Body)
	}

	if err := repo.DeleteTaskComment(ctx, comment.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	comments, _ = repo.ListTaskComments(ctx, "task-1")
	if len(comments) != 0 {
		t.Errorf("count after delete = %d, want 0", len(comments))
	}
}
