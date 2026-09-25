package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresPreviewFeedbackLoadsScreenshotAttachmentAfterRowsClose(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	workspaceID := "workspace-preview-feedback-pg"
	taskID := "task-preview-feedback-pg"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: taskID}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	attachment := &models.TaskMessageAttachment{
		ID: "attachment-preview-feedback-pg", OwnerID: "owner-preview-feedback-pg",
		WorkspaceID: workspaceID, Name: "capture.png", MimeType: "image/png", Kind: "image",
		DeliveryMode: "prompt", SizeBytes: 128, StorageKey: "attachment-preview-feedback-pg",
		State: models.AttachmentStateStaged, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatalf("create attachment: %v", err)
	}
	item := &models.TaskPreviewFeedback{
		ID: "feedback-preview-feedback-pg", TaskID: taskID,
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "The screenshot is useful",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/checkout", PageTitle: "Checkout",
		CaptureRect:            json.RawMessage(`{"x":1,"y":2,"width":320,"height":180}`),
		ScreenshotAttachmentID: attachment.ID,
	}
	if _, err := repo.CreateTaskPreviewFeedback(ctx, item, attachment.OwnerID, workspaceID); err != nil {
		t.Fatalf("create preview feedback: %v", err)
	}
	snapshot, err := repo.ListTaskPreviewFeedback(ctx, taskID)
	if err != nil {
		t.Fatalf("list preview feedback: %v", err)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].ScreenshotAttachment == nil {
		t.Fatalf("snapshot = %#v, want screenshot attachment descriptor", snapshot)
	}
	if snapshot.Items[0].ScreenshotAttachment.ID != attachment.ID {
		t.Fatalf("screenshot attachment ID = %q, want %q", snapshot.Items[0].ScreenshotAttachment.ID, attachment.ID)
	}
}
