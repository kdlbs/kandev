package sqlite

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func admissionGrade(t *testing.T, session *models.TaskSession) string {
	t.Helper()
	raw, err := json.Marshal(session)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	grade, _ := fields["admitted_execution_grade"].(string)
	return grade
}

func TestAdmittedGradeSQLitePreservesActiveOwner(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "frozen-grade-task", Title: "History sentinel"}); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{ID: "frozen-grade-session", TaskID: "frozen-grade-task", AgentProfileID: "logical", AgentProfileSnapshot: map[string]any{"execution_grade": "Mutable-Profile"}}
	if err := json.Unmarshal([]byte(`{"admitted_execution_grade":"Frozen-A","admitted_execution_grade_known":true}`), session); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if grade := admissionGrade(t, stored); grade != "Frozen-A" {
		t.Fatalf("stored admitted grade=%q want Frozen-A", grade)
	}
	if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	writer, ok := any(repo).(interface {
		SetAdmittedExecutionGrade(context.Context, string, string) error
	})
	if !ok {
		t.Fatal("repository does not support admitted-grade writes")
	}
	if err := writer.SetAdmittedExecutionGrade(ctx, session.ID, "Changed-B"); err == nil {
		t.Fatal("active owner grade changed")
	}
	if err := writer.SetAdmittedExecutionGrade(ctx, session.ID, "Frozen-A"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"admitted_execution_grade":"Stale-Wrong"}`), session); err != nil {
		t.Fatal(err)
	}
	session.State = models.TaskSessionStateRunning
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskSessionAgentProfileSnapshot(ctx, session.ID, map[string]any{"execution_grade": "Later-Profile"}); err != nil {
		t.Fatal(err)
	}
	reopened := NewWithInitializedDB(repo.db, repo.ro, nil)
	stored, err = reopened.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if grade := admissionGrade(t, stored); grade != "Frozen-A" {
		t.Fatalf("active grade after stale/full/profile writes=%q", grade)
	}
	if err := repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateWaitingForInput, ""); err != nil {
		t.Fatal(err)
	}
	if err := writer.SetAdmittedExecutionGrade(ctx, session.ID, "Next-B"); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if grade := admissionGrade(t, stored); grade != "Next-B" {
		t.Fatalf("new admission grade=%q", grade)
	}
}

func TestAdmittedGradeSQLiteUnknownIsNotConfiguredEmpty(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "unknown-grade-task", Title: "Unknown"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"legacy-active", "known-empty"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: id, TaskID: "unknown-grade-task"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetAdmittedExecutionGrade(ctx, "known-empty", ""); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"legacy-active", "known-empty"} {
		if err := repo.UpdateTaskSessionState(ctx, id, models.TaskSessionStateRunning, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SetAdmittedExecutionGrade(ctx, "legacy-active", ""); err == nil {
		t.Fatal("legacy active unknown was relabeled as observed empty")
	}
	for _, tc := range []struct {
		id    string
		known bool
	}{{"legacy-active", false}, {"known-empty", true}} {
		stored, err := repo.GetTaskSession(ctx, tc.id)
		if err != nil {
			t.Fatal(err)
		}
		if stored.AdmittedExecutionGradeKnown != tc.known || stored.AdmittedExecutionGrade != "" {
			t.Fatalf("snapshot=%+v", stored)
		}
	}
}
