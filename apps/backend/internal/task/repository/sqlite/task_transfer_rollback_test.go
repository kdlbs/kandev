package sqlite

import (
	"context"
	"strings"
	"testing"
)

func TestTransferTaskCommitFailureRollsBackAndAudits(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE synthetic_transfer_workspaces (id TEXT PRIMARY KEY)`)
	mustExecTransferTest(t, repo, `INSERT INTO synthetic_transfer_workspaces (id) VALUES (?)`, "ws-source")
	mustExecTransferTest(t, repo, `CREATE TABLE task_delivery_ledger (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, workspace_id TEXT NOT NULL,
		FOREIGN KEY (workspace_id) REFERENCES synthetic_transfer_workspaces(id) DEFERRABLE INITIALLY DEFERRED)`)
	mustExecTransferTest(t, repo, `INSERT INTO task_delivery_ledger (id, task_id, workspace_id) VALUES (?, ?, ?)`,
		"projection-1", task.ID, "ws-source")

	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); err == nil ||
		!strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("TransferTask error = %v, want deferred foreign-key commit failure", err)
	}
	stored, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkspaceID != "ws-source" || stored.WorkflowID != "wf-source" {
		t.Fatalf("failed commit changed placement: %+v", stored)
	}
	var failed int
	if err := repo.db.Get(&failed, `SELECT COUNT(*) FROM task_transfer_audit WHERE task_id = ? AND result = 'failed'`, task.ID); err != nil {
		t.Fatalf("read failed audit: %v", err)
	}
	if failed != 1 {
		t.Fatalf("failed audit rows = %d, want 1", failed)
	}
	var operations int
	if err := repo.db.Get(&operations, `SELECT COUNT(*) FROM task_transfer_operations WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if operations != 0 {
		t.Fatalf("rollback left %d committed receipts", operations)
	}
}
