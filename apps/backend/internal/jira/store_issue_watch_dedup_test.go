package jira

import (
	"context"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
)

func createDedupWatch(t *testing.T, store *Store, workspaceID string) *IssueWatch {
	t.Helper()
	w := newTestIssueWatch(workspaceID)
	if err := store.CreateIssueWatch(context.Background(), w); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	return w
}

func seedDedupTask(t *testing.T, store *Store, taskID string, archived, completed bool) {
	t.Helper()
	var archivedAt any
	if archived {
		archivedAt = time.Now().UTC()
	}
	stepID := ""
	if completed {
		stepID = "step-completed"
	}
	if _, err := store.db.ExecContext(context.Background(), store.db.Rebind(
		`INSERT INTO tasks (id, archived_at, workflow_step_id) VALUES (?, ?, ?)`),
		taskID, archivedAt, stepID); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
}

func seedDedupMapping(t *testing.T, store *Store, watchID, issueKey string, taskID string, seedTask func()) {
	t.Helper()
	ctx := context.Background()
	if seedTask != nil {
		seedTask()
	}
	reserved, err := store.ReserveIssueWatchTask(ctx, watchID, issueKey, "https://example.test/browse/"+issueKey)
	if err != nil {
		t.Fatalf("reserve %s: %v", issueKey, err)
	}
	if !reserved {
		t.Fatalf("reserve %s: expected a new reservation", issueKey)
	}
	if taskID == "" {
		return
	}
	if err := store.AssignIssueWatchTaskID(ctx, watchID, issueKey, taskID); err != nil {
		t.Fatalf("assign %s: %v", issueKey, err)
	}
}

func listSeenIssueKeys(t *testing.T, store *Store, watchID string) map[string]struct{} {
	t.Helper()
	seen, err := store.ListSeenIssueKeys(context.Background(), watchID)
	if err != nil {
		t.Fatalf("list seen: %v", err)
	}
	return seen
}

type issueWatchTaskRecord struct {
	ID           string    `db:"id"`
	IssueWatchID string    `db:"issue_watch_id"`
	IssueKey     string    `db:"issue_key"`
	IssueURL     string    `db:"issue_url"`
	TaskID       string    `db:"task_id"`
	CreatedAt    time.Time `db:"created_at"`
}

func readIssueWatchTaskRecord(t *testing.T, store *Store, watchID, issueKey string) issueWatchTaskRecord {
	t.Helper()
	var row issueWatchTaskRecord
	if err := store.db.GetContext(context.Background(), &row, store.db.Rebind(`
		SELECT id, issue_watch_id, issue_key, issue_url, task_id, created_at
		FROM jira_issue_watch_tasks WHERE issue_watch_id = ? AND issue_key = ?`),
		watchID, issueKey); err != nil {
		t.Fatalf("read mapping: %v", err)
	}
	return row
}

func TestStore_IssueWatchDedup_MappingStates(t *testing.T) {
	cases := []struct {
		name        string
		seed        func(t *testing.T, store *Store, watchID string)
		wantSeen    bool
		wantReserve bool
	}{
		{name: "no mapping", wantReserve: true},
		{
			name: "unfinished reservation",
			seed: func(t *testing.T, store *Store, watchID string) {
				seedDedupMapping(t, store, watchID, "PROJ-1", "", nil)
			},
			wantSeen: true,
		},
		{
			name: "active task",
			seed: func(t *testing.T, store *Store, watchID string) {
				seedDedupMapping(t, store, watchID, "PROJ-1", "task-active", func() {
					seedDedupTask(t, store, "task-active", false, false)
				})
			},
			wantSeen: true,
		},
		{
			name: "archived task",
			seed: func(t *testing.T, store *Store, watchID string) {
				seedDedupMapping(t, store, watchID, "PROJ-1", "task-archived", func() {
					seedDedupTask(t, store, "task-archived", true, false)
				})
			},
			wantSeen: true,
		},
		{
			name: "completed task",
			seed: func(t *testing.T, store *Store, watchID string) {
				seedDedupMapping(t, store, watchID, "PROJ-1", "task-completed", func() {
					seedDedupTask(t, store, "task-completed", false, true)
				})
			},
			wantSeen: true,
		},
		{
			name: "missing task",
			seed: func(t *testing.T, store *Store, watchID string) {
				seedDedupMapping(t, store, watchID, "PROJ-1", "task-missing", nil)
			},
			wantReserve: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			w := createDedupWatch(t, store, "ws-1")
			if tc.seed != nil {
				tc.seed(t, store, w.ID)
			}
			_, seen := listSeenIssueKeys(t, store, w.ID)["PROJ-1"]
			if seen != tc.wantSeen {
				t.Fatalf("PROJ-1 seen = %v, want %v", seen, tc.wantSeen)
			}
			reserved, err := store.ReserveIssueWatchTask(
				context.Background(), w.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
			if err != nil {
				t.Fatalf("reserve: %v", err)
			}
			if reserved != tc.wantReserve {
				t.Fatalf("reserve = %v, want %v", reserved, tc.wantReserve)
			}
			if reserved {
				if _, ok := listSeenIssueKeys(t, store, w.ID)["PROJ-1"]; !ok {
					t.Fatal("recovered issue must remain seen through its new reservation")
				}
			}
		})
	}
}

