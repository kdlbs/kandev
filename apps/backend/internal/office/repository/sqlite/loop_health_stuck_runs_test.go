package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func TestListStuckRuns_ClaimedStuckReportedRegardlessOfAge(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	// Claimed a very long time ago — the predicate is not windowed
	// (AC-004.5): a run stuck longer than the evaluation window is
	// still reported.
	longAgo := now.Add(-72 * time.Hour)
	if _, err := db.Exec(`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`, longAgo, run.ID); err != nil {
		t.Fatalf("seed claimed state: %v", err)
	}

	rows, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1", len(rows), total)
	}
	if rows[0].Condition != "claimed_stuck" {
		t.Errorf("condition = %q, want claimed_stuck", rows[0].Condition)
	}
}

func TestListStuckRuns_QueuedStuckPositiveControl(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	old := now.Add(-10 * time.Minute)
	if _, err := db.Exec(`UPDATE runs SET requested_at = ? WHERE id = ?`, old, run.ID); err != nil {
		t.Fatalf("backdate requested_at: %v", err)
	}

	rows, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1 (positive control must be reported)", len(rows), total)
	}
	if rows[0].Condition != "queued_stuck" {
		t.Errorf("condition = %q, want queued_stuck", rows[0].Condition)
	}
}

// AC-004.17 exclusion 1: a future scheduled_retry_at is a deferral,
// not a stall, even though requested_at is old — the COALESCE handles
// this without a separate branch.
func TestListStuckRuns_ExcludesFutureScheduledRetry(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	old := now.Add(-10 * time.Minute)
	future := now.Add(5 * time.Minute)
	if _, err := db.Exec(
		`UPDATE runs SET requested_at = ?, scheduled_retry_at = ? WHERE id = ?`,
		old, future, run.ID,
	); err != nil {
		t.Fatalf("seed future retry: %v", err)
	}

	_, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 (future scheduled_retry_at is a deferral)", total)
	}
}

// AC-004.17 exclusion 2: a non-NULL routing_blocked_status is a parked
// run — parked for a human, not stuck.
func TestListStuckRuns_ExcludesRoutingBlockedRun(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	old := now.Add(-10 * time.Minute)
	if _, err := db.Exec(
		`UPDATE runs SET requested_at = ?, routing_blocked_status = 'waiting_for_provider_capacity' WHERE id = ?`,
		old, run.ID,
	); err != nil {
		t.Fatalf("seed routing blocked: %v", err)
	}

	_, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 (routing-blocked run is parked, not stuck)", total)
	}
}

// AC-004.17 exclusion 3: a claimed sibling on the same agent_profile_id
// means the queue is correctly holding this run back — a busy agent's
// steady state, not a stall.
func TestListStuckRuns_ExcludesRunWithClaimedSibling(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	sibling := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, sibling); err != nil {
		t.Fatalf("create sibling: %v", err)
	}
	if _, err := db.Exec(`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`, now, sibling.ID); err != nil {
		t.Fatalf("claim sibling: %v", err)
	}

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	old := now.Add(-10 * time.Minute)
	if _, err := db.Exec(`UPDATE runs SET requested_at = ? WHERE id = ?`, old, run.ID); err != nil {
		t.Fatalf("backdate requested_at: %v", err)
	}

	rows, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// The claimed sibling itself is not "stuck" either (claimed_at is
	// fresh), so the workspace should report nothing at all.
	if total != 0 || len(rows) != 0 {
		t.Fatalf("got %d rows, total %d, want 0/0 (busy agent is not a stall)", len(rows), total)
	}
}

// AC-004.17 aging: a run whose scheduled_retry_at has JUST passed is
// aged from that instant, not from requested_at.
func TestListStuckRuns_AgesFromScheduledRetryAtOncePassed(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	// requested_at is recent (would not itself be stuck), but
	// scheduled_retry_at passed well over the grace period ago.
	recent := now.Add(-30 * time.Second)
	pastRetry := now.Add(-3 * time.Minute)
	if _, err := db.Exec(
		`UPDATE runs SET requested_at = ?, scheduled_retry_at = ? WHERE id = ?`,
		recent, pastRetry, run.ID,
	); err != nil {
		t.Fatalf("seed past retry: %v", err)
	}

	rows, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1 (aged from scheduled_retry_at)", len(rows), total)
	}
}

// AC-004.17 (F29/F30 accepted limitation): a queued run that has
// dispatched at least once (current_route_attempt_seq > 0) reads as
// silent rather than stuck, mirroring ClaimNextEligibleRun's own gap
// rather than fixing it here.
func TestListStuckRuns_ExcludesRunWithNonZeroRouteAttemptSeq(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	run := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	old := now.Add(-10 * time.Minute)
	if _, err := db.Exec(
		`UPDATE runs SET requested_at = ?, current_route_attempt_seq = 1 WHERE id = ?`,
		old, run.ID,
	); err != nil {
		t.Fatalf("seed routed attempt: %v", err)
	}

	_, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 (accepted limitation: a dispatched-once run reads as silent)", total)
	}
}

// Ordering: a claimed-stuck run outranks an older queued-stuck one,
// and a NULL claimed_at row sorts first among claimed rows.
func TestListStuckRuns_ClaimedOutranksOlderQueuedAndNullClaimedAtSortsFirst(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")
	seedAgentProfile(t, db, "agent-b", "ws-a")
	seedAgentProfile(t, db, "agent-c", "ws-a")

	// Oldest queued-stuck run.
	queued := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, queued); err != nil {
		t.Fatalf("create queued: %v", err)
	}
	veryOld := now.Add(-60 * time.Minute)
	if _, err := db.Exec(`UPDATE runs SET requested_at = ? WHERE id = ?`, veryOld, queued.ID); err != nil {
		t.Fatalf("backdate queued: %v", err)
	}

	// Claimed-stuck run with a recorded claimed_at, younger than the
	// queued one but still past the claimed grace.
	claimedWithAge := &models.Run{AgentProfileID: "agent-b", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, claimedWithAge); err != nil {
		t.Fatalf("create claimed: %v", err)
	}
	claimedAt := now.Add(-15 * time.Minute)
	if _, err := db.Exec(`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`, claimedAt, claimedWithAge.ID); err != nil {
		t.Fatalf("claim: %v", err)
	}

	// Claimed-stuck run with NULL claimed_at.
	claimedNull := &models.Run{AgentProfileID: "agent-c", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, claimedNull); err != nil {
		t.Fatalf("create claimed-null: %v", err)
	}
	if _, err := db.Exec(`UPDATE runs SET status = 'claimed', claimed_at = NULL WHERE id = ?`, claimedNull.ID); err != nil {
		t.Fatalf("claim with null claimed_at: %v", err)
	}

	rows, total, err := repo.ListStuckRuns(ctx, "ws-a", now, 2*time.Minute, 10*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("got %d rows, total %d, want 3/3", len(rows), total)
	}
	if rows[0].RunID != claimedNull.ID {
		t.Errorf("rows[0] = %s, want the NULL-claimed_at row first", rows[0].RunID)
	}
	if rows[1].RunID != claimedWithAge.ID {
		t.Errorf("rows[1] = %s, want the aged claimed row second", rows[1].RunID)
	}
	if rows[2].RunID != queued.ID {
		t.Errorf("rows[2] = %s, want the queued row last despite being oldest", rows[2].RunID)
	}
}
