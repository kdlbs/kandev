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

	got, err := repo.GetTaskNote(ctx, "task-1", "")
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

	got, err = repo.GetTaskNote(ctx, "task-1", "")
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

	if err := repo.DeleteTaskNote(ctx, "task-1", ""); err != nil {
		t.Fatalf("DeleteTaskNote: %v", err)
	}
	got, err = repo.GetTaskNote(ctx, "task-1", "")
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

	got, err := repo.GetTaskNote(ctx, "task-1", "")
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

	err := repo.DeleteTaskNote(ctx, "task-1", "")
	if !errors.Is(err, ErrTaskNoteNotFound) {
		t.Fatalf("expected ErrTaskNoteNotFound, got %v", err)
	}
}

func TestNoteRepo_NotesAreIsolatedPerUser(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTaskNoteTask(t, ctx, repo, "task-1")

	for _, user := range []string{"", "user-a", "user-b"} {
		note := &models.TaskNote{TaskID: "task-1", UserID: user, Content: "note for " + user}
		if err := repo.UpsertTaskNote(ctx, note); err != nil {
			t.Fatalf("UpsertTaskNote(%q): %v", user, err)
		}
	}

	// Each owner reads back only their own row on the same task.
	for _, user := range []string{"", "user-a", "user-b"} {
		got, err := repo.GetTaskNote(ctx, "task-1", user)
		if err != nil {
			t.Fatalf("GetTaskNote(%q): %v", user, err)
		}
		if got == nil || got.Content != "note for "+user {
			t.Fatalf("GetTaskNote(%q) = %+v, want content %q", user, got, "note for "+user)
		}
	}

	// Deleting one owner's note leaves the others intact.
	if err := repo.DeleteTaskNote(ctx, "task-1", "user-a"); err != nil {
		t.Fatalf("DeleteTaskNote(user-a): %v", err)
	}
	if got, err := repo.GetTaskNote(ctx, "task-1", "user-a"); err != nil || got != nil {
		t.Fatalf("GetTaskNote(user-a) after delete = %+v, %v; want nil, nil", got, err)
	}
	if got, err := repo.GetTaskNote(ctx, "task-1", "user-b"); err != nil || got == nil {
		t.Fatalf("GetTaskNote(user-b) after deleting user-a = %+v, %v; want surviving note", got, err)
	}
}

func TestNoteRepo_UpsertUpdatesOnlyTheOwnersRow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTaskNoteTask(t, ctx, repo, "task-1")

	if err := repo.UpsertTaskNote(ctx, &models.TaskNote{TaskID: "task-1", UserID: "user-a", Content: "a1"}); err != nil {
		t.Fatalf("UpsertTaskNote(user-a): %v", err)
	}
	if err := repo.UpsertTaskNote(ctx, &models.TaskNote{TaskID: "task-1", UserID: "user-b", Content: "b1"}); err != nil {
		t.Fatalf("UpsertTaskNote(user-b): %v", err)
	}
	// Re-upserting user-a must hit the (task_id, user_id) conflict target and
	// leave user-b untouched rather than overwriting a single per-task row.
	if err := repo.UpsertTaskNote(ctx, &models.TaskNote{TaskID: "task-1", UserID: "user-a", Content: "a2"}); err != nil {
		t.Fatalf("UpsertTaskNote(user-a update): %v", err)
	}

	gotA, err := repo.GetTaskNote(ctx, "task-1", "user-a")
	if err != nil || gotA == nil || gotA.Content != "a2" {
		t.Fatalf("user-a note = %+v, %v; want content a2", gotA, err)
	}
	gotB, err := repo.GetTaskNote(ctx, "task-1", "user-b")
	if err != nil || gotB == nil || gotB.Content != "b1" {
		t.Fatalf("user-b note = %+v, %v; want content b1", gotB, err)
	}
}

// TestNoteRepo_MigratesLegacySingleNotePerTaskShape pins the rebuild that
// re-keys task_notes from `task_id UNIQUE` to `UNIQUE(task_id, user_id)`.
// Rows written before the change must survive as the unscoped ("") owner's
// notes, which is what the auth-disabled single-user path reads back.
func TestNoteRepo_MigratesLegacySingleNotePerTaskShape(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedTaskNoteTask(t, ctx, repo, "task-1")

	// Recreate the pre-migration shape and seed a legacy row.
	if _, err := repo.DB().ExecContext(ctx, `
		DROP TABLE task_notes;
		CREATE TABLE task_notes (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL DEFAULT '',
			updated_by TEXT NOT NULL DEFAULT 'user',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
		INSERT INTO task_notes (id, task_id, content, updated_by, created_at, updated_at)
		VALUES ('legacy-1', 'task-1', 'legacy content', 'user', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}

	if err := repo.migrateTaskNotesPerUser(); err != nil {
		t.Fatalf("migrateTaskNotesPerUser: %v", err)
	}

	got, err := repo.GetTaskNote(ctx, "task-1", "")
	if err != nil {
		t.Fatalf("GetTaskNote after migration: %v", err)
	}
	if got == nil || got.Content != "legacy content" || got.ID != "legacy-1" {
		t.Fatalf("migrated note = %+v; want legacy-1 with preserved content", got)
	}
	if got.UserID != "" {
		t.Fatalf("migrated note UserID = %q; want the unscoped owner", got.UserID)
	}

	// The new composite key must now allow a second owner on the same task.
	if err := repo.UpsertTaskNote(ctx, &models.TaskNote{TaskID: "task-1", UserID: "user-a", Content: "mine"}); err != nil {
		t.Fatalf("UpsertTaskNote(user-a) after migration: %v", err)
	}

	// Replaying the migration on the already-migrated DB is a no-op.
	if err := repo.migrateTaskNotesPerUser(); err != nil {
		t.Fatalf("migrateTaskNotesPerUser (replay): %v", err)
	}
	if got, err := repo.GetTaskNote(ctx, "task-1", "user-a"); err != nil || got == nil || got.Content != "mine" {
		t.Fatalf("note after replay = %+v, %v; want content mine", got, err)
	}
}
