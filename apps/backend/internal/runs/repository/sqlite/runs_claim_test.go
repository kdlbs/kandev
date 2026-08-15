package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// queueRunAt seeds a queued run for an agent and backdates it so
// ordering assertions are deterministic.
func queueRunAt(
	t *testing.T, repo *runssqlite.Repository, id, agentID string, requestedAt time.Time,
) *models.Run {
	t.Helper()
	run := mustCreateRun(t, repo, &models.Run{
		ID:             id,
		AgentProfileID: agentID,
		Reason:         "task_assigned",
		Payload:        `{"task_id":"` + id + `"}`,
		Status:         "queued",
		CoalescedCount: 1,
	})
	setRequestedAt(t, repo, run.ID, requestedAt)
	return run
}

// TestClaimRun_ClaimsTheOldestQueuedRunForTheAgent asserts which run
// comes back, not merely that one did: the queue is FIFO by
// requested_at, so reversing the ORDER BY must fail here.
func TestClaimRun_ClaimsTheOldestQueuedRunForTheAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	oldest := queueRunAt(t, repo, "oldest", "a1", base)
	queueRunAt(t, repo, "middle", "a1", base.Add(time.Minute))
	queueRunAt(t, repo, "newest", "a1", base.Add(2*time.Minute))
	// An older run belonging to a different agent must not be picked.
	queueRunAt(t, repo, "other-agent", "a2", base.Add(-time.Hour))

	before := time.Now().UTC().Add(-time.Second)
	claimed, err := repo.ClaimRun(ctx, "a1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != oldest.ID {
		t.Errorf("claimed %q, want %q (oldest queued run for a1)", claimed.ID, oldest.ID)
	}
	checkString(t, "returned status", string(claimed.Status), "claimed")
	if claimed.ClaimedAt == nil || !claimed.ClaimedAt.After(before) {
		t.Errorf("returned claimed_at = %v, want a fresh stamp after %s", claimed.ClaimedAt, before)
	}

	stored := mustGetRun(t, repo, oldest.ID)
	checkString(t, "stored status", string(stored.Status), "claimed")
	if stored.ClaimedAt == nil {
		t.Errorf("stored claimed_at = nil, want a stamp")
	}
	checkString(t, "sibling status", string(mustGetRun(t, repo, "middle").Status), "queued")
	checkString(t, "other agent status", string(mustGetRun(t, repo, "other-agent").Status), "queued")
}

// TestClaimRun_NoQueuedRunReturnsNoRows pins the sentinel the scheduler
// treats as "nothing to do" — including the case where the agent's only
// runs are already claimed or terminal.
func TestClaimRun_NoQueuedRunReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	run := queueRunAt(t, repo, "claimed-already", "a1", time.Now().UTC())
	setStatus(t, repo, run.ID, "claimed", timePtr(time.Now().UTC()), nil)
	done := queueRunAt(t, repo, "finished", "a1", time.Now().UTC())
	setStatus(t, repo, done.ID, "finished", nil, timePtr(time.Now().UTC()))

	if _, err := repo.ClaimRun(ctx, "a1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
	if _, err := repo.ClaimRun(ctx, "unknown-agent"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown agent err = %v, want sql.ErrNoRows", err)
	}
}

// TestClaimRun_ConcurrentClaimsAreExclusive races real goroutines at a
// fixed set of queued runs. Every run must be handed to exactly one
// caller: a weakened status guard hands the same row out twice, which
// two sequential calls would never reveal.
func TestClaimRun_ConcurrentClaimsAreExclusive(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	const queued = 3
	const claimers = 8
	for i := range queued {
		queueRunAt(t, repo, string(rune('a'+i))+"-run", "a1", base.Add(time.Duration(i)*time.Minute))
	}

	ids, noRows := runConcurrentClaims(t, claimers, func() (*models.Run, error) {
		return repo.ClaimRun(ctx, "a1")
	})

	if len(ids) != queued {
		t.Errorf("%d successful claims, want %d", len(ids), queued)
	}
	if noRows != claimers-queued {
		t.Errorf("%d sql.ErrNoRows results, want %d", noRows, claimers-queued)
	}
	assertNoDuplicates(t, ids)
	assertClaimedCount(t, repo, queued)
}

