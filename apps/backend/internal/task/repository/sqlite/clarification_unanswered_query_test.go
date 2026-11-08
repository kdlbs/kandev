package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

var unansweredBase = time.Date(2026, time.October, 1, 9, 0, 0, 0, time.UTC)

func seedUnansweredWorkspace(t *testing.T, repo *Repository, id string, enabled bool, retentionDays int) {
	t.Helper()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: id, Name: id,
		ClarificationUnansweredInboxEnabled:  enabled,
		ClarificationUnansweredRetentionDays: retentionDays,
	}); err != nil {
		t.Fatalf("CreateWorkspace(%s): %v", id, err)
	}
}

// seedSupersededQuestion creates a task/session whose turn-old asked one
// question that turn-new then superseded without an answer.
func seedSupersededQuestion(t *testing.T, repo *Repository, workspaceID, taskID, pendingID string, askedAt time.Time) {
	t.Helper()
	sessionID := "session-" + taskID
	seedInboxHistorySession(t, repo, taskID, sessionID, workspaceID, models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, taskID, sessionID, taskID+"-turn-old", askedAt, askedAt)
	createClarificationBundleMessage(t, repo, "msg-"+pendingID, taskID, sessionID, taskID+"-turn-old", pendingID, "q1", askedAt)
	addQuestionToBundle(t, repo, pendingID)
	startNewerTurn(t, repo, taskID, sessionID, askedAt.Add(time.Minute))
}

func startNewerTurn(t *testing.T, repo *Repository, taskID, sessionID string, at time.Time) {
	t.Helper()
	turnID := taskID + "-turn-" + at.Format("150405")
	createPendingActionTurn(t, repo, taskID, sessionID, turnID, at, at)
	createPendingActionMessage(t, repo, "ordinary-"+turnID, taskID, sessionID, turnID,
		models.MessageTypeMessage, "<missing>", at)
}

// addQuestionToBundle gives the bundle's first message a nested question
// prompt so duplicate detection has text to compare.
func addQuestionToBundle(t *testing.T, repo *Repository, pendingID string) {
	t.Helper()
	msgs, err := repo.FindMessagesByPendingID(context.Background(), pendingID)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("FindMessagesByPendingID(%s): %v", pendingID, err)
	}
	msg := msgs[0]
	msg.Content = "Which database should we use?"
	msg.Metadata["question"] = map[string]interface{}{"id": "q1", "prompt": "Which database should we use?"}
	if err := repo.UpdateMessage(context.Background(), msg); err != nil {
		t.Fatalf("UpdateMessage(%s): %v", msg.ID, err)
	}
}

func listUnanswered(t *testing.T, repo *Repository, workspaceIDs ...string) []models.ClarificationBundleSummary {
	t.Helper()
	page, err := repo.ListUnansweredClarificationBundles(context.Background(), models.ListUnansweredClarificationsOptions{
		WorkspaceIDs: workspaceIDs, Limit: 50, Now: unansweredBase.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ListUnansweredClarificationBundles: %v", err)
	}
	return page.Bundles
}

func unansweredPendingIDs(bundles []models.ClarificationBundleSummary) []string {
	ids := make([]string, 0, len(bundles))
	for _, b := range bundles {
		ids = append(ids, b.PendingID)
	}
	return ids
}

func bundleMetadata(t *testing.T, repo *Repository, pendingID string) map[string]interface{} {
	t.Helper()
	msgs, err := repo.FindMessagesByPendingID(context.Background(), pendingID)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("FindMessagesByPendingID(%s): %v", pendingID, err)
	}
	return msgs[0].Metadata
}

