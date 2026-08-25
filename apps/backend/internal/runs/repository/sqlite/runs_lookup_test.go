package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestFindInflightRunForAgent_ReturnsTheNewestQueuedOrClaimedRun pins
// both the in-flight status set and the newest-first ordering.
func TestFindInflightRunForAgent_ReturnsTheNewestQueuedOrClaimedRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	queueRunAt(t, repo, "older-queued", "a1", base)
	newest := queueRunAt(t, repo, "newer-claimed", "a1", base.Add(time.Minute))
	setStatus(t, repo, newest.ID, "claimed", timePtr(base.Add(time.Minute)), nil)
	// Newer still, but terminal — must not be reported as in-flight.
	terminal := queueRunAt(t, repo, "newest-finished", "a1", base.Add(2*time.Minute))
	setStatus(t, repo, terminal.ID, "finished", nil, timePtr(base.Add(3*time.Minute)))
	// Newest of all, but a different agent.
	queueRunAt(t, repo, "other-agent", "a2", base.Add(4*time.Minute))

	got, err := repo.FindInflightRunForAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("find inflight: %v", err)
	}
	if got.ID != newest.ID {
		t.Errorf("inflight = %q, want %q", got.ID, newest.ID)
	}
	checkString(t, "status", string(got.Status), "claimed")
}

// TestFindInflightRunForAgent_TerminalRunsAreNotInflight covers the
// negative half: every terminal status must yield the sentinel.
func TestFindInflightRunForAgent_TerminalRunsAreNotInflight(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, status := range []string{"finished", "failed", "cancelled"} {
		run := queueRunAt(t, repo, "run-"+status, "a1", now)
		setStatus(t, repo, run.ID, status, timePtr(now), timePtr(now))
	}

	if _, err := repo.FindInflightRunForAgent(ctx, "a1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestGetClaimedRunByTaskID_MatchesThePayloadTaskAndPrefersTheLatestClaim
// pins the payload lookup plus the claimed_at ordering.
func TestGetClaimedRunByTaskID_MatchesThePayloadTaskAndPrefersTheLatestClaim(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	older := seedTaskRun(t, repo, "older-claim", "a1", "t1", "queued")
	setStatus(t, repo, older.ID, "claimed", timePtr(base), nil)
	newer := seedTaskRun(t, repo, "newer-claim", "a2", "t1", "queued")
	setStatus(t, repo, newer.ID, "claimed", timePtr(base.Add(time.Hour)), nil)
	// Same task but still queued, and another task's claimed run.
	seedTaskRun(t, repo, "queued-same-task", "a3", "t1", "queued")
	otherTask := seedTaskRun(t, repo, "other-task", "a4", "t2", "queued")
	setStatus(t, repo, otherTask.ID, "claimed", timePtr(base.Add(2*time.Hour)), nil)

	got, err := repo.GetClaimedRunByTaskID(ctx, "t1")
	if err != nil {
		t.Fatalf("get claimed run: %v", err)
	}
	if got.ID != newer.ID {
		t.Errorf("claimed run = %q, want %q (most recent claim for t1)", got.ID, newer.ID)
	}

	if _, err := repo.GetClaimedRunByTaskID(ctx, "t-unknown"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown task err = %v, want sql.ErrNoRows", err)
	}
}

// TestGetClaimedRunByTaskAndAgent_ScopesToTheAgentEvenWhenAnotherAgentsClaimIsNewer
// pins the Review round-4 BLOCKING FINDING 2 fix: two different agents can
// each have a claimed run on the same task at once (ClaimNextEligibleRun's
// busy-lock is per-agent, not per-task), so a caller that knows which
// agent's lifecycle event it is handling must not fall back to "most
// recently claimed" like GetClaimedRunByTaskID does.
func TestGetClaimedRunByTaskAndAgent_ScopesToTheAgentEvenWhenAnotherAgentsClaimIsNewer(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	wantAgent := seedTaskRun(t, repo, "older-claim-a1", "a1", "t1", "queued")
	setStatus(t, repo, wantAgent.ID, "claimed", timePtr(base), nil)
	otherAgent := seedTaskRun(t, repo, "newer-claim-a2", "a2", "t1", "queued")
	setStatus(t, repo, otherAgent.ID, "claimed", timePtr(base.Add(time.Hour)), nil)

	got, err := repo.GetClaimedRunByTaskAndAgent(ctx, "t1", "a1")
	if err != nil {
		t.Fatalf("get claimed run: %v", err)
	}
	if got.ID != wantAgent.ID {
		t.Errorf("claimed run = %q, want %q (a1's own claim, not a2's newer one)", got.ID, wantAgent.ID)
	}

	if _, err := repo.GetClaimedRunByTaskAndAgent(ctx, "t1", "a-unknown"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown agent err = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.GetClaimedRunByTaskAndAgent(ctx, "t-unknown", "a1"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("unknown task err = %v, want sql.ErrNoRows", err)
	}
}

// TestGetClaimedTasklessRunForAgent_OnlyMatchesRunsWithoutATask pins the
// heartbeat attribution rule: a missing or empty payload task_id counts
// as taskless, a populated one never does.
func TestGetClaimedTasklessRunForAgent_OnlyMatchesRunsWithoutATask(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	missing := mustCreateRun(t, repo, &models.Run{
		ID: "missing-task-id", AgentProfileID: "a1", Reason: "heartbeat",
		Payload: `{"note":"no task"}`,
	})
	setStatus(t, repo, missing.ID, "claimed", timePtr(base), nil)
	empty := mustCreateRun(t, repo, &models.Run{
		ID: "empty-task-id", AgentProfileID: "a1", Reason: "heartbeat",
		Payload: `{"task_id":""}`,
	})
	setStatus(t, repo, empty.ID, "claimed", timePtr(base.Add(time.Hour)), nil)
	// Newest claim, but it carries a task id.
	withTask := seedTaskRun(t, repo, "with-task", "a1", "t1", "queued")
	setStatus(t, repo, withTask.ID, "claimed", timePtr(base.Add(2*time.Hour)), nil)

	got, err := repo.GetClaimedTasklessRunForAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("get taskless run: %v", err)
	}
	if got.ID != empty.ID {
		t.Errorf("taskless run = %q, want %q (most recent taskless claim)", got.ID, empty.ID)
	}
}

// TestGetClaimedTasklessRunForAgent_ScopedToTheAgentAndToClaimedRuns
// covers the two remaining predicates.
func TestGetClaimedTasklessRunForAgent_ScopedToTheAgentAndToClaimedRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	queued := mustCreateRun(t, repo, &models.Run{
		ID: "queued", AgentProfileID: "a1", Reason: "heartbeat",
	})
	otherAgent := mustCreateRun(t, repo, &models.Run{
		ID: "other-agent", AgentProfileID: "a2", Reason: "heartbeat",
	})
	setStatus(t, repo, otherAgent.ID, "claimed", timePtr(now), nil)

	if _, err := repo.GetClaimedTasklessRunForAgent(ctx, "a1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows (a1's only run is queued)", err)
	}

	setStatus(t, repo, queued.ID, "claimed", timePtr(now), nil)
	got, err := repo.GetClaimedTasklessRunForAgent(ctx, "a1")
	if err != nil {
		t.Fatalf("get taskless run: %v", err)
	}
	if got.ID != queued.ID {
		t.Errorf("taskless run = %q, want %q", got.ID, queued.ID)
	}
}

