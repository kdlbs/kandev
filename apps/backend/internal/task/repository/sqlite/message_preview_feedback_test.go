//go:build cgo

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/previewfeedback"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type taskFeedbackMessageRepository interface {
	CreateMessageWithTaskFeedback(
		context.Context,
		*models.Message,
		[]models.TaskPlanCommentRef,
		[]models.TaskPreviewFeedbackRef,
		bool,
		models.TaskSessionState,
		*messagequeue.QueueAttachmentClaim,
	) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, error)
}

func TestCreateMessageWithTaskFeedbackConsumesMixedSnapshotsAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "mixed-preview")
	preview := seedMessagePreviewFeedback(t, ctx, repo, "mixed-preview")
	writes, ok := any(repo).(taskFeedbackMessageRepository)
	if !ok {
		t.Fatal("Repository does not implement atomic task-feedback message creation")
	}
	message := planCommentMessage("mixed-preview", "message-mixed-preview")

	planSnapshot, previewSnapshot, err := writes.CreateMessageWithTaskFeedback(
		ctx,
		message,
		[]models.TaskPlanCommentRef{{ID: "comment-mixed-preview", Version: 1}},
		[]models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}},
		true,
		"",
		nil,
	)
	if err != nil {
		t.Fatalf("CreateMessageWithTaskFeedback: %v", err)
	}
	if planSnapshot == nil || len(planSnapshot.Comments) != 0 ||
		previewSnapshot == nil || previewSnapshot.Revision != 2 || len(previewSnapshot.Items) != 0 {
		t.Fatalf("snapshots plan=%#v preview=%#v", planSnapshot, previewSnapshot)
	}
	for _, want := range []string{
		"### Plan Comments", "stored mixed-preview", "typed content",
		"### Web Preview Feedback", "Generated total", "Runtime value",
		"containing_element", "node_path", "scroll_y", "viewport_width",
	} {
		if !strings.Contains(message.Content, want) {
			t.Fatalf("message content missing %q:\n%s", want, message.Content)
		}
	}
	pending, err := repo.ListTaskPreviewFeedback(ctx, message.TaskID)
	if err != nil || len(pending.Items) != 0 || pending.Revision != 2 {
		t.Fatalf("pending preview snapshot=%#v err=%v", pending, err)
	}
}

func TestCreateMessageWithTaskFeedbackStalePreviewRollsBackPlanAndMessage(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "stale-preview")
	preview := seedMessagePreviewFeedback(t, ctx, repo, "stale-preview")
	message := planCommentMessage("stale-preview", "message-stale-preview")

	_, previewSnapshot, err := repo.CreateMessageWithTaskFeedback(
		ctx,
		message,
		[]models.TaskPlanCommentRef{{ID: "comment-stale-preview", Version: 1}},
		[]models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 9}},
		false,
		"",
		nil,
	)
	if !errors.Is(err, repoerrors.ErrTaskPreviewFeedbackChanged) ||
		previewSnapshot != nil {
		t.Fatalf("stale preview snapshot=%#v err=%v", previewSnapshot, err)
	}
	if _, err := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("message persisted after stale preview: %v", err)
	}
	planPending, err := repo.ListTaskPlanComments(ctx, message.TaskID)
	if err != nil || len(planPending.Comments) != 1 {
		t.Fatalf("plan comments changed on rollback: %#v err=%v", planPending, err)
	}
	previewPending, err := repo.ListTaskPreviewFeedback(ctx, message.TaskID)
	if err != nil || len(previewPending.Items) != 1 || previewPending.Revision != 1 {
		t.Fatalf("preview feedback changed on rollback: %#v err=%v", previewPending, err)
	}
}

