package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
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
