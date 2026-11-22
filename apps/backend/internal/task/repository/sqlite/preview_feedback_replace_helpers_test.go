package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	replaceOwner     = "owner-replace"
	replaceWorkspace = "workspace-preview-replace"
	replaceTask      = "task-preview-replace"
)

func seedReplaceAttachment(t *testing.T, repo *Repository, id, ownerID string) {
	t.Helper()
	if err := repo.CreateMessageAttachment(context.Background(), &models.TaskMessageAttachment{
		ID: id, OwnerID: ownerID, WorkspaceID: replaceWorkspace,
		Name: "capture.png", MimeType: "image/png", Kind: "image", DeliveryMode: "prompt",
		SizeBytes: 128, StorageKey: id, State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func replacement(attachmentID string, version int64) *models.TaskPreviewFeedbackScreenshotReplacement {
	return &models.TaskPreviewFeedbackScreenshotReplacement{
		TaskID: replaceTask, ItemID: "replace-feedback", ExpectedVersion: version,
		CaptureRect:            json.RawMessage(`{"x":10,"y":20,"width":200,"height":100}`),
		ScreenshotAttachmentID: attachmentID,
	}
}
