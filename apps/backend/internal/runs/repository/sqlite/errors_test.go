package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestReadMethodsPropagateDatabaseErrors closes the reader pool and
// asserts every read surfaces the failure instead of degrading to an
// empty result — a swallowed error here would render an empty run list
// as "this agent has no runs".
func TestReadMethodsPropagateDatabaseErrors(t *testing.T) {
	repo, _, reader := newTestRepoWithHandles(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-1", AgentProfileID: "a1", Reason: "task_assigned",
		Payload: `{"task_id":"t1","comment_id":"cm-1"}`,
	})
	if err := reader.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	checks := map[string]func() error{
		"GetRunByID": func() error {
			_, err := repo.GetRunByID(ctx, run.ID)
			return err
		},
		"ListRuns": func() error {
			_, err := repo.ListRuns(ctx, "ws-1")
			return err
		},
		"ListRunEvents": func() error {
			_, err := repo.ListRunEvents(ctx, run.ID, -1, 0)
			return err
		},
		"CheckIdempotencyKey": func() error {
			_, err := repo.CheckIdempotencyKey(ctx, "key", 1)
			return err
		},
		"ListPendingRunsForTask": func() error {
			_, err := repo.ListPendingRunsForTask(ctx, "t1")
			return err
		},
		"ListRunsForAgentPaged(first page)": func() error {
			_, err := repo.ListRunsForAgentPaged(ctx, "a1", time.Time{}, "", 10)
			return err
		},
		"ListRunsForAgentPaged(cursor)": func() error {
			_, err := repo.ListRunsForAgentPaged(ctx, "a1", time.Now().UTC(), "cursor", 10)
			return err
		},
		"FindInflightRunForAgent": func() error {
			_, err := repo.FindInflightRunForAgent(ctx, "a1")
			return err
		},
		"GetClaimedRunByTaskID": func() error {
			_, err := repo.GetClaimedRunByTaskID(ctx, "t1")
			return err
		},
		"GetClaimedTasklessRunForAgent": func() error {
			_, err := repo.GetClaimedTasklessRunForAgent(ctx, "a1")
			return err
		},
		"GetRunsByCommentIDs": func() error {
			_, err := repo.GetRunsByCommentIDs(ctx, []string{"cm-1"})
			return err
		},
		"GetRunWithCosts": func() error {
			_, _, err := repo.GetRunWithCosts(ctx, run.ID)
			return err
		},
	}
	for name, call := range checks {
		err := call()
		if err == nil {
			t.Errorf("%s returned nil error against a closed reader", name)
			continue
		}
		if errors.Is(err, sql.ErrNoRows) {
			t.Errorf("%s reported sql.ErrNoRows for a closed reader, want the underlying failure", name)
		}
	}
}

// TestWriteMethodsPropagateDatabaseErrors is the same contract for the
// writer pool.
func TestWriteMethodsPropagateDatabaseErrors(t *testing.T) {
	repo, writer, _ := newTestRepoWithHandles(t)
	ctx := context.Background()
	run := mustCreateRun(t, repo, &models.Run{
		ID: "run-1", AgentProfileID: "a1", Reason: "task_assigned",
	})
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	checks := map[string]func() error{
		"CreateRun": func() error {
			return repo.CreateRun(ctx, &models.Run{AgentProfileID: "a1", Reason: "r"})
		},
		"UpdateRunRuntimeSnapshot": func() error {
			return repo.UpdateRunRuntimeSnapshot(ctx, run.ID, "{}", "{}", "s")
		},
		"UpdateRunPromptArtifacts": func() error {
			return repo.UpdateRunPromptArtifacts(ctx, run.ID, "p", "s")
		},
		"UpdateRunResultJSON": func() error {
			return repo.UpdateRunResultJSON(ctx, run.ID, "{}")
		},
		"UpdateRunOutputSummary": func() error {
			return repo.UpdateRunOutputSummary(ctx, run.ID, "o", "f")
		},
		"FinishRun": func() error {
			return repo.FinishRun(ctx, run.ID, "finished", nil)
		},
		"ClaimRun": func() error {
			_, err := repo.ClaimRun(ctx, "a1")
			return err
		},
		"ClaimNextEligibleRun": func() error {
			_, err := repo.ClaimNextEligibleRun(ctx)
			return err
		},
		"CoalesceRun": func() error {
			_, err := repo.CoalesceRun(ctx, "a1", "r", 60, "{}")
			return err
		},
		"ScheduleRetry": func() error {
			return repo.ScheduleRetry(ctx, run.ID, time.Now().UTC(), 1)
		},
		"CleanExpired": func() error {
			_, err := repo.CleanExpired(ctx, time.Now().UTC())
			return err
		},
		"RecoverStale": func() error {
			_, err := repo.RecoverStale(ctx, time.Now().UTC())
			return err
		},
		"CancelRunsWhere": func() error {
			_, err := repo.CancelRunsWhere(ctx, "reason", `id = ?`, run.ID)
			return err
		},
		"CancelRun": func() error {
			return repo.CancelRun(ctx, run.ID, "reason")
		},
		"BulkCancelRuns": func() error {
			return repo.BulkCancelRuns(ctx, []string{run.ID}, "reason")
		},
		"AppendRunEvent": func() error {
			_, err := repo.AppendRunEvent(ctx, run.ID, "run.progress", "info", "{}")
			return err
		},
		"SetRunRequestedAtForTest": func() error {
			return repo.SetRunRequestedAtForTest(ctx, run.ID, time.Now().UTC())
		},
		"SetRunStatusForTest": func() error {
			return repo.SetRunStatusForTest(ctx, run.ID, "queued", nil, nil)
		},
		"SetRunErrorMessageForTest": func() error {
			return repo.SetRunErrorMessageForTest(ctx, run.ID, "boom")
		},
	}
	for name, call := range checks {
		if err := call(); err == nil {
			t.Errorf("%s returned nil error against a closed writer", name)
		}
	}
}

// TestGetRunWithCosts_RollupFailureDegradesToZeroes pins the documented
// asymmetry: the run row is mandatory, the cost rollup is best-effort.
// Losing the cost table must still return the run with a zero rollup
// rather than failing the whole read.
func TestGetRunWithCosts_RollupFailureDegradesToZeroes(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()
	run := seedTaskRun(t, repo, "costed", "a1", "t1", "queued")
	seedCostEvent(t, writer, "c1", "t1", 100, 10, 20, 5)

	if _, err := writer.Exec(`DROP TABLE office_cost_events`); err != nil {
		t.Fatalf("drop cost table: %v", err)
	}

	got, rollup, err := repo.GetRunWithCosts(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with costs: %v", err)
	}
	if got == nil || got.ID != run.ID {
		t.Fatalf("run = %v, want %q", got, run.ID)
	}
	if rollup == nil {
		t.Fatalf("rollup = nil, want a zero rollup")
	}
	checkInt64(t, "input_tokens", rollup.InputTokens, 0)
	checkInt64(t, "output_tokens", rollup.OutputTokens, 0)
	checkInt64(t, "cached_tokens", rollup.CachedTokens, 0)
	checkInt64(t, "cost_subcents", rollup.CostSubcents, 0)
}
