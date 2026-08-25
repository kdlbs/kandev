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
	seedBundleTurnAt(t, repo, turnID, sessionID, taskID, time.Now().UTC())
}

// seedBundleTurnAt creates a turn with an explicit started_at/created_at, so
// a test can control which of two turns on the same session is newest per
// turnAuthorityPredicate's ORDER BY (started_at DESC, created_at DESC, id DESC).
func seedBundleTurnAt(t *testing.T, repo *Repository, turnID, sessionID, taskID string, at time.Time) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_turns
			(id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`), turnID, sessionID, taskID, at, at, at)
	if err != nil {
		t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

// seedBundleSessionWithState creates a session in an explicit state, for
// covering D4's non-terminal-session conjunct.
func seedBundleSessionWithState(t *testing.T, repo *Repository, sessionID, taskID string, state models.TaskSessionState) {
	t.Helper()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{ID: sessionID, TaskID: taskID, State: state}); err != nil {
		t.Fatalf("create session %s: %v", sessionID, err)
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

// insertParentQuestionMessage inserts one clarification_request message
// shaped like the autopilot parent-question protocol's own record
// (parent_question.go's parentQuestionMetadata): same message type, same
// status/pending_id metadata keys as an ordinary bundle, plus the
// parent_question marker.
func insertParentQuestionMessage(t *testing.T, repo *Repository, id, sessionID, messageTaskID, turnID, pendingID, questionID, status string, ts time.Time) {
	t.Helper()
	meta := map[string]interface{}{
		"pending_id":      pendingID,
		"question_id":     questionID,
		"parent_question": true,
		"question": map[string]interface{}{
			"id":     questionID,
			"title":  "title",
			"prompt": "prompt",
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
		t.Fatalf("insert parent-question message %s: %v", id, err)
	}
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

// TestListUnresolvedClarificationBundles_ExcludesParentQuestionBundle covers
// the autopilot parent-question protocol (parent_question.go): its durable
// records share this table's type and status/pending_id metadata shape with
// an ordinary bundle, but they belong to a separate, parent-only resolution
// channel and must never surface through the external listing (Review round
// 2 finding: workspace-authorized callers could otherwise discover and
// answer a running autopilot agent's question to its parent).
func TestListUnresolvedClarificationBundles_ExcludesParentQuestionBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-PQ1", "")
	seedBundleSession(t, repo, "sess-PQ1", "task-PQ1")
	seedBundleTurn(t, repo, "turn-PQ1", "sess-PQ1", "task-PQ1")
	insertParentQuestionMessage(t, repo, "msg-PQ1", "sess-PQ1", "task-PQ1", "turn-PQ1", "pending-PQ1", "q1", "pending", time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (parent-question bundle excluded)", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_ExcludesSupersededTurnAndTerminalSession
// covers D4/L2a's two conjuncts the status-only has_pending aggregate cannot
// express on its own: the bundle's turn_id must be the session's current turn
// per turnAuthorityPredicate, and the owning session must not be terminal.
// Both bundles below have an otherwise-pending message (COALESCE(status,”)
// IN (”,'pending')) and would incorrectly surface if the list query reused
// only the status conjunct instead of the same helpers claimActiveClarificationBundle
// uses (turnAuthorityPredicate, nonTerminalSessionPredicate).
func TestListUnresolvedClarificationBundles_ExcludesSupersededTurnAndTerminalSession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Now().UTC()

	// Superseded-turn bundle: its message sits on an older turn, but a newer
	// turn on the same session has since been started, so the older turn is
	// no longer authoritative (D4's second conjunct).
	seedBundleTask(t, repo, "task-B7", "")
	seedBundleSession(t, repo, "sess-B7", "task-B7")
	seedBundleTurnAt(t, repo, "turn-B7-old", "sess-B7", "task-B7", base)
	insertClarificationMessage(t, repo, "msg-B7", "sess-B7", "task-B7", "turn-B7-old", "pending-B7", "q1", "pending", 0, base)
	seedBundleTurnAt(t, repo, "turn-B7-new", "sess-B7", "task-B7", base.Add(time.Minute))

	// Terminal-session bundle: the message is on the session's only (and
	// therefore current) turn, but the session itself has already reached a
	// terminal state (D4's third conjunct).
	seedBundleTask(t, repo, "task-B8", "")
	seedBundleSessionWithState(t, repo, "sess-B8", "task-B8", models.TaskSessionStateCompleted)
	seedBundleTurn(t, repo, "turn-B8", "sess-B8", "task-B8")
	insertClarificationMessage(t, repo, "msg-B8", "sess-B8", "task-B8", "turn-B8", "pending-B8", "q1", "pending", 0, base)

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (superseded-turn and terminal-session bundles both excluded)", page.Bundles)
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

// TestListUnresolvedClarificationBundles_ScopedVisibilityAcrossWorkspacesFillsLimit
// covers L1a: a scoped caller whose visible set spans two workspaces, with
// more matching bundles across those workspaces than the requested limit,
// must still get a full page rather than a short one. Disjunct 3's
// `t.workspace_id IN (?, ?)` runs inside the same bundle-grouping subquery as
// every other predicate (L1a's "all inside the single bundle query"); a
// query that instead filtered per-workspace as a post-query step, or that
// double-counted rows across the two workspaces before applying LIMIT, would
// return fewer than limit bundles despite more existing.
func TestListUnresolvedClarificationBundles_ScopedVisibilityAcrossWorkspacesFillsLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-L1a-A", Name: "A"}); err != nil {
		t.Fatalf("create workspace A: %v", err)
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-L1a-B", Name: "B"}); err != nil {
		t.Fatalf("create workspace B: %v", err)
	}

	base := time.Now().UTC()
	seedBundleTask(t, repo, "task-L1a-A1", "ws-L1a-A")
	seedBundleSession(t, repo, "sess-L1a-A1", "task-L1a-A1")
	seedBundleTurn(t, repo, "turn-L1a-A1", "sess-L1a-A1", "task-L1a-A1")
	insertClarificationMessage(t, repo, "msg-L1a-A1", "sess-L1a-A1", "task-L1a-A1", "turn-L1a-A1", "pending-L1a-A1", "q1", "pending", 0, base)

	seedBundleTask(t, repo, "task-L1a-A2", "ws-L1a-A")
	seedBundleSession(t, repo, "sess-L1a-A2", "task-L1a-A2")
	seedBundleTurn(t, repo, "turn-L1a-A2", "sess-L1a-A2", "task-L1a-A2")
	insertClarificationMessage(t, repo, "msg-L1a-A2", "sess-L1a-A2", "task-L1a-A2", "turn-L1a-A2", "pending-L1a-A2", "q1", "pending", 0, base.Add(time.Second))

	seedBundleTask(t, repo, "task-L1a-B1", "ws-L1a-B")
	seedBundleSession(t, repo, "sess-L1a-B1", "task-L1a-B1")
	seedBundleTurn(t, repo, "turn-L1a-B1", "sess-L1a-B1", "task-L1a-B1")
	insertClarificationMessage(t, repo, "msg-L1a-B1", "sess-L1a-B1", "task-L1a-B1", "turn-L1a-B1", "pending-L1a-B1", "q1", "pending", 0, base.Add(2*time.Second))

	seedBundleTask(t, repo, "task-L1a-B2", "ws-L1a-B")
	seedBundleSession(t, repo, "sess-L1a-B2", "task-L1a-B2")
	seedBundleTurn(t, repo, "turn-L1a-B2", "sess-L1a-B2", "task-L1a-B2")
	insertClarificationMessage(t, repo, "msg-L1a-B2", "sess-L1a-B2", "task-L1a-B2", "turn-L1a-B2", "pending-L1a-B2", "q1", "pending", 0, base.Add(3*time.Second))

	opts := models.ListClarificationBundlesOptions{
		Unscoped:            false,
		VisibleWorkspaceIDs: []string{"ws-L1a-A", "ws-L1a-B"},
		Limit:               3,
	}
	page, err := repo.ListUnresolvedClarificationBundles(ctx, opts)
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 3 {
		t.Fatalf("page.Bundles = %d, want a full page of 3 (4 bundles exist across the two visible workspaces)", len(page.Bundles))
	}
	if !page.HasMore {
		t.Fatalf("page.HasMore = false, want true (a 4th bundle remains)")
	}
	want := []string{"pending-L1a-A1", "pending-L1a-A2", "pending-L1a-B1"}
	for i, id := range want {
		if page.Bundles[i].PendingID != id {
			t.Errorf("page.Bundles[%d].PendingID = %q, want %q", i, page.Bundles[i].PendingID, id)
		}
	}
}

// insertClarificationMessageRawMetadata inserts one clarification_request
// message with caller-supplied metadata verbatim, for L16 cases the flat
// insertClarificationMessage helper cannot express (its question_id and
// question.id always agree).
func insertClarificationMessageRawMetadata(t *testing.T, repo *Repository, id, sessionID, messageTaskID, turnID string, meta map[string]interface{}, ts time.Time) {
	t.Helper()
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

// TestListUnresolvedClarificationBundles_ExcludesEmptyQuestionID covers spec
// L16: a bundle in which any message carries an empty or absent question_id
// (checked via questionIDFromMetadata's flat-then-nested fallback, not just
// the flat metadata.question_id key) must be excluded from the list, the same
// way answer_question_kandev already rejects it pre-claim. Without this, an
// external caller could list a bundle whose question_id can never be
// resolved and would always fail to answer.
func TestListUnresolvedClarificationBundles_ExcludesEmptyQuestionID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-L16", "")
	seedBundleSession(t, repo, "sess-L16", "task-L16")
	seedBundleTurn(t, repo, "turn-L16", "sess-L16", "task-L16")
	insertClarificationMessage(t, repo, "msg-L16", "sess-L16", "task-L16", "turn-L16", "pending-L16", "", "pending", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (message with empty question_id excludes the bundle per L16)", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_ExcludesUnrecognizedStatus covers
// D4/L2a: has_pending must match claimActiveClarificationBundle's own claim
// predicate (COALESCE(status,”) IN (”,'pending')) exactly, not a negative
// terminal-exclusion list. A status value outside both the pending set and
// the four known terminal values (a corrupted or future-version row) is
// NOT in (”, 'pending'), so the claim predicate can never match it — a
// query that lists it anyway (because it merely isn't in the terminal list)
// would surface a bundle that can be listed but can never be answered.
func TestListUnresolvedClarificationBundles_ExcludesUnrecognizedStatus(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-B9", "")
	seedBundleSession(t, repo, "sess-B9", "task-B9")
	seedBundleTurn(t, repo, "turn-B9", "sess-B9", "task-B9")
	insertClarificationMessage(t, repo, "msg-B9", "sess-B9", "task-B9", "turn-B9", "pending-B9", "q1", "unrecognized-status", 0, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 0 {
		t.Fatalf("bundles = %+v, want none (unrecognized status value cannot be claimed, so must not be listed)", page.Bundles)
	}
}

// TestListUnresolvedClarificationBundles_IncludesNestedOnlyQuestionID proves
// the L16 exclusion predicate mirrors questionIDFromMetadata's fallback
// order rather than checking only the flat metadata.question_id key: a
// message whose flat key is empty but whose nested question.id is set has a
// resolvable question_id and must still be listed.
func TestListUnresolvedClarificationBundles_IncludesNestedOnlyQuestionID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedBundleTask(t, repo, "task-L16b", "")
	seedBundleSession(t, repo, "sess-L16b", "task-L16b")
	seedBundleTurn(t, repo, "turn-L16b", "sess-L16b", "task-L16b")
	meta := map[string]interface{}{
		"pending_id":  "pending-L16b",
		"question_id": "",
		"status":      "pending",
		"question": map[string]interface{}{
			"id":     "q-nested-only",
			"title":  "title",
			"prompt": "prompt",
		},
	}
	insertClarificationMessageRawMetadata(t, repo, "msg-L16b", "sess-L16b", "task-L16b", "turn-L16b", meta, time.Now().UTC())

	page, err := repo.ListUnresolvedClarificationBundles(ctx, unscopedOpts(50))
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-L16b" {
		t.Fatalf("bundles = %+v, want exactly pending-L16b (nested question.id resolves the fallback)", page.Bundles)
	}
}
