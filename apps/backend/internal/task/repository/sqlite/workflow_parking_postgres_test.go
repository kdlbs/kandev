package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepository "github.com/kandev/kandev/internal/workflow/repository"
)

// TestPostgresWorkflowParking runs the same stamped metadata contract against
// PostgreSQL's JSONB implementation. It is skipped by testutil when the
// isolated PostgreSQL DSN is not configured.
func TestPostgresWorkflowParking(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	seedPostgresTaskSession(t, repo, "workflow-parking-task-pg", "workflow-parking-session-pg")

	first := models.WorkflowParking{
		Stamp:            "parking-first-pg",
		ParkedAt:         time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
		WorkflowID:       "workflow-1",
		WorkflowStepID:   "step-1",
		RouteOperationID: "route-first",
		EntryIdentity:    "entry-first",
		SourceSessionID:  "workflow-parking-session-pg",
		DestinationID:    "destination-first",
	}
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "workflow-parking-session-pg", models.SessionMetaKeyWorkflowParking, first,
	))

	second := first
	second.Stamp = "parking-second-pg"
	second.RouteOperationID = "route-second"
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "workflow-parking-session-pg", models.SessionMetaKeyWorkflowParking, second,
	))

	removed, err := repo.RemoveSessionMetadataKeyIfStamp(
		ctx, "workflow-parking-session-pg", models.SessionMetaKeyWorkflowParking, first.Stamp,
	)
	require.NoError(t, err)
	require.False(t, removed, "stale parking cleanup removed the successor marker")

	removed, err = repo.RemoveSessionMetadataKeyIfStamp(
		ctx, "workflow-parking-session-pg", models.SessionMetaKeyWorkflowParking, second.Stamp,
	)
	require.NoError(t, err)
	require.True(t, removed, "matching parking cleanup did not remove the marker")
}

func TestPostgresWorkflowStepCreateSerializesWithDelete(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 6)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	taskRepo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	workflowRepo, err := workflowrepository.NewWithDB(db, db, nil)
	require.NoError(t, err)

	const (
		workspaceID = "ws-pg-workflow-step-delete-race"
		workflowID  = "wf-pg-workflow-step-delete-race"
	)
	seedWorkspace(t, taskRepo, workspaceID)
	require.NoError(t, taskRepo.CreateWorkflow(ctx, &models.Workflow{
		ID: workflowID, WorkspaceID: workspaceID, Name: "Delete race",
	}))
	require.NoError(t, workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "step-pg-workflow-delete-race", WorkflowID: workflowID, Name: "Existing", Position: 0,
	}))

	blocker, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	var lockedID string
	require.NoError(t, blocker.QueryRowxContext(ctx, db.Rebind(`
		SELECT id FROM workflows WHERE id = ? FOR UPDATE
	`), workflowID).Scan(&lockedID))

	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- taskRepo.DeleteWorkflow(ctx, workflowID)
	}()
	waitForWorkflowStepLockWaiters(t, ctx, db, 1)

	createDone := make(chan error, 1)
	go func() {
		createDone <- workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
			ID: "step-pg-workflow-delete-race-new", WorkflowID: workflowID, Name: "New", Position: 1,
		})
	}()
	waitForWorkflowStepLockWaiters(t, ctx, db, 2)

	require.NoError(t, blocker.Rollback())
	require.NoError(t, <-deleteDone)
	require.ErrorIs(t, <-createDone, repoerrors.ErrWorkflowNotFound)

	var remaining int
	require.NoError(t, db.GetContext(ctx, &remaining, db.Rebind(`
		SELECT COUNT(*) FROM workflow_steps WHERE workflow_id = ?
	`), workflowID))
	require.Equal(t, 0, remaining)
}

func waitForWorkflowStepLockWaiters(t *testing.T, ctx context.Context, db *sqlx.DB, want int) {
	t.Helper()
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiters int
		if err := db.GetContext(waitCtx, &waiters, `
			SELECT COUNT(*) FROM pg_stat_activity
			WHERE datname = current_database()
			  AND pid <> pg_backend_pid()
			  AND wait_event_type = 'Lock'
			  AND query LIKE '%FROM workflows%'
		`); err != nil {
			t.Fatalf("inspect PostgreSQL workflow row lock waiters: %v", err)
		}
		if waiters >= want {
			return
		}
		select {
		case <-waitCtx.Done():
			t.Fatalf("timed out waiting for %d workflow row lock waiters", want)
		case <-ticker.C:
		}
	}
}
