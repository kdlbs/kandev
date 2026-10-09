package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// TestWorkflowParking verifies that the durable parking marker uses the
// repository's stamped metadata operations, so a delayed cleanup cannot erase
// a newer workflow parking decision.
func TestWorkflowParking(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedForMsgTest(t, repo, "workflow-parking-task", "workflow-parking-session", "workflow-parking-turn")

	first := models.WorkflowParking{
		Stamp:            "parking-first",
		ParkedAt:         time.Date(2026, 9, 16, 20, 0, 0, 0, time.UTC),
		WorkflowID:       "workflow-1",
		WorkflowStepID:   "step-1",
		RouteOperationID: "route-first",
		EntryIdentity:    "entry-first",
		SourceSessionID:  "workflow-parking-session",
		DestinationID:    "destination-first",
	}
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "workflow-parking-session", models.SessionMetaKeyWorkflowParking, first,
	))

	stored, err := repo.GetTaskSession(ctx, "workflow-parking-session")
	require.NoError(t, err)
	parking, ok := models.LoadWorkflowParking(stored.Metadata)
	require.True(t, ok)
	require.Equal(t, first, parking)

	second := first
	second.Stamp = "parking-second"
	second.RouteOperationID = "route-second"
	require.NoError(t, repo.SetSessionMetadataKey(
		ctx, "workflow-parking-session", models.SessionMetaKeyWorkflowParking, second,
	))

	removed, err := repo.RemoveSessionMetadataKeyIfStamp(
		ctx, "workflow-parking-session", models.SessionMetaKeyWorkflowParking, first.Stamp,
	)
	require.NoError(t, err)
	require.False(t, removed, "stale parking cleanup removed the successor marker")

	removed, err = repo.RemoveSessionMetadataKeyIfStamp(
		ctx, "workflow-parking-session", models.SessionMetaKeyWorkflowParking, second.Stamp,
	)
	require.NoError(t, err)
	require.True(t, removed, "matching parking cleanup did not remove the marker")

	stored, err = repo.GetTaskSession(ctx, "workflow-parking-session")
	require.NoError(t, err)
	_, ok = models.LoadWorkflowParking(stored.Metadata)
	require.False(t, ok, "matching cleanup left workflow parking metadata behind")
}
