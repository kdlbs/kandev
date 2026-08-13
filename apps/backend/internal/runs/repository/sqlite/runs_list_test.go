package sqlite_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// runIDs projects a run slice down to its IDs so ordering assertions
// read as a single comparison.
func runIDs(runs []*models.Run) []string {
	out := make([]string, 0, len(runs))
	for _, run := range runs {
		out = append(out, run.ID)
	}
	return out
}

func checkIDOrder(t *testing.T, label string, got []*models.Run, want ...string) {
	t.Helper()
	gotIDs := runIDs(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("%s = %v, want %v", label, gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, gotIDs, want)
		}
	}
}

// TestListRuns_ScopesByWorkspaceAndOrdersNewestFirst pins the join and
// the ordering: only runs whose agent profile belongs to the workspace,
// newest first.
func TestListRuns_ScopesByWorkspaceAndOrdersNewestFirst(t *testing.T) {
	repo, writer := newTestRepoWithDB(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	seedAgentProfile(t, writer, "a1", "ws-1")
	seedAgentProfile(t, writer, "a2", "ws-1")
	seedAgentProfile(t, writer, "a3", "ws-2")

	queueRunAt(t, repo, "ws1-oldest", "a1", base)
	queueRunAt(t, repo, "ws1-newest", "a2", base.Add(2*time.Minute))
	queueRunAt(t, repo, "ws1-middle", "a1", base.Add(time.Minute))
	queueRunAt(t, repo, "ws2-only", "a3", base.Add(3*time.Minute))
	// A run whose agent profile no longer exists must not surface.
	queueRunAt(t, repo, "orphan", "gone", base.Add(4*time.Minute))

	got, err := repo.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	checkIDOrder(t, "ws-1 runs", got, "ws1-newest", "ws1-middle", "ws1-oldest")

	got, err = repo.ListRuns(ctx, "ws-2")
	if err != nil {
		t.Fatalf("list runs ws-2: %v", err)
	}
	checkIDOrder(t, "ws-2 runs", got, "ws2-only")
}

// TestListRuns_UnknownWorkspaceReturnsEmptyNonNilSlice pins the
// degenerate case handlers range over without a nil check.
func TestListRuns_UnknownWorkspaceReturnsEmptyNonNilSlice(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.ListRuns(context.Background(), "ws-none")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil slice, want an empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no runs", runIDs(got))
	}
}

// TestListRunsForAgentPaged_FirstPageIsNewestFirstAndScopedToTheAgent
// pins ordering, the limit, and the agent filter.
func TestListRunsForAgentPaged_FirstPageIsNewestFirstAndScopedToTheAgent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	queueRunAt(t, repo, "r1", "a1", base)
	queueRunAt(t, repo, "r2", "a1", base.Add(time.Minute))
	queueRunAt(t, repo, "r3", "a1", base.Add(2*time.Minute))
	queueRunAt(t, repo, "other", "a2", base.Add(3*time.Minute))

	got, err := repo.ListRunsForAgentPaged(ctx, "a1", time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	checkIDOrder(t, "first page", got, "r3", "r2")
}

// TestListRunsForAgentPaged_CursorWalksTheWholeQueueExactlyOnce follows
// the cursor contract the handler implements: feed the last row's
// (requested_at, id) back in and get the strictly-next page.
func TestListRunsForAgentPaged_CursorWalksTheWholeQueueExactlyOnce(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	for i := range 5 {
		queueRunAt(t, repo, "r"+string(rune('0'+i)), "a1", base.Add(time.Duration(i)*time.Minute))
	}

	var seen []string
	cursor := time.Time{}
	cursorID := ""
	for range 3 {
		page, err := repo.ListRunsForAgentPaged(ctx, "a1", cursor, cursorID, 2)
		if err != nil {
			t.Fatalf("page: %v", err)
		}
		if len(page) == 0 {
			break
		}
		seen = append(seen, runIDs(page)...)
		last := page[len(page)-1]
		cursor, cursorID = last.RequestedAt, last.ID
	}

	want := []string{"r4", "r3", "r2", "r1", "r0"}
	if len(seen) != len(want) {
		t.Fatalf("paged through %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("paged through %v, want %v", seen, want)
		}
	}
}

