package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-FIELD-UPDATES-001.8, AC-TASKS-FIELD-UPDATES-001.10
func TestTaskMetadataMergeSQLiteValues(t *testing.T) {
	runMetadataMergeValues(t, newRepoForEntityTests(t), false)
}

// @covers AC-TASKS-FIELD-UPDATES-001.10, AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergePostgresValues(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	runMetadataMergeValues(t, newPostgresMetadataCASRepo(t, database), true)
}

func runMetadataMergeValues(t *testing.T, repo *Repository, postgres bool) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "merge-ws", Name: "Merge"}))
	nested := `{"nested":{"new":2},"large":9007199254740993,"nullable":null}`
	pendingNested := `{"agent_title_pending":true,"agent_title_owner_session_id":"owner","nested":{"new":2},"large":9007199254740993,"nullable":null}`
	if !postgres {
		pendingNested = `{"agent_title_pending":true,"agent_title_owner_session_id":"owner","nested":{"old":1,"new":2},"large":9007199254740993}`
	}
	tests := []struct {
		name  string
		raw   interface{}
		patch map[string]interface{}
		want  string
	}{
		{"sql_null", nil, map[string]interface{}{"a": true}, `{"a":true}`},
		{"empty", "", nil, `{}`},
		{"json_null", "null", map[string]interface{}{}, `{}`},
		{"nil", `{"keep":null}`, nil, `{"keep":null}`},
		{"pending_empty", `{"agent_title_pending":true,"keep":null}`, map[string]interface{}{}, `{"agent_title_pending":true,"keep":null}`},
		{"top_level", `{"nested":{"old":1},"large":9007199254740993}`, map[string]interface{}{"nested": map[string]interface{}{"new": 2}, "nullable": nil}, nested},
		{"pending", `{"agent_title_pending":true,"agent_title_owner_session_id":"owner","nested":{"old":1},"large":9007199254740993}`, map[string]interface{}{"nested": map[string]interface{}{"new": 2}, "nullable": nil}, pendingNested},
		{"literal_keys", `{"keep":"v"}`, map[string]interface{}{"a.b": false, `q"x`: 0, "array": []interface{}{}, "object": map[string]interface{}{}}, `{"keep":"v","a.b":false,"q\"x":0,"array":[],"object":{}}`},
		{"workspace", `{"workspace":{"mode":"shared_group","group_id":"current","old":1}}`, map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent", "group_id": "forged", "new": 2}}, `{"workspace":{"mode":"shared_group","group_id":"current","new":2}}`},
		{"workspace_null", `{"workspace":{"mode":"shared_group","group_id":"current"}}`, map[string]interface{}{"workspace": nil}, `{"workspace":{"mode":"shared_group","group_id":"current"}}`},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := fmt.Sprintf("merge-value-%d", i)
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "merge-ws", Title: "Untouched"}))
			before, err := repo.GetTask(ctx, id)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), tt.raw, id)
			require.NoError(t, err)
			input, err := json.Marshal(tt.patch)
			require.NoError(t, err)
			require.NoError(t, repo.MergeTaskMetadata(ctx, id, tt.patch))
			after, err := repo.GetTask(ctx, id)
			require.NoError(t, err)
			require.Equal(t, before.Title, after.Title)
			require.True(t, after.UpdatedAt.After(before.UpdatedAt))
			afterInput, err := json.Marshal(tt.patch)
			require.NoError(t, err)
			require.Equal(t, string(input), string(afterInput))
			var raw string
			require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(`SELECT metadata FROM tasks WHERE id=?`), id).Scan(&raw))
			require.JSONEq(t, tt.want, raw)
			if _, exists := rawValueMap(t, tt.want)["large"]; exists {
				require.Equal(t, json.RawMessage("9007199254740993"), rawValueMap(t, raw)["large"])
			}
		})
	}
	t.Run("owners", func(t *testing.T) { testMetadataMergeProtectedDocument(t, repo) })
	t.Run("atomic_errors", func(t *testing.T) { testMetadataMergeAtomicErrors(t, repo, postgres) })
}

