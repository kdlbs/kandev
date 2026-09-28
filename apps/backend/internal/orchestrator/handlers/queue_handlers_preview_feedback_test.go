package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type replayPreviewFeedbackPreparer struct {
	calls   int
	err     error
	preview []v1.MessageAttachment
}

func (p *replayPreviewFeedbackPreparer) PreviewFeedbackAttachments(
	context.Context,
	string,
	[]models.TaskPreviewFeedbackRef,
) ([]v1.MessageAttachment, error) {
	p.calls++
	return p.preview, p.err
}

func (*replayPreviewFeedbackPreparer) ClaimMessageAttachments(
	context.Context,
	string,
	string,
	[]v1.MessageAttachment,
) error {
	return nil
}

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5
// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-003.6
func TestWsQueuePreviewFeedbackLostResponseReplay(t *testing.T) {
	taskRepo, queueRepo := newPreviewFeedbackQueueStores(t)
	ctx := context.Background()
	seedPreviewFeedbackQueueTask(t, ctx, taskRepo)
	preview := &models.TaskPreviewFeedback{
		ID:           "preview-handler-replay",
		TaskID:       "task-handler-replay",
		Kind:         models.TaskPreviewFeedbackText,
		Comment:      "Keep the heading visible",
		SourceKind:   models.TaskPreviewFeedbackBrowser,
		SourceLabel:  "Local app",
		PageRoute:    "/checkout",
		SelectedText: "Checkout",
		TextAnchor:   json.RawMessage(`{"start":{"node_path":[0],"offset":0},"end":{"node_path":[0],"offset":8}}`),
	}
	_, err := taskRepo.CreateTaskPreviewFeedback(ctx, preview, "", "")
	require.NoError(t, err)

	service := messagequeue.NewService(queueRepo, 10, logger.Default())
	service.SetAutoMergeEnabled(false)
	preparer := &replayPreviewFeedbackPreparer{}
	handlers := NewQueueHandlers(service, &mockEventBus{}, logger.Default(), nil, allowQueueIdentityAccess{}, nil)
	handlers.SetAttachmentClaimer(preparer)

	identity, err := service.ResolveSessionIdentity(ctx, preview.TaskID, "session-handler-replay")
	require.NoError(t, err)
	payload := map[string]interface{}{
		"task_id":                preview.TaskID,
		"session_id":             identity.SessionID,
		"session_incarnation_id": identity.SessionIncarnationID,
		"client_queue_id":        "queue-handler-replay",
		"content":                "Please review this",
		"preview_feedback_refs":  []models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}},
	}

	first, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type)
	require.Equal(t, 1, preparer.calls)

	reserved, autoRun, err := queueRepo.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.NotNil(t, reserved)
	require.NoError(t, queueRepo.AcknowledgeByIDForSession(ctx, identity, reserved))

	preparer.err = errors.New("pending feedback was consumed")
	newPending := &models.TaskPreviewFeedback{
		ID:           "preview-handler-replay-new",
		TaskID:       preview.TaskID,
		Kind:         models.TaskPreviewFeedbackText,
		Comment:      "Keep the footer visible",
		SourceKind:   models.TaskPreviewFeedbackBrowser,
		SourceLabel:  "Local app",
		PageRoute:    "/account",
		SelectedText: "Account",
		TextAnchor:   json.RawMessage(`{"start":{"node_path":[0],"offset":0},"end":{"node_path":[0],"offset":7}}`),
	}
	_, err = taskRepo.CreateTaskPreviewFeedback(ctx, newPending, "", "")
	require.NoError(t, err)
	second, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, second.Type)
	require.Equal(t, 1, preparer.calls, "an exact replay must not read consumed feedback")

	var replayed messagequeue.QueuedMessage
	require.NoError(t, json.Unmarshal(second.Payload, &replayed))
	require.Contains(t, replayed.Content, "Keep the heading visible")
	entries, err := queueRepo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Empty(t, entries)
	pending, err := taskRepo.ListTaskPreviewFeedback(ctx, preview.TaskID)
	require.NoError(t, err)
	require.Len(t, pending.Items, 1)
	require.Equal(t, newPending.ID, pending.Items[0].ID)
}

// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-003.5
// @covers AC-TASKS-WEB-PREVIEW-FEEDBACK-003.6
func TestWsQueuePreviewFeedbackScreenshotLostResponseReplay(t *testing.T) {
	taskRepo, queueRepo := newPreviewFeedbackQueueStores(t)
	ctx := context.Background()
	seedPreviewFeedbackQueueTask(t, ctx, taskRepo)
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-handler-replay", OwnerID: "owner-handler-replay",
		WorkspaceID: "workspace-handler-replay", Name: "checkout.png", MimeType: "image/png",
		Kind: "image", DeliveryMode: "prompt", SizeBytes: 128, StorageKey: "handler-replay",
		State: models.AttachmentStateStaged, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, taskRepo.CreateMessageAttachment(ctx, attachment))
	preview := &models.TaskPreviewFeedback{
		ID:                     "preview-handler-screenshot-replay",
		TaskID:                 "task-handler-replay",
		Kind:                   models.TaskPreviewFeedbackScreenshot,
		Comment:                "Move the checkout button",
		SourceKind:             models.TaskPreviewFeedbackBrowser,
		SourceLabel:            "Local app",
		PageRoute:              "/checkout",
		CaptureRect:            json.RawMessage(`{"x":4,"y":8,"width":320,"height":180}`),
		ScreenshotAttachmentID: attachment.ID,
	}
	_, err := taskRepo.CreateTaskPreviewFeedback(ctx, preview, attachment.OwnerID, attachment.WorkspaceID)
	require.NoError(t, err)

	service := messagequeue.NewService(queueRepo, 10, logger.Default())
	service.SetAutoMergeEnabled(false)
	preparer := &replayPreviewFeedbackPreparer{
		preview: []v1.MessageAttachment{{
			Type: "image", AttachmentID: attachment.ID, MimeType: attachment.MimeType,
			Name: attachment.Name, SizeBytes: attachment.SizeBytes, DeliveryMode: attachment.DeliveryMode,
		}},
	}
	handlers := NewQueueHandlers(service, &mockEventBus{}, logger.Default(), nil, allowQueueIdentityAccess{}, nil)
	handlers.SetAttachmentClaimer(preparer)

	identity, err := service.ResolveSessionIdentity(ctx, preview.TaskID, "session-handler-replay")
	require.NoError(t, err)
	payload := map[string]interface{}{
		"task_id":                preview.TaskID,
		"session_id":             identity.SessionID,
		"session_incarnation_id": identity.SessionIncarnationID,
		"client_queue_id":        "queue-handler-screenshot-replay",
		"content":                "Please review this image",
		"preview_feedback_refs":  []models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}},
	}

	first, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type)
	require.Equal(t, 1, preparer.calls)
	var firstQueued messagequeue.QueuedMessage
	require.NoError(t, json.Unmarshal(first.Payload, &firstQueued))
	require.Len(t, firstQueued.Attachments, 1)
	require.Equal(t, attachment.ID, firstQueued.Attachments[0].AttachmentID)

	reserved, autoRun, err := queueRepo.ReserveHeadIfAutoRunForSession(ctx, identity)
	require.NoError(t, err)
	require.True(t, autoRun)
	require.NotNil(t, reserved)
	require.NoError(t, queueRepo.AcknowledgeByIDForSession(ctx, identity, reserved))

	preparer.err = errors.New("pending screenshot feedback was consumed")
	second, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, second.Type)
	require.Equal(t, 1, preparer.calls, "an exact replay must not read consumed screenshot feedback")
	var replayed messagequeue.QueuedMessage
	require.NoError(t, json.Unmarshal(second.Payload, &replayed))
	require.Len(t, replayed.Attachments, 1)
	require.Equal(t, attachment.ID, replayed.Attachments[0].AttachmentID)
	entries, err := queueRepo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func newPreviewFeedbackQueueStores(t *testing.T) (*tasksqlite.Repository, messagequeue.Repository) {
	t.Helper()
	dbConn, err := dbutil.OpenSQLite(filepath.Join(t.TempDir(), "preview-feedback-queue.db"))
	require.NoError(t, err)
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	require.NoError(t, err)
	return taskRepo, queueRepo
}

func seedPreviewFeedbackQueueTask(t *testing.T, ctx context.Context, repo *tasksqlite.Repository) {
	t.Helper()
	workspaceID := "workspace-handler-replay"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Replay"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-handler-replay", WorkspaceID: workspaceID, Title: "Replay",
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-handler-replay", TaskID: "task-handler-replay",
		State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.SetSessionPrimary(ctx, "session-handler-replay"))
}
