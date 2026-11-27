package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestTransferTaskRejectsCoordinatorCallerWithoutAuthoritativeOfficeIdentity(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `CREATE TABLE agent_profiles (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL DEFAULT '', role TEXT NOT NULL DEFAULT '',
		deleted_at TIMESTAMP, enabled INTEGER NOT NULL DEFAULT 1, status TEXT NOT NULL DEFAULT 'idle')`)
	mustExecTransferTest(t, repo, `INSERT INTO agent_profiles
		(id, workspace_id, role, enabled, status) VALUES (?, ?, 'ceo', 1, 'working')`, "ceo-source", "ws-source")
	caller := &models.Task{
		ID: "caller-without-office", WorkspaceID: "ws-source", WorkflowStepID: task.WorkflowStepID,
		Title: "Synthetic non-Office caller",
	}
	if err := repo.CreateTask(context.Background(), caller); err != nil {
		t.Fatalf("CreateTask caller: %v", err)
	}
	mustExecTransferTest(t, repo, `INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, ?, ?, 'runner', ?, 0, 0)`, "caller-runner", task.WorkflowStepID, caller.ID, "ceo-source")
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: "caller-session", TaskID: caller.ID, AgentProfileID: "ceo-source", State: models.TaskSessionStateRunning,
	}); err != nil {
		t.Fatalf("CreateTaskSession caller: %v", err)
	}
	mustExecTransferTest(t, repo, `UPDATE workspaces SET office_workflow_id = '' WHERE id = ?`, "ws-source")

	command := taskTransferCommand(task)
	command.Actor = models.TaskTransferActor{
		Kind: models.TaskTransferActorCoordinator, ID: "ceo-source", SessionID: "caller-session",
		CallerTaskID: caller.ID,
	}
	_, err := repo.TransferTask(context.Background(), command)
	if !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("TransferTask error = %v, want conflict", err)
	}
}
