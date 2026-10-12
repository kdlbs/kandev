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

// TestPostgresCoalesceRun_MergesTasklessIntoTasklessRun is the Postgres
// twin of TestCoalesceRun_MergesTasklessIntoTasklessRun: the ->>'task_id'
// IS NULL fragment must still merge a taskless incoming payload into a
// taskless queued row, not just reject a task-carrying one. Untested,
// this fragment could be silently rewritten to always-false (e.g. an
// extra AND clause) and every other Postgres coalesce test would still
// pass, since none of them assert a taskless merge succeeds.
func TestPostgresCoalesceRun_MergesTasklessIntoTasklessRun(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"empty object", `{}`},
		{"other field set", `{"routine_id":"r1"}`},
		{"explicit null task_id", `{"task_id":null}`},
		{"present empty string task_id", `{"task_id":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestRepoPostgres(t)
			ctx := context.Background()

			queued := mustCreateRun(t, repo, &models.Run{
				ID: "pg-taskless", AgentProfileID: "a1", Reason: "heartbeat",
				Payload: tc.payload, Status: "queued", CoalescedCount: 1,
			})
			setRequestedAt(t, repo, queued.ID, time.Now().UTC())

			merged, err := repo.CoalesceRun(ctx, "a1", "heartbeat", 3600, `{"merged":true}`)
			if err != nil {
				t.Fatalf("coalesce: %v", err)
			}
			if !merged {
				t.Fatalf("coalesce = false for taskless into taskless, want true")
			}

			got := mustGetRun(t, repo, queued.ID)
			checkInt(t, "coalesced_count", got.CoalescedCount, 2)
			checkString(t, "payload", got.Payload, `{"merged":true}`)
		})
	}
}

// TestPostgresCoalesceRun_DoesNotMergeNonStringTaskIDIntoTasklessRun is the
// Postgres twin of TestCoalesceRun_DoesNotMergeNonStringTaskIDIntoTasklessRun:
// a payload with a present, non-string task_id must not be classified
// taskless and must not absorb a genuinely taskless queued run's payload.
func TestPostgresCoalesceRun_DoesNotMergeNonStringTaskIDIntoTasklessRun(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()

	queued := mustCreateRun(t, repo, &models.Run{
		ID: "pg-taskless-real", AgentProfileID: "a1", Reason: "custom_reason",
		Payload: `{"note":"real taskless launch"}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, queued.ID, time.Now().UTC())

	merged, err := repo.CoalesceRun(ctx, "a1", "custom_reason", 3600, `{"task_id":42}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if merged {
		t.Fatal("coalesce = true for a non-string task_id into a taskless run, want false")
	}

	got := mustGetRun(t, repo, queued.ID)
	checkInt(t, "coalesced_count", got.CoalescedCount, 1)
	checkString(t, "payload", got.Payload, `{"note":"real taskless launch"}`)
}

// TestPostgresCoalesceRun_DoesNotMergeIntoNonStringQueuedTaskID is the
// Postgres twin of TestCoalesceRun_DoesNotMergeIntoNonStringQueuedTaskID.
// Postgres's ->> operator converts a stored JSON number to text before the
// comparison, so a naive text-only predicate would let {"task_id":"42"}
// match a queued {"task_id":42} and silently overwrite it; this exercises
// exactly that dialect-specific coercion.
func TestPostgresCoalesceRun_DoesNotMergeIntoNonStringQueuedTaskID(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()

	queued := mustCreateRun(t, repo, &models.Run{
		ID: "pg-malformed", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":42}`, Status: "queued", CoalescedCount: 1,
	})
	setRequestedAt(t, repo, queued.ID, time.Now().UTC())

	merged, err := repo.CoalesceRun(ctx, "a1", "task_assigned", 3600, `{"task_id":"42"}`)
	if err != nil {
		t.Fatalf("coalesce: %v", err)
	}
	if merged {
		t.Fatal("coalesce = true into a queued row with a non-string task_id, want false")
	}

	got := mustGetRun(t, repo, queued.ID)
	checkInt(t, "coalesced_count", got.CoalescedCount, 1)
	checkString(t, "payload", got.Payload, `{"task_id":42}`)
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
