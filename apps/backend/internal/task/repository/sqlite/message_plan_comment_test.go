package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type planCommentMessageRepository interface {
	CreateMessageWithPlanComments(
		context.Context,
		*models.Message,
		[]models.TaskPlanCommentRef,
		bool,
		models.TaskSessionState,
		*messagequeue.QueueAttachmentClaim,
	) (*models.TaskPlanCommentSnapshot, error)
}

type queuedPlanCommentMessageRepository interface {
	CreateMessageWithPlanCommentsAndQueue(
		context.Context,
		*models.Message,
		*messagequeue.QueuedMessage,
		[]models.TaskPlanCommentRef,
		bool,
		models.TaskSessionState,
		*messagequeue.QueueAttachmentClaim,
		int,
	) (*models.TaskPlanCommentSnapshot, error)
}

type planCommentAdmissionLocker interface {
	AcquirePlanCommentAdmission(context.Context, string) (context.Context, func(), error)
}

func requirePlanCommentMessageRepository(t *testing.T, repo *Repository) planCommentMessageRepository {
	t.Helper()
	contract, ok := any(repo).(planCommentMessageRepository)
	if !ok {
		t.Fatal("Repository does not implement atomic plan-comment message creation")
	}
	return contract
}

func TestCreateMessageWithPlanCommentsConsumesAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "atomic")
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("atomic", "message-plan-comments")

	snapshot, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-atomic", Version: 1}}, true, "", nil)
	if err != nil {
		t.Fatalf("CreateMessageWithPlanComments: %v", err)
	}
	if snapshot.Revision != 2 || len(snapshot.Comments) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	want := "### Plan Comments\n\n```\nselected\n```\n> stored atomic\n\n---\n\ntyped content"
	if message.Content != want {
		t.Fatalf("message content = %q, want %q", message.Content, want)
	}
	stored, err := repo.GetMessageWithPromptIndex(ctx, message.ID)
	if err != nil || stored.Content != want || stored.PromptIndex != 1 {
		t.Fatalf("stored message = %#v, err=%v", stored, err)
	}
	var storedMetadata string
	if err := repo.db.GetContext(ctx, &storedMetadata, repo.db.Rebind(
		`SELECT metadata FROM task_session_messages WHERE id = ?`,
	), message.ID); err != nil {
		t.Fatalf("read stored metadata: %v", err)
	}
	if storedMetadata != "{}" {
		t.Fatalf("stored metadata = %q, want an empty JSON object", storedMetadata)
	}
}

func TestCreateMessageWithPlanCommentsRollsBackOnStaleReference(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "stale")
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("stale", "message-plan-comments-stale")

	snapshot, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-stale", Version: 9}}, false, "", nil)
	var changed *plancommenttx.CommentsChangedError
	if !errors.As(err, &changed) || snapshot != nil || changed.Snapshot == nil {
		t.Fatalf("stale result snapshot=%#v err=%#v", snapshot, err)
	}
	if _, err := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rolled-back message lookup error = %v, want sql.ErrNoRows", err)
	}
	pending, err := repo.ListTaskPlanComments(ctx, "task-message-comments-stale")
	if err != nil || pending.Revision != 1 || len(pending.Comments) != 1 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, err)
	}
}

