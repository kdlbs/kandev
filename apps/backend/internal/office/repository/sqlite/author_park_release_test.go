package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	runsmodels "github.com/kandev/kandev/internal/runs/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestReleaseAuthorExclusionParkedRuns(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	exerciseAuthorParkRelease(t, newAuthorProviderRepo(t, db), db)
}

// TestPostgresReleaseAuthorExclusionParkedRuns is the PostgreSQL twin. Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresReleaseAuthorExclusionParkedRuns(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	exerciseAuthorParkRelease(t, newAuthorProviderRepo(t, db), db)
}

func TestSettleCeilingDeferredRunRecordsTheDeferredRoute(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	exerciseDeferredRouteSettlement(t, newAuthorProviderRepo(t, db), db)
}

// TestPostgresSettleCeilingDeferredRunRecordsTheDeferredRoute is the
// PostgreSQL twin. Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSettleCeilingDeferredRunRecordsTheDeferredRoute(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	exerciseDeferredRouteSettlement(t, newAuthorProviderRepo(t, db), db)
}

func seedRoutingAgent(t *testing.T, db *sqlx.DB, id, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(db.Rebind(`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`),
		id+"-type", id+"-type", now, now)
	require.NoError(t, err)
	_, err = db.Exec(db.Rebind(`INSERT INTO agent_profiles
		(id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES (?, ?, 'name', 'display', ?, '', ?, ?)`), id, id+"-type", workspaceID, now, now)
	require.NoError(t, err)
}

// parkedRun creates a run at stepID for agentID, appends one route attempt per
// outcome, and parks it under status.
func parkedRun(
	t *testing.T, repo *sqlite.Repository, agentID, stepID, status string, outcomes ...models.RouteAttemptOutcome,
) string {
	t.Helper()
	ctx := context.Background()
	run := &runsmodels.Run{
		AgentProfileID: agentID, Reason: "task_assigned", Status: "queued", CoalescedCount: 1,
		Payload: `{"task_id":"t-1","workflow_step_id":"` + stepID + `"}`,
	}
	require.NoError(t, repo.CreateRun(ctx, run))
	for _, outcome := range outcomes {
		appendAttempt(t, repo, run.ID, models.RouteAttempt{ProviderID: "claude-acp", Model: "opus", Outcome: outcome})
	}
	require.NoError(t, repo.ParkRunForProviderCapacity(ctx, run.ID, status, time.Time{}))
	return run.ID
}

func appendAttempt(t *testing.T, repo *sqlite.Repository, runID string, attempt models.RouteAttempt) {
	t.Helper()
	ctx := context.Background()
	seq, err := repo.IncrementRouteAttemptSeq(ctx, runID)
	require.NoError(t, err)
	attempt.RunID, attempt.Seq, attempt.Tier, attempt.StartedAt = runID, seq, "balanced", time.Now().UTC()
	require.NoError(t, repo.AppendRouteAttempt(ctx, &attempt))
}

func runIsParked(t *testing.T, repo *sqlite.Repository, runID string) bool {
	t.Helper()
	got, err := repo.GetRunByID(context.Background(), runID)
	require.NoError(t, err)
	return got.RoutingBlockedStatus != nil && *got.RoutingBlockedStatus != ""
}

