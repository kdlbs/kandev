package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestListUnarchivedTasksWithActiveSessionsReturnsOnlyHolders(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-active-waiting", "task-active-done", "task-arch-running")
	ctx := context.Background()

	sessions := []struct {
		id     string
		taskID string
		state  models.TaskSessionState
	}{
		{"session-active-waiting", "task-active-waiting", models.TaskSessionStateWaitingForInput},
		{"session-active-done", "task-active-done", models.TaskSessionStateCompleted},
		{"session-arch-running", "task-arch-running", models.TaskSessionStateRunning},
	}
	for _, session := range sessions {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: session.id, TaskID: session.taskID, State: session.state,
		}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.id, err)
		}
	}
	if err := repo.ArchiveTask(ctx, "task-arch-running"); err != nil {
		t.Fatalf("ArchiveTask(task-arch-running): %v", err)
	}

	tasks, err := repo.ListUnarchivedTasksWithActiveSessions(ctx)
	if err != nil {
		t.Fatalf("ListUnarchivedTasksWithActiveSessions: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("candidate tasks = %d (%v), want exactly task-active-waiting", len(tasks), taskIDsOf(tasks))
	}
	if tasks[0].ID != "task-active-waiting" {
		t.Fatalf("candidate task ID = %q, want task-active-waiting", tasks[0].ID)
	}
	if tasks[0].WorkspaceID != archiveWorkspaceID {
		t.Fatalf("candidate workspace ID = %q, want %q", tasks[0].WorkspaceID, archiveWorkspaceID)
	}
}

func taskIDsOf(tasks []*models.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func TestGetLastMessageTimeBySessionIDsReturnsNewestPerSession(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-msg")
	ctx := context.Background()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-msg", TaskID: "task-msg", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-silent", TaskID: "task-msg", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	oldest := time.Now().Add(-3 * time.Hour).UTC().Truncate(time.Second)
	newest := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-msg", TaskSessionID: "session-msg", TaskID: "task-msg", StartedAt: oldest,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	for _, msg := range []*models.Message{
		{ID: "msg-old", TaskSessionID: "session-msg", TaskID: "task-msg", TurnID: "turn-msg", Content: "old", UpdatedAt: oldest},
		{ID: "msg-new", TaskSessionID: "session-msg", TaskID: "task-msg", TurnID: "turn-msg", Content: "new", UpdatedAt: newest},
	} {
		if err := repo.CreateMessage(ctx, msg); err != nil {
			t.Fatalf("CreateMessage(%s): %v", msg.ID, err)
		}
	}

	got, err := repo.GetLastMessageTimeBySessionIDs(ctx, []string{"session-msg", "session-silent"})
	if err != nil {
		t.Fatalf("GetLastMessageTimeBySessionIDs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("result sessions = %v, want only session-msg", got)
	}
	if !got["session-msg"].Equal(newest) {
		t.Fatalf("last message time = %v, want the newest message time %v", got["session-msg"], newest)
	}

	empty, err := repo.GetLastMessageTimeBySessionIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetLastMessageTimeBySessionIDs(nil): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("result for empty input = %v, want empty", empty)
	}
}
