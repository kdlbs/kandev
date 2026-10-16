package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestTransferTaskRejectsProjectBoundSourceWithAccurateConflict(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	mustExecTransferTest(t, repo, `UPDATE tasks SET project_id = ? WHERE id = ?`, "project-source", task.ID)
	task, _ = repo.GetTask(context.Background(), task.ID)

	_, err := repo.TransferTask(context.Background(), taskTransferCommand(task))
	if !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
		t.Fatalf("TransferTask error = %v, want conflict", err)
	}
	if !strings.Contains(err.Error(), "source task is project-bound") {
		t.Fatalf("TransferTask error = %q, want source project constraint", err)
	}
}

func TestTransferTaskRequiredPredicateConflictsLeaveBoardsUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*models.TaskTransferCommand)
	}{
		{name: "source workspace", mutate: func(command *models.TaskTransferCommand) {
			command.ExpectedSourceWorkspaceID = "wrong-workspace"
		}},
		{name: "source workflow", mutate: func(command *models.TaskTransferCommand) {
			command.ExpectedSourceWorkflowID = "wrong-workflow"
		}},
		{name: "destination lane missing", mutate: func(command *models.TaskTransferCommand) {
			command.DestinationStepID = "missing-step"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, task := seedTaskTransferFixture(t)
			command := taskTransferCommand(task)
			tt.mutate(&command)

			if _, err := repo.TransferTask(context.Background(), command); !errors.Is(err, repoerrors.ErrTaskTransferConflict) {
				t.Fatalf("TransferTask error = %v, want conflict", err)
			}
			assertTaskTransferBoardCounts(t, repo, task.ID, 1, 0)
		})
	}
}

func TestTransferTaskCommitAppearsOnExactlyOneBoard(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	if _, err := repo.TransferTask(context.Background(), taskTransferCommand(task)); err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	assertTaskTransferBoardCounts(t, repo, task.ID, 0, 1)
}

func TestTransferTaskPreservesBlockedReceiptWithoutResuming(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	now := time.Now().UTC()
	mustExecTransferTest(t, repo, `CREATE TABLE task_blockers (
		task_id TEXT NOT NULL, blocker_task_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
		PRIMARY KEY (task_id, blocker_task_id))`)
	mustExecTransferTest(t, repo, `INSERT INTO task_blockers (task_id, blocker_task_id, created_at) VALUES (?, ?, ?)`,
		task.ID, "blocker-task", now)
	mustExecTransferTest(t, repo, `UPDATE tasks SET workflow_step_id = ?, queued_for_step_id = '',
		state = ?, updated_at = ? WHERE id = ?`, "step-source-blocked", v1.TaskStateWaitingForInput, now, task.ID)
	mustExecTransferTest(t, repo, `UPDATE task_sessions SET state = ? WHERE id = ?`,
		models.TaskSessionStateWaitingForInput, "session-running")
	task, _ = repo.GetTask(context.Background(), task.ID)
	command := taskTransferCommand(task)
	command.ExpectedSourceStepID = "step-source-blocked"
	command.DestinationStepID = "step-destination-blocked"

	receipt, err := repo.TransferTask(context.Background(), command)
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	if receipt.PreservationCounts["task_blockers"] != 1 || len(receipt.Sessions) != 1 ||
		receipt.Sessions[0].State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("blocked preservation receipt = %+v", receipt)
	}
	var blockers int
	if err := repo.db.Get(&blockers, `SELECT COUNT(*) FROM task_blockers
		WHERE task_id = ? AND blocker_task_id = ?`, task.ID, "blocker-task"); err != nil {
		t.Fatalf("read blocker: %v", err)
	}
	if blockers != 1 {
		t.Fatalf("blocker rows = %d, want 1", blockers)
	}
}

func assertTaskTransferBoardCounts(
	t *testing.T,
	repo *Repository,
	taskID string,
	wantSource, wantDestination int,
) {
	t.Helper()
	var source, destination int
	if err := repo.db.Get(&source, `SELECT COUNT(*) FROM tasks WHERE id = ? AND workspace_id = ?`,
		taskID, "ws-source"); err != nil {
		t.Fatalf("count source board: %v", err)
	}
	if err := repo.db.Get(&destination, `SELECT COUNT(*) FROM tasks WHERE id = ? AND workspace_id = ?`,
		taskID, "ws-destination"); err != nil {
		t.Fatalf("count destination board: %v", err)
	}
	if source != wantSource || destination != wantDestination {
		t.Fatalf("board counts source=%d destination=%d, want source=%d destination=%d",
			source, destination, wantSource, wantDestination)
	}
}