func TestUnansweredClarificationsRequireWorkspaceSetting(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedUnansweredWorkspace(t, repo, "ws-off", false, 14)
	seedUnansweredWorkspace(t, repo, "ws-on", true, 14)
	seedSupersededQuestion(t, repo, "ws-off", "task-off", "pending-off", unansweredBase)
	seedSupersededQuestion(t, repo, "ws-on", "task-on", "pending-on", unansweredBase)

	if got := listUnanswered(t, repo, "ws-off"); len(got) != 0 {
		t.Fatalf("setting off listed %v, want none", unansweredPendingIDs(got))
	}
	got := listUnanswered(t, repo, "ws-off", "ws-on")
	if ids := unansweredPendingIDs(got); len(ids) != 1 || ids[0] != "pending-on" {
		t.Fatalf("unanswered = %v, want [pending-on]", ids)
	}
	if got[0].TaskID != "task-on" || got[0].SessionID != "session-task-on" {
		t.Fatalf("bundle identity = %+v", got[0])
	}
	if meta := bundleMetadata(t, repo, "pending-off"); meta["status"] != "pending" || meta[models.ClarificationMetaClosedReason] != nil {
		t.Fatalf("setting off mutated metadata: %v", meta)
	}

	page, err := repo.ListUnansweredClarificationBundles(context.Background(), models.ListUnansweredClarificationsOptions{
		AnyWorkspace: true, Limit: 50, Now: unansweredBase.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ListUnansweredClarificationBundles(any): %v", err)
	}
	if ids := unansweredPendingIDs(page.Bundles); len(ids) != 1 || ids[0] != "pending-on" {
		t.Fatalf("any-workspace unanswered = %v, want [pending-on]", ids)
	}
}

func TestUnansweredClarificationsExcludeActiveAndOperationalUnchanged(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedInboxHistorySession(t, repo, "task-active", "session-active", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-active", "session-active", "turn-current", unansweredBase, unansweredBase)
	createClarificationBundleMessage(t, repo, "msg-active", "task-active", "session-active", "turn-current", "pending-active", "q1", unansweredBase)

	if got := listUnanswered(t, repo, "ws-1"); len(got) != 0 {
		t.Fatalf("current-turn bundle listed as unanswered: %v", unansweredPendingIDs(got))
	}
	active, err := repo.ListUnresolvedClarificationBundles(ctx, models.ListClarificationBundlesOptions{
		Unscoped: true, WorkspaceID: "ws-1", Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListUnresolvedClarificationBundles: %v", err)
	}
	if ids := unansweredPendingIDs(active.Bundles); len(ids) != 1 || ids[0] != "pending-active" {
		t.Fatalf("active bundles = %v, want [pending-active]", ids)
	}
	closed, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-active", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedByUser, At: unansweredBase,
	})
	if err != nil {
		t.Fatalf("CloseUnansweredClarificationBundle(active): %v", err)
	}
	if len(closed) != 0 {
		t.Fatalf("closing an active bundle changed %d rows", len(closed))
	}
	if meta := bundleMetadata(t, repo, "pending-active"); meta[models.ClarificationMetaClosedReason] != nil {
		t.Fatalf("active bundle metadata mutated: %v", meta)
	}
}

func TestUnansweredLateAnswerClosesOnlyThatQuestion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedSupersededQuestion(t, repo, "ws-1", "task-a", "pending-a", unansweredBase)
	seedSupersededQuestion(t, repo, "ws-1", "task-b", "pending-b", unansweredBase.Add(time.Second))

	answeredAt := unansweredBase.Add(30 * time.Minute)
	// A late answer closes the question only with a durable admission receipt:
	// a user message bound to the question, admitted after it was asked.
	lateMessageID := "late:pending-a:550e8400-e29b-41d4-a716-446655440001"
	if _, err := repo.db.Exec(`
		INSERT INTO task_session_messages
		(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, 'session-task-a', 'task-a', 'task-a-turn-090100', 'user', '', 'Postgres', 0, 'message', '{}', ?)`,
		lateMessageID, answeredAt.Add(-time.Minute)); err != nil {
		t.Fatalf("insert late answer message: %v", err)
	}
	closed, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-a", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedAnsweredLate, At: answeredAt,
		MessageID: lateMessageID, SessionID: "session-task-a", SourceSessionID: "session-task-a",
	})
	if err != nil {
		t.Fatalf("CloseUnansweredClarificationBundle: %v", err)
	}
	if len(closed) != 1 {
		t.Fatalf("closed rows = %d, want 1", len(closed))
	}
	meta := bundleMetadata(t, repo, "pending-a")
	if meta[models.ClarificationMetaClosedReason] != string(models.ClarificationClosedAnsweredLate) ||
		meta[models.ClarificationMetaLateAnswerMessageID] != lateMessageID ||
		meta[models.ClarificationMetaLateAnsweredAt] != answeredAt.Format(time.RFC3339Nano) ||
		meta["status"] != "pending" {
		t.Fatalf("late-answer metadata = %v", meta)
	}
	if other := bundleMetadata(t, repo, "pending-b"); other[models.ClarificationMetaClosedReason] != nil {
		t.Fatalf("other bundle metadata mutated: %v", other)
	}
	if ids := unansweredPendingIDs(listUnanswered(t, repo, "ws-1")); len(ids) != 1 || ids[0] != "pending-b" {
		t.Fatalf("unanswered after late answer = %v, want [pending-b]", ids)
	}

	again, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-a", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedByUser, At: answeredAt,
	})
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second close changed %d rows, want 0", len(again))
	}
	if _, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-b", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedAnsweredLate, At: answeredAt,
	}); err == nil {
		t.Fatal("late-answer close without a message id succeeded")
	}
}

