package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

// newTestRepoPostgres mirrors newTestRepoWithHandles but backs the runs
// repository with an isolated Postgres schema instead of a temp SQLite
// file, following the boot order used elsewhere (task repo schema before
// office repo schema — see failure_postgres_test.go).
func newTestRepoPostgres(t *testing.T) *runssqlite.Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	return officeRepo.RunsRepository()
}

// TestPostgresCoalesceRun_DoesNotMergeDifferentTaskRuns is the PostgreSQL
// twin of TestCoalesceRun_DoesNotMergeDifferentTaskRuns and
// TestCoalesceRun_DoesNotMergeDifferentTaskRuns_ForOtherReasons.
// CoalesceRun's task-scoping predicate is built from
// dialect.JSONExtract, which emits payload::jsonb->>'task_id' on
// Postgres versus json_extract(payload, '$.task_id') on SQLite — two
// different SQL fragments doing the same job, so a passing SQLite
// assertion is not evidence the Postgres fragment even parses, let alone
// scopes correctly. Runs the predicate for both the original
// "task_assigned" reason and a second reactivity reason, since the fix
// (d79cec9f3) generalized the guard from one hardcoded reason to any
// task-scoped payload. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresCoalesceRun_DoesNotMergeDifferentTaskRuns(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()

	for _, reason := range []string{"task_assigned", "task_blockers_resolved"} {
		t.Run(reason, func(t *testing.T) {
			queued := mustCreateRun(t, repo, &models.Run{
				ID: "pg-task-a-" + reason, AgentProfileID: "a1", Reason: reason,
				Payload: `{"task_id":"pg-task-a"}`, Status: "queued", CoalescedCount: 1,
			})
			setRequestedAt(t, repo, queued.ID, time.Now().UTC())

			merged, err := repo.CoalesceRun(ctx, "a1", reason, 3600, `{"task_id":"pg-task-b"}`)
			if err != nil {
				t.Fatalf("coalesce: %v", err)
			}
			if merged {
				t.Fatal("coalesce = true for a different task, want false")
			}

			got := mustGetRun(t, repo, queued.ID)
			checkInt(t, "coalesced_count", got.CoalescedCount, 1)
			checkString(t, "payload", got.Payload, `{"task_id":"pg-task-a"}`)
		})
	}
}

// TestPostgresCoalesceRun_DoesNotMergeTasklessIntoTaskCarryingRun is the
// PostgreSQL twin of TestCoalesceRun_DoesNotMergeTasklessIntoTaskCarryingRun:
// the guard must also reject a taskless incoming payload against a
// task-carrying queued row, not just the reverse.
func TestPostgresCoalesceRun_DoesNotMergeTasklessIntoTaskCarryingRun(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()

	queued := mustCreateRun(t, repo, &models.Run{
		ID: "pg-task-c", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"pg-task-c"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, queued.ID, time.Now().UTC())

	merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 3600, `{}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if merged {
		t.Fatal("coalesce = true for a taskless payload into a task-carrying run, want false")
	}

	got := mustGetRun(t, repo, queued.ID)
	checkInt(t, "coalesced_count", got.CoalescedCount, 1)
	checkString(t, "payload", got.Payload, `{"task_id":"pg-task-c"}`)
}

// TestPostgresCoalesceRun_MergesSameTask is the positive-path twin: a
// same-task, same-agent, same-reason request still merges, proving the
// Postgres JSONExtract fragment is not just "always false" (which would
// also make the negative test above pass for the wrong reason).
func TestPostgresCoalesceRun_MergesSameTask(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()

	queued := mustCreateRun(t, repo, &models.Run{
		ID: "pg-merge-task", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"pg-merge-task"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, queued.ID, time.Now().UTC())

	merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 3600, `{"task_id":"pg-merge-task"}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if !merged {
		t.Fatal("coalesce = false for the same task, want true")
	}

	got := mustGetRun(t, repo, queued.ID)
	checkInt(t, "coalesced_count", got.CoalescedCount, 2)
}
