package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedFailedInboxTask creates a task in the given state/workspace/origin,
// satisfying the failed-task predicate's own fields
// (AC-UI-INBOX-FAILED-001.6, .7, .9a).
func seedFailedInboxTask(t *testing.T, repo *Repository, id, workspaceID string, state v1.TaskState, origin string) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: id, WorkspaceID: workspaceID, Title: "task " + id, State: state, Origin: origin,
	}); err != nil {
		t.Fatalf("create task %s: %v", id, err)
	}
}

type failedInboxSessionSeed struct {
	ID           string
	TaskID       string
	IsPrimary    bool
	StartedAt    time.Time
	CompletedAt  *time.Time
	ErrorMessage string
}

func seedFailedInboxSession(t *testing.T, repo *Repository, s failedInboxSessionSeed) {
	t.Helper()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID:           s.ID,
		TaskID:       s.TaskID,
		IsPrimary:    s.IsPrimary,
		StartedAt:    s.StartedAt,
		CompletedAt:  s.CompletedAt,
		ErrorMessage: s.ErrorMessage,
		State:        models.TaskSessionStateFailed,
	}); err != nil {
		t.Fatalf("create session %s: %v", s.ID, err)
	}
}

func mustTime(offsetSeconds int) time.Time {
	return time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC).Add(time.Duration(offsetSeconds) * time.Second)
}

func timePtr(tm time.Time) *time.Time { return &tm }

func TestListFailedInboxTasks_OnlyTaskStateFailedListed(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-failed", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxTask(t, repo, "task-running", "ws1", v1.TaskStateInProgress, "")
	// A task whose primary session is failed but whose own state is not
	// terminal-failed must list zero rows for it (AC .6).
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-running", TaskID: "task-running", IsPrimary: true,
		StartedAt: mustTime(0), CompletedAt: timePtr(mustTime(10)),
	})

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].TaskID != "task-failed" {
		t.Fatalf("expected exactly [task-failed], got %+v", page.Rows)
	}
}

func TestListFailedInboxTasks_ArchivedTaskNeverListed(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-archived", "ws1", v1.TaskStateFailed, "")
	if err := repo.ArchiveTask(ctx, "task-archived"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 0 {
		t.Fatalf("expected no rows for an archived task, got %+v", page.Rows)
	}
}

func TestListFailedInboxTasks_WorkspaceScoped(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-ws1", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxTask(t, repo, "task-ws2", "ws2", v1.TaskStateFailed, "")

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].TaskID != "task-ws1" {
		t.Fatalf("expected exactly [task-ws1], got %+v", page.Rows)
	}
}

func TestListFailedInboxTasks_SessionResolutionPrefersPrimaryThenMostRecent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-1", "ws1", v1.TaskStateFailed, "")
	// Two non-primary sessions, one primary: primary must win regardless of
	// recency (AC .30).
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-old", TaskID: "task-1", IsPrimary: false,
		StartedAt: mustTime(0), CompletedAt: timePtr(mustTime(5)), ErrorMessage: "old non-primary",
	})
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-primary", TaskID: "task-1", IsPrimary: true,
		StartedAt: mustTime(10), CompletedAt: timePtr(mustTime(15)), ErrorMessage: "primary reason",
	})
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-newer-non-primary", TaskID: "task-1", IsPrimary: false,
		StartedAt: mustTime(20), CompletedAt: timePtr(mustTime(25)), ErrorMessage: "newer non-primary",
	})

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("expected 1 row, got %+v", page.Rows)
	}
	if page.Rows[0].Reason != "primary reason" {
		t.Fatalf("expected primary session's reason, got %q", page.Rows[0].Reason)
	}
}

func TestListFailedInboxTasks_SessionResolutionMostRecentWhenNonePrimary(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-1", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-old", TaskID: "task-1", IsPrimary: false,
		StartedAt: mustTime(0), CompletedAt: timePtr(mustTime(5)), ErrorMessage: "old",
	})
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-new", TaskID: "task-1", IsPrimary: false,
		StartedAt: mustTime(10), CompletedAt: timePtr(mustTime(15)), ErrorMessage: "new",
	})

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Reason != "new" {
		t.Fatalf("expected the most recently started session's reason, got %+v", page.Rows)
	}
}

