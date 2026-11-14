package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func requireTaskExecutionGrade(t *testing.T, value any, want string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatal(err)
	}
	if body["execution_grade"] != want {
		t.Fatalf("grade=%v want %q", body["execution_grade"], want)
	}
}

func TestTaskExecutionGradeServiceCreate(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Grade"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "grade-wf", WorkspaceID: "ws-1", Name: "Grade"}); err != nil {
		t.Fatal(err)
	}
	var req CreateTaskRequest
	if err := json.Unmarshal([]byte(`{"workspace_id":"ws-1","workflow_id":"grade-wf","title":"Explicit grade","execution_grade":"Tier-A"}`), &req); err != nil {
		t.Fatal(err)
	}
	result, err := svc.CreateTask(ctx, &req)
	if err != nil {
		t.Fatal(err)
	}
	requireTaskExecutionGrade(t, result.Task, "Tier-A")
	requireTaskExecutionGrade(t, result.Task.ToAPI(), "Tier-A")
	stored, err := repo.GetTask(ctx, result.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireTaskExecutionGrade(t, stored, "Tier-A")
	req.Title = "Invalid"
	if err := json.Unmarshal([]byte(`{"execution_grade":" bad"}`), &req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTask(ctx, &req); err == nil {
		t.Fatal("invalid grade created task")
	}
}

func TestTaskExecutionGradeStaleServiceRequest(t *testing.T) {
	services, _, gates := taskFieldServicePair(t)
	seedFieldTask(t, gates[0].Repository, nil)
	title := "Rename"
	result, err := runFieldInterleaving(t, services[0], gates[0], &UpdateTaskRequest{Title: &title}, func(ctx context.Context) {
		var req UpdateTaskRequest
		if err := json.Unmarshal([]byte(`{"execution_grade":"Concurrent"}`), &req); err != nil {
			t.Fatal(err)
		}
		if _, err := services[1].UpdateTask(ctx, "field-task", &req); err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	requireTaskExecutionGrade(t, result, "Concurrent")
	if result.Title != title {
		t.Fatal("rename lost")
	}
}