func TestCreateMessageWithPlanCommentsRejectsArchivedTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "archived")
	message := planCommentMessage("archived", "message-plan-comments-archived")
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
		UPDATE tasks SET archived_at = ?, updated_at = ? WHERE id = ?
	`), time.Now().UTC(), time.Now().UTC(), message.TaskID); err != nil {
		t.Fatal(err)
	}

	snapshot, err := repo.CreateMessageWithPlanComments(
		ctx,
		message,
		[]models.TaskPlanCommentRef{{ID: "comment-archived", Version: 1}},
		true,
		models.TaskSessionStateWaitingForInput,
		nil,
	)
	if !errors.Is(err, messagequeue.ErrTaskInactive) || snapshot != nil {
		t.Fatalf("archived admission snapshot=%#v err=%v, want ErrTaskInactive", snapshot, err)
	}
	if _, lookupErr := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(lookupErr, sql.ErrNoRows) {
		t.Fatalf("archived admission persisted message: %v", lookupErr)
	}
	pending, listErr := repo.ListTaskPlanComments(ctx, message.TaskID)
	if listErr != nil || len(pending.Comments) != 1 {
		t.Fatalf("archived admission comments=%#v err=%v", pending, listErr)
	}
}

func TestPlanCommentAdmissionLeaseSerializesPreflightThroughCommit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "lease")
	locker, ok := any(repo).(planCommentAdmissionLocker)
	if !ok {
		t.Fatal("Repository does not implement plan-comment admission leases")
	}
	admissionCtx, release, err := locker.AcquirePlanCommentAdmission(ctx, "task-message-comments-lease")
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	refs := []models.TaskPlanCommentRef{{ID: "comment-lease", Version: 1}}
	if err := repo.ValidateMessagePlanComments(
		admissionCtx, "task-message-comments-lease", "session-message-comments-lease",
		plancomments.WithPlaceholder("typed content"), refs, true, "",
	); err != nil {
		t.Fatal(err)
	}
	updateDone := make(chan error, 1)
	go func() {
		_, updateErr := repo.UpdateTaskPlanComment(ctx, &models.TaskPlanComment{
			ID: "comment-lease", TaskID: "task-message-comments-lease", PlanID: "plan-lease", Body: "changed",
		}, 1)
		updateDone <- updateErr
	}()
	select {
	case updateErr := <-updateDone:
		t.Fatalf("comment update crossed active admission lease: %v", updateErr)
	case <-time.After(50 * time.Millisecond):
	}
	message := planCommentMessage("lease", "message-plan-comments-lease")
	if _, err := repo.CreateMessageWithPlanComments(admissionCtx, message, refs, true, "", nil); err != nil {
		t.Fatal(err)
	}
	release()
	released = true
	select {
	case updateErr := <-updateDone:
		if !errors.Is(updateErr, repoerrors.ErrTaskPlanCommentsChanged) {
			t.Fatalf("post-admission update error = %v", updateErr)
		}
	case <-time.After(time.Second):
		t.Fatal("comment update did not resume after admission lease release")
	}
}

func TestPlanCommentAdmissionLeaseBlocksPlanDeletionThroughCommit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "delete-lease")
	admissionCtx, release, err := repo.AcquirePlanCommentAdmission(
		ctx, "task-message-comments-delete-lease",
	)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	refs := []models.TaskPlanCommentRef{{ID: "comment-delete-lease", Version: 1}}
	if err := repo.ValidateMessagePlanComments(
		admissionCtx, "task-message-comments-delete-lease", "session-message-comments-delete-lease",
		plancomments.WithPlaceholder("typed content"), refs, true, "",
	); err != nil {
		t.Fatal(err)
	}
	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- repo.DeleteTaskPlan(ctx, "task-message-comments-delete-lease")
	}()
	select {
	case deleteErr := <-deleteDone:
		t.Fatalf("plan deletion crossed active admission lease: %v", deleteErr)
	case <-time.After(50 * time.Millisecond):
	}
	message := planCommentMessage("delete-lease", "message-plan-comments-delete-lease")
	if _, err := repo.CreateMessageWithPlanComments(
		admissionCtx, message, refs, true, "", nil,
	); err != nil {
		t.Fatal(err)
	}
	release()
	released = true
	if err := <-deleteDone; err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetMessageWithPromptIndex(ctx, message.ID); err != nil {
		t.Fatalf("accepted message disappeared after plan deletion: %v", err)
	}
}

func TestCreateMessageWithPlanCommentsRollsBackAttachmentClaimWhenInsertFails(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	attachment := seedPlanCommentMessageAttachment(t, ctx, repo, "attachment-rollback", "rollback")
	message := planCommentMessage("attachment-rollback", "message-plan-comments-attachment-rollback")
	if _, err := repo.db.ExecContext(ctx, `
		CREATE TRIGGER fail_plan_comment_message_insert
		BEFORE INSERT ON task_session_messages
		BEGIN
			SELECT RAISE(ABORT, 'forced message insert failure');
		END
	`); err != nil {
		t.Fatal(err)
	}

	_, err := requirePlanCommentMessageRepository(t, repo).CreateMessageWithPlanComments(
		ctx, message, []models.TaskPlanCommentRef{{ID: "comment-attachment-rollback", Version: 1}}, true, "",
		&messagequeue.QueueAttachmentClaim{
			OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID},
		},
	)
	if err == nil {
		t.Fatal("message admission with forced insert failure succeeded")
	}
	stored, lookupErr := repo.GetMessageAttachment(ctx, attachment.ID)
	if lookupErr != nil {
		t.Fatal(lookupErr)
	}
	if stored.State != models.AttachmentStateStaged || stored.TaskID != "" || stored.SessionID != "" || stored.MessageID != "" {
		t.Fatalf("attachment changed after rolled-back message admission: %+v", stored)
	}
}

func TestCreateMessageWithPlanCommentsClaimsAttachmentOnCommit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	attachment := seedPlanCommentMessageAttachment(t, ctx, repo, "attachment-commit", "commit")
	message := planCommentMessage("attachment-commit", "message-plan-comments-attachment-commit")

	_, err := requirePlanCommentMessageRepository(t, repo).CreateMessageWithPlanComments(
		ctx, message, []models.TaskPlanCommentRef{{ID: "comment-attachment-commit", Version: 1}}, true, "",
		&messagequeue.QueueAttachmentClaim{
			OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateClaimed || stored.TaskID != message.TaskID ||
		stored.SessionID != message.TaskSessionID || stored.MessageID != message.ID {
		t.Fatalf("committed attachment claim = %+v", stored)
	}
}

func TestCreateMessageWithPlanCommentsMessageIDConflictRollsBackLosingAttachment(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	winnerAttachment := seedPlanCommentMessageAttachment(t, ctx, repo, "attachment-conflict", "winner")
	loserAttachment := seedPlanCommentMessageAttachmentOnly(t, ctx, repo, "attachment-conflict", "loser")
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-attachment-conflict-loser", TaskID: "task-message-comments-attachment-conflict",
		PlanID: "plan-attachment-conflict", Body: "loser", SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
	writes := requirePlanCommentMessageRepository(t, repo)
	winner := planCommentMessage("attachment-conflict", "message-plan-comments-attachment-conflict")
	winner.Metadata = map[string]interface{}{plancomments.MetadataRequestFingerprint: "winner"}
	if _, err := writes.CreateMessageWithPlanComments(
		ctx, winner, []models.TaskPlanCommentRef{{ID: "comment-attachment-conflict", Version: 1}}, true, "",
		&messagequeue.QueueAttachmentClaim{
			OwnerID: winnerAttachment.OwnerID, WorkspaceID: winnerAttachment.WorkspaceID, IDs: []string{winnerAttachment.ID},
		},
	); err != nil {
		t.Fatal(err)
	}
	loser := planCommentMessage("attachment-conflict", winner.ID)
	loser.Metadata = map[string]interface{}{plancomments.MetadataRequestFingerprint: "loser"}
	if _, err := writes.CreateMessageWithPlanComments(
		ctx, loser, []models.TaskPlanCommentRef{{ID: "comment-attachment-conflict-loser", Version: 1}}, true, "",
		&messagequeue.QueueAttachmentClaim{
			OwnerID: loserAttachment.OwnerID, WorkspaceID: loserAttachment.WorkspaceID, IDs: []string{loserAttachment.ID},
		},
	); err == nil {
		t.Fatal("conflicting message ID admission succeeded")
	}
	storedWinner, err := repo.GetMessageAttachment(ctx, winnerAttachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedLoser, err := repo.GetMessageAttachment(ctx, loserAttachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedWinner.State != models.AttachmentStateClaimed || storedWinner.MessageID != winner.ID {
		t.Fatalf("winner attachment = %+v", storedWinner)
	}
	if storedLoser.State != models.AttachmentStateStaged || storedLoser.TaskID != "" || storedLoser.MessageID != "" {
		t.Fatalf("loser attachment changed = %+v", storedLoser)
	}
	pending, err := repo.ListTaskPlanComments(ctx, loser.TaskID)
	if err != nil || len(pending.Comments) != 1 || pending.Comments[0].ID != "comment-attachment-conflict-loser" {
		t.Fatalf("pending comments after conflict = %#v, err=%v", pending, err)
	}
}

func TestCreateMessageWithPlanCommentsRejectsStalePrimary(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "primary")
	secondaryID := "session-message-comments-secondary"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: secondaryID, TaskID: "task-message-comments-primary", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("primary", "message-plan-comments-primary")
	message.TaskSessionID = secondaryID

	_, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-primary", Version: 1}}, true, "", nil)
	var changed *plancommenttx.PrimarySessionChangedError
	if !errors.As(err, &changed) || changed.SessionID != "session-message-comments-primary" {
		t.Fatalf("primary guard error = %#v", err)
	}
	pending, listErr := repo.ListTaskPlanComments(ctx, "task-message-comments-primary")
	if listErr != nil || len(pending.Comments) != 1 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, listErr)
	}
}

func TestCreateMessageWithPlanCommentsRejectsTerminalSessionAtAdmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "terminal")
	refs := []models.TaskPlanCommentRef{{ID: "comment-terminal", Version: 1}}
	if err := repo.UpdateTaskSessionState(
		ctx, "session-message-comments-terminal", models.TaskSessionStateWaitingForInput, "",
	); err != nil {
		t.Fatal(err)
	}
	if err := repo.ValidateMessagePlanComments(
		ctx, "task-message-comments-terminal", "session-message-comments-terminal",
		plancomments.WithPlaceholder("typed content"), refs, true,
		models.TaskSessionStateWaitingForInput,
	); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateTaskSessionState(
		ctx, "session-message-comments-terminal", models.TaskSessionStateCompleted, "",
	); err != nil {
		t.Fatal(err)
	}
	message := planCommentMessage("terminal", "message-plan-comments-terminal")

	snapshot, err := requirePlanCommentMessageRepository(t, repo).CreateMessageWithPlanComments(
		ctx, message, refs, true, models.TaskSessionStateWaitingForInput, nil,
	)
	if !errors.Is(err, repoerrors.ErrTaskSessionUnavailable) || snapshot != nil {
		t.Fatalf("terminal admission snapshot=%#v err=%v, want rejection", snapshot, err)
	}
	if _, lookupErr := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(lookupErr, sql.ErrNoRows) {
		t.Fatalf("terminal admission persisted message: %v", lookupErr)
	}
	pending, listErr := repo.ListTaskPlanComments(ctx, message.TaskID)
	if listErr != nil || len(pending.Comments) != 1 {
		t.Fatalf("terminal admission comments=%#v err=%v", pending, listErr)
	}
}

func TestCreateMessageWithPlanCommentsAndQueueIsAtomic(t *testing.T) {
	tests := []struct {
		name             string
		occupyQueueID    bool
		wantErr          error
		wantCommentCount int
		wantMessage      bool
	}{
		{name: "commits message queue and consumption", wantMessage: true},
		{
			name: "queue conflict rolls back message and consumption", occupyQueueID: true,
			wantErr: messagequeue.ErrQueueIDConflict, wantCommentCount: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			seedMessagePlanComment(t, ctx, repo, "queued")
			queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
			if err != nil {
				t.Fatal(err)
			}
			if test.occupyQueueID {
				err = queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
					ID: "message-plan-comments-queued", SessionID: "session-message-comments-queued",
					TaskID: "task-message-comments-queued", Content: "different", QueuedBy: messagequeue.QueuedByUser,
				}, 10)
				if err != nil {
					t.Fatal(err)
				}
			}
			writes, ok := any(repo).(queuedPlanCommentMessageRepository)
			if !ok {
				t.Fatal("Repository does not implement atomic queued plan-comment message creation")
			}
			message := planCommentMessage("queued", "message-plan-comments-queued")
			queued := &messagequeue.QueuedMessage{
				ID: message.ID, SessionID: message.TaskSessionID, TaskID: message.TaskID,
				Content: message.Content, Model: "test-model", PlanMode: true,
				Metadata: map[string]interface{}{"user_message_recorded": true}, QueuedBy: messagequeue.QueuedByUser,
			}

			snapshot, err := writes.CreateMessageWithPlanCommentsAndQueue(
				ctx, message, queued, []models.TaskPlanCommentRef{{ID: "comment-queued", Version: 1}}, true, "", nil, 10,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("CreateMessageWithPlanCommentsAndQueue error = %v, want %v", err, test.wantErr)
			}
			pending, listErr := repo.ListTaskPlanComments(ctx, message.TaskID)
			if listErr != nil || len(pending.Comments) != test.wantCommentCount {
				t.Fatalf("pending snapshot = %#v, err=%v", pending, listErr)
			}
			stored, messageErr := repo.GetMessageWithPromptIndex(ctx, message.ID)
			if test.wantMessage {
				if messageErr != nil || stored.Content != queued.Content || snapshot == nil || snapshot.Revision != 2 {
					t.Fatalf("stored message=%#v queued=%#v snapshot=%#v err=%v", stored, queued, snapshot, messageErr)
				}
			} else if !errors.Is(messageErr, sql.ErrNoRows) {
				t.Fatalf("rolled-back message lookup error = %v, want sql.ErrNoRows", messageErr)
			}
			entries, queueErr := queueRepo.ListBySession(ctx, message.TaskSessionID)
			if queueErr != nil {
				t.Fatal(queueErr)
			}
			if test.wantMessage {
				if len(entries) != 1 || entries[0].ID != message.ID || entries[0].Content != message.Content {
					t.Fatalf("queued entries = %#v", entries)
				}
			} else if len(entries) != 1 || entries[0].Content != "different" {
				t.Fatalf("conflicting queue entry changed: %#v", entries)
			}
		})
	}
}

func seedMessagePlanComment(t *testing.T, ctx context.Context, repo *Repository, suffix string) {
	t.Helper()
	taskID := "task-message-comments-" + suffix
	sessionID := "session-message-comments-" + suffix
	seedForMsgTest(t, repo, taskID, sessionID, "turn-message-comments-"+suffix)
	if err := repo.SetSessionPrimary(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-" + suffix, TaskID: taskID, Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-" + suffix, TaskID: taskID, PlanID: "plan-" + suffix,
		Body: "stored " + suffix, SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
}

func planCommentMessage(suffix, id string) *models.Message {
	return &models.Message{
		ID: id, TaskID: "task-message-comments-" + suffix,
		TaskSessionID: "session-message-comments-" + suffix,
		TurnID:        "turn-message-comments-" + suffix,
		AuthorType:    models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: plancomments.WithPlaceholder("typed content"),
	}
}

func seedPlanCommentMessageAttachment(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	suffix, attachmentSuffix string,
) *models.TaskMessageAttachment {
	t.Helper()
	seedMessagePlanComment(t, ctx, repo, suffix)
	seedWorkspace(t, repo, "workspace-"+suffix)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workspace_id = ? WHERE id = ?`,
	), "workspace-"+suffix, "task-message-comments-"+suffix); err != nil {
		t.Fatal(err)
	}
	return seedPlanCommentMessageAttachmentOnly(t, ctx, repo, suffix, attachmentSuffix)
}

func seedPlanCommentMessageAttachmentOnly(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	suffix, attachmentSuffix string,
) *models.TaskMessageAttachment {
	t.Helper()
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-message-comments-" + attachmentSuffix, OwnerID: "owner-" + suffix,
		WorkspaceID: "workspace-" + suffix, Name: "notes.txt", MimeType: "text/plain",
		Kind: "resource", DeliveryMode: "path", SizeBytes: 5,
		StorageKey: "attachment-message-comments-" + attachmentSuffix,
		State:      models.AttachmentStateStaged, ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	return attachment
}
