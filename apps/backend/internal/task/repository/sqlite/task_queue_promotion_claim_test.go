package sqlite

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSQLiteClaimQueuePromotionEntry(t *testing.T) {
	verifyQueuePromotionEntryClaims(t, newRepoForEntityTests(t), "sqlite")
}

func TestPostgresClaimQueuePromotionEntry(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	verifyQueuePromotionEntryClaims(t, repo, "postgres")
}

func TestSQLiteCrossStepPromotionPinsEntryForLegacyClaim(t *testing.T) {
	verifyCrossStepPromotionPinsEntry(t, newRepoForEntityTests(t), "sqlite")
}

func TestPostgresCrossStepPromotionPinsEntryForLegacyClaim(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	verifyCrossStepPromotionPinsEntry(t, repo, "postgres")
}

func verifyQueuePromotionEntryClaims(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()

	t.Run("claims the current entry once", func(t *testing.T) {
		taskID := prefix + "-claim-current"
		entryID := seedQueuePromotionClaimTask(t, repo, taskID)

		claimed, err := repo.ClaimQueuePromotionEntry(ctx, taskID, entryID)
		require.NoError(t, err)
		require.True(t, claimed)
		assertQueuePromotionMarker(t, repo, taskID, false, "")
	})

	t.Run("duplicate delivery does not claim twice", func(t *testing.T) {
		taskID := prefix + "-claim-duplicate"
		entryID := seedQueuePromotionClaimTask(t, repo, taskID)

		claimed, err := repo.ClaimQueuePromotionEntry(ctx, taskID, entryID)
		require.NoError(t, err)
		require.True(t, claimed)
		claimed, err = repo.ClaimQueuePromotionEntry(ctx, taskID, entryID)
		require.NoError(t, err)
		require.False(t, claimed)
	})

	t.Run("stale entry preserves the newer marker", func(t *testing.T) {
		taskID := prefix + "-claim-stale"
		oldEntryID := seedQueuePromotionClaimTask(t, repo, taskID)
		snapshot, err := repo.GetTask(ctx, taskID)
		require.NoError(t, err)

		moveReady := make(chan struct{})
		moveDone := make(chan error, 1)
		go func() {
			<-moveReady
			moved := *snapshot
			moved.WorkflowStepID = "step-newer"
			if err := repo.UpdateTask(ctx, &moved); err != nil {
				moveDone <- err
				return
			}
			moveDone <- repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyQueuePromotionPending,
				map[string]interface{}{"from_step_id": "step-newer-source"})
		}()
		close(moveReady)
		require.NoError(t, <-moveDone)
		newEntryID, err := repo.GetLatestTaskStepTransitionID(ctx, taskID)
		require.NoError(t, err)
		require.Greater(t, newEntryID, oldEntryID)

		claimed, err := repo.ClaimQueuePromotionEntry(ctx, taskID, oldEntryID)
		require.NoError(t, err)
		require.False(t, claimed)
		assertQueuePromotionMarker(t, repo, taskID, true, "step-newer-source")

		claimed, err = repo.ClaimQueuePromotionEntry(ctx, taskID, newEntryID)
		require.NoError(t, err)
		require.True(t, claimed)
		assertQueuePromotionMarker(t, repo, taskID, false, "")
	})

	t.Run("absent marker remains unclaimed", func(t *testing.T) {
		taskID := prefix + "-claim-absent"
		entryID := seedQueuePromotionClaimTaskWithoutMarker(t, repo, taskID)
		claimed, err := repo.ClaimQueuePromotionEntry(ctx, taskID, entryID)
		require.NoError(t, err)
		require.False(t, claimed)
		assertQueuePromotionMarker(t, repo, taskID, false, "")
	})

	t.Run("legacy delivery can claim an unpinned marker", func(t *testing.T) {
		taskID := prefix + "-claim-legacy"
		seedQueuePromotionClaimTask(t, repo, taskID)
		claimed, err := repo.ClaimQueuePromotionEntry(ctx, taskID, 0)
		require.NoError(t, err)
		require.True(t, claimed)
		assertQueuePromotionMarker(t, repo, taskID, false, "")
	})

	t.Run("legacy delivery preserves an identity-pinned marker", func(t *testing.T) {
		taskID := prefix + "-claim-pinned"
		entryID := seedQueuePromotionClaimTask(t, repo, taskID)
		require.NoError(t, repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyQueuePromotionPending,
			map[string]interface{}{
				"from_step_id": "step-source",
				models.QueuePromotionEntryTransitionIDKey: strconv.FormatInt(entryID, 10),
			}))

		claimed, err := repo.ClaimQueuePromotionEntry(ctx, taskID, 0)
		require.NoError(t, err)
		require.False(t, claimed)
		assertQueuePromotionMarker(t, repo, taskID, true, "step-source")
	})
}

