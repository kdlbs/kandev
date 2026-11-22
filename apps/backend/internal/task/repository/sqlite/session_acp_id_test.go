package sqlite

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSetSessionACPSessionIDSQLiteBehavior(t *testing.T) {
	testSessionACPIDBehavior(t, newRepoForSessionTests(t))
}

func TestSetSessionACPSessionIDPostgresBehavior(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	testSessionACPIDBehavior(t, repo)
}

func testSessionACPIDBehavior(t *testing.T, repo *Repository) {
	t.Helper()
	for _, tc := range []struct {
		name      string
		metadata  any
		token     string
		wantWrite bool
	}{
		{"sql_null", nil, "current", true},
		{"empty", "", "current", true},
		{"json_null", "null", "current", true},
		{"empty_object", "{}", "current", true},
		{"merge", `{"keep":{"nested":true},"acp":{"session_id":"old","title":"Keep title","options":[1,2]}}`, "current", true},
		{"acp_null", `{"acp":null}`, "current", true},
		{"acp_scalar", `{"acp":"legacy"}`, "current", true},
		{"acp_array", `{"acp":[1,2]}`, "current", true},
		{"current_id", `{"acp":{"session_id":"current"}}`, "current", false},
		{"rotated_execution", `{"acp":{"session_id":"successor"}}`, "successor", false},
		{"missing_execution", "{}", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			sessionID, taskID := "session-"+tc.name, "task-"+tc.name
			before := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, '', 'ACP test', ?, ?)`), taskID, before, before)
			require.NoError(t, err)
			_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO task_sessions (id, task_id, started_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?)`), sessionID, taskID, before, before, tc.metadata)
			require.NoError(t, err)
			if tc.token != "" {
				require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{SessionID: sessionID, TaskID: taskID, ResumeToken: tc.token, Status: "ready"}))
			}
			wrote, err := repo.SetSessionACPSessionID(ctx, sessionID, "current")
			require.NoError(t, err)
			require.Equal(t, tc.wantWrite, wrote)
			var raw sql.NullString
			var updated time.Time
			require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`SELECT metadata, updated_at FROM task_sessions WHERE id = ?`), sessionID).Scan(&raw, &updated))
			if !tc.wantWrite {
				require.Equal(t, tc.metadata, raw.String)
				require.True(t, updated.Equal(before))
				return
			}
			require.True(t, updated.After(before))
			assertSessionACPIDMetadata(t, raw.String, tc.name == "merge")
			wrote, err = repo.SetSessionACPSessionID(ctx, sessionID, "current")
			require.NoError(t, err)
			require.False(t, wrote)
			var after time.Time
			require.NoError(t, repo.db.QueryRowContext(ctx, repo.db.Rebind(`SELECT updated_at FROM task_sessions WHERE id = ?`), sessionID).Scan(&after))
			require.True(t, after.Equal(updated))
		})
	}
}

func assertSessionACPIDMetadata(t *testing.T, raw string, merged bool) {
	t.Helper()
	var metadata map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &metadata))
	acp, ok := metadata["acp"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "current", acp["session_id"])
	if merged {
		require.Equal(t, "Keep title", acp["title"])
		require.Equal(t, []any{float64(1), float64(2)}, acp["options"])
		require.Equal(t, map[string]any{"nested": true}, metadata["keep"])
	}
}