// TestGetRunWithCosts_SumsOnlyTheRunsOwnTaskEvents pins the rollup
// join: cost events are matched on the task id inside the run payload.
func TestGetRunWithCosts_SumsOnlyTheRunsOwnTaskEvents(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()

	run := seedTaskRun(t, repo, "costed", "a1", "t1", "claimed")
	seedCostEvent(t, writer, "c1", "t1", 100, 10, 20, 5)
	seedCostEvent(t, writer, "c2", "t1", 200, 20, 30, 7)
	seedCostEvent(t, writer, "c3", "t2", 999, 999, 999, 999)

	got, rollup, err := repo.GetRunWithCosts(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with costs: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("run = %q, want %q", got.ID, run.ID)
	}
	checkInt64(t, "input_tokens", rollup.InputTokens, 300)
	checkInt64(t, "cached_tokens", rollup.CachedTokens, 30)
	checkInt64(t, "output_tokens", rollup.OutputTokens, 50)
	checkInt64(t, "cost_subcents", rollup.CostSubcents, 12)
}

// TestGetRunWithCosts_TasklessRunRollsUpToZero pins the non-empty
// task_id guard: a run without a task must not absorb the cost events
// that carry no task id either.
func TestGetRunWithCosts_TasklessRunRollsUpToZero(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()

	run := mustCreateRun(t, repo, &models.Run{
		ID: "taskless", AgentProfileID: "a1", Reason: "heartbeat",
	})
	seedCostEvent(t, writer, "orphan", "", 500, 50, 60, 70)

	got, rollup, err := repo.GetRunWithCosts(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run with costs: %v", err)
	}
	if got.ID != run.ID {
		t.Errorf("run = %q, want %q", got.ID, run.ID)
	}
	checkInt64(t, "input_tokens", rollup.InputTokens, 0)
	checkInt64(t, "cached_tokens", rollup.CachedTokens, 0)
	checkInt64(t, "output_tokens", rollup.OutputTokens, 0)
	checkInt64(t, "cost_subcents", rollup.CostSubcents, 0)
}

