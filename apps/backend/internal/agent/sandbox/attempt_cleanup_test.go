package sandbox

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxSessionCleanupInventorySQLite(t *testing.T) {
	testCleanupInventory(t, sqlitePolicyDB(t))
}

func TestSandboxSessionCleanupInventoryPostgres(t *testing.T) {
	testCleanupInventory(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testCleanupInventory(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, original, _ := approvalStoreFixture(t, database)
	ctx := t.Context()
	present, err := service.HasSessionAttempts(ctx, "session")
	require.NoError(t, err)
	require.True(t, present)
	present, err = service.HasSessionAttempts(ctx, "another-session")
	require.NoError(t, err)
	require.False(t, present)
	for i := 0; i < 70; i++ {
		id := fmt.Sprintf("history-%03d", i)
		snapshot := original.Snapshot
		snapshot.ExecutionID = id
		snapshot.Launch = original.Snapshot.Launch.Clone()
		snapshot.Launch.AttemptID = id
		raw, err := json.Marshal(snapshot)
		require.NoError(t, err)
		_, err = database.ExecContext(ctx, database.Rebind(`INSERT INTO sandbox_attempts (org_id,attempt_id,execution_id,session_id,generation,version,state,snapshot_json,owner_json,updated_at) VALUES (?,?,?,?,?,1,?,?,?,?)`),
			"org", id, id, "session", 1, AttemptStopped, string(raw), "null", original.UpdatedAt.Format(time.RFC3339Nano))
		require.NoError(t, err)
	}
	seen := map[string]bool{}
	cursor := CleanupCursor{}
	for {
		page, err := service.SessionCleanupAttempts(ctx, "", "session", cursor)
		require.NoError(t, err)
		require.LessOrEqual(t, len(page), 64)
		if len(page) == 0 {
			break
		}
		for _, attempt := range page {
			require.False(t, seen[attempt.ID])
			seen[attempt.ID] = true
			cursor = CleanupCursor{OrgID: attempt.Snapshot.OrgID, AttemptID: attempt.ID}
		}
	}
	require.Len(t, seen, 71)
	_, err = service.SessionCleanupAttempts(ctx, "foreign-task", "session", CleanupCursor{})
	require.ErrorIs(t, err, ErrConflict)
	missing, err := service.SessionCleanupAttempts(ctx, "", "another-session", CleanupCursor{})
	require.NoError(t, err)
	require.Empty(t, missing)
	_, err = database.ExecContext(ctx, database.Rebind(`UPDATE sandbox_attempts SET snapshot_json=? WHERE attempt_id=?`), `{}`, original.ID)
	require.NoError(t, err)
	_, err = service.SessionCleanupAttempts(ctx, "", "session", CleanupCursor{})
	require.Error(t, err, "invalid ownership evidence must not disappear from the cleanup inventory")
}
