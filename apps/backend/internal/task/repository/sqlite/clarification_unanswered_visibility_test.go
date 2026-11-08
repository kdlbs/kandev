package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func listUnansweredAt(t *testing.T, repo *Repository, now time.Time, workspaceIDs ...string) []string {
	t.Helper()
	page, err := repo.ListUnansweredClarificationBundles(context.Background(), models.ListUnansweredClarificationsOptions{
		WorkspaceIDs: workspaceIDs, Limit: 50, Now: now,
	})
	if err != nil {
		t.Fatalf("ListUnansweredClarificationBundles: %v", err)
	}
	return unansweredPendingIDs(page.Bundles)
}

func setSessionState(t *testing.T, repo *Repository, sessionID string, state models.TaskSessionState) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
	}
	session.State = state
	if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
		t.Fatalf("UpdateTaskSession(%s): %v", sessionID, err)
	}
}

func TestUnansweredTerminalSessionKeepsExpiredQuestionsExpired(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)

	// The session ends while its current-turn question is pending: the
	// lifecycle expires it, and it stays out of the unanswered list.
	seedInboxHistorySession(t, repo, "task-expired", "session-expired", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-expired", "session-expired", "turn-expired", unansweredBase, unansweredBase)
	createClarificationBundleMessage(t, repo, "msg-expired", "task-expired", "session-expired", "turn-expired", "pending-expired", "q1", unansweredBase)
	if _, err := repo.ExpireActiveClarificationBundle(ctx, "session-expired", "pending-expired"); err != nil {
		t.Fatalf("ExpireActiveClarificationBundle: %v", err)
	}
	setSessionState(t, repo, "session-expired", models.TaskSessionStateCompleted)

	// An older turn's question superseded before the session ended remains
	// unanswered so a late answer can still reach the task.
	seedSupersededQuestion(t, repo, "ws-1", "task-ended", "pending-superseded-ended", unansweredBase)
	setSessionState(t, repo, "session-task-ended", models.TaskSessionStateFailed)

	ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-1")
	if len(ids) != 1 || ids[0] != "pending-superseded-ended" {
		t.Fatalf("unanswered = %v, want [pending-superseded-ended]", ids)
	}
	if meta := bundleMetadata(t, repo, "pending-expired"); meta["status"] != "expired" || meta[models.ClarificationMetaClosedReason] != nil {
		t.Fatalf("expired bundle metadata = %v", meta)
	}
}

func TestUnansweredArchivedTaskHiddenUntilUnarchived(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedSupersededQuestion(t, repo, "ws-1", "task-archive", "pending-archive", unansweredBase)

	if err := repo.ArchiveTask(ctx, "task-archive"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	if ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-1"); len(ids) != 0 {
		t.Fatalf("archived task listed %v", ids)
	}
	if closed, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-archive", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedByUser, At: unansweredBase,
	}); err != nil || len(closed) != 0 {
		t.Fatalf("close on archived task = (%d rows, %v), want (0, nil)", len(closed), err)
	}
	if _, err := repo.UnarchiveTask(ctx, "task-archive"); err != nil {
		t.Fatalf("UnarchiveTask: %v", err)
	}
	if ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-1"); len(ids) != 1 || ids[0] != "pending-archive" {
		t.Fatalf("unarchived task unanswered = %v, want [pending-archive]", ids)
	}
}

func TestUnansweredRetentionExpiresOldQuestionsOnRead(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedUnansweredWorkspace(t, repo, "ws-1", true, 7)
	seedSupersededQuestion(t, repo, "ws-1", "task-old", "pending-old", unansweredBase)
	seedSupersededQuestion(t, repo, "ws-1", "task-recent", "pending-recent", unansweredBase.AddDate(0, 0, 3))

	now := unansweredBase.AddDate(0, 0, 8)
	ids := listUnansweredAt(t, repo, now, "ws-1")
	if len(ids) != 1 || ids[0] != "pending-recent" {
		t.Fatalf("unanswered = %v, want [pending-recent]", ids)
	}
	meta := bundleMetadata(t, repo, "pending-old")
	if meta[models.ClarificationMetaClosedReason] != string(models.ClarificationClosedExpiredUnanswered) ||
		meta[models.ClarificationMetaClosedAt] != now.Format(time.RFC3339Nano) || meta["status"] != "pending" {
		t.Fatalf("retention metadata = %v", meta)
	}
	if recent := bundleMetadata(t, repo, "pending-recent"); recent[models.ClarificationMetaClosedReason] != nil {
		t.Fatalf("recent bundle closed: %v", recent)
	}
}

