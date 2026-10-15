package sqlite

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

func TestListSessionIDsWithOpenBackgroundWork(t *testing.T) {
	repo := newRepoForSessionTests(t)
	assertListSessionIDsWithOpenBackgroundWork(t, repo)
}

func assertListSessionIDsWithOpenBackgroundWork(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	wsID := uuid.New().String()
	taskID := uuid.New().String()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: wsID, Name: "WS"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: wsID, Title: "Task"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	openA, openB, closed := uuid.New().String(), uuid.New().String(), uuid.New().String()
	for _, sessionID := range []string{openA, openB, closed} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
			t.Fatalf("CreateTaskSession: %v", err)
		}
	}
	rows := []struct{ session, id, state string }{
		{openA, "a-running", "running"},
		{openA, "a-second", "waiting"},
		{openB, "b-unknown", "unknown"},
		{closed, "c-done", "completed"},
		{closed, "c-ended", "ended"},
	}
	for _, row := range rows {
		work := &models.BackgroundWorkload{
			ID: row.id, TaskID: taskID, SessionID: row.session, Kind: "shell", State: row.state, Revision: 1,
		}
		if err := repo.UpsertBackgroundWorkload(ctx, work); err != nil {
			t.Fatalf("UpsertBackgroundWorkload(%s): %v", row.id, err)
		}
	}

	got, err := repo.ListSessionIDsWithOpenBackgroundWork(ctx)
	if err != nil {
		t.Fatalf("ListSessionIDsWithOpenBackgroundWork: %v", err)
	}
	want := []string{openA, openB}
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sessions = %v, want %v", got, want)
	}
}
