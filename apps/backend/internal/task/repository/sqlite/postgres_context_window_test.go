package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresUpdateSessionContextWindowCompactionEligibility is the
// environment-gated PostgreSQL parity check for the eligibility guards. A skip
// is not parity evidence.
func TestPostgresUpdateSessionContextWindowCompactionEligibility(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)

	ctx := context.Background()
	now := time.Now().UTC()
	_, err = db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`), "task-compaction-eligibility-pg", "ws-compaction-eligibility-pg", "Compaction eligibility", now, now)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-compaction-eligibility-pg", TaskID: "task-compaction-eligibility-pg",
		State: models.TaskSessionStateWaitingForInput,
	}))

	const sessionID = "session-compaction-eligibility-pg"
	update := func(used int64, disabled bool) int64 {
		t.Helper()
		count, err := repo.UpdateSessionContextWindow(ctx, sessionID, contextWindowSample(used, disabled))
		require.NoError(t, err)
		return count
	}

	require.EqualValues(t, 0, update(120000, false))
	require.EqualValues(t, 0, update(80000, true))
	require.EqualValues(t, 0, update(60000, false))
	require.EqualValues(t, 1, update(50000, false))
	require.EqualValues(t, 1, update(50000, false))
	require.EqualValues(t, 1, update(10000, true))
	require.EqualValues(t, 1, update(4000, false))
	require.EqualValues(t, 2, update(3000, false))
	require.EqualValues(t, 2, update(2500, true))

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.EqualValues(t, 2, session.Metadata[models.SessionMetaKeyContextCompactionCount])
	window, ok := session.Metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
	require.True(t, ok)
	require.EqualValues(t, 2500, window["used"])
	require.Equal(t, true, window["compaction_inference_disabled"])
}