func testMetadataMergeProtectedDocument(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	id := "merge-owners"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "merge-ws", Title: "Untouched"}))
	original := `{"deferred_launch":null,"step_handoff_carry":{"key":"current"},"handoff_source":{"task_id":"source"},"handoffs":[{"task_id":"child"}],"office_carrier_causation_depth":3,"agent_title_pending":true,"agent_title_owner_session_id":"owner","ordinary":null}`
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), original, id)
	require.NoError(t, err)
	patch := map[string]interface{}{"new": "allowed"}
	for _, key := range []string{models.MetaKeyDeferredLaunch, models.MetaKeyStepHandoffCarry, models.MetaKeyHandoffSource, models.MetaKeyHandoffs, models.MetaKeyOfficeCarrierCausationDepth, models.MetaKeyAgentTitlePending, models.MetaKeyAgentTitleOwnerSessionID} {
		patch[key] = "forged"
	}
	require.NoError(t, repo.MergeTaskMetadata(ctx, id, patch))
	current, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, true, current.Metadata[models.MetaKeyAgentTitlePending])
	require.True(t, models.IsAgentTitleOwner(current.Metadata, "owner"))
	for key, value := range rawValueMap(t, original) {
		var raw string
		require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(`SELECT metadata FROM tasks WHERE id=?`), id).Scan(&raw))
		require.JSONEq(t, string(value), string(rawValueMap(t, raw)[key]))
	}
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata='{}' WHERE id=?`), id)
	require.NoError(t, err)
	require.NoError(t, repo.MergeTaskMetadata(ctx, id, patch))
	current, err = repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, map[string]interface{}{"new": "allowed"}, current.Metadata)
}

func rawValueMap(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	var values map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &values))
	return values
}

// @covers AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergeSQLiteAtomicity(t *testing.T) {
	repo := newRepoForEntityTests(t)
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "merge-ws", Name: "Merge"}))
	testMetadataMergeAtomicErrors(t, repo, false)
}

func testMetadataMergeAtomicErrors(t *testing.T, repo *Repository, postgres bool) {
	t.Helper()
	ctx := context.Background()
	id := "merge-atomic"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "merge-ws", Title: "Untouched", Metadata: map[string]interface{}{"keep": true}}))
	before, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	err = repo.MergeTaskMetadata(ctx, id, map[string]interface{}{"a": "valid", "b": func() {}})
	var encoding *json.UnsupportedTypeError
	require.ErrorAs(t, err, &encoding)
	after, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, before, after)
	reject := `CREATE TRIGGER reject_merge BEFORE UPDATE OF metadata ON tasks BEGIN SELECT RAISE(ABORT,'merge rejected'); END`
	if postgres {
		_, err := repo.DB().Exec(`CREATE FUNCTION reject_merge() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'merge rejected'; END; $$`)
		require.NoError(t, err)
		reject = `CREATE TRIGGER reject_merge BEFORE UPDATE OF metadata ON tasks FOR EACH ROW EXECUTE FUNCTION reject_merge()`
	}
	_, err = repo.DB().Exec(reject)
	require.NoError(t, err)
	require.Error(t, repo.MergeTaskMetadata(ctx, id, map[string]interface{}{"a": 1, "b": 2}))
	after, err = repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, before, after)
	drop := `DROP TRIGGER reject_merge`
	if postgres {
		drop += ` ON tasks`
	}
	_, err = repo.DB().Exec(drop)
	require.NoError(t, err)
	for _, raw := range []string{`{"bad":`, `[]`, `"text"`} {
		_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), raw, id)
		require.NoError(t, err)
		var timestamp time.Time
		require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(`SELECT updated_at FROM tasks WHERE id=?`), id).Scan(&timestamp))
		require.Error(t, repo.MergeTaskMetadata(ctx, id, map[string]interface{}{"a": 1, "b": 2}))
		var stored string
		var actual time.Time
		require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(`SELECT metadata,updated_at FROM tasks WHERE id=?`), id).Scan(&stored, &actual))
		require.Equal(t, raw, stored)
		require.Equal(t, timestamp, actual)
	}
	require.ErrorIs(t, repo.MergeTaskMetadata(ctx, "missing", map[string]interface{}{"a": 1}), ErrTaskNotFound)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, repo.MergeTaskMetadata(cancelled, id, map[string]interface{}{"a": 1}), context.Canceled)
}
