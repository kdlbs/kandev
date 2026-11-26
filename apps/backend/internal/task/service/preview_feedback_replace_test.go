package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type recordingPreviewCleaner struct {
	deleted []*models.TaskMessageAttachment
}

func (c *recordingPreviewCleaner) DeleteDescriptors(
	_ context.Context,
	attachments []*models.TaskMessageAttachment,
) error {
	c.deleted = append(c.deleted, attachments...)
	return nil
}

func stageServiceScreenshot(t *testing.T, repo *sqliterepo.Repository, id string) {
	t.Helper()
	if err := repo.CreateMessageAttachment(context.Background(), &models.TaskMessageAttachment{
		ID: id, OwnerID: "owner-preview", WorkspaceID: "ws-plan",
		Name: "capture.png", MimeType: "image/png", Kind: "image", DeliveryMode: "prompt",
		SizeBytes: 128, StorageKey: id, State: models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewFeedbackServiceReplacesScreenshot(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := ctxAs("owner-preview")
	taskID := "task-preview-replace-service"
	seedTask(t, context.Background(), repo, taskID)
	validator := &recordingPreviewScreenshotValidator{}
	cleaner := &recordingPreviewCleaner{}
	svc.SetPreviewScreenshotValidator(validator)
	svc.SetPreviewAttachmentCleaner(cleaner)
	stageServiceScreenshot(t, repo, "service-shot-old")
	stageServiceScreenshot(t, repo, "service-shot-new")
	itemID := "0c3b2a93-5b5e-4a37-a3cf-0d1e64b0d2d1"
	if _, err := svc.CreatePreviewFeedback(ctx, &models.TaskPreviewFeedback{
		ID: itemID, TaskID: taskID, Kind: models.TaskPreviewFeedbackScreenshot,
		Comment: "Clipped", SourceKind: models.TaskPreviewFeedbackBrowser,
		SourceLabel: "Checkout", PageRoute: "/checkout",
		CaptureRect:            json.RawMessage(`{"x":1,"y":2,"width":320,"height":180}`),
		ScreenshotAttachmentID: "service-shot-old",
	}); err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()

	snapshot, err := svc.ReplacePreviewFeedbackScreenshot(ctx, &models.TaskPreviewFeedbackScreenshotReplacement{
		TaskID: taskID, ItemID: itemID, ExpectedVersion: 1,
		CaptureRect:            json.RawMessage(`{"x":5,"y":6,"width":100,"height":50}`),
		ScreenshotAttachmentID: "service-shot-new",
	})
	if err != nil || snapshot.Items[0].ScreenshotAttachmentID != "service-shot-new" {
		t.Fatalf("ReplacePreviewFeedbackScreenshot = %#v, %v", snapshot, err)
	}
	if validator.attachmentID != "service-shot-new" || validator.workspaceID != "ws-plan" {
		t.Fatalf("validator scope = %#v", validator)
	}
	if len(cleaner.deleted) != 1 || cleaner.deleted[0].ID != "service-shot-old" {
		t.Fatalf("cleaned = %#v", cleaner.deleted)
	}
	if len(eventBus.GetPublishedEvents()) != 1 {
		t.Fatal("replace did not publish one snapshot")
	}

	eventBus.ClearEvents()
	stale, err := svc.ReplacePreviewFeedbackScreenshot(ctx, &models.TaskPreviewFeedbackScreenshotReplacement{
		TaskID: taskID, ItemID: itemID, ExpectedVersion: 1,
		CaptureRect:            json.RawMessage(`{"x":5,"y":6,"width":100,"height":50}`),
		ScreenshotAttachmentID: "service-shot-new",
	})
	if !errors.Is(err, ErrTaskPreviewFeedbackChanged) || stale == nil {
		t.Fatalf("stale replace = %#v, %v", stale, err)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatal("failed replace published an event")
	}
}

func TestPreviewFeedbackServiceReplaceRejectsInvalidScreenshotBeforeClaim(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	taskID := "task-preview-replace-invalid"
	seedTask(t, context.Background(), repo, taskID)
	validationErr := errors.New("bad png")
	svc.SetPreviewScreenshotValidator(&recordingPreviewScreenshotValidator{err: validationErr})

	_, err := svc.ReplacePreviewFeedbackScreenshot(ctxAs("owner-preview"), &models.TaskPreviewFeedbackScreenshotReplacement{
		TaskID: taskID, ItemID: "0c3b2a93-5b5e-4a37-a3cf-0d1e64b0d2d1", ExpectedVersion: 1,
		CaptureRect:            json.RawMessage(`{"x":5,"y":6,"width":100,"height":50}`),
		ScreenshotAttachmentID: "service-shot-new",
	})
	if !errors.Is(err, validationErr) {
		t.Fatalf("error = %v, want validator error", err)
	}
}
