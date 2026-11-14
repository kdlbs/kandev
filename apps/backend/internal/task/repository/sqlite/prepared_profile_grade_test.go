package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPreparedProfileGradeSQLiteSnapshot(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "snapshot-grade-task", Title: "Grade"}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "snapshot-grade-session", TaskID: "snapshot-grade-task", AgentProfileID: "profile-grade", AgentProfileSnapshot: map[string]any{"id": "profile-grade", "execution_grade": "Grade-A"}}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskSessionAgentProfileSnapshot(ctx, session.ID, map[string]any{"id": "profile-grade", "execution_grade": func() {}}); err == nil {
		t.Fatal("unserializable snapshot accepted")
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AgentProfileSnapshot["execution_grade"] != "Grade-A" || stored.AgentProfileSnapshot["id"] != "profile-grade" {
		t.Fatalf("snapshot changed after rejected write: %v", stored.AgentProfileSnapshot)
	}
	reopened := NewWithInitializedDB(repo.db, repo.ro, nil)
	stored, err = reopened.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AgentProfileSnapshot["execution_grade"] != "Grade-A" {
		t.Fatal("snapshot lost after repository reopen")
	}
}
