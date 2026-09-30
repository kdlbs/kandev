package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type previewFeedbackServiceContract interface {
	ListPreviewFeedback(context.Context, string) (*models.TaskPreviewFeedbackSnapshot, error)
	CreatePreviewFeedback(context.Context, *models.TaskPreviewFeedback) (*models.TaskPreviewFeedbackSnapshot, error)
	UpdatePreviewFeedback(context.Context, string, string, string, int64) (*models.TaskPreviewFeedbackSnapshot, error)
	DeletePreviewFeedback(context.Context, string, string, int64) (*models.TaskPreviewFeedbackSnapshot, error)
	ClearPreviewFeedback(context.Context, string, int64) (*models.TaskPreviewFeedbackSnapshot, error)
}

type recordingPreviewScreenshotValidator struct {
	ownerID, workspaceID, taskID, attachmentID string
	err                                        error
}

func (v *recordingPreviewScreenshotValidator) ValidatePreviewScreenshot(
	_ context.Context,
	ownerID, workspaceID, taskID, attachmentID string,
) error {
	v.ownerID = ownerID
	v.workspaceID = workspaceID
	v.taskID = taskID
	v.attachmentID = attachmentID
	return v.err
}

func requirePreviewFeedbackService(t *testing.T, svc *PlanService) previewFeedbackServiceContract {
	t.Helper()
	contract, ok := any(svc).(previewFeedbackServiceContract)
	if !ok {
		t.Fatal("PlanService does not implement task preview feedback operations")
	}
	return contract
}

func TestPreviewFeedbackServiceAuthorizesPublishesAndMapsConflicts(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-preview-feedback-service")
	eventBus.ClearEvents()

	var authorized []string
	svc.SetTaskAuthorizer(func(_ context.Context, taskID string) error {
		authorized = append(authorized, taskID)
		if taskID == "task-preview-denied" {
			return errors.New("denied")
		}
		return nil
	})
	feedback := requirePreviewFeedbackService(t, svc)
	created, err := feedback.CreatePreviewFeedback(ctx, &models.TaskPreviewFeedback{
		ID: "7b014fea-f9ea-4365-a8c6-17ad1b0b0c4f", TaskID: "task-preview-feedback-service",
		Kind: models.TaskPreviewFeedbackText, Comment: "Increase contrast",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/products", PageTitle: "Products", SelectedText: "Choose a plan",
		TextAnchor: json.RawMessage(`{"start":{"path":[0],"offset":0}}`),
	})
	if err != nil || created.Revision != 1 || len(created.Items) != 1 {
		t.Fatalf("CreatePreviewFeedback = %#v, %v", created, err)
	}
	if len(authorized) != 1 || authorized[0] != "task-preview-feedback-service" {
		t.Fatalf("authorized task IDs = %v", authorized)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != "task.preview_feedback.changed" {
		t.Fatalf("published events = %#v", published)
	}

	eventBus.ClearEvents()
	stale, err := feedback.UpdatePreviewFeedback(
		ctx, "task-preview-feedback-service", created.Items[0].ID, "Stale", 9,
	)
	if err == nil || stale.Revision != 1 {
		t.Fatalf("stale update = %#v, %v", stale, err)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatal("failed mutation published an event")
	}
	if _, err := feedback.ListPreviewFeedback(ctx, "task-preview-denied"); err == nil {
		t.Fatal("denied list succeeded")
	}
}

func TestPreviewFeedbackServiceValidatesScreenshotBeforeClaim(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	seedTask(t, context.Background(), repo, "task-preview-screenshot-validation")
	validationErr := errors.New("invalid screenshot bytes")
	validator := &recordingPreviewScreenshotValidator{err: validationErr}
	svc.SetPreviewScreenshotValidator(validator)

	_, err := svc.CreatePreviewFeedback(ctxAs("owner-preview"), &models.TaskPreviewFeedback{
		ID: "db6b016a-717b-4ae8-b9a0-f4eb8eb723bf", TaskID: "task-preview-screenshot-validation",
		Kind: models.TaskPreviewFeedbackScreenshot, Comment: "The form clips here",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Checkout",
		PageRoute: "/checkout", PageTitle: "Checkout",
		CaptureRect:            json.RawMessage(`{"x":1,"y":2,"width":320,"height":180}`),
		ScreenshotAttachmentID: "staged-screenshot",
	})
	if !errors.Is(err, validationErr) {
		t.Fatalf("CreatePreviewFeedback error = %v, want validator error", err)
	}
	if validator.ownerID != "owner-preview" || validator.workspaceID != "ws-plan" ||
		validator.taskID != "task-preview-screenshot-validation" ||
		validator.attachmentID != "staged-screenshot" {
		t.Fatalf("validator scope = %#v", validator)
	}
	snapshot, listErr := svc.ListPreviewFeedback(context.Background(), "task-preview-screenshot-validation")
	if listErr != nil || len(snapshot.Items) != 0 {
		t.Fatalf("snapshot after rejected screenshot = %#v, %v", snapshot, listErr)
	}
}
