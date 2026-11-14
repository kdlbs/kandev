package sqlite_test

import (
	"context"
	"testing"
	"time"

	runsmodels "github.com/kandev/kandev/internal/runs/models"
)

func TestListStaleClaimedTaskRuns_ReturnsOnlyStaleTaskBoundClaims(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	stale := queueRunAt(t, repo, "stale-task", "a1", cutoff.Add(-time.Hour))
	setStatus(t, repo, stale.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	if _, err := repo.SetRunSessionID(ctx, stale.ID, "session-1"); err != nil {
		t.Fatalf("set session: %v", err)
	}
	fresh := queueRunAt(t, repo, "fresh-task", "a2", cutoff.Add(-time.Hour))
	setStatus(t, repo, fresh.ID, "claimed", timePtr(cutoff.Add(time.Minute)), nil)
	taskless := mustCreateRun(t, repo, &runsmodels.Run{
		ID: "stale-taskless", AgentProfileID: "a3", WorkspaceID: "ws-a3",
		Reason: "heartbeat", Payload: `{}`, Status: "queued", CoalescedCount: 1,
	})
	setStatus(t, repo, taskless.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	queued := queueRunAt(t, repo, "queued-task", "a4", cutoff.Add(-time.Hour))

	runs, err := repo.ListStaleClaimedTaskRuns(ctx, cutoff)
	if err != nil {
		t.Fatalf("list stale claimed task runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %+v, want only %s", runs, stale.ID)
	}
	got := runs[0]
	checkString(t, "id", got.ID, stale.ID)
	checkString(t, "agent", got.AgentProfileID, "a1")
	checkString(t, "task", got.TaskID, stale.ID)
	checkString(t, "session", got.SessionID, "session-1")
	checkString(t, "queued run untouched", string(mustGetRun(t, repo, queued.ID).Status), "queued")
}

func TestRenewStaleClaim_RenewsOnlyAStillStaleClaim(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	now := cutoff.Add(30 * time.Minute)

	stale := queueRunAt(t, repo, "stale", "a1", cutoff.Add(-time.Hour))
	setStatus(t, repo, stale.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	finished := queueRunAt(t, repo, "finished", "a2", cutoff.Add(-time.Hour))
	setStatus(t, repo, finished.ID, "finished",
		timePtr(cutoff.Add(-time.Hour)), timePtr(cutoff.Add(-time.Minute)))

	renewed, err := repo.RenewStaleClaim(ctx, stale.ID, cutoff, now)
	if err != nil || !renewed {
		t.Fatalf("renew stale = (%v, %v), want (true, nil)", renewed, err)
	}
	got := mustGetRun(t, repo, stale.ID)
	checkString(t, "status", string(got.Status), "claimed")
	if got.ClaimedAt == nil || !got.ClaimedAt.Equal(now) {
		t.Errorf("claimed_at = %v, want %s", got.ClaimedAt, now)
	}

	again, err := repo.RenewStaleClaim(ctx, stale.ID, cutoff, now.Add(time.Minute))
	if err != nil || again {
		t.Errorf("second renew = (%v, %v), want (false, nil) for a fresh claim", again, err)
	}
	terminal, err := repo.RenewStaleClaim(ctx, finished.ID, cutoff, now)
	if err != nil || terminal {
		t.Errorf("renew finished = (%v, %v), want (false, nil)", terminal, err)
	}
	recovered, err := repo.RecoverStale(ctx, cutoff)
	if err != nil || recovered != 0 {
		t.Errorf("recover after renew = (%d, %v), want (0, nil)", recovered, err)
	}
}