func verifyCrossStepPromotionPinsEntry(t *testing.T, repo *Repository, prefix string) {
	t.Helper()
	ctx := context.Background()
	taskID := prefix + "-cross-step-promotion"
	workspaceID := "workspace-" + taskID
	workflowID := "workflow-" + taskID
	fromStepID := "step-feeder-" + taskID
	destinationStepID := "step-destination-" + taskID
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID}))
	seedCASWorkflowStep(t, repo, workflowID, fromStepID, 0)
	seedCASWorkflowStep(t, repo, workflowID, destinationStepID, 1)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: fromStepID,
		WIPAdmitted: true, Title: taskID, Metadata: map[string]interface{}{},
	}))
	candidate, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	candidate.WorkflowStepID = destinationStepID
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.Metadata[models.MetaKeyQueuePromotionPending] = map[string]interface{}{"from_step_id": fromStepID}

	claimed, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, candidate, fromStepID, destinationStepID, 0)
	require.NoError(t, err)
	require.True(t, claimed)
	entryID := candidate.WorkflowStepTransitionID
	require.Positive(t, entryID)
	assertPinnedPromotionEntry(t, repo, taskID, entryID)

	claimed, err = repo.ClaimQueuePromotionEntry(ctx, taskID, 0)
	require.NoError(t, err)
	require.False(t, claimed, "the legacy path must preserve an identified cross-step promotion")
	assertQueuePromotionMarker(t, repo, taskID, true, fromStepID)

	claimed, err = repo.ClaimQueuePromotionEntry(ctx, taskID, entryID)
	require.NoError(t, err)
	require.True(t, claimed)
	assertQueuePromotionMarker(t, repo, taskID, false, "")
}

func seedQueuePromotionClaimTask(t *testing.T, repo *Repository, taskID string) int64 {
	t.Helper()
	entryID := seedQueuePromotionClaimTaskWithoutMarker(t, repo, taskID)
	require.NoError(t, repo.SetTaskMetadataKey(context.Background(), taskID, models.MetaKeyQueuePromotionPending,
		map[string]interface{}{"from_step_id": "step-source"}))
	return entryID
}

func seedQueuePromotionClaimTaskWithoutMarker(t *testing.T, repo *Repository, taskID string) int64 {
	t.Helper()
	ctx := context.Background()
	workspaceID := "workspace-" + taskID
	workflowID := "workflow-" + taskID
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID,
		WorkflowStepID: "step-source", Title: taskID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	entryID, err := repo.GetLatestTaskStepTransitionID(ctx, taskID)
	require.NoError(t, err)
	require.Positive(t, entryID)
	return entryID
}

func assertQueuePromotionMarker(t *testing.T, repo *Repository, taskID string, wantPresent bool, wantFromStep string) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	markerValue, present := task.Metadata[models.MetaKeyQueuePromotionPending]
	require.Equal(t, wantPresent, present)
	if !wantPresent {
		return
	}
	marker, ok := markerValue.(map[string]interface{})
	require.True(t, ok, fmt.Sprintf("promotion marker has type %T", markerValue))
	require.Equal(t, wantFromStep, marker["from_step_id"])
}

func assertPinnedPromotionEntry(t *testing.T, repo *Repository, taskID string, wantEntryID int64) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	marker, ok := task.Metadata[models.MetaKeyQueuePromotionPending].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, strconv.FormatInt(wantEntryID, 10), marker[models.QueuePromotionEntryTransitionIDKey])
}
