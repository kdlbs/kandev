package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestResetPluginExecutorEnvironmentRejectsForeignTaskBeforeProviderCleanup(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reset-owner", Name: "Owner workspace", OwnerID: "user-b"}); err != nil {
		t.Fatalf("CreateWorkspace(): %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reset-owner", WorkspaceID: "ws-reset-owner", Name: "Owner flow"}); err != nil {
		t.Fatalf("CreateWorkflow(): %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-reset-owner", WorkspaceID: "ws-reset-owner", WorkflowID: "wf-reset-owner",
		WorkflowStepID: "step-reset-owner", Title: "Owner task", State: v1.TaskStateCreated, Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(): %v", err)
	}
	environment := &models.TaskEnvironment{
		ID: "env-reset-owner", TaskID: "task-reset-owner", ExecutorType: string(models.ExecutorTypePluginRemote), OwnershipGeneration: 4,
	}
	if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
		t.Fatalf("CreateTaskEnvironment(): %v", err)
	}
	destroyer := &stubDestroyer{}
	svc.SetSessionRunningChecker(&stubRunningChecker{running: false})
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(ctxAs("user-a"), "task-reset-owner", ResetOptions{})
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("ResetTaskEnvironment() error = %v, want task-not-found for a foreign task", err)
	}
	if len(destroyer.pluginCalls) != 0 {
		t.Fatalf("foreign-task reset made provider cleanup calls: %v", destroyer.pluginCalls)
	}
	retained, getErr := repo.GetTaskEnvironment(ctx, environment.ID)
	if getErr != nil || retained == nil {
		t.Fatalf("foreign environment after rejected reset = %+v, err=%v", retained, getErr)
	}
}
