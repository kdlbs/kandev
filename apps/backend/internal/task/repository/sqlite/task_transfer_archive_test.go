package sqlite

import (
	"context"
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestTransferTaskPreservesDoneAutoArchiveClock(t *testing.T) {
	repo, task := seedTaskTransferFixture(t)
	oldUpdatedAt := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	mustExecTransferTest(t, repo, `UPDATE workflow_steps SET auto_archive_after_hours = 1 WHERE id IN (?, ?)`,
		"step-source-done", "step-destination-done")
	mustExecTransferTest(t, repo, `UPDATE tasks SET workflow_step_id = ?, queued_for_step_id = '',
		state = ?, updated_at = ? WHERE id = ?`, "step-source-done", v1.TaskStateCompleted, oldUpdatedAt, task.ID)
	task, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	command := taskTransferCommand(task)
	command.ExpectedSourceStepID = "step-source-done"
	command.DestinationStepID = "step-destination-done"
	receipt, err := repo.TransferTask(context.Background(), command)
	if err != nil {
		t.Fatalf("TransferTask: %v", err)
	}
	stored, err := repo.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("GetTask transferred: %v", err)
	}
	if !stored.UpdatedAt.Equal(oldUpdatedAt) || !receipt.TaskGeneration.Equal(oldUpdatedAt) {
		t.Fatalf("archive clock changed: task=%s receipt=%s want=%s",
			stored.UpdatedAt, receipt.TaskGeneration, oldUpdatedAt)
	}
	candidates, err := repo.ListTasksForAutoArchive(context.Background())
	if err != nil {
		t.Fatalf("ListTasksForAutoArchive: %v", err)
	}
	found := false
	for _, candidate := range candidates {
		found = found || candidate.ID == task.ID
	}
	if !found {
		t.Fatal("transferred Done task lost its existing auto-archive eligibility")
	}
}
