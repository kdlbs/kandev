package coordinator

import (
	"context"
	"testing"
	"time"
)

func seedMergeRows(t *testing.T, store *Store, c *Coordinator) {
	t.Helper()
	now := time.Now().UTC()
	for _, s := range []struct {
		q    string
		args []any
	}{
		{`INSERT INTO coordinator_merge_repos (coordinator_id, repository_id, policy, updated_at) VALUES (?, 'r', 'manual', ?)`, []any{c.ID, now}},
		{`INSERT INTO coordinator_merge_deps (coordinator_id, task_id, revision, refs_json, updated_at) VALUES (?, 't', 1, '[]', ?)`, []any{c.ID, now}},
		{`INSERT INTO coordinator_merge_audit (id, coordinator_id, kind, subject, revision, created_at) VALUES ('au-'||?, ?, 'k', 's', 1, ?)`, []any{c.ID, c.ID, now}},
		{`INSERT INTO coordinator_merge_proposals (proposal_id, workspace_id, coordinator_id, repository_id, change_key, head_sha, method, created_at) VALUES ('p-'||?, ?, ?, 'r', 'ck', 'sha', 'merge', ?)`, []any{c.ID, c.WorkspaceID, c.ID, now}},
		{`INSERT INTO coordinator_merge_dispatches (proposal_id, workspace_id, change_key, head_sha, claim_token, dispatched_at) VALUES ('p-'||?, ?, 'ck', 'sha', 'tok', ?)`, []any{c.ID, c.WorkspaceID, now}},
		{`INSERT INTO coordinator_merge_protection (workspace_id, repository_id, branch, pipeline, approvals, threads, observed_at) VALUES (?, 'r', 'main', 'u', 'u', 'u', ?)`, []any{c.WorkspaceID, now}},
	} {
		if _, err := store.db.ExecContext(context.Background(), store.db.Rebind(s.q), s.args...); err != nil {
			t.Fatal(err)
		}
	}
}

var mergeTables = []string{
	"coordinator_merge_repos", "coordinator_merge_deps", "coordinator_merge_audit",
	"coordinator_merge_proposals", "coordinator_merge_dispatches", "coordinator_merge_protection",
}

func TestDeleteCleansMergeRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	a := newTestCoordinator(t, store, "ws-del")
	b := newTestCoordinator(t, store, "ws-keep")
	seedMergeRows(t, store, a)
	seedMergeRows(t, store, b)
	if err := store.DeleteCoordinator(ctx, "ws-del", a.ID); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range mergeTables {
		want := 1
		if tbl == "coordinator_merge_protection" {
			want = 2 // workspace-scoped: the coordinator delete leaves it
		}
		if n := countRows(t, store, tbl); n != want {
			t.Fatalf("%s rows after coordinator delete = %d, want %d", tbl, n, want)
		}
	}
	if err := store.DeleteWorkspaceState(ctx, "ws-del"); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteWorkspaceState(ctx, "ws-keep"); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range mergeTables {
		if n := countRows(t, store, tbl); n != 0 {
			t.Fatalf("%s rows after workspace delete = %d", tbl, n)
		}
	}
}
