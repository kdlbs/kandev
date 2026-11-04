package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPreparedSessionPrimaryElection(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	const taskID = "task-prepared-election"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Prepared election"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	first := &models.TaskSession{ID: "prepared-first", TaskID: taskID, IsPrimary: true}
	if err := repo.CreateTaskSession(ctx, first); err != nil {
		t.Fatalf("create first session: %v", err)
	}
	if !first.IsPrimary {
		t.Fatal("first prepared session must commit as primary")
	}

	second := &models.TaskSession{ID: "prepared-second", TaskID: taskID, IsPrimary: true}
	if err := repo.CreateTaskSession(ctx, second); err != nil {
		t.Fatalf("create second session: %v", err)
	}
	if second.IsPrimary {
		t.Fatal("a second prepared session must not demote the committed primary")
	}

	stored, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	primary := 0
	for _, session := range stored {
		if session.IsPrimary {
			primary++
			if session.ID != first.ID {
				t.Fatalf("unexpected primary session %q", session.ID)
			}
		}
	}
	if primary != 1 {
		t.Fatalf("primary sessions = %d, want 1", primary)
	}
}

func TestPreparedSessionSeedAtomicity(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	const taskID = "task-prepared-seed"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Prepared seed"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	preview := models.NewInitialPromptPreview("submitted text", nil)
	submission, err := models.NewInitialPromptSubmission("submitted text", false, nil)
	if err != nil {
		t.Fatalf("NewInitialPromptSubmission: %v", err)
	}
	seed := &models.PreparedSessionSeed{
		InitialPromptPreview:    preview,
		InitialPromptSubmission: submission,
	}
	session := &models.TaskSession{
		ID: "prepared-seed-session", TaskID: taskID,
		Metadata: seed.ApplyToMetadata(nil),
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create seeded session: %v", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if _, ok := stored.Metadata[models.SessionMetaKeyInitialPromptPreview]; !ok {
		t.Fatal("initial prompt preview was not committed with the session")
	}
	loaded, found, err := models.LoadInitialPromptSubmission(stored.Metadata)
	if err != nil || !found || loaded.Content != "submitted text" {
		t.Fatalf("initial prompt submission = %+v found=%v err=%v", loaded, found, err)
	}

	duplicate := &models.TaskSession{
		ID: session.ID, TaskID: taskID,
		Metadata: seed.ApplyToMetadata(nil),
	}
	if err := repo.CreateTaskSession(ctx, duplicate); err == nil {
		t.Fatal("duplicate session insert must fail")
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("failed insert changed stored sessions: %+v", sessions)
	}
}

func TestPreparedSessionCancellationWritesNothing(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	const taskID = "task-prepared-cancel"
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Prepared cancel"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	err := repo.CreateTaskSession(cancelled, &models.TaskSession{
		ID: "prepared-cancel-session", TaskID: taskID, IsPrimary: true,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled insert error = %v, want context.Canceled", err)
	}
	sessions, err := repo.ListTaskSessions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("cancelled insert persisted sessions: %+v", sessions)
	}
}
