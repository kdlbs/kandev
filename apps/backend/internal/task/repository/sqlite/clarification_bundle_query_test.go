package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedBundleTask creates a task with the given workspace_id, satisfying the
// FK a clarification bundle's messages ultimately resolve through (M5).
func seedBundleTask(t *testing.T, repo *Repository, taskID, workspaceID string) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "bundle task"}); err != nil {
		t.Fatalf("create task %s: %v", taskID, err)
	}
}

func seedBundleSession(t *testing.T, repo *Repository, sessionID, taskID string) {
	t.Helper()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
	}
}

func seedBundleTurn(t *testing.T, repo *Repository, turnID, sessionID, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO task_session_turns
			(id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, sessionID, taskID, now, now, now)
	if err != nil {
		t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

// insertClarificationMessage inserts one clarification_request message. An
// empty status omits the metadata key entirely, matching D3's "absent status
// counts as pending" case. An empty messageTaskID writes task_id = ” on the
// row (M5's fallback-to-session case).
func insertClarificationMessage(t *testing.T, repo *Repository, id, sessionID, messageTaskID, turnID, pendingID, questionID, status string, questionIndex int, ts time.Time) {
	t.Helper()
	meta := map[string]interface{}{
		"pending_id":     pendingID,
		"question_id":    questionID,
		"question_index": questionIndex,
		"question_total": 1,
		"context":        "why",
		"question": map[string]interface{}{
			"id":     questionID,
			"title":  "title",
			"prompt": "prompt",
			"options": []map[string]interface{}{
				{"option_id": "opt1", "label": "Yes", "description": "desc"},
			},
		},
	}
	if status != "" {
		meta["status"] = status
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, ?, ?, ?, 'agent', '', 'q', 1, 'clarification_request', ?, ?)
	`), id, sessionID, messageTaskID, turnID, string(metaJSON), ts)
	if err != nil {
		t.Fatalf("insert clarification message %s: %v", id, err)
	}
}

func unscopedOpts(limit int) models.ListClarificationBundlesOptions {
	return models.ListClarificationBundlesOptions{Unscoped: true, Limit: limit}
}

func TestListUnresolvedClarificationBundles_ReturnsPendingBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B1", "")
	seedBundleSession(t, repo, "sess-B1", "task-B1")
	seedBundleTurn(t, repo, "turn-B1", "sess-B1", "task-B1")
	insertClarificationMessage(t, repo, "msg-B1", "sess-B1", "task-B1", "turn-B1", "pending-B1", "q1", "pending", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-B1" {
		t.Fatalf("bundles = %+v, want exactly pending-B1", page.Bundles)
	}
	if page.Bundles[0].SessionID != "sess-B1" || page.Bundles[0].TaskID != "task-B1" {
		t.Fatalf("bundle identity = %+v, want session sess-B1 / task task-B1", page.Bundles[0])
	}
	if page.HasMore {
		t.Fatalf("HasMore = true, want false")
	}
}

// TestListUnresolvedClarificationBundles_AbsentStatusCountsAsPending covers
// D3: a message with no status key at all is effectively pending.
func TestListUnresolvedClarificationBundles_AbsentStatusCountsAsPending(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B2", "")
	seedBundleSession(t, repo, "sess-B2", "task-B2")
	seedBundleTurn(t, repo, "turn-B2", "sess-B2", "task-B2")
	insertClarificationMessage(t, repo, "msg-B2", "sess-B2", "task-B2", "turn-B2", "pending-B2", "q1", "", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-B2" {
		t.Fatalf("bundles = %+v, want exactly pending-B2", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_ExcludesResolvedBundle covers D4a
// conjunct 1: a resolution row excludes the bundle even if a message's
// status metadata was never fully applied (the R5 half-applied state).
func TestListUnresolvedClarificationBundles_ExcludesResolvedBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B3", "")
	seedBundleSession(t, repo, "sess-B3", "task-B3")
	seedBundleTurn(t, repo, "turn-B3", "sess-B3", "task-B3")
	insertClarificationMessage(t, repo, "msg-B3", "sess-B3", "task-B3", "turn-B3", "pending-B3", "q1", "pending", 0, time.Now().UTC())

	res := newTestClarificationResolution("pending-B3", "sess-B3", "task-B3")
	if _, _, err := repo.InsertClarificationResolution(ctx, res); err != nil {
		t.Fatalf("claim resolution: %v", err)
	}

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (resolution row excludes it)", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_ExcludesAllTerminalLegacyBundle
// covers D4a conjunct 2: a pre-upgrade bundle with no resolution row but
// every message terminal must not resurface (M3/D4a).
func TestListUnresolvedClarificationBundles_ExcludesAllTerminalLegacyBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B4", "")
	seedBundleSession(t, repo, "sess-B4", "task-B4")
	seedBundleTurn(t, repo, "turn-B4", "sess-B4", "task-B4")
	insertClarificationMessage(t, repo, "msg-B4", "sess-B4", "task-B4", "turn-B4", "pending-B4", "q1", "answered", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (all-terminal legacy bundle)", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_IncludesMixedStatusBundle covers
// L12: a bundle with no resolution row whose messages disagree on status is
// the half-applied case and must be listed so a caller can finish it.
func TestListUnresolvedClarificationBundles_IncludesMixedStatusBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B5", "")
	seedBundleSession(t, repo, "sess-B5", "task-B5")
	seedBundleTurn(t, repo, "turn-B5", "sess-B5", "task-B5")
	base := time.Now().UTC()
	insertClarificationMessage(t, repo, "msg-B5-1", "sess-B5", "task-B5", "turn-B5", "pending-B5", "q1", "answered", 0, base)
	insertClarificationMessage(t, repo, "msg-B5-2", "sess-B5", "task-B5", "turn-B5", "pending-B5", "q2", "pending", 1, base.Add(time.Second))

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-B5" {
		t.Fatalf("bundles = %+v, want exactly pending-B5", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_ResolvesTaskIDFromSession covers
// M5: when the message's own task_id is empty, the bundle's task_id resolves
// from the session row instead.
func TestListUnresolvedClarificationBundles_ResolvesTaskIDFromSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B6", "")
	seedBundleSession(t, repo, "sess-B6", "task-B6")
	seedBundleTurn(t, repo, "turn-B6", "sess-B6", "task-B6")
	insertClarificationMessage(t, repo, "msg-B6", "sess-B6", "", "turn-B6", "pending-B6", "q1", "pending", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].TaskID != "task-B6" {
		t.Fatalf("bundles = %+v, want task_id resolved to task-B6 via the session row", page.Bundles)
	}
}

func TestListUnresolvedClarificationBundles_Visibility(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()

	// Disjunct 1: task workspace_id is empty — visible to any scoped caller.
	seedBundleTask(t, repo, "task-V1", "")
	seedBundleSession(t, repo, "sess-V1", "task-V1")
	seedBundleTurn(t, repo, "turn-V1", "sess-V1", "task-V1")
	insertClarificationMessage(t, repo, "msg-V1", "sess-V1", "task-V1", "turn-V1", "pending-V1", "q1", "pending", 0, time.Now().UTC())

	// Disjunct 2: task workspace_id names no existing workspace row.
	seedBundleTask(t, repo, "task-V2", "ws-dangling")
	seedBundleSession(t, repo, "sess-V2", "task-V2")
	seedBundleTurn(t, repo, "turn-V2", "sess-V2", "task-V2")
	insertClarificationMessage(t, repo, "msg-V2", "sess-V2", "task-V2", "turn-V2", "pending-V2", "q1", "pending", 0, time.Now().UTC())

	// Disjunct 3: task workspace_id is in the caller's visible set.
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-visible", Name: "visible"}); err != nil {
		t.Fatalf("create workspace ws-visible: %v", err)
	}
	seedBundleTask(t, repo, "task-V3", "ws-visible")
	seedBundleSession(t, repo, "sess-V3", "task-V3")
	seedBundleTurn(t, repo, "turn-V3", "sess-V3", "task-V3")
	insertClarificationMessage(t, repo, "msg-V3", "sess-V3", "task-V3", "turn-V3", "pending-V3", "q1", "pending", 0, time.Now().UTC())

	// Excluded: task workspace_id is a real workspace NOT in the visible set.
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-hidden", Name: "hidden"}); err != nil {
		t.Fatalf("create workspace ws-hidden: %v", err)
	}
	seedBundleTask(t, repo, "task-V4", "ws-hidden")
	seedBundleSession(t, repo, "sess-V4", "task-V4")
	seedBundleTurn(t, repo, "turn-V4", "sess-V4", "task-V4")
	insertClarificationMessage(t, repo, "msg-V4", "sess-V4", "task-V4", "turn-V4", "pending-V4", "q1", "pending", 0, time.Now().UTC())

	scoped := models.ListClarificationBundlesOptions{
		Unscoped:            false,
		VisibleWorkspaceIDs: []string{"ws-visible"},
		Limit:               50,
	}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, scoped)
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	got := map[string]bool{}
	for _, b := range page.Bundles {
		got[b.PendingID] = true
	}
	want := map[string]bool{"pending-V1": true, "pending-V2": true, "pending-V3": true}
	if len(got) != len(want) {
		t.Fatalf("visible bundles = %v, want exactly %v", got, want)
	}
	for id := range want {
		if !got[id] {
			t.Errorf("missing expected visible bundle %s", id)
		}
	}
	if got["pending-V4"] {
		t.Errorf("pending-V4 (workspace not in visible set) leaked into results")
	}

	// L1b: an empty visible-workspace set must not error (no `IN ()`), and
	// disjuncts 1/2 alone still admit V1 and V2.
	scopedEmpty := models.ListClarificationBundlesOptions{Unscoped: false, VisibleWorkspaceIDs: nil, Limit: 50}
	pageEmpty, err := repo.ListUnresolvedClarificationBundles(ctx, scopedEmpty)
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles with empty visible set: %v", err)
	}
	gotEmpty := map[string]bool{}
	for _, b := range pageEmpty.Bundles {
		gotEmpty[b.PendingID] = true
	}
	if !gotEmpty["pending-V1"] || !gotEmpty["pending-V2"] {
		t.Fatalf("bundles with empty visible set = %v, want V1 and V2 via disjuncts 1/2", gotEmpty)
	}
	if gotEmpty["pending-V3"] || gotEmpty["pending-V4"] {
		t.Fatalf("bundles with empty visible set = %v, want V3/V4 excluded (owned workspaces)", gotEmpty)
	}

	// An unscoped caller sees everything regardless of workspace.
	pageUnscoped, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles unscoped: %v", err)
	}
	if len(pageUnscoped.Bundles) != 4 {
		t.Fatalf("unscoped bundles = %d, want 4 (every bundle visible)", len(pageUnscoped.Bundles))
	}
}

// TestListUnresolvedClarificationBundles_WorkspaceIDFilter covers L7: an
// explicit workspace_id narrows results to that workspace's tasks only.
func TestListUnresolvedClarificationBundles_WorkspaceIDFilter(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-A", Name: "A"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-B", Name: "B"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	seedBundleTask(t, repo, "task-W1", "ws-A")
	seedBundleSession(t, repo, "sess-W1", "task-W1")
	seedBundleTurn(t, repo, "turn-W1", "sess-W1", "task-W1")
	insertClarificationMessage(t, repo, "msg-W1", "sess-W1", "task-W1", "turn-W1", "pending-W1", "q1", "pending", 0, time.Now().UTC())

	seedBundleTask(t, repo, "task-W2", "ws-B")
	seedBundleSession(t, repo, "sess-W2", "task-W2")
	seedBundleTurn(t, repo, "turn-W2", "sess-W2", "task-W2")
	insertClarificationMessage(t, repo, "msg-W2", "sess-W2", "task-W2", "turn-W2", "pending-W2", "q1", "pending", 0, time.Now().UTC())

	opts := unscopedOpts(50)
	opts.WorkspaceID = "ws-A"
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-W1" {
		t.Fatalf("bundles = %+v, want only pending-W1", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_CreatedSinceFilter covers L8.
func TestListUnresolvedClarificationBundles_CreatedSinceFilter(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-S1", "")
	seedBundleSession(t, repo, "sess-S1", "task-S1")
	seedBundleTurn(t, repo, "turn-S1", "sess-S1", "task-S1")
	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	insertClarificationMessage(t, repo, "msg-S1", "sess-S1", "task-S1", "turn-S1", "pending-S1-old", "q1", "pending", 0, older)
	insertClarificationMessage(t, repo, "msg-S2", "sess-S1", "task-S1", "turn-S1", "pending-S1-new", "q1", "pending", 0, newer)

	since := time.Now().UTC().Add(-30 * time.Minute)
	opts := unscopedOpts(50)
	opts.CreatedSince = &since
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-S1-new" {
		t.Fatalf("bundles = %+v, want only the newer bundle", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_OrderingAndCursorPagination covers
// L6 (created_at asc, pending_id asc tiebreak) and L9/D6 (cursor pagination,
// next-page HasMore).
func TestListUnresolvedClarificationBundles_OrderingAndCursorPagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-P1", "")
	seedBundleSession(t, repo, "sess-P1", "task-P1")
	seedBundleTurn(t, repo, "turn-P1", "sess-P1", "task-P1")

	base := time.Now().UTC()
	ids := []string{"pending-P1", "pending-P2", "pending-P3"}
	for i, id := range ids {
		insertClarificationMessage(t, repo, "msg-"+id, "sess-P1", "task-P1", "turn-P1", id, "q1", "pending", 0, base.Add(time.Duration(i)*time.Second))
	}

	firstPage, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(2))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles first page: %v", err)
	}
	if len(firstPage.Bundles) != 2 || firstPage.Bundles[0].PendingID != "pending-P1" || firstPage.Bundles[1].PendingID != "pending-P2" {
		t.Fatalf("first page = %+v, want [pending-P1, pending-P2] in order", firstPage.Bundles)
	}
	if !firstPage.HasMore {
		t.Fatalf("first page HasMore = false, want true")
	}

	last := firstPage.Bundles[len(firstPage.Bundles)-1]
	opts := unscopedOpts(2)
	opts.CursorCreatedAt = last.CreatedAt
	opts.CursorPendingID = last.PendingID
	secondPage, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles second page: %v", err)
	}
	if len(secondPage.Bundles) != 1 || secondPage.Bundles[0].PendingID != "pending-P3" {
		t.Fatalf("second page = %+v, want exactly [pending-P3]", secondPage.Bundles)
	}
	if secondPage.HasMore {
		t.Fatalf("second page HasMore = true, want false (exhausted)")
	}
}
