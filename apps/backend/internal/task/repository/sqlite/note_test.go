package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedTaskNoteTask(t *testing.T, ctx context.Context, repo *Repository, taskID string) {
	t.Helper()
	seedWalkthroughTask(t, ctx, repo, taskID)
}

func TestNoteRepo_SchemaCreatesTaskNotesTable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	var name string
	err := repo.DB().QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'task_notes'`).Scan(&name)
	if err != nil {
		t.Fatalf("expected task_notes table to exist: %v", err)
	}
	if name != "task_notes" {
		t.Fatalf("expected task_notes table, got %q", name)
	}
}

func TestNoteRepo_UpsertGetUpdateDelete(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTaskNoteTask(t, ctx, repo, "task-1")

	note := &models.TaskNote{TaskID: "task-1", Content: "first", UpdatedBy: "user"}
	if err := repo.UpsertTaskNote(ctx, note); err != nil {
		t.Fatalf("UpsertTaskNote(create): %v", err)
	}
	if note.ID == "" || note.CreatedAt.IsZero() || note.UpdatedAt.IsZero() {
		t.Fatalf("expected persisted identity+timestamps, got %+v", note)
	}

	got, err := repo.GetTaskNote(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskNote(create): %v", err)
	}
	if got == nil || got.Content != "first" || got.UpdatedBy != "user" {
		t.Fatalf("unexpected created note: %+v", got)
	}

	createdAt := got.CreatedAt
	updatedAt := got.UpdatedAt
	updated := &models.TaskNote{TaskID: "task-1", Content: "second", UpdatedBy: "agent"}
	if err := repo.UpsertTaskNote(ctx, updated); err != nil {
		t.Fatalf("UpsertTaskNote(update): %v", err)
	}

	got, err = repo.GetTaskNote(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskNote(update): %v", err)
	}
	if got == nil || got.Content != "second" || got.UpdatedBy != "agent" {
		t.Fatalf("unexpected updated note: %+v", got)
	}
	if !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected created_at %v to be preserved, got %v", createdAt, got.CreatedAt)
	}
	if !got.UpdatedAt.After(updatedAt) && !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("expected updated_at >= %v, got %v", updatedAt, got.UpdatedAt)
	}

	if err := repo.DeleteTaskNote(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteTaskNote: %v", err)
	}
	got, err = repo.GetTaskNote(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskNote(after delete): %v", err)
	}
	if got != nil {
		t.Fatalf("expected note to be deleted, got %+v", got)
	}
}

func TestNoteRepo_GetMissingReturnsNil(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTaskNoteTask(t, ctx, repo, "task-1")

	got, err := repo.GetTaskNote(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTaskNote: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil note, got %+v", got)
	}
}

func TestNoteRepo_DeleteMissingReturnsNotFound(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTaskNoteTask(t, ctx, repo, "task-1")

	err := repo.DeleteTaskNote(ctx, "task-1")
	if !errors.Is(err, ErrTaskNoteNotFound) {
		t.Fatalf("expected ErrTaskNoteNotFound, got %v", err)
	}
}
