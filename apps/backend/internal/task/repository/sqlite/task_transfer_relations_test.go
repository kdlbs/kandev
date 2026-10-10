package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestInspectTaskTransferRelationsRejectsUnapprovedWorkspaceOwner(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE task_transfer_unknown_owner (task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO task_transfer_unknown_owner VALUES (?, ?)`, task.ID, task.WorkspaceID)
	_, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	if !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("unmapped task relation: %v, want conflict", err)
	}
	stored, err := repo.GetTask(context.Background(), task.ID)
	if err != nil || stored.WorkspaceID != task.WorkspaceID {
		t.Fatalf("task mutated after conflict: %+v, %v", stored, err)
	}
}

func TestTransferTaskIgnoresUnapprovedRelationsForOtherTasks(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE task_transfer_unknown_owner (task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO task_transfer_unknown_owner VALUES ('other-task', 'ws-source')`)
	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); err != nil {
		t.Fatalf("transfer without unmapped task rows: %v", err)
	}
	var workspace string
	if err := repo.db.Get(&workspace, `SELECT workspace_id FROM task_transfer_unknown_owner WHERE task_id = 'other-task'`); err != nil {
		t.Fatal(err)
	}
	if workspace != "ws-source" {
		t.Fatalf("unrelated workspace fence changed: %s", workspace)
	}
}

func TestTransferTaskPreservesUnmappedInventoryRecoveryFence(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `INSERT INTO workspace_inventory_recovery_receipts
		(id, task_id, workspace_id, session_id, task_environment_id, task_repository_id,
		 environment_repo_id, repository_id, idempotency_key, request_hash, result_code, receipt_json, created_at)
		VALUES ('recovery', ?, 'ws-source', 'session-running', 'environment-task', 'task-repository-1',
		 'environment-repository', 'repository-source', 'recovery-key', 'hash', 'repaired', '{}', CURRENT_TIMESTAMP)`, task.ID)
	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("unmapped recovery fence: %v, want conflict", err)
	}
	var workspace string
	if err := repo.db.Get(&workspace, `SELECT workspace_id FROM workspace_inventory_recovery_receipts WHERE id = 'recovery'`); err != nil {
		t.Fatal(err)
	}
	if workspace != "ws-source" {
		t.Fatalf("immutable recovery fence changed: %s", workspace)
	}
}

func TestInspectTaskTransferRelationsApprovesTaskScopedPluginInstances(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	mustExecTransferTest(t, repo, `CREATE TABLE plugin_instances (task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)

	projections, _, _, err := repo.inspectTaskTransferRelations(context.Background())
	if err != nil {
		t.Fatalf("inspectTaskTransferRelations: %v", err)
	}
	for _, projection := range projections {
		if projection.table == "plugin_instances" {
			return
		}
	}
	t.Fatal("plugin_instances was not registered as a workspace projection")
}
