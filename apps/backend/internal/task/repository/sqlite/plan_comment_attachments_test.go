package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestPlanCommentFilesSurviveExpiryAndAreReleasedOnDelete(t *testing.T) {
	assertPlanCommentFileLifecycle(t, newRepoForSessionTests(t))
}

func TestPostgresPlanCommentFilesSurviveExpiryAndAreReleasedOnDelete(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	assertPlanCommentFileLifecycle(t, repo)
}

func assertPlanCommentFileLifecycle(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	seedWorkspace(t, repo, "workspace")
	seedAttachmentTask(t, repo, "task", "workspace")
	require.NoError(t, repo.CreateTaskPlan(ctx, &models.TaskPlan{ID: "plan", TaskID: "task", Content: "Plan"}))

	for _, id := range []string{"first", "second"} {
		require.NoError(t, repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
			ID: id, OwnerID: "owner", WorkspaceID: "workspace", Name: "same.txt",
			Kind: "resource", MimeType: "text/plain", DeliveryMode: "path", SizeBytes: 4, ExpiresAt: time.Now().UTC().Add(time.Hour),
		}))
	}

	comment := &models.TaskPlanComment{ID: "comment", TaskID: "task", PlanID: "plan", Body: "Feedback", AttachmentIDs: []string{"second", "first"}}
	snapshot, err := repo.CreateTaskPlanComment(ctx, comment)
	require.NoError(t, err)
	require.Equal(t, []string{"second", "first"}, snapshot.Comments[0].AttachmentIDs)

	reopened, err := NewWithDB(repo.db, repo.db, nil)
	require.NoError(t, err)
	reloaded, err := reopened.ListTaskPlanComments(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, []string{"second", "first"}, reloaded.Comments[0].AttachmentIDs)
	require.Equal(t, "same.txt", reloaded.Comments[0].Attachments[0].Name)
	require.ErrorIs(t, repo.ExpireStagedMessageAttachment(ctx, "first", "owner"), models.ErrAttachmentClaimConflict)

	expired, err := repo.MarkExpiredMessageAttachments(ctx, time.Now().UTC().Add(2*time.Hour))
	require.NoError(t, err)
	require.Empty(t, expired)

	edit := &models.TaskPlanComment{ID: "comment", TaskID: "task", PlanID: "plan", Body: "Edited"}
	snapshot, err = repo.UpdateTaskPlanComment(ctx, edit, 1)
	require.NoError(t, err)
	require.Equal(t, []string{"second", "first"}, snapshot.Comments[0].AttachmentIDs)

	edit.AttachmentIDs = []string{"first"}
	snapshot, err = repo.UpdateTaskPlanComment(ctx, edit, 2)
	require.NoError(t, err)
	require.Equal(t, []string{"first"}, snapshot.Comments[0].AttachmentIDs)
	released, err := repo.GetMessageAttachment(ctx, "second")
	require.NoError(t, err)
	require.Equal(t, models.AttachmentStateExpired, released.State)

	_, err = repo.DeleteTaskPlanComment(ctx, "task", "plan", "comment", 3)
	require.NoError(t, err)
	expired, err = repo.MarkExpiredMessageAttachments(ctx, time.Now().UTC().Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, expired, 2)
}

