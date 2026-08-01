package service

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateTaskRejectsTitleLongerThanLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title", Name: "Titles"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-title", WorkspaceID: "ws-title", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-title",
		WorkflowID:  "wf-title",
		Title:       strings.Repeat("x", 61),
	})
	if err == nil {
		t.Fatal("CreateTask accepted a title longer than 60 characters")
	}
	if !strings.Contains(err.Error(), "60") {
		t.Fatalf("CreateTask error = %q, want 60-character validation", err)
	}

	tasks, listErr := repo.ListTasks(ctx, "wf-title")
	if listErr != nil {
		t.Fatalf("ListTasks: %v", listErr)
	}
	if len(tasks) != 0 {
		t.Fatalf("persisted %d tasks after title validation failure, want none", len(tasks))
	}
}

func TestCreateTaskAcceptsTitleAtLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title", Name: "Titles"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-title", WorkspaceID: "ws-title", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-title",
		WorkflowID:  "wf-title",
		Title:       strings.Repeat("x", TaskTitleMaxLength),
	})
	if err != nil {
		t.Fatalf("CreateTask rejected a title at the limit: %v", err)
	}
}

func TestUpdateTaskRejectsTitleLongerThanLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title", Name: "Titles"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-title", WorkspaceID: "ws-title", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-title", WorkspaceID: "ws-title", WorkflowID: "wf-title", Title: "Original", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	title := strings.Repeat("x", 61)
	_, err := svc.UpdateTask(ctx, "task-title", &UpdateTaskRequest{Title: &title})
	if err == nil {
		t.Fatal("UpdateTask accepted a title longer than 60 characters")
	}
	if !strings.Contains(err.Error(), "60") {
		t.Fatalf("UpdateTask error = %q, want 60-character validation", err)
	}

	stored, getErr := repo.GetTask(ctx, "task-title")
	if getErr != nil {
		t.Fatalf("GetTask: %v", getErr)
	}
	if stored.Title != "Original" {
		t.Fatalf("stored title = %q, want unchanged original title", stored.Title)
	}
}

func TestUpdateTaskWithoutTitlePreservesLegacyOverlongTitle(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title", Name: "Titles"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-title", WorkspaceID: "ws-title", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	legacyTitle := strings.Repeat("legacy ", 12)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "legacy-title", WorkspaceID: "ws-title", WorkflowID: "wf-title", Title: legacyTitle, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	description := "updated description"
	if _, err := svc.UpdateTask(ctx, "legacy-title", &UpdateTaskRequest{Description: &description}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	stored, err := repo.GetTask(ctx, "legacy-title")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Title != legacyTitle {
		t.Fatalf("stored title = %q, want legacy title preserved", stored.Title)
	}
}

func TestTruncateTaskTitlePreservesCharacterLimit(t *testing.T) {
	longTitle := strings.Repeat("😀", TaskTitleMaxLength+5)

	got := TruncateTaskTitle(longTitle)
	if got != strings.Repeat("😀", TaskTitleMaxLength-1)+"…" {
		t.Fatalf("TruncateTaskTitle = %q, want a 60-character ellipsis title", got)
	}
	if gotLen := len([]rune(got)); gotLen != TaskTitleMaxLength {
		t.Fatalf("TruncateTaskTitle rune length = %d, want %d", gotLen, TaskTitleMaxLength)
	}
}

func TestTruncateTaskTitleLeavesShortTitlesUnchanged(t *testing.T) {
	for _, title := range []string{"", "short", strings.Repeat("x", TaskTitleMaxLength)} {
		if got := TruncateTaskTitle(title); got != title {
			t.Errorf("TruncateTaskTitle(%q) = %q, want unchanged title", title, got)
		}
	}
}
