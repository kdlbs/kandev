package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func createTestNoteService(t *testing.T) (*NoteService, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewNoteService(repo, eventBus, log)
	return svc, eventBus, repo
}

func TestNoteService_GetNoteMissingReturnsNil(t *testing.T) {
	svc, _, repo := createTestNoteService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	note, err := svc.GetNote(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetNote: %v", err)
	}
	if note != nil {
		t.Fatalf("expected nil note, got %+v", note)
	}
}

func TestNoteService_UpsertNotePublishesUpdate(t *testing.T) {
	svc, eventBus, repo := createTestNoteService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	note, err := svc.UpsertNote(ctx, "task-1", "first note", "")
	if err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}
	if note == nil || note.Content != "first note" {
		t.Fatalf("unexpected note: %+v", note)
	}
	if note.UpdatedBy != "user" {
		t.Fatalf("expected default updated_by=user, got %q", note.UpdatedBy)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskNoteUpdated)
	payload, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", event.Data)
	}
	if got := payload["task_id"]; got != "task-1" {
		t.Fatalf("expected task_id task-1, got %v", got)
	}
	if got := payload["content"]; got != "first note" {
		t.Fatalf("expected content first note, got %v", got)
	}
	if got := payload["updated_by"]; got != "user" {
		t.Fatalf("expected updated_by user, got %v", got)
	}
	if got := payload["workspace_id"]; got != "ws-plan" {
		t.Fatalf("expected workspace_id ws-plan, got %v", got)
	}
}

func TestNoteService_ResolveWorkspaceIDFailsClosedForUnknownTask(t *testing.T) {
	svc, _, _ := createTestNoteService(t)
	ctx := context.Background()

	workspaceID, ok := svc.resolveWorkspaceID(ctx, "missing-task")
	if ok {
		t.Fatalf("expected ok=false for unknown task, got workspace_id=%q", workspaceID)
	}
	if workspaceID != "" {
		t.Fatalf("expected empty workspace_id, got %q", workspaceID)
	}
}

func TestNoteService_PublishEventSkippedWhenWorkspaceUnresolved(t *testing.T) {
	svc, eventBus, _ := createTestNoteService(t)
	ctx := context.Background()

	// note.TaskID references no seeded task, so the workspace can't be
	// resolved and the event must not be published unscoped.
	svc.publishEvent(ctx, events.TaskNoteUpdated, &models.TaskNote{TaskID: "missing-task", Content: "note"})
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("expected no events published when workspace can't be resolved, got %d", len(eventBus.GetPublishedEvents()))
	}

	svc.publishDeletedEvent(ctx, "missing-task")
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("expected no delete event published when workspace can't be resolved, got %d", len(eventBus.GetPublishedEvents()))
	}
}

func TestNoteService_DeleteNote(t *testing.T) {
	svc, eventBus, repo := createTestNoteService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")
	if _, err := svc.UpsertNote(ctx, "task-1", "first note", "agent"); err != nil {
		t.Fatalf("UpsertNote(seed): %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.DeleteNote(ctx, "task-1"); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
	got, err := svc.GetNote(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetNote(after delete): %v", err)
	}
	if got != nil {
		t.Fatalf("expected note to be deleted, got %+v", got)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskNoteDeleted)
	payload, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map payload, got %T", event.Data)
	}
	if gotTaskID := payload["task_id"]; gotTaskID != "task-1" {
		t.Fatalf("expected deleted task_id task-1, got %v", gotTaskID)
	}
}

func TestNoteService_DeleteMissingReturnsNotFound(t *testing.T) {
	svc, _, repo := createTestNoteService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	err := svc.DeleteNote(ctx, "task-1")
	if !errors.Is(err, ErrTaskNoteNotFound) {
		t.Fatalf("expected ErrTaskNoteNotFound, got %v", err)
	}
}

func TestNoteService_NotesAreScopedToTheCallerIdentity(t *testing.T) {
	svc, _, repo := createTestNoteService(t)
	base := context.Background()
	seedTask(t, base, repo, "task-1")

	ctxA := authn.WithIdentity(base, authn.Identity{UserID: "user-a"})
	ctxB := authn.WithIdentity(base, authn.Identity{UserID: "user-b"})

	if _, err := svc.UpsertNote(ctxA, "task-1", "a's note", ""); err != nil {
		t.Fatalf("UpsertNote(user-a): %v", err)
	}
	if _, err := svc.UpsertNote(ctxB, "task-1", "b's note", ""); err != nil {
		t.Fatalf("UpsertNote(user-b): %v", err)
	}

	gotA, err := svc.GetNote(ctxA, "task-1")
	if err != nil || gotA == nil || gotA.Content != "a's note" {
		t.Fatalf("user-a GetNote = %+v, %v; want a's note", gotA, err)
	}
	gotB, err := svc.GetNote(ctxB, "task-1")
	if err != nil || gotB == nil || gotB.Content != "b's note" {
		t.Fatalf("user-b GetNote = %+v, %v; want b's note", gotB, err)
	}

	// Deleting one user's note must not touch the other's.
	if err := svc.DeleteNote(ctxA, "task-1"); err != nil {
		t.Fatalf("DeleteNote(user-a): %v", err)
	}
	if got, err := svc.GetNote(ctxB, "task-1"); err != nil || got == nil {
		t.Fatalf("user-b note after user-a delete = %+v, %v; want it to survive", got, err)
	}
}

func TestNoteService_SyntheticAndAbsentIdentityShareTheUnscopedNote(t *testing.T) {
	svc, _, repo := createTestNoteService(t)
	base := context.Background()
	seedTask(t, base, repo, "task-1")

	// Auth disabled injects a synthetic identity; internal callers carry none.
	// Both must resolve to the same single-user note rather than forking one
	// note per call path.
	synthetic := authn.WithIdentity(base, authn.Identity{UserID: "admin", Synthetic: true})

	if _, err := svc.UpsertNote(synthetic, "task-1", "single-user note", ""); err != nil {
		t.Fatalf("UpsertNote(synthetic): %v", err)
	}
	got, err := svc.GetNote(base, "task-1")
	if err != nil || got == nil || got.Content != "single-user note" {
		t.Fatalf("GetNote(no identity) = %+v, %v; want the synthetic caller's note", got, err)
	}
	if got.UserID != "" {
		t.Fatalf("UserID = %q; want the unscoped owner", got.UserID)
	}
}
