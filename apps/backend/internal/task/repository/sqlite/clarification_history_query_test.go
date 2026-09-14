package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// seedInboxHistorySession creates a workspace-scoped task and a session in
// the given state, mirroring seedPendingActionSession but with an explicit
// workspace and (optionally) archived task, both of which
// ListInboxHistoryBundles must filter on (AC .2 workspace scope, AC .4
// archived exclusion).
func seedInboxHistorySession(t *testing.T, repo *Repository, taskID, sessionID, workspaceID string, sessionState models.TaskSessionState, archived bool) {
	t.Helper()
	ctx := context.Background()
	task := &models.Task{ID: taskID, Title: taskID, WorkspaceID: workspaceID}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask(%s): %v", taskID, err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: sessionState,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
	if archived {
		if err := repo.ArchiveTask(ctx, taskID); err != nil {
			t.Fatalf("ArchiveTask(%s): %v", taskID, err)
		}
	}
}

func unscopedHistoryOpts(workspaceID string, limit int) models.ListClarificationHistoryOptions {
	return models.ListClarificationHistoryOptions{WorkspaceID: workspaceID, Limit: limit}
}

func findHistoryBundle(bundles []models.ClarificationHistoryBundleSummary, pendingID string) (models.ClarificationHistoryBundleSummary, bool) {
	for _, b := range bundles {
		if b.PendingID == pendingID {
			return b, true
		}
	}
	return models.ClarificationHistoryBundleSummary{}, false
}