// TestClaimNextEligibleRun_PicksTheOldestEligibleRun asserts the queue
// order across agents.
func TestClaimNextEligibleRun_PicksTheOldestEligibleRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	queueRunAt(t, repo, "newer", "a1", base.Add(time.Minute))
	oldest := queueRunAt(t, repo, "older", "a2", base)

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != oldest.ID {
		t.Errorf("claimed %q, want %q (oldest queued run)", claimed.ID, oldest.ID)
	}
	checkString(t, "status", string(claimed.Status), "claimed")
	checkString(t, "untouched sibling", string(mustGetRun(t, repo, "newer").Status), "queued")
}

// TestClaimNextEligibleRun_SkipsAgentThatAlreadyHasAClaimedRun is the
// one-run-per-agent guard. The busy agent owns the *older* queued run,
// so a guard that stops counting claimed runs correctly returns the
// wrong row rather than merely returning nothing.
func TestClaimNextEligibleRun_SkipsAgentThatAlreadyHasAClaimedRun(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	busy := queueRunAt(t, repo, "busy-agent-inflight", "a1", base.Add(-time.Hour))
	setStatus(t, repo, busy.ID, "claimed", timePtr(base), nil)
	queueRunAt(t, repo, "busy-agent-queued", "a1", base)
	idle := queueRunAt(t, repo, "idle-agent-queued", "a2", base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != idle.ID {
		t.Errorf("claimed %q, want %q (a1 already has a claimed run)", claimed.ID, idle.ID)
	}
	checkString(t, "busy agent queued run", string(mustGetRun(t, repo, "busy-agent-queued").Status), "queued")
}

// TestClaimNextEligibleRun_HonoursScheduledRetryWindow pins the retry
// gate in both directions.
func TestClaimNextEligibleRun_HonoursScheduledRetryWindow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	future := queueRunAt(t, repo, "retry-future", "a1", base)
	if err := repo.ScheduleRetry(ctx, future.ID, time.Now().UTC().Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule future retry: %v", err)
	}
	setRequestedAt(t, repo, future.ID, base)
	due := queueRunAt(t, repo, "retry-due", "a2", base.Add(time.Minute))
	if err := repo.ScheduleRetry(ctx, due.ID, time.Now().UTC().Add(-time.Hour), 1); err != nil {
		t.Fatalf("schedule due retry: %v", err)
	}
	setRequestedAt(t, repo, due.ID, base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != due.ID {
		t.Errorf("claimed %q, want %q (the future retry must be skipped)", claimed.ID, due.ID)
	}

	// With the due run taken, the future retry is still not eligible.
	if _, err := repo.ClaimNextEligibleRun(ctx); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("second claim err = %v, want sql.ErrNoRows", err)
	}
}

// TestClaimNextEligibleRun_SkipsRoutingBlockedRuns pins the parked-run
// filter: a run parked by the provider router is not dispatchable even
// though it is queued and older than everything else.
func TestClaimNextEligibleRun_SkipsRoutingBlockedRuns(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	parked := queueRunAt(t, repo, "parked", "a1", base)
	if _, err := writer.Exec(
		`UPDATE runs SET routing_blocked_status = 'waiting_for_provider_capacity' WHERE id = ?`,
		parked.ID,
	); err != nil {
		t.Fatalf("park run: %v", err)
	}
	open := queueRunAt(t, repo, "open", "a2", base.Add(time.Minute))

	claimed, err := repo.ClaimNextEligibleRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != open.ID {
		t.Errorf("claimed %q, want %q (parked run must be skipped)", claimed.ID, open.ID)
	}
	checkString(t, "parked run status", string(mustGetRun(t, repo, parked.ID).Status), "queued")
}

