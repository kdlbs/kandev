package sqlite

import (
	"context"
	"testing"
)

func TestTransferTaskRebindsTaskScopedPluginInstances(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE plugin_instances (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO plugin_instances (id, task_id, workspace_id) VALUES (?, ?, ?)`,
		"plugin-instance-1", task.ID, "ws-source")

	receipt, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.PreservationCounts["plugin_instances"] != 1 {
		t.Fatalf("plugin instance census = %+v", receipt.PreservationCounts)
	}
	var workspaceID string
	if err := repo.db.Get(&workspaceID, `SELECT workspace_id FROM plugin_instances WHERE id = ?`, "plugin-instance-1"); err != nil {
		t.Fatalf("read plugin instance: %v", err)
	}
	if workspaceID != "ws-destination" {
		t.Fatalf("plugin instance workspace = %q, want ws-destination", workspaceID)
	}
}

func TestTransferTaskRebindsTaskScopedCanvasLifecycleMetadata(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE canvas_lifecycle_metadata (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)
	mustExecTransferTest(t, repo, `INSERT INTO canvas_lifecycle_metadata (id, task_id, workspace_id) VALUES (?, ?, ?)`,
		"canvas-metadata-1", task.ID, "ws-source")

	receipt, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.PreservationCounts["canvas_lifecycle_metadata"] != 1 {
		t.Fatalf("canvas lifecycle census = %+v", receipt.PreservationCounts)
	}
	var workspaceID string
	if err := repo.db.Get(&workspaceID,
		`SELECT workspace_id FROM canvas_lifecycle_metadata WHERE id = ?`, "canvas-metadata-1"); err != nil {
		t.Fatalf("read canvas lifecycle metadata: %v", err)
	}
	if workspaceID != "ws-destination" {
		t.Fatalf("canvas lifecycle workspace = %q, want ws-destination", workspaceID)
	}
}
