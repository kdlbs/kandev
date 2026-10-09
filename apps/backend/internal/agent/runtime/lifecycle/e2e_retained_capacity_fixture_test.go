package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// The managed browser fixture invokes this test after killing its own child.
// No production endpoint or reduced production quota is needed for seeding.
func TestE2ERetainedCapacityFixture(t *testing.T) {
	root := os.Getenv("KANDEV_E2E_CAPACITY_ROOT")
	if root == "" {
		t.Skip("used only by the isolated managed browser fixture")
	}
	require.True(t, strings.HasPrefix(filepath.Base(root), "kandev-e2e-"))
	info, err := os.Lstat(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	connection, err := db.OpenSQLite(filepath.Join(root, "kandev.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	sqlConnection := sqlx.NewDb(connection, "sqlite3")
	repo, err := tasksqlite.NewWithDB(sqlConnection, sqlConnection, nil)
	require.NoError(t, err)
	ctx := context.Background()
	session, err := repo.GetTaskSession(ctx, os.Getenv("KANDEV_E2E_CAPACITY_SESSION"))
	require.NoError(t, err)
	recovery, valid := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, valid)
	terminated, err := processidentity.OwnedSessionTerminated(recovery.OriginalRuntime)
	require.NoError(t, err)
	require.True(t, terminated, "fixture must never open a live owner's journal")
	location, err := journal.ResolveLocation(filepath.Join(root, ".kandev"), session.ID)
	require.NoError(t, err)
	retained, err := journal.Open(journal.Config{Path: location.Path, ExistingOnly: true})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, retained.Close()) })
	if os.Getenv("KANDEV_E2E_CAPACITY_ACTION") == "assert_pruned" {
		assertE2ERetainedCapacityPruned(t, repo, retained, recovery)
		return
	}
	fillE2ERetainedCapacity(t, repo, retained, recovery)
}

func fillE2ERetainedCapacity(t *testing.T, repo *tasksqlite.Repository, retained *journal.Journal, recovery models.AgentDeliveryRecovery) {
	t.Helper()
	ctx := context.Background()
	cursor, err := repo.GetAgentDeliveryCursor(ctx, recovery.StreamID)
	require.NoError(t, err)
	descriptor, err := retained.RecoveryDescriptor(ctx, recovery.SessionID, recovery.IncarnationID, uint64(recovery.HarnessGeneration), recovery.StreamID)
	require.NoError(t, err)
	require.NotNil(t, descriptor.Stream)
	require.EqualValues(t, descriptor.Stream.HighWater, cursor.ProjectedSequence, "seed starts after fully projected original output")
	payload, err := json.Marshal(map[string]string{"type": "heartbeat", "fixture_padding": strings.Repeat("x", 256<<10)})
	require.NoError(t, err)
	full := false
	for range 2048 {
		event, appendErr := retained.Append(ctx, journal.Event{
			SessionID: recovery.SessionID, IncarnationID: recovery.IncarnationID,
			HarnessGeneration: uint64(recovery.HarnessGeneration), StreamID: recovery.StreamID,
			SubmissionID: recovery.SubmissionID, Type: "heartbeat", Payload: payload,
		})
		if errors.Is(appendErr, journal.ErrStreamFull) {
			full = true
			break
		}
		require.NoError(t, appendErr)
		projectE2ECapacityEvent(t, repo, event)
	}
	require.True(t, full, "fixture must reach the unchanged shipped stream quota")
	capacity, err := retained.Capacity(ctx, recovery.StreamID)
	require.NoError(t, err)
	require.True(t, capacity.Guarded)
	require.Greater(t, capacity.StreamBytes, int64(250<<20))
	cursor, err = repo.GetAgentDeliveryCursor(ctx, recovery.StreamID)
	require.NoError(t, err)
	descriptor, err = retained.RecoveryDescriptor(ctx, recovery.SessionID, recovery.IncarnationID, uint64(recovery.HarnessGeneration), recovery.StreamID)
	require.NoError(t, err)
	require.Greater(t, cursor.ProjectedSequence, int64(descriptor.Stream.Acknowledged))
	fmt.Println("KANDEV_E2E_CAPACITY_FIXTURE:full_projected_backlog")
}

func projectE2ECapacityEvent(t *testing.T, repo *tasksqlite.Repository, event journal.Event) {
	t.Helper()
	stored := &models.AgentDeliveryEvent{
		SessionID: event.SessionID, IncarnationID: event.IncarnationID, HarnessGeneration: int64(event.HarnessGeneration),
		StreamID: event.StreamID, Sequence: int64(event.Sequence), SubmissionID: event.SubmissionID,
		EventType: event.Type, Payload: event.Payload, ReceivedAt: event.CreatedAt,
	}
	_, err := repo.ReceiveAgentDeliveryEvent(context.Background(), stored, stored.Sequence)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(context.Background(), stored, nil)
	require.NoError(t, err)
}

func assertE2ERetainedCapacityPruned(t *testing.T, repo *tasksqlite.Repository, retained *journal.Journal, recovery models.AgentDeliveryRecovery) {
	t.Helper()
	ctx := context.Background()
	capacity, err := retained.Capacity(ctx, recovery.StreamID)
	require.NoError(t, err)
	require.True(t, capacity.Recovered)
	require.Zero(t, capacity.StreamBytes)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, recovery.StreamID)
	require.NoError(t, err)
	descriptor, err := retained.RecoveryDescriptor(ctx, recovery.SessionID, recovery.IncarnationID, uint64(recovery.HarnessGeneration), recovery.StreamID)
	require.NoError(t, err)
	require.EqualValues(t, cursor.ProjectedSequence, descriptor.Stream.Acknowledged)
	fmt.Println("KANDEV_E2E_CAPACITY_FIXTURE:projected_backlog_pruned")
}
