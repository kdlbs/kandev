package sqlite_test

import (
	"context"
	"testing"
	"time"

	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

func TestPostgresRunClaimRecoveryCAS(t *testing.T) {
	repo := newTestRepoPostgres(t)
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-30 * time.Minute)
	run := mustCreateRun(t, repo, &runsmodels.Run{
		ID: "pg-run-claim-recovery-cas", AgentProfileID: "a1", WorkspaceID: "ws-a1",
		Reason: "task_assigned", Payload: `{"task_id":"task-1"}`,
		Status: "queued", CoalescedCount: 1,
	})
	setStatus(t, repo, run.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	candidates, err := repo.ListStaleClaimedRuns(ctx, cutoff)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("list stale claims: candidates=%#v err=%v", candidates, err)
	}

	completedAt := time.Now().UTC()
	setStatus(t, repo, run.ID, "finished", candidates[0].ClaimedAt, timePtr(completedAt))
	completed, err := repo.GetRunByID(ctx, run.ID)
	if err != nil || completed.FinishedAt == nil {
		t.Fatalf("read completed row before recovery: run=%#v err=%v", completed, err)
	}
	wrote, err := repo.RecoverStaleClaimedRun(ctx, candidates[0], cutoff)
	if err != nil {
		t.Fatalf("recover stale claim: %v", err)
	}
	if wrote {
		t.Fatal("recovery overwrote a completed row")
	}
	got := mustGetRun(t, repo, run.ID)
	if got.Status != runsmodels.RunStatusFinished || got.FinishedAt == nil || !got.FinishedAt.Equal(*completed.FinishedAt) {
		t.Fatalf("completed row changed after recovery: %+v", got)
	}
}
