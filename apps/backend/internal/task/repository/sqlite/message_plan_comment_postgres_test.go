package sqlite

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresPlanCommentMessageAdmissionBoundaries(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	t.Run("failed message insert rolls back attachment claim", func(t *testing.T) {
		message, attachment := seedPostgresPlanCommentMessageFixture(t, ctx, repo, "rollback")
		message.TurnID = "missing-turn"
		_, err := repo.CreateMessageWithPlanComments(
			ctx, message, []models.TaskPlanCommentRef{{ID: "comment-pg-rollback", Version: 1}}, true,
			models.TaskSessionStateWaitingForInput, attachmentClaim(attachment),
		)
		if err == nil {
			t.Fatal("message admission with invalid turn succeeded")
		}
		assertStagedAttachment(t, ctx, repo, attachment.ID)
		pending, listErr := repo.ListTaskPlanComments(ctx, message.TaskID)
		if listErr != nil || len(pending.Comments) != 1 {
			t.Fatalf("comments after rolled-back admission = %#v, err=%v", pending, listErr)
		}
	})

	t.Run("terminal session rejects without consuming", func(t *testing.T) {
		message, attachment := seedPostgresPlanCommentMessageFixture(t, ctx, repo, "terminal")
		refs := []models.TaskPlanCommentRef{{ID: "comment-pg-terminal", Version: 1}}
		if err := repo.ValidateMessagePlanComments(
			ctx, message.TaskID, message.TaskSessionID, message.Content, refs, true,
			models.TaskSessionStateWaitingForInput,
		); err != nil {
			t.Fatal(err)
		}
		if err := repo.UpdateTaskSessionState(ctx, message.TaskSessionID, models.TaskSessionStateCompleted, ""); err != nil {
			t.Fatal(err)
		}
		_, err := repo.CreateMessageWithPlanComments(
			ctx, message, refs, true,
			models.TaskSessionStateWaitingForInput, attachmentClaim(attachment),
		)
		if !errors.Is(err, repoerrors.ErrTaskSessionUnavailable) {
			t.Fatalf("terminal admission error = %v", err)
		}
		assertStagedAttachment(t, ctx, repo, attachment.ID)
		pending, listErr := repo.ListTaskPlanComments(ctx, message.TaskID)
		if listErr != nil || len(pending.Comments) != 1 {
			t.Fatalf("comments after terminal admission = %#v, err=%v", pending, listErr)
		}
	})

	t.Run("two repositories preserve losing attachment and comment", func(t *testing.T) {
		messageA, attachmentA := seedPostgresPlanCommentMessageFixture(t, ctx, repo, "race")
		attachmentB := seedPostgresPlanCommentAttachment(t, ctx, repo, "race", "b")
		if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
			ID: "comment-pg-race-b", TaskID: messageA.TaskID, PlanID: "plan-pg-race",
			Body: "stored b", SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
		}); err != nil {
			t.Fatal(err)
		}
		secondDB := openSecondPostgresConnection(t, dsn, db)
		repoB, err := NewWithDB(secondDB, secondDB, nil)
		if err != nil {
			t.Fatal(err)
		}
		messageB := *messageA
		messageA.Metadata = map[string]interface{}{plancomments.MetadataRequestFingerprint: "fingerprint-a"}
		messageB.Metadata = map[string]interface{}{plancomments.MetadataRequestFingerprint: "fingerprint-b"}
		refs := [][]models.TaskPlanCommentRef{
			{{ID: "comment-pg-race", Version: 1}},
			{{ID: "comment-pg-race-b", Version: 1}},
		}
		messages := []*models.Message{messageA, &messageB}
		attachments := []*models.TaskMessageAttachment{attachmentA, attachmentB}
		repositories := []*Repository{repo, repoB}
		errs := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range repositories {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				<-start
				admissionCtx := plancommenttx.WithLocalAdmission(ctx, messageA.TaskID)
				_, errs[index] = repositories[index].CreateMessageWithPlanComments(
					admissionCtx, messages[index], refs[index], true, models.TaskSessionStateWaitingForInput,
					attachmentClaim(attachments[index]),
				)
			}(i)
		}
		close(start)
		wg.Wait()
		if (errs[0] == nil) == (errs[1] == nil) {
			t.Fatalf("concurrent message admissions = %#v, want one success", errs)
		}
		winner, loser := 0, 1
		if errs[1] == nil {
			winner, loser = 1, 0
		}
		storedWinner, err := repo.GetMessageAttachment(ctx, attachments[winner].ID)
		if err != nil {
			t.Fatal(err)
		}
		if storedWinner.State != models.AttachmentStateClaimed || storedWinner.MessageID != messageA.ID {
			t.Fatalf("winner attachment = %+v", storedWinner)
		}
		assertStagedAttachment(t, ctx, repo, attachments[loser].ID)
		pending, err := repo.ListTaskPlanComments(ctx, messageA.TaskID)
		if err != nil || len(pending.Comments) != 1 || pending.Comments[0].ID != refs[loser][0].ID {
			t.Fatalf("losing comment = %#v, err=%v", pending, err)
		}
	})
}