// TestListRunsForAgentPaged_TieBreaksOnIDWhenTimestampsMatch pins the
// secondary sort: without it, rows sharing a requested_at could repeat
// or vanish across pages.
func TestListRunsForAgentPaged_TieBreaksOnIDWhenTimestampsMatch(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	same := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	queueRunAt(t, repo, "run-a", "a1", same)
	queueRunAt(t, repo, "run-b", "a1", same)
	queueRunAt(t, repo, "run-c", "a1", same)

	first, err := repo.ListRunsForAgentPaged(ctx, "a1", time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	checkIDOrder(t, "first page", first, "run-c", "run-b")

	last := first[len(first)-1]
	next, err := repo.ListRunsForAgentPaged(ctx, "a1", last.RequestedAt, last.ID, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	checkIDOrder(t, "second page", next, "run-a")
}

// TestListRunsForAgentPaged_NonPositiveLimitFallsBackTo25 pins the
// default page size.
func TestListRunsForAgentPaged_NonPositiveLimitFallsBackTo25(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	const seeded = 27
	for i := range seeded {
		queueRunAt(t, repo, "r"+padded(i), "a1", base.Add(time.Duration(i)*time.Minute))
	}

	for _, limit := range []int{0, -5} {
		got, err := repo.ListRunsForAgentPaged(ctx, "a1", time.Time{}, "", limit)
		if err != nil {
			t.Fatalf("list with limit %d: %v", limit, err)
		}
		if len(got) != 25 {
			t.Errorf("limit %d returned %d rows, want the 25-row default", limit, len(got))
		}
		if len(got) > 0 && got[0].ID != "r"+padded(seeded-1) {
			t.Errorf("limit %d returned %q first, want the newest run", limit, got[0].ID)
		}
	}
}

// TestListRunsForAgentPaged_UnknownAgentReturnsEmptyNonNilSlice pins the
// degenerate case.
func TestListRunsForAgentPaged_UnknownAgentReturnsEmptyNonNilSlice(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.ListRunsForAgentPaged(context.Background(), "nobody", time.Time{}, "", 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got %v, want an empty non-nil slice", runIDs(got))
	}
}

// TestListPendingRunsForTask_OnlyQueuedRetriesForThatTask pins all
// three predicates: queued, a scheduled retry, and the task id inside
// the payload.
func TestListPendingRunsForTask_OnlyQueuedRetriesForThatTask(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	now := time.Now().UTC()

	pending := seedTaskRun(t, repo, "pending", "a1", "t1", "queued")
	if err := repo.ScheduleRetry(ctx, pending.ID, now.Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	seedTaskRun(t, repo, "queued-no-retry", "a1", "t1", "queued")
	claimedRetry := seedTaskRun(t, repo, "claimed-retry", "a1", "t1", "queued")
	if err := repo.ScheduleRetry(ctx, claimedRetry.ID, now.Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule retry (claimed): %v", err)
	}
	setStatus(t, repo, claimedRetry.ID, "claimed", timePtr(now), nil)
	otherTask := seedTaskRun(t, repo, "other-task", "a1", "t2", "queued")
	if err := repo.ScheduleRetry(ctx, otherTask.ID, now.Add(time.Hour), 1); err != nil {
		t.Fatalf("schedule retry (other task): %v", err)
	}

	got, err := repo.ListPendingRunsForTask(ctx, "t1")
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	checkIDOrder(t, "pending runs for t1", got, "pending")

	empty, err := repo.ListPendingRunsForTask(ctx, "t-unknown")
	if err != nil {
		t.Fatalf("list pending (unknown task): %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("got %v, want an empty non-nil slice", runIDs(empty))
	}
}

// seedTaskRun creates a run carrying a task id in its payload.
func seedTaskRun(
	t *testing.T, repo *runssqlite.Repository, id, agentID, taskID, status string,
) *models.Run {
	t.Helper()
	return mustCreateRun(t, repo, &models.Run{
		ID:             id,
		AgentProfileID: agentID,
		Reason:         "task_assigned",
		Payload:        `{"task_id":"` + taskID + `"}`,
		Status:         models.RunStatus(status),
		CoalescedCount: 1,
	})
}

// padded keeps seeded run IDs sortable as strings so an ID tie-break
// cannot masquerade as a timestamp ordering.
func padded(i int) string {
	return fmt.Sprintf("%02d", i)
}
