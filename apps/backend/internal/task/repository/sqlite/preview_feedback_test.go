//go:build cgo

package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type previewFeedbackRepositoryContract interface {
	ListTaskPreviewFeedback(context.Context, string) (*models.TaskPreviewFeedbackSnapshot, error)
	CreateTaskPreviewFeedback(context.Context, *models.TaskPreviewFeedback, string, string) (*models.TaskPreviewFeedbackSnapshot, error)
	UpdateTaskPreviewFeedback(context.Context, string, string, string, int64) (*models.TaskPreviewFeedbackSnapshot, error)
	DeleteTaskPreviewFeedback(context.Context, string, string, int64) (*models.TaskPreviewFeedbackSnapshot, []*models.TaskMessageAttachment, error)
	ClearTaskPreviewFeedback(context.Context, string, int64) (*models.TaskPreviewFeedbackSnapshot, []*models.TaskMessageAttachment, error)
}

func TestPreviewFeedbackScreenshotClaimAndClearAreAtomic(t *testing.T) {
	repo := newRepoForEntityTests(t)
	contract := requirePreviewFeedbackRepository(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-preview-feedback")
	seedAttachmentTask(t, repo, "task-preview-screenshot", "workspace-preview-feedback")
	attachment := &models.TaskMessageAttachment{
		ID: "preview-screenshot", OwnerID: "owner-preview", WorkspaceID: "workspace-preview-feedback",
		Name: "capture.png", MimeType: "image/png", Kind: "image", DeliveryMode: "prompt",
		SizeBytes: 128, StorageKey: "preview-screenshot", State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	item := &models.TaskPreviewFeedback{
		ID: "screenshot-feedback", TaskID: "task-preview-screenshot",
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "The form clips here",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Checkout",
		PageRoute: "/checkout", PageTitle: "Checkout",
		CaptureRect:            json.RawMessage(`{"x":1,"y":2,"width":320,"height":180}`),
		ScreenshotAttachmentID: attachment.ID,
	}
	created, err := contract.CreateTaskPreviewFeedback(
		ctx, item, attachment.OwnerID, attachment.WorkspaceID,
	)
	if err != nil || created.Revision != 1 || created.Items[0].ScreenshotAttachment == nil {
		t.Fatalf("create screenshot feedback = %#v, %v", created, err)
	}
	claimed, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil || claimed.State != models.AttachmentStateClaimed ||
		claimed.TaskID != item.TaskID || claimed.SessionID != "" {
		t.Fatalf("claimed attachment = %#v, %v", claimed, err)
	}

	stale, released, err := contract.ClearTaskPreviewFeedback(ctx, item.TaskID, 0)
	if !errors.Is(err, repoerrors.ErrTaskPreviewFeedbackChanged) || stale.Revision != 1 || len(released) != 0 {
		t.Fatalf("stale clear = %#v, released=%#v, err=%v", stale, released, err)
	}
	cleared, released, err := contract.ClearTaskPreviewFeedback(ctx, item.TaskID, 1)
	if err != nil || cleared.Revision != 2 || len(cleared.Items) != 0 || len(released) != 1 {
		t.Fatalf("clear = %#v, released=%#v, err=%v", cleared, released, err)
	}
	if released[0].State != models.AttachmentStateExpired {
		t.Fatalf("released attachment state = %q", released[0].State)
	}
}

func TestPreviewFeedbackScreenshotClaimRequiresPromptDelivery(t *testing.T) {
	repo := newRepoForEntityTests(t)
	contract := requirePreviewFeedbackRepository(t, repo)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-preview-feedback-mode")
	seedAttachmentTask(t, repo, "task-preview-screenshot-mode", "workspace-preview-feedback-mode")
	attachment := &models.TaskMessageAttachment{
		ID: "preview-screenshot-path", OwnerID: "owner-preview", WorkspaceID: "workspace-preview-feedback-mode",
		Name: "capture.png", MimeType: "image/png", Kind: "image", DeliveryMode: "path",
		SizeBytes: 128, StorageKey: "preview-screenshot-path", State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	item := &models.TaskPreviewFeedback{
		ID: "screenshot-feedback-path", TaskID: "task-preview-screenshot-mode",
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "The form clips here",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Checkout",
		PageRoute: "/checkout", PageTitle: "Checkout",
		CaptureRect:            json.RawMessage(`{"x":1,"y":2,"width":320,"height":180}`),
		ScreenshotAttachmentID: attachment.ID,
	}

	if _, err := contract.CreateTaskPreviewFeedback(
		ctx, item, attachment.OwnerID, attachment.WorkspaceID,
	); !errors.Is(err, models.ErrAttachmentClaimConflict) {
		t.Fatalf("CreateTaskPreviewFeedback error = %v, want attachment conflict", err)
	}
	stored, err := repo.GetMessageAttachment(ctx, attachment.ID)
	if err != nil || stored.State != models.AttachmentStateStaged {
		t.Fatalf("attachment after rejected claim = %#v, %v", stored, err)
	}
}

func requirePreviewFeedbackRepository(t *testing.T, repo *Repository) previewFeedbackRepositoryContract {
	t.Helper()
	contract, ok := any(repo).(previewFeedbackRepositoryContract)
	if !ok {
		t.Fatal("Repository does not implement task preview feedback persistence")
	}
	return contract
}

func TestPreviewFeedbackSchemaExistsOnFreshDatabase(t *testing.T) {
	repo := newRepoForEntityTests(t)

	for _, table := range []string{
		"task_preview_feedback_collections",
		"task_preview_feedback",
		"task_preview_feedback_admissions",
	} {
		var count int
		if err := repo.db.Get(&count, `
			SELECT COUNT(*) FROM sqlite_master
			WHERE type = 'table' AND name = ?
		`, table); err != nil {
			t.Fatalf("inspect %s: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("%s table count = %d, want 1", table, count)
		}
	}
}

func TestPreviewFeedbackRepositoryCRUDUsesOptimisticVersions(t *testing.T) {
	repo := newRepoForEntityTests(t)
	contract := requirePreviewFeedbackRepository(t, repo)
	ctx := context.Background()
	seedTaskForDocs(t, repo, "task-preview-feedback")

	initial, err := contract.ListTaskPreviewFeedback(ctx, "task-preview-feedback")
	if err != nil {
		t.Fatalf("ListTaskPreviewFeedback: %v", err)
	}
	if initial.Revision != 0 || len(initial.Items) != 0 {
		t.Fatalf("initial snapshot = %#v", initial)
	}

	item := &models.TaskPreviewFeedback{
		ID: "feedback-1", TaskID: "task-preview-feedback",
		Kind: models.TaskPreviewFeedbackText, Comment: "Increase contrast",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/products?plan=pro", PageTitle: "Products",
		SelectedText: "Choose a plan", TextAnchor: json.RawMessage(`{"start":{"path":[0],"offset":0}}`),
		CaptureRect: json.RawMessage(`{"x":20,"y":40,"width":120,"height":24}`),
	}
	created, err := contract.CreateTaskPreviewFeedback(ctx, item, "", "")
	if err != nil {
		t.Fatalf("CreateTaskPreviewFeedback: %v", err)
	}
	if created.Revision != 1 || len(created.Items) != 1 || created.Items[0].Version != 1 {
		t.Fatalf("created snapshot = %#v", created)
	}

	replayed, err := contract.CreateTaskPreviewFeedback(ctx, item, "", "")
	if err != nil || replayed.Revision != 1 {
		t.Fatalf("idempotent replay = %#v, %v", replayed, err)
	}

	updated, err := contract.UpdateTaskPreviewFeedback(ctx, item.TaskID, item.ID, "Use the stronger color", 1)
	if err != nil {
		t.Fatalf("UpdateTaskPreviewFeedback: %v", err)
	}
	if updated.Revision != 2 || updated.Items[0].Version != 2 || updated.Items[0].Comment != "Use the stronger color" {
		t.Fatalf("updated snapshot = %#v", updated)
	}

	stale, _, err := contract.DeleteTaskPreviewFeedback(ctx, item.TaskID, item.ID, 1)
	if err == nil || stale.Revision != 2 || len(stale.Items) != 1 {
		t.Fatalf("stale delete = %#v, %v", stale, err)
	}

	deleted, released, err := contract.DeleteTaskPreviewFeedback(ctx, item.TaskID, item.ID, 2)
	if err != nil || deleted.Revision != 3 || len(deleted.Items) != 0 || len(released) != 0 {
		t.Fatalf("deleted snapshot = %#v, released=%#v, err=%v", deleted, released, err)
	}
}

func TestPreviewFeedbackRepositoryRejectsInvalidCapture(t *testing.T) {
	repo := newRepoForEntityTests(t)
	contract := requirePreviewFeedbackRepository(t, repo)
	seedTaskForDocs(t, repo, "task-invalid-preview-feedback")

	item := &models.TaskPreviewFeedback{
		ID: "invalid-feedback", TaskID: "task-invalid-preview-feedback",
		Kind: models.TaskPreviewFeedbackText, Comment: "   ",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/", SelectedText: "Selected",
		TextAnchor: json.RawMessage(`{"start":{"path":[0],"offset":0}}`),
	}
	if snapshot, err := contract.CreateTaskPreviewFeedback(context.Background(), item, "", ""); err == nil {
		t.Fatalf("invalid capture was stored: %#v", snapshot)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_preview_feedback`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("stored invalid preview feedback rows = %d, want 0", count)
	}
}