// TestGetRunWithCosts_NoCostEventsYieldsAZeroRollup pins the documented
// "run exists, costs not produced yet" shape.
func TestGetRunWithCosts_NoCostEventsYieldsAZeroRollup(t *testing.T) {
	repo := newTestRepo(t)
	run := seedTaskRun(t, repo, "no-costs", "a1", "t1", "queued")

	got, rollup, err := repo.GetRunWithCosts(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run with costs: %v", err)
	}
	if got == nil || rollup == nil {
		t.Fatalf("run = %v, rollup = %v, want both non-nil", got, rollup)
	}
	checkInt64(t, "input_tokens", rollup.InputTokens, 0)
	checkInt64(t, "cost_subcents", rollup.CostSubcents, 0)
}

// TestGetRunWithCosts_UnknownRunReturnsNoRows pins the sentinel.
func TestGetRunWithCosts_UnknownRunReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)
	run, rollup, err := repo.GetRunWithCosts(context.Background(), "nope")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
	if run != nil || rollup != nil {
		t.Errorf("run = %v, rollup = %v, want both nil on error", run, rollup)
	}
}

func checkInt64(t *testing.T, field string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", field, got, want)
	}
}

// TestGetRunWorkspaceID_ResolvesViaAgentProfileJoin pins the WS gateway's
// run.subscribe authorization lookup (WO-02): the run's owning workspace
// comes from its agent profile, the same join idiom ListRuns uses.
func TestGetRunWorkspaceID_ResolvesViaAgentProfileJoin(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, writer, "a1", "ws-1")
	run := seedTaskRun(t, repo, "run-1", "a1", "t1", "queued")

	got, err := repo.GetRunWorkspaceID(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run workspace id: %v", err)
	}
	checkString(t, "workspace_id", got, "ws-1")
}

// TestGetRunWorkspaceID_StillResolvesWhenAgentProfileSoftDeleted pins the
// deliberate choice of a raw join over GetAgentInstance: GetAgentInstance
// filters deleted_at IS NULL, which would make a run under a soft-deleted
// agent profile permanently unresolvable (and so permanently deniable) to
// its own owner.
func TestGetRunWorkspaceID_StillResolvesWhenAgentProfileSoftDeleted(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, writer, "a1", "ws-1")
	run := seedTaskRun(t, repo, "run-1", "a1", "t1", "queued")

	if _, err := writer.Exec(`UPDATE agent_profiles SET deleted_at = ? WHERE id = 'a1'`, time.Now().UTC()); err != nil {
		t.Fatalf("soft-delete agent profile: %v", err)
	}

	got, err := repo.GetRunWorkspaceID(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run workspace id after soft delete: %v", err)
	}
	checkString(t, "workspace_id", got, "ws-1")
}

// TestGetRunWorkspaceID_UnknownRunReturnsNoRows pins the sentinel the
// gateway's subscribe hook denies on.
func TestGetRunWorkspaceID_UnknownRunReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.GetRunWorkspaceID(context.Background(), "nope"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestGetRunWorkspaceID_ResolvesToEmptyStringForUnscopedAgentProfile pins the
// WO-02 round-2 fix: an ordinary (non-Office) agent profile has
// workspace_id=” (schema default, no backfill), and the join returns that
// empty string as-is rather than an error. The gateway's subscribe hook (not
// this repository) is responsible for denying on an empty result.
func TestGetRunWorkspaceID_ResolvesToEmptyStringForUnscopedAgentProfile(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()

	seedAgentProfile(t, writer, "a1", "")
	run := seedTaskRun(t, repo, "run-1", "a1", "t1", "queued")

	got, err := repo.GetRunWorkspaceID(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run workspace id: %v", err)
	}
	checkString(t, "workspace_id", got, "")
}
