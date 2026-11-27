package sqlite

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/internal/testutil"
)

func TestListTaskIDsWithLaunchQueueSummary(t *testing.T) {
	repo, db := newTaskStatusSummaryTestRepo(t)
	assertLaunchQueueRecoveryCandidates(t, repo, db)
}

func TestPostgresListTaskIDsWithLaunchQueueSummary(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertLaunchQueueRecoveryCandidates(t, repo, db)
}

// Both dialects use the real schema and validated summary persistence. A
// cleared authoritative record must remain discoverable through its stale
// summary, while a successfully cleared summary must leave the recovery set.
func assertLaunchQueueRecoveryCandidates(t *testing.T, repo *Repository, db *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, id := range []string{"queue-b", "queue-a", "nil-queue", "no-summary", "deleted-queue"} {
		seedTaskForStatusSummary(t, db, id, "workspace-recovery")
		if id == "no-summary" {
			continue
		}
		summary := statussummary.TaskStatusSummary{Revision: 1}
		if id != "nil-queue" {
			summary.LaunchQueue = &statussummary.LaunchQueueSummary{
				QueuedAt: now, Reason: statussummary.LaunchQueueReasonSessionCapacity, Retrying: true,
			}
		}
		if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
			TaskID: id, WorkspaceID: "workspace-recovery", Summary: summary,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(db.Rebind("DELETE FROM tasks WHERE id = ?"), "deleted-queue"); err != nil {
		t.Fatal(err)
	}
	ids, err := repo.ListTaskIDsWithLaunchQueueSummary(ctx)
	if err != nil || !reflect.DeepEqual(ids, []string{"queue-a", "queue-b"}) {
		t.Fatalf("recovery candidates = %v, %v", ids, err)
	}
	if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "queue-a", WorkspaceID: "workspace-recovery",
		Summary: statussummary.TaskStatusSummary{Revision: 2},
	}); err != nil {
		t.Fatal(err)
	}
	ids, err = repo.ListTaskIDsWithLaunchQueueSummary(ctx)
	if err != nil || !reflect.DeepEqual(ids, []string{"queue-b"}) {
		t.Fatalf("candidates after projection clear = %v, %v", ids, err)
	}
}
