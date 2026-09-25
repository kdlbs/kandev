package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
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