// TestListInboxHistoryBundlesRealSequenceSupersededVsOperational drives
// AC .8's exact regression sequence: ask a question, then start a new turn
// without answering it. The operational listing must not return it and the
// history listing must.
func TestListInboxHistoryBundlesRealSequenceSupersededVsOperational(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-superseded", "session-superseded", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-superseded", "session-superseded", "turn-old", base, base)
	createClarificationBundleMessage(t, repo, "clar-old", "task-superseded", "session-superseded", "turn-old", "pending-superseded", "q1", base)

	// Start a new turn without answering the question.
	createPendingActionTurn(t, repo, "task-superseded", "session-superseded", "turn-new", base.Add(time.Minute), base.Add(time.Minute))
	createPendingActionMessage(t, repo, "ordinary-new", "task-superseded", "session-superseded", "turn-new",
		models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	operational, err := repo.ListUnresolvedClarificationBundles(ctx, models.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: "ws-1", Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if _, found := findBundleByPendingID(operational.Bundles, "pending-superseded"); found {
		t.Fatal("operational listing returned a superseded bundle")
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-superseded")
	if !found {
		t.Fatalf("history listing did not return the superseded bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonSuperseded {
		t.Fatalf("reason = %q, want superseded", bundle.Reason)
	}
	if bundle.AskingTurnID != "turn-old" {
		t.Fatalf("asking turn = %q, want turn-old", bundle.AskingTurnID)
	}
	if bundle.SupersedingTurnID != "turn-new" {
		t.Fatalf("superseding turn = %q, want turn-new", bundle.SupersedingTurnID)
	}
}

func findBundleByPendingID(bundles []models.ClarificationBundleSummary, pendingID string) (models.ClarificationBundleSummary, bool) {
	for _, b := range bundles {
		if b.PendingID == pendingID {
			return b, true
		}
	}
	return models.ClarificationBundleSummary{}, false
}

// TestListInboxHistoryBundlesSessionEndedOmitsSupersedingTurn pins the
// session_ended reason and AC .10's omission rule for it.
func TestListInboxHistoryBundlesSessionEndedOmitsSupersedingTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-ended", "session-ended", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-ended", "session-ended", "turn-ended", base, base)
	createClarificationBundleMessage(t, repo, "clar-ended", "task-ended", "session-ended", "turn-ended", "pending-ended", "q1", base)

	session, err := repo.GetTaskSession(ctx, "session-ended")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.State = models.TaskSessionStateCompleted
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-ended")
	if !found {
		t.Fatalf("history listing did not return the session_ended bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonSessionEnded {
		t.Fatalf("reason = %q, want session_ended", bundle.Reason)
	}
	if bundle.SupersedingTurnID != "" {
		t.Fatalf("superseding turn = %q, want omitted", bundle.SupersedingTurnID)
	}
}

// TestListInboxHistoryBundlesUnreadableClarificationOnly pins that
// unreadable applies only to clarification bundles and never to a
// permission bundle, which carries no resolvable question identifier by
// construction (system design "A permission record is a different shape").
func TestListInboxHistoryBundlesUnreadableClarificationOnly(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 11, 0, 0, 0, time.UTC)

	// Unreadable clarification: current turn, missing question_id, so it's
	// otherwise live except for the missing identifier.
	seedInboxHistorySession(t, repo, "task-unreadable", "session-unreadable", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-unreadable", "session-unreadable", "turn-unreadable", base, base)
	createClarificationBundleMessage(t, repo, "clar-unreadable", "task-unreadable", "session-unreadable", "turn-unreadable", "pending-unreadable", "", base)

	// Live, current-turn, non-terminal-session permission request: must
	// NOT be listed at all (it is live -- none of the three reasons hold).
	seedInboxHistorySession(t, repo, "task-live-perm", "session-live-perm", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-live-perm", "session-live-perm", "turn-live-perm", base, base)
	createInteractionMessage(t, repo, "perm-live", "task-live-perm", "session-live-perm", "turn-live-perm",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-live-perm"}, base)

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-unreadable")
	if !found {
		t.Fatalf("history listing did not return the unreadable bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonUnreadable {
		t.Fatalf("reason = %q, want unreadable", bundle.Reason)
	}
	if _, found := findHistoryBundle(page.Bundles, "pending-live-perm"); found {
		t.Fatal("a live, current-turn, non-terminal-session permission request must not be listed")
	}
}

// TestListInboxHistoryBundlesPrecedenceSupersededOverUnreadable pins AC .2's
// precedence test: a bundle that is both superseded and unreadable reports
// superseded.
func TestListInboxHistoryBundlesPrecedenceSupersededOverUnreadable(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-both", "session-both", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-both", "session-both", "turn-both-old", base, base)
	createClarificationBundleMessage(t, repo, "clar-both", "task-both", "session-both", "turn-both-old", "pending-both", "", base)
	createPendingActionTurn(t, repo, "task-both", "session-both", "turn-both-new", base.Add(time.Minute), base.Add(time.Minute))
	createPendingActionMessage(t, repo, "ordinary-both", "task-both", "session-both", "turn-both-new",
		models.MessageTypeMessage, "<missing>", base.Add(time.Minute))

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-both")
	if !found {
		t.Fatalf("history listing did not return the bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonSuperseded {
		t.Fatalf("reason = %q, want superseded (precedence over unreadable)", bundle.Reason)
	}
}

// TestListInboxHistoryBundlesPermissionSameTurnSupersession pins AC .2's
// second superseded clause: an older permission request superseded by a
// newer one on the SAME turn is superseded with no separate superseding
// turn to name (AC .10), while the newest stays live and unlisted.
func TestListInboxHistoryBundlesPermissionSameTurnSupersession(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 13, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-multi-perm", "session-multi-perm", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-multi-perm", "session-multi-perm", "turn-multi-perm", base, base)
	createInteractionMessage(t, repo, "perm-older", "task-multi-perm", "session-multi-perm", "turn-multi-perm",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-perm-older"}, base)
	createInteractionMessage(t, repo, "perm-newer", "task-multi-perm", "session-multi-perm", "turn-multi-perm",
		models.MessageTypePermissionRequest, map[string]interface{}{"pending_id": "pending-perm-newer"}, base.Add(time.Second))

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	older, found := findHistoryBundle(page.Bundles, "pending-perm-older")
	if !found {
		t.Fatalf("history listing did not return the superseded permission: %+v", page.Bundles)
	}
	if older.Reason != models.ClarificationHistoryReasonSuperseded {
		t.Fatalf("older permission reason = %q, want superseded", older.Reason)
	}
	if older.SupersedingTurnID != "" {
		t.Fatalf("same-turn superseding turn = %q, want omitted", older.SupersedingTurnID)
	}
	if _, found := findHistoryBundle(page.Bundles, "pending-perm-newer"); found {
		t.Fatal("the newest permission on the current turn is live and must not be listed")
	}
}

// TestListInboxHistoryBundlesPendingIDReuseAcrossTurns pins the message.go /
// service_messages.go / event_handlers_streaming.go documented behavior that
// a provider may reuse a permission's pending_id for a later, unrelated
// request once the original resolves. The reused id must not let an old,
// already-resolved permission (a different turn, a different request) merge
// with a live, currently-answerable one on the session's current turn: the
// live request must stay unlisted rather than being reported superseded.
func TestListInboxHistoryBundlesPendingIDReuseAcrossTurns(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 15, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-reused-pending", "session-reused-pending", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-reused-pending", "session-reused-pending", "turn-a-old", base, base)
	createInteractionMessage(t, repo, "perm-reused-old", "task-reused-pending", "session-reused-pending", "turn-a-old",
		models.MessageTypePermissionRequest,
		map[string]interface{}{"pending_id": "pending-reused", "request_id": "req-old", "status": "approved"},
		base)

	createPendingActionTurn(t, repo, "task-reused-pending", "session-reused-pending", "turn-b-new", base.Add(time.Minute), base.Add(time.Minute))
	createInteractionMessage(t, repo, "perm-reused-new", "task-reused-pending", "session-reused-pending", "turn-b-new",
		models.MessageTypePermissionRequest,
		map[string]interface{}{"pending_id": "pending-reused", "request_id": "req-new"},
		base.Add(time.Minute))

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	if bundle, found := findHistoryBundle(page.Bundles, "pending-reused"); found {
		t.Fatalf("a live current-turn permission must not be merged with a resolved reused-pending_id request into a superseded bundle: %+v", bundle)
	}
}

// TestListInboxHistoryBundlesPendingIDReusePermissionGroupKeyDistinguishesRequests
// pins that once a reused pending_id's second request is itself superseded and
// returned, its PermissionGroupKey is its own request_id rather than the first
// (already-resolved) request's -- message hydration keys off this field to
// keep the two requests' rendered content apart even though
// FindMessagesByPendingIDs returns every message sharing the raw pending_id.
func TestListInboxHistoryBundlesPendingIDReusePermissionGroupKeyDistinguishesRequests(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 17, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-reused-group-key", "session-reused-group-key", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-reused-group-key", "session-reused-group-key", "turn-a-old", base, base)
	createInteractionMessage(t, repo, "perm-group-key-old", "task-reused-group-key", "session-reused-group-key", "turn-a-old",
		models.MessageTypePermissionRequest,
		map[string]interface{}{"pending_id": "pending-reused-group-key", "request_id": "req-old-group-key", "status": "approved"},
		base)

	createPendingActionTurn(t, repo, "task-reused-group-key", "session-reused-group-key", "turn-b-new", base.Add(time.Minute), base.Add(time.Minute))
	createInteractionMessage(t, repo, "perm-group-key-new", "task-reused-group-key", "session-reused-group-key", "turn-b-new",
		models.MessageTypePermissionRequest,
		map[string]interface{}{"pending_id": "pending-reused-group-key", "request_id": "req-new-group-key"},
		base.Add(time.Minute))

	// A third turn supersedes turn-b-new without answering it, making the new
	// request itself History-eligible.
	createPendingActionTurn(t, repo, "task-reused-group-key", "session-reused-group-key", "turn-c", base.Add(2*time.Minute), base.Add(2*time.Minute))
	createPendingActionMessage(t, repo, "ordinary-group-key", "task-reused-group-key", "session-reused-group-key", "turn-c",
		models.MessageTypeMessage, "<missing>", base.Add(2*time.Minute))

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	bundle, found := findHistoryBundle(page.Bundles, "pending-reused-group-key")
	if !found {
		t.Fatalf("history listing did not return the superseded reused-pending_id bundle: %+v", page.Bundles)
	}
	if bundle.Reason != models.ClarificationHistoryReasonSuperseded {
		t.Fatalf("reason = %q, want superseded", bundle.Reason)
	}
	if bundle.PermissionGroupKey != "req-new-group-key" {
		t.Fatalf("PermissionGroupKey = %q, want req-new-group-key (must not resolve to the old, already-resolved request's key)", bundle.PermissionGroupKey)
	}
}

// TestListInboxHistoryBundlesExcludesArchivedAndParentQuestion pins AC .4
// (archived task) and AC .2's parent_question eligibility clause.
func TestListInboxHistoryBundlesExcludesArchivedAndParentQuestion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 14, 0, 0, 0, time.UTC)

	seedInboxHistorySession(t, repo, "task-archived", "session-archived", "ws-1", models.TaskSessionStateCompleted, true)
	createPendingActionTurn(t, repo, "task-archived", "session-archived", "turn-archived", base, base)
	createClarificationBundleMessage(t, repo, "clar-archived", "task-archived", "session-archived", "turn-archived", "pending-archived", "q1", base)

	seedInboxHistorySession(t, repo, "task-parent-question", "session-parent-question", "ws-1", models.TaskSessionStateCompleted, false)
	createPendingActionTurn(t, repo, "task-parent-question", "session-parent-question", "turn-parent-question", base, base)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "clar-parent-question", TaskSessionID: "session-parent-question", TaskID: "task-parent-question",
		TurnID: "turn-parent-question", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{
			"pending_id": "pending-parent-question", "question_id": "q1", "status": "pending",
			"parent_question": true,
		},
		CreatedAt: base,
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	if _, found := findHistoryBundle(page.Bundles, "pending-archived"); found {
		t.Fatal("archived task's bundle must not be listed")
	}
	if _, found := findHistoryBundle(page.Bundles, "pending-parent-question"); found {
		t.Fatal("parent_question bundle must not be listed")
	}
}

// TestListInboxHistoryBundlesOrderingAndPagination pins AC .18 (ordering)
// and AC .20 (limit+1 detection, exclusive keyset cursor, workspace scope).
func TestListInboxHistoryBundlesOrderingAndPagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 15, 0, 0, 0, time.UTC)

	for i, suffix := range []string{"a", "b", "c"} {
		taskID, sessionID, turnID := "task-order-"+suffix, "session-order-"+suffix, "turn-order-"+suffix
		seedInboxHistorySession(t, repo, taskID, sessionID, "ws-1", models.TaskSessionStateCompleted, false)
		createdAt := base.Add(time.Duration(i) * time.Minute)
		createPendingActionTurn(t, repo, taskID, sessionID, turnID, createdAt, createdAt)
		createClarificationBundleMessage(t, repo, "clar-order-"+suffix, taskID, sessionID, turnID, "pending-order-"+suffix, "q1", createdAt)
	}

	// Another workspace's bundle must never appear.
	seedInboxHistorySession(t, repo, "task-order-other-ws", "session-order-other-ws", "ws-2", models.TaskSessionStateCompleted, false)
	createPendingActionTurn(t, repo, "task-order-other-ws", "session-order-other-ws", "turn-order-other-ws", base, base)
	createClarificationBundleMessage(t, repo, "clar-order-other-ws", "task-order-other-ws", "session-order-other-ws", "turn-order-other-ws", "pending-order-other-ws", "q1", base)

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 2))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	if len(page.Bundles) != 2 || !page.HasMore {
		t.Fatalf("page = %+v, want 2 bundles with HasMore", page)
	}
	if page.Bundles[0].PendingID != "pending-order-a" || page.Bundles[1].PendingID != "pending-order-b" {
		t.Fatalf("bundles = %v, want [pending-order-a pending-order-b]", []string{page.Bundles[0].PendingID, page.Bundles[1].PendingID})
	}

	last := page.Bundles[len(page.Bundles)-1]
	next, err := repo.ListInboxHistoryBundles(ctx, models.ListClarificationHistoryOptions{
		WorkspaceID: "ws-1", Limit: 50, CursorCreatedAt: last.CreatedAt, CursorPendingID: last.PendingID,
	})
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles (cursor): %v", err)
	}
	if len(next.Bundles) != 1 || next.Bundles[0].PendingID != "pending-order-c" {
		t.Fatalf("next page = %v, want exactly [pending-order-c]", next.Bundles)
	}
	if next.HasMore {
		t.Fatal("final page reported HasMore")
	}
}

