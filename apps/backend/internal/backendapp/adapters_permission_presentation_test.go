package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func createPermissionPresentationFixture(t *testing.T) (context.Context, *messageCreatorAdapter, string, string) {
	t.Helper()
	ctx := context.Background()
	harness := newBootStateTestHarness(t)
	if err := harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-permission-presentation", Name: "Permission presentation"}); err != nil {
		t.Fatal(err)
	}
	if err := harness.taskRepo.CreateTask(ctx, &models.Task{
		ID: "task-permission-presentation", WorkspaceID: "ws-permission-presentation",
		Title: "Permission presentation", State: "todo",
	}); err != nil {
		t.Fatal(err)
	}
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-permission-presentation", TaskID: "task-permission-presentation",
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	return ctx, &messageCreatorAdapter{svc: harness.taskSvc}, "task-permission-presentation", "session-permission-presentation"
}

func TestAutomaticPermissionPresentationIsStoredBeforePublication(t *testing.T) {
	ctx, adapter, taskID, sessionID := createPermissionPresentationFixture(t)
	_, err := adapter.CreatePermissionRequestMessage(
		ctx, taskID, sessionID, "request-1", "pending-1", "tool-1", "Run git status", "",
		[]map[string]interface{}{{"option_id": "allow-once", "kind": "allow_once"}}, "command",
		map[string]interface{}{"command": "git status"},
		models.PermissionRequestMessageOptions{Presentation: models.PermissionPresentationAutomatic},
	)
	if err != nil {
		t.Fatal(err)
	}

	messages, err := adapter.svc.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	metadata := messages[0].Metadata
	if got := metadata[models.PermissionPresentationMetadataKey]; got != string(models.PermissionPresentationAutomatic) {
		t.Fatalf("permission_presentation = %#v, want automatic", got)
	}
	if _, ok := metadata["status"]; ok {
		t.Fatalf("automatic candidate must remain pending: status = %#v", metadata["status"])
	}
	if _, ok := metadata["permission_decision"]; ok {
		t.Fatalf("automatic candidate must not be stored approved: %#v", metadata["permission_decision"])
	}
}

func TestCreatePermissionRequestMessageDefaultsToInteractivePresentation(t *testing.T) {
	ctx, adapter, taskID, sessionID := createPermissionPresentationFixture(t)
	if _, err := adapter.CreatePermissionRequestMessage(
		ctx, taskID, sessionID, "request-manual", "pending-manual", "tool-manual", "Run tests", "",
		nil, "command", nil, models.PermissionRequestMessageOptions{},
	); err != nil {
		t.Fatal(err)
	}

	messages, err := adapter.svc.ListMessages(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("message count = %d, want 1", len(messages))
	}
	if got := messages[0].Metadata[models.PermissionPresentationMetadataKey]; got != string(models.PermissionPresentationInteractive) {
		t.Fatalf("permission_presentation = %#v, want interactive", got)
	}
}

func TestAutomaticPermissionPresentationPromotionRequiresLiveUnclaimedRow(t *testing.T) {
	ctx, adapter, taskID, sessionID := createPermissionPresentationFixture(t)
	if _, err := adapter.CreatePermissionRequestMessage(
		ctx, taskID, sessionID, "request-promote", "pending-promote", "tool-promote", "Run tests", "",
		nil, "command", nil,
		models.PermissionRequestMessageOptions{Presentation: models.PermissionPresentationAutomatic},
	); err != nil {
		t.Fatal(err)
	}

	promoted, updated, err := adapter.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: taskID, SessionID: sessionID, RequestID: "request-promote", PendingID: "pending-promote",
		Presentation:        models.PermissionPresentationInteractive,
		RequirePresentation: models.PermissionPresentationAutomatic,
		RequireUnclaimed:    true,
	})
	if err != nil || !updated || promoted == nil {
		t.Fatalf("promotion = %+v updated=%v err=%v", promoted, updated, err)
	}

	if _, err := adapter.svc.ClaimPermissionResolution(ctx, models.PermissionResolutionClaimRequest{
		TaskID: taskID, SessionID: sessionID,
		Audit: models.PermissionResolutionAudit{
			ClaimID: "claim-1", ActorKind: models.PermissionActorAutomation,
			Source: models.PermissionSourceAutoApprove, RequestID: "request-promote",
			PendingID: "pending-promote", OptionID: "allow-once", OptionKind: "allow_once",
		},
	}); err != nil {
		t.Fatal(err)
	}

	_, updated, err = adapter.UpdatePermissionPresentation(ctx, models.PermissionPresentationUpdate{
		TaskID: taskID, SessionID: sessionID, RequestID: "request-promote", PendingID: "pending-promote",
		Presentation:        models.PermissionPresentationInteractive,
		RequirePresentation: models.PermissionPresentationAutomatic,
		RequireUnclaimed:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("a claimed request must not be promoted")
	}
}