// TestClaimNextEligibleRun_EmptyQueueReturnsNoRows pins the sentinel.
func TestClaimNextEligibleRun_EmptyQueueReturnsNoRows(t *testing.T) {
	repo := newTestRepo(t)
	if _, err := repo.ClaimNextEligibleRun(context.Background()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

// TestClaimNextEligibleRun_ConcurrentClaimsRespectOnePerAgent is the
// reservation guard under real contention: three agents with two queued
// runs each must yield exactly three claims, one per agent, no run
// handed out twice. Sequential calls cannot distinguish a count guard
// of "= 0" from one that accepts any count; concurrent ones do.
func TestClaimNextEligibleRun_ConcurrentClaimsRespectOnePerAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	agents := []string{"a1", "a2", "a3"}
	for i, agent := range agents {
		for run := range 2 {
			queueRunAt(t, repo, agent+"-"+string(rune('0'+run)), agent,
				base.Add(time.Duration(i*2+run)*time.Minute))
		}
	}

	const claimers = 10
	ids, noRows := runConcurrentClaims(t, claimers, func() (*models.Run, error) {
		return repo.ClaimNextEligibleRun(ctx)
	})

	if len(ids) != len(agents) {
		t.Errorf("%d successful claims, want %d (one per agent)", len(ids), len(agents))
	}
	if noRows != claimers-len(agents) {
		t.Errorf("%d sql.ErrNoRows results, want %d", noRows, claimers-len(agents))
	}
	assertNoDuplicates(t, ids)
	assertClaimedCount(t, repo, len(agents))
	assertOneClaimPerAgent(t, repo)
}

// runConcurrentClaims fires n goroutines at claim, released together by
// closing a start channel, and returns the claimed IDs plus the number
// of sql.ErrNoRows results. Any other error fails the test.
func runConcurrentClaims(
	t *testing.T, n int, claim func() (*models.Run, error),
) ([]string, int) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan *models.Run, n)
	errs := make(chan error, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			<-start
			run, err := claim()
			if err != nil {
				errs <- err
				return
			}
			results <- run
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	var ids []string
	for run := range results {
		ids = append(ids, run.ID)
	}
	noRows := 0
	for err := range errs {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("unexpected claim error: %v", err)
			continue
		}
		noRows++
	}
	return ids, noRows
}

func assertNoDuplicates(t *testing.T, ids []string) {
	t.Helper()
	seen := make(map[string]int, len(ids))
	for _, id := range ids {
		seen[id]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("run %q was claimed %d times, want exactly 1", id, count)
		}
	}
}

func assertClaimedCount(t *testing.T, repo *runssqlite.Repository, want int) {
	t.Helper()
	var got int
	if err := repo.Reader().Get(&got, `SELECT COUNT(*) FROM runs WHERE status = 'claimed'`); err != nil {
		t.Fatalf("count claimed rows: %v", err)
	}
	if got != want {
		t.Errorf("%d rows in status=claimed, want %d", got, want)
	}
}

func assertOneClaimPerAgent(t *testing.T, repo *runssqlite.Repository) {
	t.Helper()
	type agentCount struct {
		AgentProfileID string `db:"agent_profile_id"`
		Claimed        int    `db:"claimed"`
	}
	var rows []agentCount
	if err := sqlx.Select(repo.Reader(), &rows, `
		SELECT agent_profile_id, COUNT(*) AS claimed
		FROM runs WHERE status = 'claimed'
		GROUP BY agent_profile_id
	`); err != nil {
		t.Fatalf("group claimed rows: %v", err)
	}
	for _, row := range rows {
		if row.Claimed != 1 {
			t.Errorf("agent %q holds %d claimed runs, want 1", row.AgentProfileID, row.Claimed)
		}
	}
}
