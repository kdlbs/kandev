package gitlab

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "gitlab.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	if _, err := sqlxDB.Exec(`CREATE TABLE workspaces (
		id TEXT PRIMARY KEY,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		archived_at DATETIME
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	store, err := NewStore(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func seedWorkspace(t *testing.T, store *Store, workspaceID string) {
	t.Helper()
	if _, err := store.db.Exec(`INSERT INTO workspaces (id) VALUES (?)`, workspaceID); err != nil {
		t.Fatalf("seed workspace %s: %v", workspaceID, err)
	}
}

func seedTask(t *testing.T, store *Store, taskID, workspaceID string) {
	t.Helper()
	if _, err := store.db.Exec(
		`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`,
		taskID, workspaceID,
	); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func newTestMR(taskID, repoID, project string, iid int) *TaskMR {
	return &TaskMR{
		TaskID:       taskID,
		RepositoryID: repoID,
		Host:         "https://gitlab.com",
		ProjectPath:  project,
		MRIID:        iid,
		MRURL:        "https://gitlab.com/" + project + "/-/merge_requests/1",
		MRTitle:      "test MR",
		HeadBranch:   "feat/x",
		BaseBranch:   "main",
		State:        "open",
		CreatedAt:    time.Now().UTC(),
	}
}

func TestStore_UpsertTaskMR_InsertsThenUpdates(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tm := newTestMR("task-1", "", "acme/api", 42)
	if err := store.UpsertTaskMR(ctx, tm); err != nil {
		t.Fatalf("initial upsert: %v", err)
	}
	if tm.ID == "" {
		t.Fatal("expected ID populated after insert")
	}
	if tm.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt populated after insert")
	}
	originalID := tm.ID
	originalCreated := tm.CreatedAt

	// Second upsert with new mutable fields — id and created_at must be
	// preserved while title and state get the new values.
	tm.MRTitle = "updated title"
	tm.State = "merged"
	if err := store.UpsertTaskMR(ctx, tm); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if tm.ID != originalID {
		t.Errorf("ID changed across upsert: %q -> %q", originalID, tm.ID)
	}
	if !tm.CreatedAt.Equal(originalCreated) {
		t.Errorf("CreatedAt clobbered: %v vs %v", originalCreated, tm.CreatedAt)
	}

	got, err := store.ListTaskMRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows = %d, want 1 (upsert must not duplicate)", len(got))
	}
	if got[0].MRTitle != "updated title" || got[0].State != "merged" {
		t.Errorf("title/state not updated: %+v", got[0])
	}
}

func TestStore_UpsertTaskMR_ConcurrentCallsPreserveOriginalIdentity(t *testing.T) {
	store := newTestStore(t)
	const writers = 16
	start := make(chan struct{})
	results := make(chan *TaskMR, writers)
	errs := make(chan error, writers)
	var wg sync.WaitGroup

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			row := newTestMR("task-concurrent", "repo-1", "group/subgroup/project", 7)
			row.ID = fmt.Sprintf("candidate-%02d", index)
			row.CreatedAt = time.Date(2026, 1, 1, 0, 0, index, 0, time.UTC)
			row.MRTitle = fmt.Sprintf("writer-%02d", index)
			errs <- store.UpsertTaskMR(context.Background(), row)
			results <- row
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent upsert: %v", err)
		}
	}
	rows, err := store.ListTaskMRsByTask(context.Background(), "task-concurrent")
	if err != nil {
		t.Fatalf("list task MRs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("stored rows = %d, want exactly one", len(rows))
	}
	stored := rows[0]
	for result := range results {
		if result.ID != stored.ID {
			t.Fatalf("caller ID = %q, stored ID = %q", result.ID, stored.ID)
		}
		if !result.CreatedAt.Equal(stored.CreatedAt) {
			t.Fatalf("caller created_at = %v, stored created_at = %v", result.CreatedAt, stored.CreatedAt)
		}
	}
}

