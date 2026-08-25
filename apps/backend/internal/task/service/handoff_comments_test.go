package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
)

// fakeCommentReader is a minimal in-memory CommentReader. Comments for a
// task are stored ascending by insertion order (matching the ordering the
// real repository presents); ListTaskCommentsWindow slices the newest
// `limit` while preserving that ascending order, mirroring the production
// contract without touching SQLite.
type fakeCommentReader struct {
	byTask map[string][]*officemodels.TaskComment
	err    error
}

func (f *fakeCommentReader) ListTaskCommentsWindow(_ context.Context, taskID string, limit int) ([]*officemodels.TaskComment, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	all := f.byTask[taskID]
	total := len(all)
	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	start := total - limit
	if start < 0 {
		start = 0
	}
	window := make([]*officemodels.TaskComment, len(all[start:]))
	copy(window, all[start:])
	return window, total, nil
}

func (f *fakeCommentReader) seed(taskID string, n int, bodyLen int) {
	if f.byTask == nil {
		f.byTask = map[string][]*officemodels.TaskComment{}
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		body := strings.Repeat("x", bodyLen)
		f.byTask[taskID] = append(f.byTask[taskID], &officemodels.TaskComment{
			ID:         "c-" + taskID + "-" + itoa(i),
			TaskID:     taskID,
			AuthorType: "agent",
			AuthorID:   "agent-1",
			Source:     "run",
			Body:       body,
			CreatedAt:  base.Add(time.Duration(i) * time.Minute),
		})
	}
}

func TestListCommentsForCallerEnforcesReadAccess(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{}
	reader.seed("parent", 1, 10)
	reader.seed("stranger", 1, 10)
	svc.SetCommentReader(reader)
	ctx := context.Background()

	if _, err := svc.ListCommentsForCaller(ctx, "child-a", "parent", 20); err != nil {
		t.Fatalf("ancestor read: %v", err)
	}
	if _, err := svc.ListCommentsForCaller(ctx, "child-a", "stranger", 20); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("unrelated read = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.ListCommentsForCaller(ctx, "child-a", "foreign", 20); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("cross-workspace read = %v, want ErrAccessDenied", err)
	}
	// AC-001.5: a nonexistent target reports the identical access-denied
	// error as an unrelated existing task.
	if _, err := svc.ListCommentsForCaller(ctx, "child-a", "does-not-exist", 20); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("nonexistent target = %v, want ErrAccessDenied", err)
	}
}

// AC-005.4/005.6/005.8: an absent, empty, whitespace-only, or "self"
// (after trimming) task_id targets the caller task.
func TestListCommentsForCallerSelfFallback(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{}
	reader.seed("child-a", 2, 10)
	svc.SetCommentReader(reader)
	ctx := context.Background()

	for _, target := range []string{"", "  ", "self"} {
		w, err := svc.ListCommentsForCaller(ctx, "child-a", target, 20)
		if err != nil {
			t.Fatalf("target %q: %v", target, err)
		}
		if w.Total != 2 {
			t.Fatalf("target %q: total = %d, want 2", target, w.Total)
		}
	}
}

// AC-005.5: task_id resolves to empty (absent) AND there is no caller
// identity — a validation error naming task_id, not a plain deny.
func TestListCommentsForCallerRequiresTaskIDWhenNoCaller(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	svc.SetCommentReader(&fakeCommentReader{})
	ctx := context.Background()

	for _, target := range []string{"", "  ", "self", " self "} {
		if _, err := svc.ListCommentsForCaller(ctx, "", target, 20); !errors.Is(err, ErrDocumentTaskRequired) {
			t.Fatalf("caller=%q target=%q: err = %v, want ErrDocumentTaskRequired", "", target, err)
		}
	}
}

// AC-005.1: an accessible target with no comments succeeds with a non-nil
// empty list and total zero.
func TestListCommentsForCallerEmptySuccess(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	svc.SetCommentReader(&fakeCommentReader{})
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "parent", 20)
	if err != nil {
		t.Fatalf("ListCommentsForCaller: %v", err)
	}
	if w.Total != 0 || w.Returned != 0 || w.HasMore {
		t.Fatalf("window = %+v, want empty zero window", w)
	}
	if w.Comments == nil {
		t.Fatal("Comments must be non-nil (empty), got nil")
	}
}

// AC-005.2/005.3: an unconfigured comment store is a distinct internal
// error, never an empty-but-successful result.
func TestListCommentsForCallerDependencyNotConfigured(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "parent", 20)
	if err == nil {
		t.Fatal("want an error when the comment reader is unconfigured")
	}
	if errors.Is(err, ErrAccessDenied) || errors.Is(err, ErrDocumentTaskRequired) {
		t.Fatalf("unconfigured dependency must not reuse the access/validation sentinels, got %v", err)
	}
	if w != nil {
		t.Fatalf("window = %+v, want nil on error", w)
	}
}

