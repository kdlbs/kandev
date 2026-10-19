package coordinator

import (
	"context"
	"testing"
	"time"
)

// Rows with no coordinator parent (seeded before phase 2 existed) are still
// removed by the workspace delete, and other workspaces' rows are untouched.
func TestDeleteWorkspaceState_RemovesOrphanPhase2Rows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seed := func(ws, id string) {
		t.Helper()
		stmts := []struct {
			q    string
			args []any
		}{
			{`INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf', ?, ?)`, []any{id, ws, now}},
			{`INSERT INTO coordinator_watch_projects (coordinator_id, entry_kind, entry_id, workspace_id, created_at) VALUES (?, 'repository', 'repo', ?, ?)`, []any{id, ws, now}},
			{`INSERT INTO coordinator_standing_orders (id, coordinator_id, workspace_id, text, created_by, created_at) VALUES (?, ?, ?, 't', 'u', ?)`, []any{"so-" + id, id, ws, now}},
			{`INSERT INTO coordinator_goals (id, coordinator_id, workspace_id, name, status, baseline_json, set_at, created_at, updated_at) VALUES (?, ?, ?, 'n', 'active', '{}', ?, ?, ?)`, []any{"g-" + id, id, ws, now, now, now}},
		}
		for _, s := range stmts {
			if _, err := store.db.ExecContext(ctx, store.db.Rebind(s.q), s.args...); err != nil {
				t.Fatal(err)
			}
		}
		row := validRow(id)
		row.WorkspaceID = ws
		if _, err := store.db.ExecContext(ctx, store.db.Rebind(
			`INSERT INTO coordinator_activity (id, coordinator_id, workspace_id, action_class, outcome, "authorization", refusal_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`),
			"a-"+id, id, ws, string(row.ActionClass), string(row.Outcome), string(row.Authorization), now, now); err != nil {
			t.Fatal(err)
		}
	}
	seed("ws-orphan", "gone-1")
	seed("ws-other", "gone-2")

	if err := store.DeleteWorkspaceState(ctx, "ws-orphan"); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range phase2Tables {
		if n := countRows(t, store, tbl); n != 1 {
			t.Fatalf("%s rows = %d, want only the other workspace's row", tbl, n)
		}
		var ws string
		if err := store.db.Get(&ws, `SELECT workspace_id FROM `+tbl); err != nil || ws != "ws-other" {
			t.Fatalf("%s survivor workspace = %q err=%v", tbl, ws, err)
		}
	}
}