func TestUnansweredExcludesParentQuestionsAndOtherWorkspaces(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedUnansweredWorkspace(t, repo, "ws-2", true, 14)
	seedSupersededQuestion(t, repo, "ws-2", "task-foreign", "pending-foreign", unansweredBase)

	seedInboxHistorySession(t, repo, "task-parent", "session-parent", "ws-1", models.TaskSessionStateWaitingForInput, false)
	createPendingActionTurn(t, repo, "task-parent", "session-parent", "turn-parent-old", unansweredBase, unansweredBase)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "msg-parent", TaskSessionID: "session-parent", TaskID: "task-parent", TurnID: "turn-parent-old",
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeClarificationRequest,
		Metadata: map[string]interface{}{
			"pending_id": "pending-parent", "question_id": "q1", "status": "pending",
			models.MetaKeyParentQuestion: true,
		},
		CreatedAt: unansweredBase,
	}); err != nil {
		t.Fatalf("CreateMessage(parent question): %v", err)
	}
	startNewerTurn(t, repo, "task-parent", "session-parent", unansweredBase.Add(time.Minute))

	if ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-1"); len(ids) != 0 {
		t.Fatalf("ws-1 unanswered = %v, want none", ids)
	}
	if ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour)); len(ids) != 0 {
		t.Fatalf("empty workspace set listed %v", ids)
	}
	if ids := listUnansweredAt(t, repo, unansweredBase.Add(time.Hour), "ws-2"); len(ids) != 1 || ids[0] != "pending-foreign" {
		t.Fatalf("ws-2 unanswered = %v, want [pending-foreign]", ids)
	}
}

func TestUnansweredHonorsInboxSidecarAndPagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedUnansweredWorkspace(t, repo, "ws-1", true, 14)
	seedSupersededQuestion(t, repo, "ws-1", "task-1", "pending-1", unansweredBase)
	seedSupersededQuestion(t, repo, "ws-1", "task-2", "pending-2", unansweredBase.Add(time.Second))
	seedSupersededQuestion(t, repo, "ws-1", "task-3", "pending-3", unansweredBase.Add(2*time.Second))
	now := unansweredBase.Add(time.Hour)
	if err := repo.UpsertClarificationInboxSidecar(ctx, "user-1", "pending-2", models.ClarificationSidecarDismissed, nil, now); err != nil {
		t.Fatalf("UpsertClarificationInboxSidecar: %v", err)
	}

	sidecar := &models.ClarificationSidecarFilter{UserID: "user-1", Now: now}
	first, err := repo.ListUnansweredClarificationBundles(ctx, models.ListUnansweredClarificationsOptions{
		WorkspaceIDs: []string{"ws-1"}, Limit: 1, Now: now, Sidecar: sidecar,
	})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if ids := unansweredPendingIDs(first.Bundles); len(ids) != 1 || ids[0] != "pending-1" || !first.HasMore {
		t.Fatalf("first page = %v hasMore=%v", ids, first.HasMore)
	}
	last := first.Bundles[0]
	second, err := repo.ListUnansweredClarificationBundles(ctx, models.ListUnansweredClarificationsOptions{
		WorkspaceIDs: []string{"ws-1"}, Limit: 5, Now: now, Sidecar: sidecar,
		CursorCreatedAt: last.CreatedAt, CursorPendingID: last.PendingID,
	})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if ids := unansweredPendingIDs(second.Bundles); len(ids) != 1 || ids[0] != "pending-3" || second.HasMore {
		t.Fatalf("second page = %v hasMore=%v, want [pending-3] without the dismissed bundle", ids, second.HasMore)
	}
}

func TestWorkspaceClarificationUnansweredSettingsRoundTrip(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-defaults", Name: "defaults"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	created, err := repo.GetWorkspace(ctx, "ws-defaults")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	if created.ClarificationUnansweredInboxEnabled || created.ClarificationUnansweredRetentionDays != 14 {
		t.Fatalf("defaults = (%v, %d), want (false, 14)", created.ClarificationUnansweredInboxEnabled, created.ClarificationUnansweredRetentionDays)
	}

	enabled := true
	updated, err := repo.UpdateWorkspaceFields(ctx, "ws-defaults", models.WorkspaceFieldUpdate{
		ClarificationUnansweredInboxEnabled: &enabled,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateWorkspaceFields(enabled): %v", err)
	}
	if !updated.ClarificationUnansweredInboxEnabled || updated.ClarificationUnansweredRetentionDays != 14 {
		t.Fatalf("partial update = (%v, %d), want (true, 14)", updated.ClarificationUnansweredInboxEnabled, updated.ClarificationUnansweredRetentionDays)
	}
	days := 30
	updated, err = repo.UpdateWorkspaceFields(ctx, "ws-defaults", models.WorkspaceFieldUpdate{
		ClarificationUnansweredRetentionDays: &days,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateWorkspaceFields(retention): %v", err)
	}
	if !updated.ClarificationUnansweredInboxEnabled || updated.ClarificationUnansweredRetentionDays != 30 {
		t.Fatalf("retention update = (%v, %d), want (true, 30)", updated.ClarificationUnansweredInboxEnabled, updated.ClarificationUnansweredRetentionDays)
	}
	listed, err := repo.ListWorkspaces(ctx)
	if err != nil || len(listed) == 0 {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	for _, workspace := range listed {
		if workspace.ID == "ws-defaults" && workspace.ClarificationUnansweredRetentionDays != 30 {
			t.Fatalf("ListWorkspaces retention = %d, want 30", workspace.ClarificationUnansweredRetentionDays)
		}
	}
	tooLong := 91
	if _, err := repo.UpdateWorkspaceFields(ctx, "ws-defaults", models.WorkspaceFieldUpdate{
		ClarificationUnansweredRetentionDays: &tooLong,
	}, nil); err == nil {
		t.Fatal("retention 91 passed the schema check")
	}
}
