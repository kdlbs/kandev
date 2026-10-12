package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func seedFinishedRun(
	t *testing.T, repo *sqlite.Repository, db *sqlx.DB,
	agentID, outcome, sessionID string, requestedAt time.Time, finishedAt *time.Time,
) string {
	t.Helper()
	ctx := context.Background()
	run := &models.Run{
		AgentProfileID: agentID, Reason: "task_assigned", Payload: "{}",
		Status: "queued", CoalescedCount: 1, SessionID: sessionID,
	}
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE runs SET status = 'finished', outcome = ?, requested_at = ?, finished_at = ? WHERE id = ?`,
		outcome, requestedAt, finishedAt, run.ID,
	); err != nil {
		t.Fatalf("finish run: %v", err)
	}
	return run.ID
}

func TestListSilentSuccesses_ReturnsFinishedProcessedSessionlessRun(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	finishedAt := now.Add(-time.Hour)
	seedFinishedRun(t, repo, db, "agent-a", "processed", "", now.Add(-2*time.Hour), &finishedAt)

	windowStart := now.Add(-24 * time.Hour)
	activationAt := now.Add(-48 * time.Hour)
	rows, total, err := repo.ListSilentSuccesses(context.Background(), "ws-a", windowStart, activationAt, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1", len(rows), total)
	}
}

// AC-005.6, applied to the AC-004.5/.7 evidence list: a legacy run
// requested before the activation instant is pre_activation, never a
// silent success, even when it is otherwise indistinguishable from one
// and falls inside the evaluation window.
func TestListSilentSuccesses_PreActivationRunExcluded(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	finishedAt := now.Add(-time.Hour)
	seedFinishedRun(t, repo, db, "agent-a", "processed", "", now.Add(-2*time.Hour), &finishedAt)

	windowStart := now.Add(-24 * time.Hour)
	activationAt := now.Add(-time.Minute) // activated after this run was requested
	_, total, err := repo.ListSilentSuccesses(context.Background(), "ws-a", windowStart, activationAt, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 (run predates activation, must not read as silent success)", total)
	}
}

// AC-004.16: a terminal run with a NULL finished_at still counts
// within the window, aged from requested_at via COALESCE.
func TestListSilentSuccesses_NullFinishedAtStillInsideWindow(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	seedFinishedRun(t, repo, db, "agent-a", "processed", "", now.Add(-time.Hour), nil)

	windowStart := now.Add(-24 * time.Hour)
	activationAt := now.Add(-48 * time.Hour)
	rows, total, err := repo.ListSilentSuccesses(context.Background(), "ws-a", windowStart, activationAt, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1 (NULL finished_at falls back to requested_at)", len(rows), total)
	}
	if rows[0].FinishedAt != nil {
		t.Errorf("FinishedAt = %v, want nil", rows[0].FinishedAt)
	}
}

func TestListSilentSuccesses_LaunchedRunIsNotSilent(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	finishedAt := now.Add(-time.Hour)
	seedFinishedRun(t, repo, db, "agent-a", "processed", "session-1", now.Add(-2*time.Hour), &finishedAt)

	windowStart := now.Add(-24 * time.Hour)
	activationAt := now.Add(-48 * time.Hour)
	_, total, err := repo.ListSilentSuccesses(context.Background(), "ws-a", windowStart, activationAt, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 (a run with a session id is not silent)", total)
	}
}

func TestListSilentSuccesses_OutsideWindowExcluded(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	finishedAt := now.Add(-30 * time.Hour)
	seedFinishedRun(t, repo, db, "agent-a", "processed", "", now.Add(-31*time.Hour), &finishedAt)

	windowStart := now.Add(-24 * time.Hour)
	activationAt := now.Add(-48 * time.Hour)
	_, total, err := repo.ListSilentSuccesses(context.Background(), "ws-a", windowStart, activationAt, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0 (outside the 24h evaluation window)", total)
	}
}

func TestListTerminalRunsInWindow_ExcludesQueuedAndClaimed(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	seedAgentProfile(t, db, "agent-a", "ws-a")

	queued := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, queued); err != nil {
		t.Fatalf("create queued: %v", err)
	}
	claimed := &models.Run{AgentProfileID: "agent-a", Reason: "task_assigned", Payload: "{}", Status: "queued", CoalescedCount: 1}
	if err := repo.CreateRun(ctx, claimed); err != nil {
		t.Fatalf("create claimed: %v", err)
	}
	if _, err := db.Exec(`UPDATE runs SET status = 'claimed', claimed_at = ? WHERE id = ?`, now, claimed.ID); err != nil {
		t.Fatalf("claim: %v", err)
	}
	finishedAt := now.Add(-time.Hour)
	seedFinishedRun(t, repo, db, "agent-a", "processed", "session-1", now.Add(-2*time.Hour), &finishedAt)

	rows, err := repo.ListTerminalRunsInWindow(ctx, "ws-a", now.Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1 (queued/claimed excluded)", len(rows))
	}
	if rows[0].Status != "finished" {
		t.Errorf("status = %q, want finished", rows[0].Status)
	}
}