// TestCountInboxHistoryBundlesIsWorkspaceWideTotal pins AC .20's total: it
// counts every eligible bundle for the workspace, independent of page size.
func TestCountInboxHistoryBundlesIsWorkspaceWideTotal(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Date(2026, time.September, 1, 16, 0, 0, 0, time.UTC)

	for i, suffix := range []string{"a", "b", "c"} {
		taskID, sessionID, turnID := "task-count-"+suffix, "session-count-"+suffix, "turn-count-"+suffix
		seedInboxHistorySession(t, repo, taskID, sessionID, "ws-1", models.TaskSessionStateCompleted, false)
		createdAt := base.Add(time.Duration(i) * time.Minute)
		createPendingActionTurn(t, repo, taskID, sessionID, turnID, createdAt, createdAt)
		createClarificationBundleMessage(t, repo, "clar-count-"+suffix, taskID, sessionID, turnID, "pending-count-"+suffix, "q1", createdAt)
	}

	total, err := repo.CountInboxHistoryBundles(ctx, models.ListClarificationHistoryOptions{WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("CountInboxHistoryBundles: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	page, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 1))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	if len(page.Bundles) != 1 {
		t.Fatalf("page bundles = %d, want 1 (total must stay independent of page size)", len(page.Bundles))
	}
}