func TestCreateMessageWithTaskFeedbackTransfersScreenshotToAcceptedMessage(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "screenshot-preview")
	seedWorkspace(t, repo, "workspace-screenshot-preview")
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workspace_id = ? WHERE id = ?`,
	), "workspace-screenshot-preview", "task-message-comments-screenshot-preview"); err != nil {
		t.Fatal(err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-screenshot-preview", OwnerID: "owner-screenshot-preview",
		WorkspaceID: "workspace-screenshot-preview", Name: "checkout.png", MimeType: "image/png",
		Kind: "image", DeliveryMode: "prompt", SizeBytes: 512, StorageKey: "attachment-screenshot-preview",
		State: models.AttachmentStateStaged, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	item := &models.TaskPreviewFeedback{
		ID: "preview-screenshot-transfer", TaskID: "task-message-comments-screenshot-preview",
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "Form is clipped",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/checkout", CaptureRect: json.RawMessage(`{"x":10,"y":20,"width":320,"height":180}`),
		ScreenshotAttachmentID: attachment.ID,
	}
	if _, err := repo.CreateTaskPreviewFeedback(
		ctx, item, attachment.OwnerID, attachment.WorkspaceID,
	); err != nil {
		t.Fatal(err)
	}
	message := planCommentMessage("screenshot-preview", "message-screenshot-preview")
	message.Content = "typed content"
	_, _, err := repo.CreateMessageWithTaskFeedback(
		ctx, message, nil, []models.TaskPreviewFeedbackRef{{ID: item.ID, Version: 1}},
		true, "", &messagequeue.QueueAttachmentClaim{
			OwnerID: attachment.OwnerID, WorkspaceID: attachment.WorkspaceID, IDs: []string{attachment.ID},
		},
	)
	if err != nil {
		t.Fatalf("CreateMessageWithTaskFeedback: %v", err)
	}
	transferred, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if transferred.State != models.AttachmentStateClaimed ||
		transferred.TaskID != item.TaskID || transferred.SessionID != message.TaskSessionID ||
		transferred.MessageID != message.ID {
		t.Fatalf("transferred attachment = %#v", transferred)
	}
}

func TestCreateMessageWithTaskFeedbackRejectsCombinedScreenshotAndOrdinaryAttachmentLimit(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "screenshot-limit")
	seedWorkspace(t, repo, "workspace-screenshot-limit")
	taskID := "task-message-comments-screenshot-limit"
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE tasks SET workspace_id = ? WHERE id = ?`,
	), "workspace-screenshot-limit", taskID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	screenshot := &models.TaskMessageAttachment{
		ID: "attachment-screenshot-limit", OwnerID: "owner-screenshot-limit",
		WorkspaceID: "workspace-screenshot-limit", Name: "screen.png", MimeType: "image/png",
		Kind: "image", DeliveryMode: "prompt", SizeBytes: previewfeedback.MaxScreenshotBytes,
		StorageKey: "attachment-screenshot-limit", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour),
	}
	ordinary := &models.TaskMessageAttachment{
		ID: "attachment-ordinary-limit", OwnerID: screenshot.OwnerID,
		WorkspaceID: screenshot.WorkspaceID, Name: "video.bin", MimeType: "application/octet-stream",
		Kind: "resource", DeliveryMode: "path",
		SizeBytes:  models.MaxMessageAttachmentBytes - previewfeedback.MaxScreenshotBytes + 1,
		StorageKey: "attachment-ordinary-limit", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour),
	}
	for _, attachment := range []*models.TaskMessageAttachment{screenshot, ordinary} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}
	item := &models.TaskPreviewFeedback{
		ID: "preview-screenshot-limit", TaskID: taskID,
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "Capture is clipped",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app", PageRoute: "/",
		CaptureRect:            json.RawMessage(`{"x":0,"y":0,"width":100,"height":100}`),
		ScreenshotAttachmentID: screenshot.ID,
	}
	if _, err := repo.CreateTaskPreviewFeedback(
		ctx, item, screenshot.OwnerID, screenshot.WorkspaceID,
	); err != nil {
		t.Fatal(err)
	}
	message := planCommentMessage("screenshot-limit", "message-screenshot-limit")
	_, _, err := repo.CreateMessageWithTaskFeedback(
		ctx, message, nil, []models.TaskPreviewFeedbackRef{{ID: item.ID, Version: 1}},
		true, "", &messagequeue.QueueAttachmentClaim{
			OwnerID: screenshot.OwnerID, WorkspaceID: screenshot.WorkspaceID,
			IDs: []string{screenshot.ID, ordinary.ID},
		},
	)
	if !errors.Is(err, models.ErrAttachmentTotalTooLarge) {
		t.Fatalf("CreateMessageWithTaskFeedback error = %v", err)
	}
	if _, err := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("message persisted after rejected combined limit: %v", err)
	}
	pending, err := repo.ListTaskPreviewFeedback(ctx, taskID)
	if err != nil || len(pending.Items) != 1 {
		t.Fatalf("preview feedback changed after rejected combined limit: %#v err=%v", pending, err)
	}
}

func seedMessagePreviewFeedback(
	t *testing.T,
	ctx context.Context,
	repo *Repository,
	suffix string,
) *models.TaskPreviewFeedback {
	t.Helper()
	item := &models.TaskPreviewFeedback{
		ID: "preview-" + suffix, TaskID: "task-message-comments-" + suffix,
		Kind: models.TaskPreviewFeedbackText, Comment: "Generated total",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/totals", PageTitle: "Totals", SelectedText: "Runtime value",
		TextAnchor: json.RawMessage(`{
			"start":{"selector":"#total","node_path":[0],"offset":0},
			"end":{"selector":"#total","node_path":[0],"offset":13},
			"rects":[{"x":10,"y":20,"width":90,"height":20}],
			"scroll_y":300,"viewport_width":1024,"viewport_height":768,
			"containing_element":{"tag":"span","outer_html":"<span id=\"total\">Runtime value</span>"}
		}`),
	}
	if _, err := repo.CreateTaskPreviewFeedback(ctx, item, "", ""); err != nil {
		t.Fatal(err)
	}
	return item
}