func TestPostgresPlanCommentAdmissionLeaseBlocksMutationThroughCommit(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConn(t, dsn, 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	message, attachment := seedPostgresPlanCommentMessageFixture(t, ctx, repo, "lease")

	secondDB := openSecondPostgresConnection(t, dsn, db)
	secondRepo, err := NewWithDB(secondDB, secondDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	observerDB := openSecondPostgresConnection(t, dsn, db)
	admissionCtx, release, err := repo.AcquirePlanCommentAdmission(ctx, message.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	released := false
	defer func() {
		if !released {
			release()
		}
	}()
	refs := []models.TaskPlanCommentRef{{ID: "comment-pg-lease", Version: 1}}
	if err := repo.ValidateMessagePlanComments(
		admissionCtx, message.TaskID, message.TaskSessionID, message.Content, refs, true,
		models.TaskSessionStateWaitingForInput,
	); err != nil {
		t.Fatal(err)
	}

	mutationPID := pgBackendPID(t, secondDB)
	mutationDone := make(chan struct{})
	mutationResult := make(chan error, 1)
	go func() {
		defer close(mutationDone)
		_, updateErr := secondRepo.UpdateTaskPlanComment(
			plancommenttx.WithLocalAdmission(ctx, message.TaskID),
			&models.TaskPlanComment{
				ID: "comment-pg-lease", TaskID: message.TaskID, PlanID: "plan-pg-lease",
				Body: "changed while the prompt was preparing",
			},
			1,
		)
		mutationResult <- updateErr
	}()
	if err := waitForPostgresLock(ctx, observerDB, mutationPID, mutationDone); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateMessageWithPlanComments(
		admissionCtx, message, refs, true, models.TaskSessionStateWaitingForInput,
		attachmentClaim(attachment),
	); err != nil {
		t.Fatal(err)
	}
	release()
	released = true

	if err := <-mutationResult; !errors.Is(err, repoerrors.ErrTaskPlanCommentsChanged) {
		t.Fatalf("post-admission mutation error = %v", err)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateClaimed || stored.MessageID != message.ID {
		t.Fatalf("committed attachment = %+v", stored)
	}
}

func TestPostgresCombinedPlanCommentMessageAdmissionLeavesWriteConnection(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConn(t, dsn, 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	message, attachment := seedPostgresPlanCommentMessageFixture(t, ctx, repo, "combined-lease")
	admissionCtx, release, err := repo.AcquirePlanCommentAndMessageAdmission(
		ctx, message.TaskID, message.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	refs := []models.TaskPlanCommentRef{{ID: "comment-pg-combined-lease", Version: 1}}
	if err := repo.ValidateMessagePlanComments(
		admissionCtx, message.TaskID, message.TaskSessionID, message.Content, refs, true,
		models.TaskSessionStateWaitingForInput,
	); err != nil {
		t.Fatalf("validate through combined lease: %v", err)
	}
	if _, err := repo.CreateMessageWithPlanComments(
		admissionCtx, message, refs, true, models.TaskSessionStateWaitingForInput,
		attachmentClaim(attachment),
	); err != nil {
		t.Fatalf("commit through combined lease: %v", err)
	}
}

func TestPostgresPlanCommentMessageAdmissionLosesToArchive(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := openIsolatedPostgresMultiConn(t, dsn, 3)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	message, attachment := seedPostgresPlanCommentMessageFixture(t, ctx, repo, "archive-race")

	// Repository initialization takes schema locks, so it precedes the row-lock barrier.
	admissionDB := openSecondPostgresConnection(t, dsn, db)
	admissionRepo, err := NewWithDB(admissionDB, admissionDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	observerDB := openSecondPostgresConnection(t, dsn, db)
	admissionPID := pgBackendPID(t, admissionDB)

	archiveTx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = archiveTx.Rollback() }()
	if _, err := archiveTx.ExecContext(ctx, db.Rebind(`
		UPDATE tasks SET archived_at = ?, updated_at = ? WHERE id = ?
	`), time.Now().UTC(), time.Now().UTC(), message.TaskID); err != nil {
		t.Fatal(err)
	}

	admissionDone := make(chan struct{})
	admissionResult := make(chan error, 1)
	go func() {
		defer close(admissionDone)
		_, createErr := admissionRepo.CreateMessageWithPlanComments(
			ctx,
			message,
			[]models.TaskPlanCommentRef{{ID: "comment-pg-archive-race", Version: 1}},
			true,
			models.TaskSessionStateWaitingForInput,
			attachmentClaim(attachment),
		)
		admissionResult <- createErr
	}()
	if err := waitForPostgresLock(ctx, observerDB, admissionPID, admissionDone); err != nil {
		t.Fatal(err)
	}
	if err := archiveTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-admissionResult; !errors.Is(err, messagequeue.ErrTaskInactive) {
		t.Fatalf("post-archive admission error = %v, want ErrTaskInactive", err)
	}
	assertStagedAttachment(t, ctx, repo, attachment.ID)
	pending, err := repo.ListTaskPlanComments(ctx, message.TaskID)
	if err != nil || len(pending.Comments) != 1 {
		t.Fatalf("post-archive comments = %#v, err=%v", pending, err)
	}
}

func TestPostgresPlanCommentAdmissionLeaseRejectsSingleConnectionPool(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = repo.AcquirePlanCommentAdmission(context.Background(), "task-single-connection")
	if err == nil || !strings.Contains(err.Error(), "at least two database connections") {
		t.Fatalf("single-connection admission error = %v", err)
	}
}

func seedPostgresPlanCommentMessageFixture(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	suffix string,
) (*models.Message, *models.TaskMessageAttachment) {
	t.Helper()
	workspaceID := "workspace-pg-" + suffix
	taskID := "task-pg-" + suffix
	sessionID := "session-pg-" + suffix
	turnID := "turn-pg-" + suffix
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSessionPrimary(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: sessionID, TaskID: taskID}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{ID: "plan-pg-" + suffix, TaskID: taskID, Content: "Plan"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-pg-" + suffix, TaskID: taskID, PlanID: "plan-pg-" + suffix,
		Body: "stored " + suffix, SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
	message := &models.Message{
		ID: "message-pg-" + suffix, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: plancomments.WithPlaceholder("typed content"),
	}
	return message, seedPostgresPlanCommentAttachment(t, ctx, repo, suffix, "a")
}

func seedPostgresPlanCommentAttachment(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	suffix, idSuffix string,
) *models.TaskMessageAttachment {
	t.Helper()
	now := time.Now().UTC()
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-pg-" + suffix + "-" + idSuffix, OwnerID: "owner-pg-" + suffix,
		WorkspaceID: "workspace-pg-" + suffix, Name: "notes.txt", MimeType: "text/plain",
		Kind: "resource", DeliveryMode: "path", SizeBytes: 5, StorageKey: "attachment-pg-" + suffix + "-" + idSuffix,
		State: models.AttachmentStateStaged, ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	return attachment
}

func attachmentClaim(attachment *models.TaskMessageAttachment) *messagequeue.QueueAttachmentClaim {
	return &messagequeue.QueueAttachmentClaim{
		OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID},
	}
}

func assertStagedAttachment(t *testing.T, ctx context.Context, repo *Repository, id string) {
	t.Helper()
	stored, err := repo.GetMessageAttachment(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateStaged || stored.TaskID != "" || stored.SessionID != "" || stored.MessageID != "" {
		t.Fatalf("attachment changed after rejected admission = %+v", stored)
	}
}
