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
