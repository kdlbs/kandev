package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// commentRun seeds a run for the comment-status lookup with an explicit
// idempotency key (pass nil for a key-less run) and a fixed
// requested_at so "most recent wins" is deterministic.
func commentRun(
	t *testing.T, repo *runssqlite.Repository,
	id, reason, payload string, key *string, requestedAt time.Time,
) *models.Run {
	t.Helper()
	run := mustCreateRun(t, repo, &models.Run{
		ID:             id,
		AgentProfileID: "a1",
		Reason:         reason,
		Payload:        payload,
		Status:         "queued",
		CoalescedCount: 1,
		IdempotencyKey: key,
	})
	setRequestedAt(t, repo, run.ID, requestedAt)
	return run
}

// TestGetRunsByCommentIDs_EmptyInputReturnsEmptyMap pins the degenerate
// case callers range over without a nil check.
func TestGetRunsByCommentIDs_EmptyInputReturnsEmptyMap(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.GetRunsByCommentIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil map, want an empty non-nil map")
	}
	if len(got) != 0 {
		t.Errorf("got %v, want an empty map", got)
	}
}

// TestGetRunsByCommentIDs_MapsCanonicalKeysAndCarriesStatus covers the
// same-task wake: the canonical idempotency key resolves the comment,
// and the badge fields (run id, status, error message) come back intact.
func TestGetRunsByCommentIDs_MapsCanonicalKeysAndCarriesStatus(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	runA := commentRun(t, repo, "run-a", "task_comment",
		`{"task_id":"t1","comment_id":"cm-A"}`, strPtr("task_comment:cm-A"), base)
	runB := commentRun(t, repo, "run-b", "task_comment",
		`{"task_id":"t1","comment_id":"cm-B"}`, strPtr("task_comment:cm-B"), base)
	setStatus(t, repo, runB.ID, "failed", nil, timePtr(base))
	if err := repo.SetRunErrorMessageForTest(ctx, runB.ID, "agent exploded"); err != nil {
		t.Fatalf("set error message: %v", err)
	}

	got, err := repo.GetRunsByCommentIDs(ctx, []string{"cm-A", "cm-B", "cm-none"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2 (cm-none has no run): %+v", len(got), got)
	}
	checkCommentStatus(t, got, "cm-A", runssqlite.CommentRunStatus{
		RunID: runA.ID, Status: "queued", ErrorMessage: "",
	})
	checkCommentStatus(t, got, "cm-B", runssqlite.CommentRunStatus{
		RunID: runB.ID, Status: "failed", ErrorMessage: "agent exploded",
	})
	if _, present := got["cm-none"]; present {
		t.Errorf("cm-none is present (%+v), want absent", got["cm-none"])
	}
}

// TestGetRunsByCommentIDs_SaltedKeysResolveThroughThePayload covers the
// cross-task fan-out: the key carries salt after the comment id, so the
// persisted payload.comment_id is what links the run back.
func TestGetRunsByCommentIDs_SaltedKeysResolveThroughThePayload(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	salted := commentRun(t, repo, "salted", "task_comment",
		`{"task_id":"target","source_task_id":"source","comment_id":"cm-X"}`,
		strPtr("task_comment:cm-X:work:target:a1:abcd1234"), base)
	keyless := commentRun(t, repo, "keyless", "task_comment",
		`{"task_id":"target","comment_id":"cm-Y"}`, nil, base)

	got, err := repo.GetRunsByCommentIDs(ctx, []string{"cm-X", "cm-Y"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	checkCommentStatus(t, got, "cm-X", runssqlite.CommentRunStatus{RunID: salted.ID, Status: "queued"})
	checkCommentStatus(t, got, "cm-Y", runssqlite.CommentRunStatus{RunID: keyless.ID, Status: "queued"})
}

// TestGetRunsByCommentIDs_MostRecentRunWinsPerComment pins the
// ordering: with a canonical run and a later salted run for the same
// comment, the badge must reflect the newer one.
func TestGetRunsByCommentIDs_MostRecentRunWinsPerComment(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	older := commentRun(t, repo, "older", "task_comment",
		`{"task_id":"t1","comment_id":"cm-A"}`, strPtr("task_comment:cm-A"), base)
	setStatus(t, repo, older.ID, "finished", nil, timePtr(base))
	newer := commentRun(t, repo, "newer", "task_comment",
		`{"task_id":"t2","comment_id":"cm-A"}`,
		strPtr("task_comment:cm-A:work:t2:a1:beef"), base.Add(time.Hour))

	got, err := repo.GetRunsByCommentIDs(ctx, []string{"cm-A"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(got), got)
	}
	checkCommentStatus(t, got, "cm-A", runssqlite.CommentRunStatus{RunID: newer.ID, Status: "queued"})
}

// TestGetRunsByCommentIDs_IgnoresUnrelatedRuns pins the filters: a run
// for another comment, and a key-less run whose reason is not
// task_comment, must both stay out of the map.
func TestGetRunsByCommentIDs_IgnoresUnrelatedRuns(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	commentRun(t, repo, "other-comment", "task_comment",
		`{"task_id":"t1","comment_id":"cm-other"}`, strPtr("task_comment:cm-other"), base)
	commentRun(t, repo, "wrong-reason", "task_assigned",
		`{"task_id":"t1","comment_id":"cm-A"}`, nil, base.Add(time.Hour))
	wanted := commentRun(t, repo, "wanted", "task_comment",
		`{"task_id":"t1","comment_id":"cm-A"}`, strPtr("task_comment:cm-A"), base)

	got, err := repo.GetRunsByCommentIDs(ctx, []string{"cm-A"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1: %+v", len(got), got)
	}
	checkCommentStatus(t, got, "cm-A", runssqlite.CommentRunStatus{RunID: wanted.ID, Status: "queued"})
}

// TestGetRunsByCommentIDs_CanonicalKeyResolvesUnusablePayloads covers
// the payload-parsing fallbacks: a payload with no comment_id, or with
// a non-string one, must not stop the canonical key from resolving.
// (A syntactically malformed payload cannot be seeded — the runs table
// carries a partial index over json_extract(payload, '$.comment_id'),
// so SQLite rejects the INSERT outright.)
func TestGetRunsByCommentIDs_CanonicalKeyResolvesUnusablePayloads(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		commentID string
		payload   string
	}{
		{"cm-numeric", `{"comment_id":42}`},
		{"cm-absent", `{"task_id":"t1"}`},
		// The payload names a comment that was not asked about: the
		// canonical key still decides which comment this run belongs to.
		{"cm-mismatched", `{"comment_id":"cm-not-requested"}`},
	}
	wantIDs := make(map[string]string, len(cases))
	for i, tc := range cases {
		run := commentRun(t, repo, "run-"+tc.commentID, "task_comment", tc.payload,
			strPtr("task_comment:"+tc.commentID), base.Add(time.Duration(i)*time.Minute))
		wantIDs[tc.commentID] = run.ID
	}

	ids := make([]string, 0, len(cases))
	for _, tc := range cases {
		ids = append(ids, tc.commentID)
	}
	got, err := repo.GetRunsByCommentIDs(ctx, ids)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != len(cases) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(cases), got)
	}
	for commentID, runID := range wantIDs {
		checkCommentStatus(t, got, commentID, runssqlite.CommentRunStatus{RunID: runID, Status: "queued"})
	}
}

func checkCommentStatus(
	t *testing.T,
	got map[string]runssqlite.CommentRunStatus,
	commentID string,
	want runssqlite.CommentRunStatus,
) {
	t.Helper()
	status, ok := got[commentID]
	if !ok {
		t.Fatalf("comment %q missing from the map: %+v", commentID, got)
	}
	if status != want {
		t.Errorf("comment %q = %+v, want %+v", commentID, status, want)
	}
}
