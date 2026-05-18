package service_test

import (
	"context"
	"os"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestDeleteWorkspaceStopsTasksDeletesDataAndConfig(t *testing.T) {
	ctx := context.Background()
	taskSvc := &fakeWorkspaceTaskService{
		workspace: &taskmodels.Workspace{ID: "ws-delete", Name: "default"},
		tasks: []*taskmodels.Task{
			{ID: "task-1", WorkspaceID: "ws-delete"},
			{ID: "task-2", WorkspaceID: "ws-delete"},
		},
	}
	svc := newTestService(t, service.ServiceOptions{
		TaskWorkspace: taskSvc,
		TaskCanceller: &fakeTaskCanceller{},
	})

	createTestAgent(t, svc, "ws-delete", "agent-delete")
	if err := svc.CreateSkill(ctx, &officemodels.Skill{
		ID:          "skill-delete",
		WorkspaceID: "ws-delete",
		Name:        "Delete Skill",
		Slug:        "delete-skill",
	}); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	if err := svc.DeleteWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	if taskSvc.deletedWorkspace != "ws-delete" {
		t.Fatalf("deleted workspace = %q, want ws-delete", taskSvc.deletedWorkspace)
	}
	if got := taskSvc.deletedTasks; len(got) != 2 || got[0] != "task-1" || got[1] != "task-2" {
		t.Fatalf("deleted tasks = %#v, want task-1/task-2", got)
	}
	if _, err := os.Stat(svc.ConfigWriter().WorkspacePath("default")); !os.IsNotExist(err) {
		t.Fatalf("workspace config should be removed, stat err: %v", err)
	}
}

type fakeWorkspaceTaskService struct {
	workspace        *taskmodels.Workspace
	tasks            []*taskmodels.Task
	deletedTasks     []string
	deletedWorkspace string
}

func (f *fakeWorkspaceTaskService) GetWorkspace(context.Context, string) (*taskmodels.Workspace, error) {
	return f.workspace, nil
}

func (f *fakeWorkspaceTaskService) ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error) {
	return []*taskmodels.Workspace{f.workspace}, nil
}

func (f *fakeWorkspaceTaskService) DeleteWorkspace(_ context.Context, id string) error {
	f.deletedWorkspace = id
	return nil
}

func (f *fakeWorkspaceTaskService) ListTasksByWorkspace(
	context.Context,
	string,
	string,
	string,
	string,
	int,
	int,
	bool,
	bool,
	bool,
	bool,
) ([]*taskmodels.Task, int, error) {
	return f.tasks, len(f.tasks), nil
}

func (f *fakeWorkspaceTaskService) DeleteTask(_ context.Context, id string) error {
	f.deletedTasks = append(f.deletedTasks, id)
	return nil
}

func (f *fakeWorkspaceTaskService) GetLastAgentMessage(context.Context, string) (string, error) {
	return "", nil
}

type fakeTaskCanceller struct {
	taskIDs []string
}

func (f *fakeTaskCanceller) CancelTaskExecution(_ context.Context, taskID string, _ string, _ bool) error {
	f.taskIDs = append(f.taskIDs, taskID)
	return nil
}