func TestStore_UpsertTaskMR_KeyedByRepoIDAndProjectAndIID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Three rows under the same task, distinguished by repository_id and IID.
	rows := []*TaskMR{
		newTestMR("task-1", "repo-a", "acme/api", 1),
		newTestMR("task-1", "repo-b", "acme/api", 1), // same project + iid, different repo
		newTestMR("task-1", "repo-a", "acme/web", 2), // same repo, different project + iid
	}
	for _, r := range rows {
		if err := store.UpsertTaskMR(ctx, r); err != nil {
			t.Fatalf("upsert %s/%d: %v", r.ProjectPath, r.MRIID, err)
		}
	}

	got, err := store.ListTaskMRsByTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("rows = %d, want 3 (composite key must allow all three)", len(got))
	}
}

func TestStore_ListTaskMRsByWorkspaceID_IsolatesWorkspaces(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	seedTask(t, store, "task-a", "ws-1")
	seedTask(t, store, "task-b", "ws-2")
	if err := store.UpsertTaskMR(ctx, newTestMR("task-a", "", "acme/api", 1)); err != nil {
		t.Fatalf("upsert ws-1 MR: %v", err)
	}
	if err := store.UpsertTaskMR(ctx, newTestMR("task-b", "", "other/proj", 2)); err != nil {
		t.Fatalf("upsert ws-2 MR: %v", err)
	}

	gotWS1, err := store.ListTaskMRsByWorkspaceID(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list ws-1: %v", err)
	}
	if len(gotWS1) != 1 || len(gotWS1["task-a"]) != 1 {
		t.Fatalf("ws-1 result = %+v, want one task with one MR", gotWS1)
	}
	if _, leaked := gotWS1["task-b"]; leaked {
		t.Error("ws-1 query leaked task-b from ws-2")
	}

	gotWS2, err := store.ListTaskMRsByWorkspaceID(ctx, "ws-2")
	if err != nil {
		t.Fatalf("list ws-2: %v", err)
	}
	if len(gotWS2) != 1 || gotWS2["task-b"][0].ProjectPath != "other/proj" {
		t.Errorf("ws-2 result = %+v, want task-b/other/proj", gotWS2)
	}

	gotEmpty, err := store.ListTaskMRsByWorkspaceID(ctx, "ws-nonexistent")
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(gotEmpty) != 0 {
		t.Errorf("empty-workspace result = %+v, want empty map", gotEmpty)
	}
}

func TestStore_ListTaskMRsByWorkspaceID_OrdersByCreatedAtAsc(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedTask(t, store, "task-1", "ws-1")

	older := newTestMR("task-1", "", "acme/api", 1)
	older.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := newTestMR("task-1", "", "acme/api", 2)
	newer.CreatedAt = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	// Insert newer first to verify ORDER BY is not relying on insertion order.
	if err := store.UpsertTaskMR(ctx, newer); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}
	if err := store.UpsertTaskMR(ctx, older); err != nil {
		t.Fatalf("upsert older: %v", err)
	}

	got, err := store.ListTaskMRsByWorkspaceID(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	mrs := got["task-1"]
	if len(mrs) != 2 {
		t.Fatalf("rows = %d, want 2", len(mrs))
	}
	if mrs[0].MRIID != 1 || mrs[1].MRIID != 2 {
		t.Errorf("order = [%d, %d], want [1, 2] (created_at ASC)", mrs[0].MRIID, mrs[1].MRIID)
	}
}

func TestStore_ListTaskMRsByTask_ReturnsEmptyForUnknownTask(t *testing.T) {
	store := newTestStore(t)
	got, err := store.ListTaskMRsByTask(context.Background(), "missing-task")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("rows = %d, want 0 for unknown task", len(got))
	}
}

func TestStore_DeleteTaskMR_RemovesByID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	tm := newTestMR("task-1", "", "acme/api", 1)
	if err := store.UpsertTaskMR(ctx, tm); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := store.DeleteTaskMR(ctx, tm.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := store.ListTaskMRsByTask(ctx, "task-1")
	if len(got) != 0 {
		t.Errorf("rows after delete = %d, want 0", len(got))
	}
}

func TestStore_DeleteTaskMR_UnknownIDIsNoOp(t *testing.T) {
	store := newTestStore(t)
	if err := store.DeleteTaskMR(context.Background(), "no-such-id"); err != nil {
		t.Errorf("delete unknown = %v, want nil (DELETE of 0 rows is not an error)", err)
	}
}

func TestStore_NewStore_RejectsNilDB(t *testing.T) {
	if _, err := NewStore(nil, nil); err == nil {
		t.Fatal("expected error when both db handles are nil")
	}
}
