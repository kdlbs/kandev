package jira

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresIssueWatchDedup(t *testing.T) {
	// This covers mapping states and a simultaneous burst. The waiting-claimant
	// schedule is covered by TestPostgresIssueWatchDedup_WaitingClaimantPreservesReplacement.
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(4)
	database.SetMaxIdleConns(4)
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := database.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		archived_at TIMESTAMP,
		workflow_step_id TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks fixture: %v", err)
	}

	first := createDedupWatch(t, store, "ws-1")
	second := createDedupWatch(t, store, "ws-2")
	seedDedupTask(t, store, "task-live", false, false)
	seedDedupMapping(t, store, first.ID, "PROJ-LIVE", "task-live", nil)
	seedDedupMapping(t, store, second.ID, "PROJ-LIVE", "task-live", nil)
	seedDedupMapping(t, store, first.ID, "PROJ-PENDING", "", nil)
	seedDedupMapping(t, store, first.ID, "PROJ-MISSING", "task-missing", nil)

	firstSeen := listSeenIssueKeys(t, store, first.ID)
	for _, key := range []string{"PROJ-LIVE", "PROJ-PENDING"} {
		if _, ok := firstSeen[key]; !ok {
			t.Fatalf("%s must remain protected", key)
		}
	}
	if _, ok := firstSeen["PROJ-MISSING"]; ok {
		t.Fatal("missing task must not remain protected")
	}
	if _, ok := listSeenIssueKeys(t, store, second.ID)["PROJ-LIVE"]; !ok {
		t.Fatal("another watch's live task must remain protected")
	}
	for _, key := range []string{"PROJ-LIVE", "PROJ-PENDING"} {
		reserved, err := store.ReserveIssueWatchTask(ctx, first.ID, key, "https://example.test/browse/"+key)
		if err != nil || reserved {
			t.Fatalf("%s protected reservation: reserved=%v err=%v", key, reserved, err)
		}
	}

	seedDedupMapping(t, store, first.ID, "PROJ-RACE", "task-race", func() {
		seedDedupTask(t, store, "task-race", false, false)
	})
	if _, err := database.ExecContext(ctx, `DELETE FROM tasks WHERE id = $1`, "task-race"); err != nil {
		t.Fatalf("delete raced task: %v", err)
	}
	start := make(chan struct{})
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reserved, err := store.ReserveIssueWatchTask(ctx, first.ID, "PROJ-RACE", "https://example.test/browse/PROJ-RACE")
			results <- reserved
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	winners := 0
	for reserved := range results {
		if reserved {
			winners++
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent reserve: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent reclaim winners = %d, want 1", winners)
	}
	row := readIssueWatchTaskRecord(t, store, first.ID, "PROJ-RACE")
	if row.TaskID != "" {
		t.Fatalf("winner reservation task_id = %q, want empty", row.TaskID)
	}

	seedDedupTask(t, store, "task-replacement", false, false)
	if err := store.AssignIssueWatchTaskID(ctx, first.ID, "PROJ-RACE", "task-replacement"); err != nil {
		t.Fatalf("attach replacement: %v", err)
	}
	reserved, err := store.ReserveIssueWatchTask(ctx, first.ID, "PROJ-RACE", "https://example.test/browse/PROJ-RACE")
	if err != nil {
		t.Fatalf("post-attach reserve: %v", err)
	}
	if reserved {
		t.Fatal("an existing replacement task must remain protected")
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO tasks (id) VALUES ($1)`, "task-archived"); err != nil {
		t.Fatalf("seed archived task: %v", err)
	}
	seedDedupMapping(t, store, first.ID, "PROJ-ARCHIVED", "task-archived", nil)
	if _, err := database.ExecContext(ctx, `UPDATE tasks SET archived_at = $1 WHERE id = $2`, time.Now().UTC(), "task-archived"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if _, ok := listSeenIssueKeys(t, store, first.ID)["PROJ-ARCHIVED"]; !ok {
		t.Fatal("archived task must remain protected")
	}
}
