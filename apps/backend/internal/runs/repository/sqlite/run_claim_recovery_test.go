package sqlite_test

import (
	"context"
	"testing"
	"time"

	runsmodels "github.com/kandev/kandev/internal/runs/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

func TestRunClaimRecoveryListsOnlyStrictlyStaleClaims(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

	stale := queueRunAt(t, repo, "claim-recovery-stale", "a1", cutoff.Add(-time.Hour))
	setStatus(t, repo, stale.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	atCutoff := queueRunAt(t, repo, "claim-recovery-at-cutoff", "a2", cutoff.Add(-time.Hour))
	setStatus(t, repo, atCutoff.ID, "claimed", timePtr(cutoff), nil)
	fresh := queueRunAt(t, repo, "claim-recovery-fresh", "a3", cutoff.Add(-time.Hour))
	setStatus(t, repo, fresh.ID, "claimed", timePtr(cutoff.Add(time.Minute)), nil)
	queueRunAt(t, repo, "claim-recovery-queued", "a4", cutoff.Add(-time.Hour))

	got, err := repo.ListStaleClaimedRuns(ctx, cutoff)
	if err != nil {
		t.Fatalf("list stale claimed runs: %v", err)
	}
	if len(got) != 1 || got[0].ID != stale.ID {
		t.Fatalf("stale claims = %#v, want only %q", got, stale.ID)
	}
}

func TestRunClaimRecoveryCASDoesNotOverwriteChangedRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*testing.T, *runssqlite.Repository, string, time.Time)
		status string
	}{
		{
			name: "cancelled",
			mutate: func(t *testing.T, repo *runssqlite.Repository, id string, at time.Time) {
				setStatus(t, repo, id, "cancelled", timePtr(at), timePtr(at.Add(time.Minute)))
			},
			status: "cancelled",
		},
		{
			name: "completed",
			mutate: func(t *testing.T, repo *runssqlite.Repository, id string, at time.Time) {
				setStatus(t, repo, id, "finished", timePtr(at), timePtr(at.Add(time.Minute)))
			},
			status: "finished",
		},
		{
			name: "newer claim",
			mutate: func(t *testing.T, repo *runssqlite.Repository, id string, at time.Time) {
				setStatus(t, repo, id, "claimed", timePtr(at.Add(time.Hour)), nil)
			},
			status: "claimed",
		},
		{
			name: "route attempt changed",
			mutate: func(t *testing.T, repo *runssqlite.Repository, id string, _ time.Time) {
				t.Helper()
				if _, err := repo.Writer().Exec(`UPDATE runs SET current_route_attempt_seq = 2 WHERE id = ?`, id); err != nil {
					t.Fatalf("change route attempt: %v", err)
				}
			},
			status: "claimed",
		},
		{
			name: "session changed",
			mutate: func(t *testing.T, repo *runssqlite.Repository, id string, _ time.Time) {
				t.Helper()
				if _, err := repo.Writer().Exec(`UPDATE runs SET session_id = 'new-session' WHERE id = ?`, id); err != nil {
					t.Fatalf("change session: %v", err)
				}
			},
			status: "claimed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newTestRepo(t)
			ctx := context.Background()
			cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
			run := mustCreateRun(t, repo, &runsmodels.Run{
				ID: "claim-recovery-cas", AgentProfileID: "a1", WorkspaceID: "ws-a1",
				Reason: "task_assigned", Payload: `{"task_id":"task-1"}`,
				Status: "queued", CoalescedCount: 1, CurrentRouteAttemptSeq: 1,
			})
			setStatus(t, repo, run.ID, "claimed", timePtr(cutoff.Add(-time.Hour)), nil)
			candidates, err := repo.ListStaleClaimedRuns(ctx, cutoff)
			if err != nil || len(candidates) != 1 {
				t.Fatalf("list stale claims: candidates=%#v err=%v", candidates, err)
			}
			observed := candidates[0]
			tc.mutate(t, repo, run.ID, *observed.ClaimedAt)

			wrote, err := repo.RecoverStaleClaimedRun(ctx, observed, cutoff)
			if err != nil {
				t.Fatalf("recover stale claimed run: %v", err)
			}
			if wrote {
				t.Fatal("recovery wrote over a row changed after candidate read")
			}
			if got := mustGetRun(t, repo, run.ID); string(got.Status) != tc.status {
				t.Fatalf("status after no-op recovery = %q, want %q", got.Status, tc.status)
			}
		})
	}
}