func TestStore_IssueWatchDedup_ConcurrentReclaim(t *testing.T) {
	// This covers a simultaneous burst. The waiting-claimant schedule is
	// covered by TestStore_IssueWatchDedup_WaitingClaimantPreservesReplacement.
	path := filepath.Join(t.TempDir(), "jira-dedup.db")
	openStore := func() *Store {
		raw, err := dbutil.OpenSQLite(path)
		if err != nil {
			t.Fatalf("open writer: %v", err)
		}
		db := sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = db.Close() })
		store, err := NewStore(db, db)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			archived_at DATETIME,
			workflow_step_id TEXT NOT NULL DEFAULT ''
		)`); err != nil {
			t.Fatalf("create tasks fixture: %v", err)
		}
		return store
	}
	first, second := openStore(), openStore()
	ctx := context.Background()
	w := createDedupWatch(t, first, "ws-1")
	seedDedupMapping(t, first, w.ID, "PROJ-1", "task-missing", nil)

	start := make(chan struct{})
	results := make(chan struct {
		store string
		ok    bool
		err   error
	}, 2)
	var wg sync.WaitGroup
	for name, store := range map[string]*Store{"first": first, "second": second} {
		wg.Add(1)
		go func(name string, store *Store) {
			defer wg.Done()
			<-start
			ok, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
			results <- struct {
				store string
				ok    bool
				err   error
			}{store: name, ok: ok, err: err}
		}(name, store)
	}
	close(start)
	wg.Wait()
	close(results)

	winners, losers := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("%s reserve: %v", result.store, result.err)
		}
		if result.ok {
			winners++
		} else {
			losers++
		}
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("concurrent reclaim winners=%d losers=%d, want 1 and 1", winners, losers)
	}
	row := readIssueWatchTaskRecord(t, first, w.ID, "PROJ-1")
	if row.TaskID != "" {
		t.Fatalf("winner reservation task_id = %q, want empty", row.TaskID)
	}

	seedDedupTask(t, first, "task-replacement", false, false)
	if err := first.AssignIssueWatchTaskID(ctx, w.ID, "PROJ-1", "task-replacement"); err != nil {
		t.Fatalf("attach replacement task: %v", err)
	}
	reserved, err := second.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
	if err != nil {
		t.Fatalf("post-attach reserve: %v", err)
	}
	if reserved {
		t.Fatal("a waiting claimant must not replace an attached task")
	}
}

func TestStore_IssueWatchDedup_WatchIsolation(t *testing.T) {
	store := newTestStore(t)
	first := createDedupWatch(t, store, "ws-1")
	second := createDedupWatch(t, store, "ws-2")

	seedDedupMapping(t, store, first.ID, "PROJ-1", "task-missing", nil)
	seedDedupMapping(t, store, second.ID, "PROJ-1", "task-live", func() {
		seedDedupTask(t, store, "task-live", false, false)
	})
	seedDedupMapping(t, store, first.ID, "PROJ-2", "task-archived", func() {
		seedDedupTask(t, store, "task-archived", true, false)
	})
	seedDedupMapping(t, store, first.ID, "PROJ-3", "", nil)

	firstSeen := listSeenIssueKeys(t, store, first.ID)
	if _, ok := firstSeen["PROJ-1"]; ok {
		t.Fatal("missing task must not suppress its own watch")
	}
	for _, key := range []string{"PROJ-2", "PROJ-3"} {
		if _, ok := firstSeen[key]; !ok {
			t.Fatalf("%s must remain protected", key)
		}
	}
	if _, ok := listSeenIssueKeys(t, store, second.ID)["PROJ-1"]; !ok {
		t.Fatal("another watch's existing task must remain protected")
	}

	if reserved, err := store.ReserveIssueWatchTask(context.Background(), first.ID, "PROJ-1", "https://example.test/browse/PROJ-1"); err != nil || !reserved {
		t.Fatalf("first watch reclaim: reserved=%v err=%v", reserved, err)
	}
	secondRow := readIssueWatchTaskRecord(t, store, second.ID, "PROJ-1")
	if secondRow.TaskID != "task-live" {
		t.Fatalf("another watch's mapping changed to task_id=%q", secondRow.TaskID)
	}
	if reserved, err := store.ReserveIssueWatchTask(context.Background(), second.ID, "PROJ-1", "https://example.test/browse/PROJ-1"); err != nil || reserved {
		t.Fatalf("second watch existing task reservation: reserved=%v err=%v", reserved, err)
	}
	for _, key := range []string{"PROJ-2", "PROJ-3"} {
		if reserved, err := store.ReserveIssueWatchTask(context.Background(), first.ID, key, "https://example.test/browse/"+key); err != nil || reserved {
			t.Fatalf("%s protected reservation: reserved=%v err=%v", key, reserved, err)
		}
	}
}

func TestStore_IssueWatchDedup_SharedMissingTask(t *testing.T) {
	store := newTestStore(t)
	watches := []*IssueWatch{
		createDedupWatch(t, store, "ws-1"),
		createDedupWatch(t, store, "ws-2"),
		createDedupWatch(t, store, "ws-3"),
	}
	for _, w := range watches {
		seedDedupMapping(t, store, w.ID, "PROJ-1", "task-missing", nil)
	}
	for _, w := range watches {
		if _, ok := listSeenIssueKeys(t, store, w.ID)["PROJ-1"]; ok {
			t.Fatalf("watch %s must not suppress an issue whose shared task is missing", w.ID)
		}
	}
	ids := make(map[string]struct{}, len(watches))
	for _, w := range watches {
		reserved, err := store.ReserveIssueWatchTask(context.Background(), w.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
		if err != nil || !reserved {
			t.Fatalf("watch %s reclaim: reserved=%v err=%v", w.ID, reserved, err)
		}
		row := readIssueWatchTaskRecord(t, store, w.ID, "PROJ-1")
		if row.TaskID != "" {
			t.Fatalf("watch %s reservation task_id = %q, want empty", w.ID, row.TaskID)
		}
		if _, duplicate := ids[row.ID]; duplicate {
			t.Fatalf("watch %s reused reservation ID %s", w.ID, row.ID)
		}
		ids[row.ID] = struct{}{}
	}
}

func TestStore_IssueWatchDedup_ReclaimRollback(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	w := createDedupWatch(t, store, "ws-1")
	seedDedupMapping(t, store, w.ID, "PROJ-1", "task-missing", nil)
	before := readIssueWatchTaskRecord(t, store, w.ID, "PROJ-1")

	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER fail_jira_issue_watch_task_insert
		BEFORE INSERT ON jira_issue_watch_tasks
		BEGIN
			SELECT RAISE(ABORT, 'injected insert failure');
		END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.test/browse/PROJ-1"); err == nil {
		t.Fatal("expected inserted reservation failure")
	}
	after := readIssueWatchTaskRecord(t, store, w.ID, "PROJ-1")
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rollback changed mapping:\nbefore=%+v\nafter=%+v", before, after)
	}

	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER fail_jira_issue_watch_task_insert`); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	reserved, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
	if err != nil || !reserved {
		t.Fatalf("retry after rollback: reserved=%v err=%v", reserved, err)
	}
	retried := readIssueWatchTaskRecord(t, store, w.ID, "PROJ-1")
	if retried.ID == before.ID || retried.TaskID != "" {
		t.Fatalf("retried mapping = %+v, want a fresh empty reservation", retried)
	}
}

