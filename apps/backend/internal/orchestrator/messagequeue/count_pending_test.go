package messagequeue

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// repositoriesUnderTest returns one SQLite and one memory Repository so the
// pending-count contract is proven on both storage backends.
func repositoriesUnderTest(t *testing.T) map[string]Repository {
	t.Helper()
	return map[string]Repository{
		"sqlite": newTestSQLiteRepo(t),
		"memory": NewMemoryRepository(),
	}
}

func TestCountPendingByTaskIDsAccumulatesAcrossSessions(t *testing.T) {
	for name, repo := range repositoriesUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			seedLiveSessionsIfSQLite(t, repo, "s1", "s2")
			insert := func(sessionID, taskID string) {
				t.Helper()
				msg := &QueuedMessage{SessionID: sessionID, TaskID: taskID, Content: "x", QueuedBy: QueuedByUser}
				if err := repo.Insert(ctx, msg, 0); err != nil {
					t.Fatalf("insert %s/%s: %v", sessionID, taskID, err)
				}
			}
			insert("s1", "t1")
			insert("s1", "t1")
			insert("s2", "t1")
			insert("s2", "t2")

			got, err := repo.CountPendingByTaskIDs(ctx, []string{"t1", "t2"})
			if err != nil {
				t.Fatalf("CountPendingByTaskIDs: %v", err)
			}
			want := map[string]int{"t1": 3, "t2": 1}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("counts = %v, want %v", got, want)
			}
		})
	}
}

func TestCountPendingByTaskIDsExcludesReservedInFlight(t *testing.T) {
	for name, repo := range repositoriesUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			seedLiveSessionsIfSQLite(t, repo, "s1")
			insert := func(sessionID, taskID, content string, metadata map[string]interface{}) {
				t.Helper()
				msg := &QueuedMessage{SessionID: sessionID, TaskID: taskID, Content: content, QueuedBy: QueuedByWorkflow, Metadata: metadata}
				if err := repo.Insert(ctx, msg, 0); err != nil {
					t.Fatalf("insert: %v", err)
				}
			}
			// The durable lifecycle row must be the head so ReserveHead marks IT
			// in flight; the ordinary pending row stays pending below it.
			insert("s1", "t1", "lifecycle", map[string]interface{}{MetadataLifecycleDurable: true})
			insert("s1", "t1", "pending", nil)
			// Reserve the durable head: it becomes in-flight and must not count.
			if _, err := repo.ReserveHead(ctx, "s1"); err != nil {
				t.Fatalf("reserve: %v", err)
			}

			got, err := repo.CountPendingByTaskIDs(ctx, []string{"t1"})
			if err != nil {
				t.Fatalf("CountPendingByTaskIDs: %v", err)
			}
			if got["t1"] != 1 {
				t.Errorf("pending count for t1 = %d, want 1 (reserved in-flight row excluded)", got["t1"])
			}
		})
	}
}

func TestCountPendingByTaskIDsReturnsZeroForEmptyAndMissing(t *testing.T) {
	for name, repo := range repositoriesUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			got, err := repo.CountPendingByTaskIDs(ctx, []string{"missing"})
			if err != nil {
				t.Fatalf("CountPendingByTaskIDs: %v", err)
			}
			want := map[string]int{"missing": 0}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("counts = %v, want %v", got, want)
			}
		})
	}
}

func TestCountPendingByTaskIDsIsolatesTasks(t *testing.T) {
	for name, repo := range repositoriesUnderTest(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			seedLiveSessionsIfSQLite(t, repo, "s1")
			for _, taskID := range []string{"ta", "tb", "tc"} {
				msg := &QueuedMessage{SessionID: "s1", TaskID: taskID, Content: "x", QueuedBy: QueuedByUser}
				if err := repo.Insert(ctx, msg, 0); err != nil {
					t.Fatalf("insert %s: %v", taskID, err)
				}
			}
			got, err := repo.CountPendingByTaskIDs(ctx, []string{"ta", "tc"})
			if err != nil {
				t.Fatalf("CountPendingByTaskIDs: %v", err)
			}
			want := map[string]int{"ta": 1, "tc": 1}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("counts = %v, want %v", got, want)
			}
		})
	}
}

func TestCountPendingByTaskIDsIgnoresOrphanSessions(t *testing.T) {
	// Pre-fix session delete left queue rows whose session_id no longer exists.
	// The sidebar badge must not count those orphans.
	repo := newTestSQLiteRepo(t)
	ctx := context.Background()
	seedLiveSessions(t, repo, "live")
	for _, sessionID := range []string{"live", "dead"} {
		msg := &QueuedMessage{SessionID: sessionID, TaskID: "t1", Content: "x", QueuedBy: QueuedByUser}
		if err := repo.Insert(ctx, msg, 0); err != nil {
			t.Fatalf("insert %s: %v", sessionID, err)
		}
	}
	// second orphan on dead session
	if err := repo.Insert(ctx, &QueuedMessage{SessionID: "dead", TaskID: "t1", Content: "y", QueuedBy: QueuedByUser}, 0); err != nil {
		t.Fatalf("insert dead-2: %v", err)
	}

	got, err := repo.CountPendingByTaskIDs(ctx, []string{"t1"})
	if err != nil {
		t.Fatalf("CountPendingByTaskIDs: %v", err)
	}
	if got["t1"] != 1 {
		t.Fatalf("pending count = %d, want 1 (live session only; orphans ignored)", got["t1"])
	}
}

// seedLiveSessionsIfSQLite is a no-op for memory repos (they have no session
// join); SQLite count joins task_sessions and needs stubs.
func seedLiveSessionsIfSQLite(t *testing.T, repo Repository, sessionIDs ...string) {
	t.Helper()
	if _, ok := repo.(*sqliteRepository); ok {
		seedLiveSessions(t, repo, sessionIDs...)
	}
}

func TestServiceCountPendingByTask(t *testing.T) {
	svc := NewServiceMemory(logger.Default())
	ctx := context.Background()
	msg := &QueuedMessage{SessionID: "s1", TaskID: "t1", Content: "x", QueuedBy: QueuedByUser}
	if err := svc.repo.Insert(ctx, msg, 0); err != nil {
		t.Fatalf("insert: %v", err)
	}

	count, err := svc.CountPendingByTask(ctx, "t1")
	if err != nil {
		t.Fatalf("CountPendingByTask: %v", err)
	}
	if count != 1 {
		t.Errorf("CountPendingByTask(t1) = %d, want 1", count)
	}

	empty, err := svc.CountPendingByTask(ctx, "nope")
	if err != nil {
		t.Fatalf("CountPendingByTask empty: %v", err)
	}
	if empty != 0 {
		t.Errorf("CountPendingByTask(nope) = %d, want 0", empty)
	}
}