func TestRunClaimRecoveryCASRequeuesUnchangedClaim(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	cutoff := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	periodic := mustCreateRun(t, repo, &runsmodels.Run{
		ID: "claim-recovery-unchanged", AgentProfileID: "a1", WorkspaceID: "ws-a1",
		Reason: "heartbeat", Payload: `{}`, Status: "queued", CoalescedCount: 1,
		PriorityClass: runsmodels.PriorityClassPeriodic,
	})
	human := mustCreateRun(t, repo, &runsmodels.Run{
		ID: "claim-recovery-human", AgentProfileID: "a2", WorkspaceID: "ws-a2",
		Reason: "manual_resume_after_failure", Payload: `{}`, Status: "queued", CoalescedCount: 1,
		PriorityClass: runsmodels.PriorityClassHuman,
	})
	setStatus(t, repo, periodic.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	setStatus(t, repo, human.ID, "claimed", timePtr(cutoff.Add(-time.Minute)), nil)
	candidates, err := repo.ListStaleClaimedRuns(ctx, cutoff)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("list stale claims: candidates=%#v err=%v", candidates, err)
	}
	for _, candidate := range candidates {
		wrote, err := repo.RecoverStaleClaimedRun(ctx, candidate, cutoff)
		if err != nil || !wrote {
			t.Fatalf("recover unchanged claim %q: wrote=%v err=%v, want true", candidate.ID, wrote, err)
		}
	}
	gotPeriodic := mustGetRun(t, repo, periodic.ID)
	if gotPeriodic.Status != runsmodels.RunStatusQueued || gotPeriodic.ClaimedAt != nil {
		t.Fatalf("periodic run after recovery = status %q claimed_at %v, want queued and no claim", gotPeriodic.Status, gotPeriodic.ClaimedAt)
	}
	if gotPeriodic.PriorityClass != runsmodels.PriorityClassRecovery {
		t.Fatalf("periodic priority after recovery = %v, want recovery", gotPeriodic.PriorityClass)
	}
	gotHuman := mustGetRun(t, repo, human.ID)
	if gotHuman.PriorityClass != runsmodels.PriorityClassHuman {
		t.Fatalf("human priority after recovery = %v, want human", gotHuman.PriorityClass)
	}
}

func TestRunClaimRecoveryCASMatchesLegacySQLiteTimestamp(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	stale := mustCreateRun(t, repo, &runsmodels.Run{
		ID: "claim-recovery-legacy-timestamp", AgentProfileID: "a1", WorkspaceID: "ws-a1",
		Reason: "task_assigned", Payload: `{}`, Status: "queued", CoalescedCount: 1,
	})
	fresh := mustCreateRun(t, repo, &runsmodels.Run{
		ID: "claim-recovery-legacy-fresh", AgentProfileID: "a2", WorkspaceID: "ws-a2",
		Reason: "task_assigned", Payload: `{}`, Status: "queued", CoalescedCount: 1,
	})
	now := time.Now().UTC()
	cutoff := now.Add(-30 * time.Minute)
	if _, err := repo.Writer().Exec(`UPDATE runs SET status = 'claimed', claimed_at = datetime(?) WHERE id = ?`,
		now.Add(-35*time.Minute), stale.ID); err != nil {
		t.Fatalf("set legacy stale claim timestamp: %v", err)
	}
	if _, err := repo.Writer().Exec(`UPDATE runs SET status = 'claimed', claimed_at = datetime(?) WHERE id = ?`,
		now.Add(-20*time.Minute), fresh.ID); err != nil {
		t.Fatalf("set legacy fresh claim timestamp: %v", err)
	}
	candidates, err := repo.ListStaleClaimedRuns(ctx, cutoff)
	if err != nil || len(candidates) != 1 || candidates[0].ID != stale.ID {
		t.Fatalf("list legacy stale claim: candidates=%#v err=%v", candidates, err)
	}

	wrote, err := repo.RecoverStaleClaimedRun(ctx, candidates[0], cutoff)
	if err != nil || !wrote {
		t.Fatalf("recover legacy claim: wrote=%v err=%v, want true", wrote, err)
	}
}
