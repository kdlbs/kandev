package sqlite

import (
	"context"
	"testing"
	"time"
)

func TestTransferTaskRebindsExactTaskCommandOperationWorkspace(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	now := time.Now().UTC()
	mustExecTransferTest(t, repo, `
		INSERT INTO exact_task_command_operations (
			operation_id, workspace_id, task_id, payload_digest, result_resource_version, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, "operation-1", "ws-source", task.ID, "payload-digest", "resource-version", now)
	mustExecTransferTest(t, repo, `
		INSERT INTO task_completion_gate_history (
			id, task_id, workspace_id, revision, action, actor_kind, actor_id, reason, details, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "history-1", task.ID, "ws-source", 1, "verified", "user", "user-1", "accepted", "{}", now)
	mustExecTransferTest(t, repo, `
		INSERT INTO task_completion_gate_operations (
			operation_id, workspace_id, task_id, action, payload_digest, result_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "completion-operation-1", "ws-source", task.ID, "verify", "payload-digest", "{}", now)
	mustExecTransferTest(t, repo, `
		INSERT INTO task_completion_sets (task_id, workspace_id, revision, updated_at)
		VALUES (?, ?, ?, ?)
	`, task.ID, "ws-source", 1, now)
	mustExecTransferTest(t, repo, `
		INSERT INTO task_management_claims (
			task_id, workspace_id, generation, resource_version, updated_at
		) VALUES (?, ?, ?, ?, ?)
	`, task.ID, "ws-source", 0, "resource-version", now)
	mustExecTransferTest(t, repo, `
		INSERT INTO task_management_claim_history (
			id, task_id, workspace_id, action, generation, resource_version, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, "claim-history-1", task.ID, "ws-source", "released", 0, "resource-version", now)
	mustExecTransferTest(t, repo, `
		CREATE TABLE office_deferred_assignments (
			task_id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			agent_profile_id TEXT NOT NULL,
			assignment_generation INTEGER NOT NULL DEFAULT 0,
			pause_id TEXT NOT NULL DEFAULT '',
			actor_type TEXT NOT NULL DEFAULT '',
			actor_id TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			outcome TEXT NOT NULL DEFAULT ''
		)
	`)
	mustExecTransferTest(t, repo, `
		INSERT INTO office_deferred_assignments (
			task_id, workspace_id, agent_profile_id, created_at
		) VALUES (?, ?, ?, ?)
	`, task.ID, "ws-source", "agent-1", now)

	receipt, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.PreservationCounts["exact_task_command_operations"] != 1 {
		t.Fatalf("preservation counts = %v, want one exact command operation", receipt.PreservationCounts)
	}
	if receipt.PreservationCounts["task_completion_gate_history"] != 1 {
		t.Fatalf("preservation counts = %v, want one completion gate history row", receipt.PreservationCounts)
	}
	if receipt.PreservationCounts["task_completion_gate_operations"] != 1 {
		t.Fatalf("preservation counts = %v, want one completion gate operation", receipt.PreservationCounts)
	}
	if receipt.PreservationCounts["task_completion_sets"] != 1 {
		t.Fatalf("preservation counts = %v, want one completion set", receipt.PreservationCounts)
	}
	if receipt.PreservationCounts["task_management_claims"] != 1 || receipt.PreservationCounts["task_management_claim_history"] != 1 {
		t.Fatalf("preservation counts = %v, want claim and claim history rows", receipt.PreservationCounts)
	}
	if receipt.PreservationCounts["office_deferred_assignments"] != 1 {
		t.Fatalf("preservation counts = %v, want one deferred assignment", receipt.PreservationCounts)
	}

	var workspaceID string
	if err := repo.db.Get(&workspaceID,
		`SELECT workspace_id FROM exact_task_command_operations WHERE operation_id = ?`, "operation-1"); err != nil {
		t.Fatalf("read exact command operation workspace: %v", err)
	}
	if workspaceID != "ws-destination" {
		t.Fatalf("exact command operation workspace = %q, want ws-destination", workspaceID)
	}
	if err := repo.db.Get(&workspaceID,
		`SELECT workspace_id FROM task_completion_gate_history WHERE id = ?`, "history-1"); err != nil {
		t.Fatalf("read completion gate history workspace: %v", err)
	}
	if workspaceID != "ws-destination" {
		t.Fatalf("completion gate history workspace = %q, want ws-destination", workspaceID)
	}
	if err := repo.db.Get(&workspaceID,
		`SELECT workspace_id FROM task_completion_gate_operations WHERE operation_id = ?`, "completion-operation-1"); err != nil {
		t.Fatalf("read completion gate operation workspace: %v", err)
	}
	if workspaceID != "ws-destination" {
		t.Fatalf("completion gate operation workspace = %q, want ws-destination", workspaceID)
	}
	if err := repo.db.Get(&workspaceID,
		`SELECT workspace_id FROM task_completion_sets WHERE task_id = ?`, task.ID); err != nil {
		t.Fatalf("read completion set workspace: %v", err)
	}
	if workspaceID != "ws-destination" {
		t.Fatalf("completion set workspace = %q, want ws-destination", workspaceID)
	}
	for table, row := range map[string][2]string{
		"task_management_claims":        {"task_id", task.ID},
		"task_management_claim_history": {"id", "claim-history-1"},
		"office_deferred_assignments":   {"task_id", task.ID},
	} {
		if err := repo.db.Get(&workspaceID, `SELECT workspace_id FROM `+table+` WHERE `+row[0]+` = ?`, row[1]); err != nil {
			t.Fatalf("read %s workspace: %v", table, err)
		}
		if workspaceID != "ws-destination" {
			t.Fatalf("%s workspace = %q, want ws-destination", table, workspaceID)
		}
	}
}
