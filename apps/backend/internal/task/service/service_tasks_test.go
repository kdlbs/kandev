package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeriveProvisionalTaskTitle(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        string
	}{
		{name: "first six normalized words", description: "  Ship\n this   task with a concise title now  ", want: "Ship this task with a concise"},
		{name: "short prompt uses every word", description: "  Fix   login flow ", want: "Fix login flow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := deriveProvisionalTaskTitle(tt.description)
			if err != nil {
				t.Fatalf("derive provisional title: %v", err)
			}
			if got != tt.want {
				t.Fatalf("title = %q, want %q", got, tt.want)
			}
		})
	}

	if _, err := deriveProvisionalTaskTitle("  \n\t "); !errors.Is(err, ErrAutoTitlePromptRequired) {
		t.Fatalf("empty prompt error = %v, want %v", err, ErrAutoTitlePromptRequired)
	}
}

func TestPrepareAutoTitleRejectsOfficeRequests(t *testing.T) {
	err := prepareAutoTitle(&CreateTaskRequest{
		ProjectID:   "office-project",
		AutoTitle:   true,
		Description: "Create an Office task",
	})
	if !errors.Is(err, ErrAutoTitleUnsupportedForOffice) {
		t.Fatalf("prepare office auto title error = %v, want %v", err, ErrAutoTitleUnsupportedForOffice)
	}
}

func TestCreateTaskAutoTitlePersistsPendingMarker(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-auto-title", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-auto-title", WorkspaceID: "ws-auto-title", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-auto-title",
		WorkflowID:  "wf-auto-title",
		Description: "  Add\n a better   task title for this request  ",
		AutoTitle:   true,
	})
	if err != nil {
		t.Fatalf("create auto-titled task: %v", err)
	}
	if task.Title != "Add a better task title for" {
		t.Fatalf("title = %q, want first six words", task.Title)
	}
	if !models.IsAgentTitlePending(task.Metadata) {
		t.Fatalf("metadata = %#v, want pending agent title", task.Metadata)
	}
	if len(eventBus.GetPublishedEvents()) != 1 || eventBus.GetPublishedEvents()[0].Type != "task.created" {
		t.Fatalf("events = %#v, want one task.created event", eventBus.GetPublishedEvents())
	}
	reloaded, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("reload auto-titled task: %v", err)
	}
	if !models.IsAgentTitlePending(reloaded.Metadata) {
		t.Fatalf("reloaded metadata = %#v, want pending marker", reloaded.Metadata)
	}
}

func TestCreateTaskAutoTitleSupportsSubtasks(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-auto-subtask", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-auto-subtask", WorkspaceID: "ws-auto-subtask", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-auto-subtask",
		WorkflowID:  "wf-auto-subtask",
		Title:       "Parent task",
	})
	if err != nil {
		t.Fatalf("create parent task: %v", err)
	}
	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-auto-subtask",
		WorkflowID:  "wf-auto-subtask",
		ParentID:    parent.ID,
		Description: "Investigate child behavior",
		AutoTitle:   true,
	})
	if err != nil {
		t.Fatalf("create auto-titled subtask: %v", err)
	}
	if child.ParentID != parent.ID || child.Title != "Investigate child behavior" || !models.IsAgentTitlePending(child.Metadata) {
		t.Fatalf("child = %#v, want parent, title, and pending marker", child)
	}
}

func TestSetPendingAgentTitleIsOneShotAndHumanRenameWins(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-agent-title", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-agent-title", WorkspaceID: "ws-agent-title", Name: "Workflow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	task, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-agent-title",
		WorkflowID:  "wf-agent-title",
		Title:       "Temporary prompt title",
		Description: "Do the work",
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true},
	})
	if err != nil {
		t.Fatalf("create pending task: %v", err)
	}
	eventBus.ClearEvents()

	updated, accepted, err := svc.SetPendingAgentTitle(ctx, task.ID, "Ship login fix")
	if err != nil || !accepted {
		t.Fatalf("set pending title = (%v, %v), want accepted", updated, err)
	}
	if updated.Title != "Ship login fix" || models.IsAgentTitlePending(updated.Metadata) {
		t.Fatalf("updated task = %#v, want resolved title and marker removed", updated)
	}
	if len(eventBus.GetPublishedEvents()) != 1 || eventBus.GetPublishedEvents()[0].Type != "task.updated" {
		t.Fatalf("events = %#v, want one task.updated event", eventBus.GetPublishedEvents())
	}
	_, accepted, err = svc.SetPendingAgentTitle(ctx, task.ID, "Late overwrite")
	if err != nil || accepted {
		t.Fatalf("second set = (accepted=%v, err=%v), want idempotent rejection", accepted, err)
	}

	pendingTask, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-agent-title",
		WorkflowID:  "wf-agent-title",
		Title:       "Temporary again",
		Description: "Do more work",
		Metadata:    map[string]interface{}{models.MetaKeyAgentTitlePending: true},
	})
	if err != nil {
		t.Fatalf("create second pending task: %v", err)
	}
	humanTitle := "Human chosen title"
	if _, err := svc.UpdateTask(ctx, pendingTask.ID, &UpdateTaskRequest{Title: &humanTitle}); err != nil {
		t.Fatalf("human rename: %v", err)
	}
	updated, accepted, err = svc.SetPendingAgentTitle(ctx, pendingTask.ID, "Agent overwrite")
	if err != nil || accepted || updated.Title != humanTitle {
		t.Fatalf("late agent rename = (%v, %v, %v), want human title preserved", updated.Title, accepted, err)
	}
}

// TestPerformTaskCleanup_QuickChatDir verifies that performTaskCleanup removes
// quick-chat workspace directories for both ephemeral and non-ephemeral tasks,
// and that tasks with no directory on disk produce no error.
func TestPerformTaskCleanup_QuickChatDir(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()
	quickChatDir := t.TempDir()
	svc.SetQuickChatDir(quickChatDir)

	makeSession := func(id string) *models.TaskSession {
		return &models.TaskSession{ID: id}
	}

	t.Run("ephemeral task with dir — dir removed", func(t *testing.T) {
		sessID := "sess-ephemeral"
		dir := filepath.Join(quickChatDir, sessID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		errs := svc.performTaskCleanup(ctx, "task-eph", []*models.TaskSession{makeSession(sessID)}, nil, nil, taskEnvironmentCleanup{}, nil)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected dir %s to be removed", dir)
		}
	})

	t.Run("non-ephemeral task with dir — dir removed", func(t *testing.T) {
		sessID := "sess-nonephemeral"
		dir := filepath.Join(quickChatDir, sessID)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		errs := svc.performTaskCleanup(ctx, "task-noeph", []*models.TaskSession{makeSession(sessID)}, nil, nil, taskEnvironmentCleanup{}, nil)
		if len(errs) != 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected dir %s to be removed", dir)
		}
	})

	t.Run("task with no dir on disk — no error", func(t *testing.T) {
		sessID := "sess-nodir"
		// Do not create the directory.
		errs := svc.performTaskCleanup(ctx, "task-nodir", []*models.TaskSession{makeSession(sessID)}, nil, nil, taskEnvironmentCleanup{}, nil)
		if len(errs) != 0 {
			t.Fatalf("expected no errors when dir absent, got: %v", errs)
		}
	})
}
