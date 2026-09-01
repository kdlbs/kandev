package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestServiceTransferTaskAuthorizesBothWorkspacesAndPublishesOnlyTransfer(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})

	receipt, err := svc.TransferTask(ctx, serviceTaskTransferCommand(task))
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.TaskID != task.ID || receipt.DestinationWorkspaceID != "transfer-ws-destination" {
		t.Fatalf("receipt = %+v", receipt)
	}
	if _, err := repo.DB().Exec(`UPDATE workflows SET workspace_id = ? WHERE id = ?`,
		"transfer-ws-source", "transfer-wf-destination"); err != nil {
		t.Fatalf("invalidate live destination workflow: %v", err)
	}
	replayed, err := svc.TransferTask(ctx, serviceTaskTransferCommand(task))
	if err != nil || replayed.OperationID != receipt.OperationID {
		t.Fatalf("TransferTask replay = %+v, %v", replayed, err)
	}
	replayActor, found, err := svc.ResolveTaskTransferReplayActor(ctx, serviceTaskTransferCommand(task))
	if err != nil || !found || replayActor.ID != "owner-1" {
		t.Fatalf("ResolveTaskTransferReplayActor = %+v, found=%v, err=%v", replayActor, found, err)
	}

	var transferred, moved, updated int
	for _, event := range eventBus.GetPublishedEvents() {
		switch event.Type {
		case events.TaskTransferred:
			transferred++
		case events.TaskMoved:
			moved++
		case events.TaskUpdated:
			updated++
		}
	}
	if transferred != 1 || moved != 0 || updated != 0 {
		t.Fatalf("events transferred=%d moved=%d updated=%d", transferred, moved, updated)
	}
}

func TestServiceTransferTaskRejectsForeignDestinationWithoutMutation(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-2")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})

	_, err := svc.TransferTask(ctx, serviceTaskTransferCommand(task))
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("TransferTask error = %v, want workspace not found", err)
	}
	stored, getErr := repo.GetTask(context.Background(), task.ID)
	if getErr != nil {
		t.Fatalf("GetTask: %v", getErr)
	}
	if stored.WorkspaceID != "transfer-ws-source" || stored.WorkflowID != "transfer-wf-source" ||
		stored.WorkflowStepID != "transfer-step-source" {
		t.Fatalf("denied transfer mutated task: %+v", stored)
	}
	var denied int
	if err := repo.DB().QueryRow(
		`SELECT COUNT(*) FROM task_transfer_audit WHERE task_id = ? AND result = 'denied'`, task.ID,
	).Scan(&denied); err != nil {
		t.Fatalf("read denied audit: %v", err)
	}
	if denied != 1 {
		t.Fatalf("denied audit rows = %d, want 1", denied)
	}
}

func TestServiceTransferTaskReplayRequiresCurrentDestinationAccess(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
	command := serviceTaskTransferCommand(task)
	if _, err := svc.TransferTask(ctx, command); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if _, err := repo.DB().Exec(`UPDATE workspaces SET owner_id = ? WHERE id = ?`,
		"owner-2", "transfer-ws-destination"); err != nil {
		t.Fatalf("change destination owner: %v", err)
	}
	if _, err := svc.TransferTask(ctx, command); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("TransferTask replay error = %v, want task not found", err)
	}
	if _, found, err := svc.ResolveTaskTransferReplayActor(ctx, command); err == nil || found {
		t.Fatalf("ResolveTaskTransferReplayActor found=%v err=%v, want denied", found, err)
	}
}

func seedServiceTaskTransfer(t *testing.T, repo *sqliterepo.Repository, destinationOwner string) *models.Task {
	t.Helper()
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "transfer-ws-source", Name: "Source", OwnerID: "owner-1"},
		{ID: "transfer-ws-destination", Name: "Destination", OwnerID: destinationOwner},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace: %v", err)
		}
	}
	for _, workflow := range []*models.Workflow{
		{ID: "transfer-wf-source", WorkspaceID: "transfer-ws-source", Name: "Source"},
		{ID: "transfer-wf-destination", WorkspaceID: "transfer-ws-destination", Name: "Destination"},
	} {
		if err := repo.CreateWorkflow(ctx, workflow); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	for _, step := range []struct{ id, workflow string }{
		{"transfer-step-source", "transfer-wf-source"},
		{"transfer-step-destination", "transfer-wf-destination"},
	} {
		if _, err := repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position) VALUES (?, ?, 'Work', 0)`,
			step.id, step.workflow); err != nil {
			t.Fatalf("insert workflow step: %v", err)
		}
	}
	task := &models.Task{ID: "transfer-task", WorkspaceID: "transfer-ws-source", WorkflowID: "transfer-wf-source",
		WorkflowStepID: "transfer-step-source", Title: "Synthetic", State: v1.TaskStateInProgress}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return stored
}

func serviceTaskTransferCommand(task *models.Task) models.TaskTransferCommand {
	return models.TaskTransferCommand{
		TaskID: task.ID, ExpectedSourceWorkspaceID: "transfer-ws-source",
		ExpectedSourceWorkflowID: "transfer-wf-source", ExpectedSourceStepID: "transfer-step-source",
		ExpectedTaskUpdatedAt: task.UpdatedAt, DestinationWorkspaceID: "transfer-ws-destination",
		DestinationWorkflowID: "transfer-wf-destination", DestinationStepID: "transfer-step-destination",
		IdempotencyKey: "service-transfer-key", PreservationPolicy: models.TaskTransferPreservationPolicyV1,
		Actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"},
	}
}
