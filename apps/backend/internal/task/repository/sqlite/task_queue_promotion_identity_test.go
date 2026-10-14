package sqlite

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSQLiteSameStepPromotionPreservesLargeEntryIdentity(t *testing.T) {
	verifySameStepPromotionPreservesLargeEntryIdentity(t, newRepoForEntityTests(t))
}

func TestPostgresSameStepPromotionPreservesLargeEntryIdentity(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	verifySameStepPromotionPreservesLargeEntryIdentity(t, repo)
}

func verifySameStepPromotionPreservesLargeEntryIdentity(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-promotion-identity")
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-promotion-identity", WorkspaceID: "workspace-promotion-identity", Name: "Workflow",
	}))
	seedCASWorkflowStep(t, repo, "workflow-promotion-identity", "step-identity-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-promotion-identity", "step-identity-queued", 1)
	task := &models.Task{
		ID: "task-promotion-identity", WorkspaceID: "workspace-promotion-identity",
		WorkflowID: "workflow-promotion-identity", WorkflowStepID: "step-identity-source",
		Title: "Queued candidate", WIPAdmitted: true,
		Metadata: map[string]interface{}{},
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	task.WorkflowStepID = "step-identity-queued"
	task.WIPAdmitted = false
	task.QueuedForStepID = task.WorkflowStepID
	require.NoError(t, repo.UpdateTask(ctx, task))
	queuedID, err := repo.GetLatestTaskStepTransitionID(ctx, task.ID)
	require.NoError(t, err)
	const entryID int64 = 1<<53 + 1
	result, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_step_transitions SET id = ? WHERE id = ? AND task_id = ?`), entryID, queuedID, task.ID)
	require.NoError(t, err)
	rows, err := result.RowsAffected()
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	candidate, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{
		"from_step_id": candidate.WorkflowStepID,
	}
	claimed, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		ctx, candidate, candidate.WorkflowStepID, candidate.WorkflowStepID, 0)
	require.NoError(t, err)
	require.True(t, claimed)
	require.Zero(t, candidate.WorkflowStepTransitionID)
	persisted, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	marker, ok := persisted.Metadata[models.MetaKeyQueuePromotionPending].(map[string]interface{})
	require.True(t, ok)
	var restoredID int64
	switch value := marker[models.QueuePromotionEntryTransitionIDKey].(type) {
	case string:
		restoredID, err = strconv.ParseInt(value, 10, 64)
		require.NoError(t, err)
	case float64:
		restoredID = int64(value)
	default:
		t.Fatalf("unsupported persisted entry identity %T", value)
	}
	require.Equal(t, entryID, restoredID, "persisted ledger identity must survive untyped JSON decoding")
	claimed, err = repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(
		ctx, candidate, candidate.WorkflowStepID, candidate.WorkflowStepID, 0)
	require.NoError(t, err)
	require.False(t, claimed, "the exact entry must be admitted only once")
}
