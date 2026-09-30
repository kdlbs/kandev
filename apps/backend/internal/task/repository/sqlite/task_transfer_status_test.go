package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestTransferTaskRejectsInactiveOrUnknownCEOStatus(t *testing.T) {
	tests := []struct {
		name              string
		sourceStatus      string
		destinationStatus string
		actor             models.TaskTransferActor
	}{
		{name: "paused source coordinator", sourceStatus: "paused", destinationStatus: "idle", actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorCoordinator, ID: "ceo-source", SessionID: "session-running",
			CallerTaskID: "task-transfer",
		}},
		{name: "unknown source coordinator status", sourceStatus: "future-status", destinationStatus: "idle", actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorCoordinator, ID: "ceo-source", SessionID: "session-running",
			CallerTaskID: "task-transfer",
		}},
		{name: "paused destination CEO", sourceStatus: "working", destinationStatus: "paused",
			actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"}},
		{name: "unknown destination CEO status", sourceStatus: "working", destinationStatus: "future-status",
			actor: models.TaskTransferActor{Kind: models.TaskTransferActorHuman, ID: "owner-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			mustExecTransferTest(t, repo, `UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`,
				"wf-source", "ws-source")
			mustExecTransferTest(t, repo, `CREATE TABLE agent_profiles (
				id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
				deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'idle')`)
			mustExecTransferTest(t, repo, `INSERT INTO agent_profiles
				(id, workspace_id, role, enabled, status) VALUES
				(?, ?, 'ceo', 1, ?), (?, ?, 'ceo', 1, ?)`,
				"ceo-source", "ws-source", tt.sourceStatus,
				"ceo-destination", "ws-destination", tt.destinationStatus)
			mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
				(id, step_id, task_id, role, agent_profile_id, decision_required, position)
				VALUES (?, ?, ?, 'runner', ?, 0, 0)`, "participant-ceo", task.WorkflowStepID, task.ID, "ceo-source")
			mustExecTransferTest(t, repo, `UPDATE task_sessions SET agent_profile_id = ? WHERE id = ?`,
				"ceo-source", "session-running")

			command := taskTransferCommand(task)
			command.Actor = tt.actor
			if _, err := repo.TransferTask(context.Background(), command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
				t.Fatalf("TransferTask error = %v, want conflict", err)
			}
		})
	}
}
