package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRecordTaskTransferAttemptPersistsRedactedSchemaRejectedFields(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	command := models.TaskTransferCommand{
		ExpectedSourceWorkspaceID: "ws-source",
		Actor: models.TaskTransferActor{
			Kind: models.TaskTransferActorRejected,
			ID:   "human-1",
		},
	}

	if err := repo.RecordTaskTransferAttempt(context.Background(), command, "failed"); err != nil {
		t.Fatalf("RecordTaskTransferAttempt: %v", err)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_transfer_audit
		WHERE task_id = '' AND source_workspace_id = ? AND result = 'failed'`, "ws-source"); err != nil {
		t.Fatalf("read task transfer audit: %v", err)
	}
	if count != 1 {
		t.Fatalf("task transfer audit rows = %d, want 1", count)
	}
}