// AC-004.1-004.5: bodies over 8192 bytes are truncated at a rune boundary
// and carry body_truncated/body_bytes; bodies at or under the cap are
// returned untouched with neither field set.
func TestListCommentsForCallerTruncatesLongBodies(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{}
	reader.seed("child-a", 1, 9000)
	svc.SetCommentReader(reader)
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "self", 20)
	if err != nil {
		t.Fatalf("ListCommentsForCaller: %v", err)
	}
	if len(w.Comments) != 1 {
		t.Fatalf("len(comments) = %d, want 1", len(w.Comments))
	}
	c := w.Comments[0]
	if !c.BodyTruncated {
		t.Fatal("want body_truncated = true for a 9000-byte body")
	}
	if c.BodyBytes != 9000 {
		t.Fatalf("body_bytes = %d, want 9000", c.BodyBytes)
	}
	if len(c.Body) != 8192 {
		t.Fatalf("truncated body len = %d, want 8192", len(c.Body))
	}
}

func TestListCommentsForCallerLeavesShortBodiesUntouched(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{}
	reader.seed("child-a", 1, 42)
	svc.SetCommentReader(reader)
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "self", 20)
	if err != nil {
		t.Fatalf("ListCommentsForCaller: %v", err)
	}
	c := w.Comments[0]
	if c.BodyTruncated {
		t.Fatal("want body_truncated = false for a 42-byte body")
	}
	if c.BodyBytes != 0 {
		t.Fatalf("body_bytes = %d, want 0 (omitted)", c.BodyBytes)
	}
	if len(c.Body) != 42 {
		t.Fatalf("body len = %d, want 42", len(c.Body))
	}
}

// AC-004.5: a body of exactly 8192 bytes is the boundary case — at the cap,
// not over it, so it must pass through untouched rather than truncate.
func TestListCommentsForCallerLeavesExactCapBodyUntouched(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{}
	reader.seed("child-a", 1, 8192)
	svc.SetCommentReader(reader)
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "self", 20)
	if err != nil {
		t.Fatalf("ListCommentsForCaller: %v", err)
	}
	c := w.Comments[0]
	if c.BodyTruncated {
		t.Fatal("want body_truncated = false for an exactly-8192-byte body")
	}
	if c.BodyBytes != 0 {
		t.Fatalf("body_bytes = %d, want 0 (omitted)", c.BodyBytes)
	}
	if len(c.Body) != 8192 {
		t.Fatalf("body len = %d, want 8192", len(c.Body))
	}
}

// AC-004.6-004.9: the aggregate 65536-byte body budget drops whole
// comments from the oldest end, never emptying a non-empty window.
func TestListCommentsForCallerBudgetDropsOldestNeverEmpties(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{}
	// 10 comments at 8192 bytes each = 81920 bytes, over the 65536 budget.
	reader.seed("child-a", 10, 8192)
	svc.SetCommentReader(reader)
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "self", 20)
	if err != nil {
		t.Fatalf("ListCommentsForCaller: %v", err)
	}
	if w.Total != 10 {
		t.Fatalf("total = %d, want 10", w.Total)
	}
	if w.Returned == 0 || w.Returned >= 10 {
		t.Fatalf("returned = %d, want some comments dropped but not empty", w.Returned)
	}
	if !w.HasMore {
		t.Fatal("want has_more = true when comments were budget-dropped")
	}
	if len(w.Comments) != w.Returned {
		t.Fatalf("len(Comments) = %d, want == Returned (%d)", len(w.Comments), w.Returned)
	}
	// The newest comment (highest index) must be the one retained.
	newest := "c-child-a-9"
	if w.Comments[len(w.Comments)-1].ID != newest {
		t.Fatalf("last retained comment = %s, want %s (the newest)", w.Comments[len(w.Comments)-1].ID, newest)
	}

	var sum int
	for _, c := range w.Comments {
		sum += len(c.Body)
	}
	if sum > 65536 {
		t.Fatalf("retained body bytes = %d, want <= 65536", sum)
	}
}

// AC-002.2/002.4: projection carries author_type/author_id/source and
// omits reply_channel_id (the struct has no such field to leak).
func TestListCommentsForCallerProjectsAuthorFields(t *testing.T) {
	svc, _ := newDocumentHandoffService(t, nil)
	reader := &fakeCommentReader{
		byTask: map[string][]*officemodels.TaskComment{
			"child-a": {{
				ID: "c1", TaskID: "child-a", AuthorType: "user", AuthorID: "user-9",
				Source: "user", Body: "hi", ReplyChannelID: "secret-channel",
				CreatedAt: time.Now().UTC(),
			}},
		},
	}
	svc.SetCommentReader(reader)
	ctx := context.Background()

	w, err := svc.ListCommentsForCaller(ctx, "child-a", "self", 20)
	if err != nil {
		t.Fatalf("ListCommentsForCaller: %v", err)
	}
	c := w.Comments[0]
	if c.AuthorType != "user" || c.AuthorID != "user-9" || c.Source != "user" {
		t.Fatalf("projection = %+v", c)
	}
}
