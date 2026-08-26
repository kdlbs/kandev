package sqlite_test

import (
	"context"
	"testing"
	"time"

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

// AC-003.1/AC-003.2: CreateTaskComment must normalize a non-zero
// caller-supplied CreatedAt to UTC. task_comments.created_at is compared
// lexicographically (ORDER BY created_at DESC, id DESC), so mixing a
// local-offset timestamp with a UTC one on the same task can invert true
// chronological order even though both values name valid instants.
// This guards CreateTaskComment's own normalization directly; it does not
// depend on any caller already passing a UTC time. Actions.PostComment
// (internal/office/runtime/actions.go) sets CreatedAt: time.Now().UTC()
// itself, but CreateTaskComment must not regress if a future or external
// caller passes a non-UTC CreatedAt.
func TestCreateTaskComment_NormalizesNonZeroCreatedAtToUTC(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// olderLocal is genuinely earlier in real time (2026-01-01T01:00:00Z)
	// but carries a +08:00 offset, so its wall-clock digits are larger
	// than newerUTC's.
	loc := time.FixedZone("test+08:00", 8*60*60)
	olderLocal := &models.TaskComment{
		TaskID:     "task-tz",
		AuthorType: "agent",
		AuthorID:   "agent-1",
		Body:       "older-local",
		Source:     "run",
		CreatedAt:  time.Date(2026, 1, 1, 9, 0, 0, 0, loc),
	}
	// newerUTC is genuinely one hour later in real time.
	newerUTC := &models.TaskComment{
		TaskID:     "task-tz",
		AuthorType: "agent",
		AuthorID:   "agent-1",
		Body:       "newer-utc",
		Source:     "run",
		CreatedAt:  time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC),
	}
	if err := repo.CreateTaskComment(ctx, olderLocal); err != nil {
		t.Fatalf("create olderLocal: %v", err)
	}
	if err := repo.CreateTaskComment(ctx, newerUTC); err != nil {
		t.Fatalf("create newerUTC: %v", err)
	}

	if !olderLocal.CreatedAt.Equal(newerUTC.CreatedAt.Add(-time.Hour)) {
		t.Fatalf("test setup invalid: olderLocal %v is not exactly 1h before newerUTC %v",
			olderLocal.CreatedAt, newerUTC.CreatedAt)
	}

	comments, err := repo.ListTaskComments(ctx, "task-tz")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("count = %d, want 2", len(comments))
	}

	// True chronological order is olderLocal, then newerUTC.
	if comments[0].Body != "older-local" || comments[1].Body != "newer-utc" {
		t.Fatalf("order = [%s, %s], want [older-local, newer-utc] (true chronological order)",
			comments[0].Body, comments[1].Body)
	}

	if comments[0].CreatedAt.Location() != time.UTC {
		t.Errorf("olderLocal.CreatedAt location = %v, want UTC (normalized on write)", comments[0].CreatedAt.Location())
	}
}