func exerciseAuthorParkRelease(t *testing.T, repo *sqlite.Repository, db *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	seedRoutingAgent(t, db, "agent-a", "ws-a")
	seedRoutingAgent(t, db, "agent-b", "ws-b")
	blocked := string(runsmodels.RoutingBlockedActionRequired)
	waiting := string(runsmodels.RoutingBlockedWaitingForCapacity)
	skip := models.RouteAttemptOutcomeSkippedAuthorProvider
	deferred := models.RouteAttemptOutcomeDeferredCapacity

	released := parkedRun(t, repo, "agent-a", "step-a", waiting, models.RouteAttemptOutcomeFailedProviderUnavail, skip)
	otherStep := parkedRun(t, repo, "agent-a", "step-b", blocked, skip)
	ceilingHold := parkedRun(t, repo, "agent-a", "step-a", blocked, deferred)
	ceilingAfterSkip := parkedRun(t, repo, "agent-a", "step-a", blocked, skip, deferred)
	recovery := parkedRun(t, repo, "agent-a", "step-a", string(runsmodels.RoutingBlockedSessionRecoveryRequired), skip)
	otherWorkspace := parkedRun(t, repo, "agent-b", "step-a", blocked, skip)
	earlierCycle := parkedRun(t, repo, "agent-a", "step-a", blocked, skip)
	require.NoError(t, repo.BumpRouteCycleBaseline(ctx, earlierCycle))
	appendAttempt(t, repo, earlierCycle, models.RouteAttempt{
		ProviderID: "codex-acp", Model: "gpt", Outcome: models.RouteAttemptOutcomeSkippedDegraded,
	})

	n, err := repo.ReleaseAuthorExclusionParkedRuns(ctx, "ws-a", []string{"step-a"})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	require.False(t, runIsParked(t, repo, released), "author-exclusion park at the step is released")
	got, err := repo.GetRunByID(ctx, released)
	require.NoError(t, err)
	require.Equal(t, "queued", string(got.Status))
	for name, id := range map[string]string{
		"park at another step":                otherStep,
		"session-ceiling hold":                ceilingHold,
		"session-ceiling hold after the skip": ceilingAfterSkip,
		"session-recovery park":               recovery,
		"park in another workspace":           otherWorkspace,
		"skip from an earlier retry cycle":    earlierCycle,
	} {
		require.True(t, runIsParked(t, repo, id), "%s must stay parked", name)
	}

	n, err = repo.ReleaseAuthorExclusionParkedRuns(ctx, "ws-a", nil)
	require.NoError(t, err)
	require.Zero(t, n)
}

func exerciseDeferredRouteSettlement(t *testing.T, repo *sqlite.Repository, db *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	seedRoutingAgent(t, db, "agent-a", "ws-a")
	// Covers AC-OFFICE-REVIEW-AUTHOR-EXCL-002.6 in both event orders.
	for _, tc := range []struct {
		name                   string
		completeBeforeSettling bool
	}{
		{name: "settle before completion"},
		{name: "complete before settlement", completeBeforeSettling: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := &runsmodels.Run{
				AgentProfileID: "agent-a", Reason: "task_assigned", Status: "queued", CoalescedCount: 1,
				Payload: `{"task_id":"t-1","workflow_step_id":"step-work"}`,
			}
			require.NoError(t, repo.CreateRun(ctx, run))
			appendAttempt(t, repo, run.ID, models.RouteAttempt{
				ExecutionProfileID: "prof-codex", ProviderID: "codex-acp", Model: "gpt",
				Outcome: models.RouteAttemptOutcomeFailedProviderUnavail,
			})
			appendAttempt(t, repo, run.ID, models.RouteAttempt{
				ExecutionProfileID: "prof-claude", ProviderID: "claude-acp", Model: "opus",
				Outcome: models.RouteAttemptOutcomeDeferredCapacity,
			})
			require.NoError(t, repo.ParkRunForProviderCapacity(
				ctx, run.ID, string(runsmodels.RoutingBlockedActionRequired), time.Time{}))

			if tc.completeBeforeSettling {
				finished, err := repo.FinishCeilingDeferredRunOnCompletion(
					ctx, run.ID, "session-1", "t-1", "agent-a", "processed",
				)
				require.NoError(t, err)
				require.NotNil(t, finished)
			} else {
				wrote, err := repo.SettleCeilingDeferredRun(ctx, run.ID, "session-1", "t-1", "agent-a")
				require.NoError(t, err)
				require.True(t, wrote)
			}

			got, err := repo.GetRunByID(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, "claude-acp", derefStr(got.ResolvedProviderID))
			require.Equal(t, "opus", derefStr(got.ResolvedModel))
			require.Equal(t, "prof-claude", derefStr(got.ResolvedExecutionProfileID))
		})
	}

	// A run with no deferred attempt settles without inventing a route.
	bare := &runsmodels.Run{
		AgentProfileID: "agent-a", Reason: "task_assigned", Status: "queued", CoalescedCount: 1,
		Payload: `{"task_id":"t-2"}`,
	}
	require.NoError(t, repo.CreateRun(ctx, bare))
	require.NoError(t, repo.ParkRunForProviderCapacity(
		ctx, bare.ID, string(runsmodels.RoutingBlockedActionRequired), time.Time{}))
	wrote, err := repo.SettleCeilingDeferredRun(ctx, bare.ID, "session-2", "t-2", "agent-a")
	require.NoError(t, err)
	require.True(t, wrote)
	got, err := repo.GetRunByID(ctx, bare.ID)
	require.NoError(t, err)
	require.Nil(t, got.ResolvedProviderID)
}