func TestListFailedInboxTasks_SessionResolutionTieBreaksByLowestSessionID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-1", "ws1", v1.TaskStateFailed, "")
	tie := mustTime(10)
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-b", TaskID: "task-1", IsPrimary: false,
		StartedAt: tie, CompletedAt: timePtr(mustTime(15)), ErrorMessage: "session b",
	})
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-a", TaskID: "task-1", IsPrimary: false,
		StartedAt: tie, CompletedAt: timePtr(mustTime(20)), ErrorMessage: "session a",
	})

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Reason != "session a" {
		t.Fatalf("expected the tie broken toward the lower session id (sess-a), got %+v", page.Rows)
	}
}

func TestListFailedInboxTasks_NoSessionLeavesBothFieldsUnresolvable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-no-session", "ws1", v1.TaskStateFailed, "")

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 {
		t.Fatalf("a failed task with no session must still be listed, got %+v", page.Rows)
	}
	row := page.Rows[0]
	if row.FailureInstant != nil {
		t.Errorf("expected unresolvable failure instant, got %v", row.FailureInstant)
	}
	if row.Reason != "" {
		t.Errorf("expected unresolvable reason, got %q", row.Reason)
	}
}

func TestListFailedInboxTasks_SessionWithNoCompletionInstantIsUnresolvable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-1", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-1", TaskID: "task-1", IsPrimary: true,
		StartedAt: mustTime(0), CompletedAt: nil, ErrorMessage: "boom",
	})

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].FailureInstant != nil {
		t.Fatalf("expected unresolvable failure instant, got %+v", page.Rows)
	}
}

func TestListFailedInboxTasks_OrderingUnresolvableFirstThenInstantDescThenTaskIDAsc(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// task-late failed most recently among resolved rows -> first of the
	// resolved group.
	seedFailedInboxTask(t, repo, "task-late", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-late", TaskID: "task-late", IsPrimary: true,
		StartedAt: mustTime(0), CompletedAt: timePtr(mustTime(100)),
	})

	seedFailedInboxTask(t, repo, "task-early", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxSession(t, repo, failedInboxSessionSeed{
		ID: "sess-early", TaskID: "task-early", IsPrimary: true,
		StartedAt: mustTime(0), CompletedAt: timePtr(mustTime(10)),
	})

	// task-unresolvable-b and task-unresolvable-a tie (both unresolvable) and
	// must sort by task id ascending, ahead of every resolved row.
	seedFailedInboxTask(t, repo, "task-unresolvable-b", "ws1", v1.TaskStateFailed, "")
	seedFailedInboxTask(t, repo, "task-unresolvable-a", "ws1", v1.TaskStateFailed, "")

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	got := make([]string, len(page.Rows))
	for i, row := range page.Rows {
		got[i] = row.TaskID
	}
	want := []string{"task-unresolvable-a", "task-unresolvable-b", "task-late", "task-early"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}

func TestListFailedInboxTasks_Pagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		seedFailedInboxTask(t, repo, "task-page-"+string(rune('a'+i)), "ws1", v1.TaskStateFailed, "")
	}

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 2})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 2 {
		t.Fatalf("expected exactly 2 rows (bound), got %d", len(page.Rows))
	}
	if !page.HasMore {
		t.Errorf("expected HasMore=true with 3 rows over a limit of 2")
	}

	full, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 3})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if full.HasMore {
		t.Errorf("expected HasMore=false when the bound exactly covers every row")
	}
}

func TestListFailedInboxTasks_RejectsSubOneLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	if _, err := repo.ListFailedInboxTasks(context.Background(), models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 0}); err == nil {
		t.Fatal("expected an error for Limit < 1")
	}
}

func TestListFailedInboxTasks_OriginCarriedThrough(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	seedFailedInboxTask(t, repo, "task-1", "ws1", v1.TaskStateFailed, models.TaskOriginAutomationRun)

	page, err := repo.ListFailedInboxTasks(ctx, models.ListFailedInboxOptions{WorkspaceID: "ws1", Limit: 50})
	if err != nil {
		t.Fatalf("ListFailedInboxTasks: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].Origin != models.TaskOriginAutomationRun {
		t.Fatalf("expected origin %q, got %+v", models.TaskOriginAutomationRun, page.Rows)
	}
}
