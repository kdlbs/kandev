package handlers

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

func TestRestoreTaskMessageQueueOwnerTransfersClaimedAttachments(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := newTestTaskService(t)
	_, target, oldSession := seedTaskWithSession(t, taskSvc, repo, models.TaskSessionStateWaitingForInput)
	newSession := taskSessionForQueueTransferTest(target.ID)
	require.NoError(t, repo.CreateTaskSession(ctx, newSession))

	attachments, err := service.NewAttachmentService(repo, t.TempDir(), nil, testLogger(t))
	require.NoError(t, err)
	taskSvc.SetAttachmentService(attachments)
	attachment, err := attachments.Stage(ctx, "user-a", "ws-1", "prompt.txt", "text/plain", "resource", "", strings.NewReader("prompt"))
	require.NoError(t, err)
	require.NoError(t, attachments.Claim(ctx, "user-a", "ws-1", target.ID, oldSession.ID, []string{attachment.ID}))

	h, orch := newMessageTaskHandler(t, taskSvc, repo)
	_, err = orch.queue.QueueMessage(ctx, oldSession.ID, target.ID, "queued prompt", "", messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{{AttachmentID: attachment.ID}})
	require.NoError(t, err)

	require.NoError(t, h.restoreTaskMessageQueueOwner(ctx, target.ID, oldSession.ID, newSession.ID))

	moved, err := repo.GetMessageAttachment(ctx, attachment.ID)
	require.NoError(t, err)
	require.Equal(t, newSession.ID, moved.SessionID)
	status := orch.queue.GetStatus(ctx, newSession.ID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, attachment.ID, status.Entries[0].Attachments[0].AttachmentID)
}

func TestRestoreTaskMessageQueueOwnerFailsClosedWithoutAttachmentService(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := newTestTaskService(t)
	_, target, oldSession := seedTaskWithSession(t, taskSvc, repo, models.TaskSessionStateWaitingForInput)
	newSession := taskSessionForQueueTransferTest(target.ID)
	require.NoError(t, repo.CreateTaskSession(ctx, newSession))

	h, orch := newMessageTaskHandler(t, taskSvc, repo)
	_, err := orch.queue.QueueMessage(
		ctx, oldSession.ID, target.ID, "queued prompt", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "unavailable-attachment"}},
	)
	require.NoError(t, err)

	require.Error(t, h.restoreTaskMessageQueueOwner(ctx, target.ID, oldSession.ID, newSession.ID))
	status := orch.queue.GetStatus(ctx, oldSession.ID)
	require.Len(t, status.Entries, 1)
	require.Empty(t, orch.queue.GetStatus(ctx, newSession.ID).Entries)
}

func taskSessionForQueueTransferTest(taskID string) *models.TaskSession {
	return &models.TaskSession{
		ID:     "replacement-session",
		TaskID: taskID,
		State:  models.TaskSessionStateWaitingForInput,
	}
}
