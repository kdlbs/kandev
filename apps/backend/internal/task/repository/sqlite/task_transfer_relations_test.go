package sqlite

import (
	"context"
	"testing"
)

func TestInspectTaskTransferRelationsRejectsUnapprovedWorkspaceOwner(t *testing.T) {
	repo := newRepoForWorkflowSourceTests(t)
	mustExecTransferTest(t, repo, `CREATE TABLE task_transfer_unknown_owner (task_id TEXT NOT NULL, workspace_id TEXT NOT NULL)`)
	_, _, _, err := repo.inspectTaskTransferRelations(context.Background())
	if err == nil {
		t.Fatal("expected unapproved workspace-owned relation to fail closed")
	}
}