func TestUnansweredDuplicateQuestionMergesIntoNewerBundle(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedSupersededQuestion(t, repo, "ws-1", "task-dup", "pending-old", unansweredBase)

	notSame := []models.ClarificationQuestionIdentity{{ID: "q1", Prompt: "Which branch should we use?"}}
	merged, err := repo.MergeUnansweredClarificationDuplicates(ctx, "task-dup", "pending-new", notSame)
	if err != nil {
		t.Fatalf("MergeUnansweredClarificationDuplicates(positional id): %v", err)
	}
	if len(merged) != 0 {
		t.Fatalf("positional q1 with different text merged %v", merged)
	}

	same := []models.ClarificationQuestionIdentity{{ID: "q1", Prompt: "  which   DATABASE should we use? "}}
	merged, err = repo.MergeUnansweredClarificationDuplicates(ctx, "task-dup", "pending-new", same)
	if err != nil {
		t.Fatalf("MergeUnansweredClarificationDuplicates: %v", err)
	}
	if len(merged) != 1 || merged[0] != "pending-old" {
		t.Fatalf("merged = %v, want [pending-old]", merged)
	}
	meta := bundleMetadata(t, repo, "pending-old")
	if meta[models.ClarificationMetaMergedInto] != "pending-new" || meta["status"] != "pending" {
		t.Fatalf("merged metadata = %v", meta)
	}
	if got := listUnanswered(t, repo, "ws-1"); len(got) != 0 {
		t.Fatalf("merged bundle still listed: %v", unansweredPendingIDs(got))
	}
	history, err := repo.ListInboxHistoryBundles(ctx, unscopedHistoryOpts("ws-1", 50))
	if err != nil {
		t.Fatalf("ListInboxHistoryBundles: %v", err)
	}
	if _, found := findHistoryBundle(history.Bundles, "pending-old"); !found {
		t.Fatal("merged bundle disappeared from history")
	}
}

func TestUnansweredDuplicateMergeMatchesAgentQuestionID(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedInboxHistorySession(t, repo, "task-id", "session-id", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-id", "session-id", "turn-old", unansweredBase, unansweredBase)
	createClarificationBundleMessage(t, repo, "msg-id", "task-id", "session-id", "turn-old", "pending-id-old", "db-choice", unansweredBase)
	startNewerTurn(t, repo, "task-id", "session-id", unansweredBase.Add(time.Minute))

	merged, err := repo.MergeUnansweredClarificationDuplicates(ctx, "task-id", "pending-id-new",
		[]models.ClarificationQuestionIdentity{{ID: "db-choice", Prompt: "Reworded question"}})
	if err != nil {
		t.Fatalf("MergeUnansweredClarificationDuplicates: %v", err)
	}
	if len(merged) != 1 || merged[0] != "pending-id-old" {
		t.Fatalf("merged = %v, want [pending-id-old]", merged)
	}
}

func TestUnansweredMergeIgnoresOtherTasksAndDisabledWorkspaces(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-on", true, 14)
	seedUnansweredWorkspace(t, repo, "ws-off", false, 14)
	seedSupersededQuestion(t, repo, "ws-on", "task-other", "pending-other", unansweredBase)
	seedSupersededQuestion(t, repo, "ws-off", "task-off", "pending-off", unansweredBase)
	same := []models.ClarificationQuestionIdentity{{Prompt: "Which database should we use?"}}

	for _, taskID := range []string{"task-new", "task-off"} {
		merged, err := repo.MergeUnansweredClarificationDuplicates(ctx, taskID, "pending-new", same)
		if err != nil {
			t.Fatalf("MergeUnansweredClarificationDuplicates(%s): %v", taskID, err)
		}
		if len(merged) != 0 {
			t.Fatalf("task %s merged %v, want none", taskID, merged)
		}
	}
	if meta := bundleMetadata(t, repo, "pending-off"); meta[models.ClarificationMetaMergedInto] != nil {
		t.Fatalf("disabled workspace bundle mutated: %v", meta)
	}
}