func TestPlanCommentFilesTransferWithQueuedPrompt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	_, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	ordinary := seedPlanCommentMessageAttachment(t, ctx, repo, "plan-files", "ordinary")
	message := planCommentMessage("plan-files", "message-files")
	for _, id := range []string{"first", "second"} {
		require.NoError(t, repo.CreateMessageAttachment(ctx, &models.TaskMessageAttachment{
			ID: id, OwnerID: "owner", WorkspaceID: "workspace-plan-files", Name: "same.txt",
			Kind: "resource", MimeType: "text/plain", DeliveryMode: "path", SizeBytes: 4, ExpiresAt: time.Now().UTC().Add(time.Hour),
		}))
	}
	for i, id := range []string{"first", "second"} {
		_, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
			ID: id + "-comment", TaskID: message.TaskID, PlanID: "plan-plan-files",
			Body: fmt.Sprintf("Feedback %d", i), AttachmentIDs: []string{id},
		})
		require.NoError(t, err)
	}

	queued := &messagequeue.QueuedMessage{
		ID: "queue-files", TaskID: message.TaskID, SessionID: message.TaskSessionID,
		QueuedBy: messagequeue.QueuedByUser,
	}
	refs := []models.TaskPlanCommentRef{{ID: "first-comment", Version: 1}, {ID: "second-comment", Version: 1}}

	// The submitted size cannot override the canonical registry size.
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE task_message_attachments SET size_bytes = ? WHERE id = ?`), models.MaxMessageAttachmentBytes, ordinary.ID)
	require.NoError(t, err)
	message.Metadata = map[string]interface{}{"attachments": []v1.MessageAttachment{{AttachmentID: ordinary.ID, SizeBytes: 1}}}
	claim := &messagequeue.QueueAttachmentClaim{IDs: []string{ordinary.ID}, OwnerID: ordinary.OwnerID, WorkspaceID: ordinary.WorkspaceID}
	_, err = repo.CreateMessageWithPlanCommentsAndQueue(ctx, message, queued, refs, true, "", claim, 2)
	require.ErrorIs(t, err, models.ErrAttachmentTotalTooLarge)
	for _, id := range []string{"first", "second"} {
		pending, err := repo.GetMessageAttachment(ctx, id)
		require.NoError(t, err)
		require.Equal(t, models.AttachmentStatePlanComment, pending.State)
		require.Empty(t, pending.SessionID)
	}
	message.Metadata = nil
	_, err = repo.CreateMessageWithPlanCommentsAndQueue(ctx, message, queued, refs, true, "", nil, 2)
	require.NoError(t, err)
	require.Contains(t, queued.Content, "Feedback 0\n\n<!-- kandev-comment-files:first-comment -->")
	require.Contains(t, queued.Content, "Feedback 1\n\n<!-- kandev-comment-files:second-comment -->")
	require.Len(t, queued.Attachments, 2)
	require.Equal(t, "first", queued.Attachments[0].AttachmentID)
	require.Equal(t, "first-comment", queued.Attachments[0].CommentID)
	require.Equal(t, "second", queued.Attachments[1].AttachmentID)
	require.Equal(t, "second-comment", queued.Attachments[1].CommentID)
	stored, err := repo.GetMessageWithPromptIndex(ctx, message.ID)
	require.NoError(t, err)
	require.Contains(t, stored.Metadata, "attachments")
	for _, id := range []string{"first", "second"} {
		got, err := repo.GetMessageAttachment(ctx, id)
		require.NoError(t, err)
		require.Equal(t, models.AttachmentStateClaimed, got.State)
		require.Equal(t, message.ID, got.MessageID)
		require.Equal(t, message.TaskSessionID, got.SessionID)
	}
}

func TestPlanCommentFilesRunReplayKeepsAcceptedDescriptors(t *testing.T) {
	repo := newRepoForSessionTests(t)
	queue, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	require.NoError(t, err)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-run-files"})
	file := seedPlanCommentMessageAttachment(t, ctx, repo, "run-files", "run-files")
	message := planCommentMessage("run-files", "message-files")
	comment := &models.TaskPlanComment{
		ID: "comment-run-files", TaskID: message.TaskID, PlanID: "plan-run-files",
		Body: "Feedback", AttachmentIDs: []string{file.ID},
	}
	_, err = repo.UpdateTaskPlanComment(ctx, comment, 1)
	require.NoError(t, err)
	identity, err := queue.ResolveSessionIdentity(ctx, message.TaskID, message.TaskSessionID)
	require.NoError(t, err)
	writer := queue.(interface {
		InsertWithPlanComments(context.Context, messagequeue.QueueSessionIdentity, *messagequeue.QueuedMessage,
			[]models.TaskPlanCommentRef, bool, *messagequeue.QueueAttachmentClaim, int) (*models.TaskPlanCommentSnapshot, bool, error)
	})
	candidate := messagequeue.QueuedMessage{ID: "run-file-entry", TaskID: message.TaskID, SessionID: message.TaskSessionID,
		QueuedBy: messagequeue.QueuedByUser, Content: message.Content,
		Metadata: map[string]interface{}{plancomments.MetadataRequestFingerprint: "run-file-fingerprint",
			plancomments.MetadataClientQueueID: "run-file-entry",
			plancomments.MetadataRefs:          []models.TaskPlanCommentRef{{ID: comment.ID, Version: 2}}},
	}
	accepted := candidate
	refs := []models.TaskPlanCommentRef{{ID: comment.ID, Version: 2}}
	snapshot, replay, err := writer.InsertWithPlanComments(ctx, identity, &accepted, refs, true, nil, 3)
	require.NoError(t, err)
	require.False(t, replay)
	require.Empty(t, snapshot.Comments)
	require.Len(t, accepted.Attachments, 1)
	require.Equal(t, comment.ID, accepted.Attachments[0].CommentID)
	require.Equal(t, file.ID, accepted.Attachments[0].AttachmentID)
	require.Contains(t, accepted.Content, "<!-- kandev-comment-files:comment-run-files -->")
	claimed, err := repo.GetMessageAttachment(ctx, file.ID)
	require.NoError(t, err)
	require.Equal(t, candidate.ID, claimed.QueueID)
	require.Equal(t, candidate.SessionID, claimed.SessionID)

	reserved, autoRun, err := queue.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.Equal(t, accepted.Attachments, reserved.Attachments)
	require.NoError(t, queue.AcknowledgeByIDForSession(ctx, identity, reserved))
	snapshot, replay, err = writer.InsertWithPlanComments(ctx, identity, &candidate, refs, true, nil, 3)
	require.NoError(t, err)
	require.True(t, replay)
	require.Nil(t, snapshot)
	require.Equal(t, accepted.Attachments, candidate.Attachments)
}

func TestPlanCommentFilesRejectIneligibleReplacementAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	seedWorkspace(t, repo, "workspace")
	seedWorkspace(t, repo, "foreign-workspace")
	seedAttachmentTask(t, repo, "task", "workspace")
	require.NoError(t, repo.CreateTaskPlan(ctx, &models.TaskPlan{ID: "plan", TaskID: "task", Content: "Plan"}))

	future := time.Now().UTC().Add(time.Hour)
	for _, file := range []models.TaskMessageAttachment{
		{ID: "kept", OwnerID: "owner", WorkspaceID: "workspace", DeliveryMode: "path", ExpiresAt: future},
		{ID: "expired", OwnerID: "owner", WorkspaceID: "workspace", DeliveryMode: "path", ExpiresAt: future.Add(-2 * time.Hour)},
		{ID: "foreign-owner", OwnerID: "other", WorkspaceID: "workspace", DeliveryMode: "path", ExpiresAt: future},
		{ID: "foreign-space", OwnerID: "owner", WorkspaceID: "foreign-workspace", DeliveryMode: "path", ExpiresAt: future},
		{ID: "prompt-file", OwnerID: "owner", WorkspaceID: "workspace", DeliveryMode: "prompt", ExpiresAt: future},
	} {
		require.NoError(t, repo.CreateMessageAttachment(ctx, &file))
	}

	initial := &models.TaskPlanComment{
		ID: "comment", TaskID: "task", PlanID: "plan", Body: "Original", AttachmentIDs: []string{"kept"},
	}
	_, err := repo.CreateTaskPlanComment(ctx, initial)
	require.NoError(t, err)
	for _, id := range []string{"expired", "foreign-owner", "foreign-space", "prompt-file", "missing"} {
		t.Run(id, func(t *testing.T) {
			update := &models.TaskPlanComment{
				ID: initial.ID, TaskID: initial.TaskID, PlanID: initial.PlanID, Body: "Changed", AttachmentIDs: []string{id},
			}
			_, err := repo.UpdateTaskPlanComment(ctx, update, 1)
			if id == "missing" {
				require.ErrorIs(t, err, models.ErrAttachmentNotFound)
			} else {
				require.ErrorIs(t, err, models.ErrAttachmentClaimConflict)
			}
			pending, err := repo.ListTaskPlanComments(ctx, "task")
			require.NoError(t, err)
			require.Equal(t, int64(1), pending.Revision)
			require.Equal(t, "Original", pending.Comments[0].Body)
			require.Equal(t, []string{"kept"}, pending.Comments[0].AttachmentIDs)
		})
	}
}
