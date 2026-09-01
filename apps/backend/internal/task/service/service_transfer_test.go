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
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestServiceTransferTaskAuthorizesBothWorkspacesAndPublishesOneCanonicalUpdate(t *testing.T) {
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

	var moved, updated int
	for _, event := range eventBus.GetPublishedEvents() {
		switch event.Type {
		case events.TaskMoved:
			moved++
		case events.TaskUpdated:
			updated++
		}
	}
	if moved != 0 || updated != 1 {
		t.Fatalf("events moved=%d updated=%d", moved, updated)
	}
}

func TestServiceTransferTaskReconcilesVacatedSourceWIP(t *testing.T) {
	svc, _, repo := createTestService(t)
	task := seedServiceTaskTransfer(t, repo, "owner-1")
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-1", Role: authn.RoleMember})
	if _, err := repo.DB().Exec(`UPDATE workflow_steps SET wip_limit = 1 WHERE id IN (?, ?)`,
		"transfer-step-source", "transfer-step-destination"); err != nil {
		t.Fatalf("set WIP limits: %v", err)
	}
	if _, err := repo.DB().Exec(`UPDATE tasks SET wip_admitted = 1, queued_for_step_id = '' WHERE id = ?`,
		task.ID); err != nil {
		t.Fatalf("admit transfer task: %v", err)
	}
	task, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask transfer task: %v", err)
	}
	queued := &models.Task{
		ID: "transfer-queued-task", WorkspaceID: "transfer-ws-source", WorkflowID: "transfer-wf-source",
		WorkflowStepID: "transfer-step-source", Title: "Queued", State: v1.TaskStateTODO,
		QueuedForStepID: "transfer-step-source", WIPAdmitted: false,
	}
	if err := repo.CreateTask(ctx, queued); err != nil {
		t.Fatalf("CreateTask queued: %v", err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"transfer-step-source": {
			ID: "transfer-step-source", WorkflowID: "transfer-wf-source", Name: "Work", WIPLimit: 1,
		},
	}})

	command := serviceTaskTransferCommand(task)
	command.ExpectedTaskUpdatedAt = task.UpdatedAt
	if _, err := svc.TransferTask(ctx, command); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	stored, err := repo.GetTask(ctx, queued.ID)
	if err != nil {
		t.Fatalf("GetTask queued: %v", err)
	}
	if !stored.WIPAdmitted || stored.QueuedForStepID != "" {
		t.Fatalf("vacated source WIP was not reconciled: %+v", stored)
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