func TestStore_IssueWatchDedup_TaskLookupFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	w := createDedupWatch(t, store, "ws-1")
	seedDedupMapping(t, store, w.ID, "PROJ-1", "task-missing", nil)
	before := readIssueWatchTaskRecord(t, store, w.ID, "PROJ-1")

	if _, err := store.db.ExecContext(ctx, `DROP TABLE tasks`); err != nil {
		t.Fatalf("drop tasks fixture: %v", err)
	}
	if _, err := store.ListSeenIssueKeys(ctx, w.ID); err == nil {
		t.Fatal("expected seen lookup failure without tasks")
	}
	if _, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.test/browse/PROJ-1"); err == nil {
		t.Fatal("expected reservation failure without tasks")
	}
	if after := readIssueWatchTaskRecord(t, store, w.ID, "PROJ-1"); !reflect.DeepEqual(before, after) {
		t.Fatalf("failed claim changed mapping:\nbefore=%+v\nafter=%+v", before, after)
	}

	if _, err := store.db.ExecContext(ctx, `CREATE TABLE tasks (
		id TEXT PRIMARY KEY,
		archived_at DATETIME,
		workflow_step_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("recreate tasks fixture: %v", err)
	}
	reserved, err := store.ReserveIssueWatchTask(ctx, w.ID, "PROJ-1", "https://example.test/browse/PROJ-1")
	if err != nil || !reserved {
		t.Fatalf("retry after recovery: reserved=%v err=%v", reserved, err)
	}
}
