package backendapp

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestOfficeCreationAttachmentsLaunchContext proves the production Office
// starter forwards the task's ordered creation descriptors into the lifecycle
// launch request. Actual prompt materialization and agentctl upload bytes are
// covered by lifecycle's TestSessionPromptMaterializesClaimedAttachments.
func TestOfficeCreationAttachmentsLaunchContext(t *testing.T) {
	ctx := context.Background()
	h := newRoutineCronHarness(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatal(err)
	}
	attachmentService, err := taskservice.NewAttachmentService(h.taskRepo, t.TempDir(), nil, log)
	if err != nil {
		t.Fatal(err)
	}
	h.taskSvc.SetAttachmentService(attachmentService)
	ownerCtx := authn.WithIdentity(ctx, authn.Identity{UserID: "office-creation-owner"})

	pngBytes := []byte("\x89PNG\r\n\x1a\ncreation")
	resourceBytes := []byte("diagnostic log bytes")
	stagedPNG, err := attachmentService.Stage(
		ownerCtx, "office-creation-owner", h.workspaceID,
		"screenshot.png", "image/png", "image", "prompt", bytes.NewReader(pngBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	stagedResource, err := attachmentService.Stage(
		ownerCtx, "office-creation-owner", h.workspaceID,
		"logs.zip", "application/zip", "resource", "path", bytes.NewReader(resourceBytes),
	)
	if err != nil {
		t.Fatal(err)
	}
	workflows, err := h.taskSvc.ListWorkflows(ctx, h.workspaceID, true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("list workflows: %v (count=%d)", err, len(workflows))
	}
	created, err := h.taskSvc.CreateTask(ownerCtx, &taskservice.CreateTaskRequest{
		WorkspaceID: h.workspaceID, WorkflowID: workflows[0].ID, Title: "Office creation attachments",
		Attachments: []v1.MessageAttachment{
			{Type: "image", AttachmentID: stagedPNG.ID, MimeType: "image/png", Name: "screenshot.png", SizeBytes: int64(len(pngBytes)), DeliveryMode: "prompt"},
			{Type: "resource", AttachmentID: stagedResource.ID, MimeType: "application/zip", Name: "logs.zip", SizeBytes: int64(len(resourceBytes)), DeliveryMode: "path"},
		},
	})
	if err != nil {
		t.Fatalf("create task with creation attachments: %v", err)
	}
	creation, err := h.taskSvc.ReadTaskCreationAttachments(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("read creation attachments: %v", err)
	}
	if len(creation) != 2 || creation[0].AttachmentID != stagedPNG.ID || creation[1].AttachmentID != stagedResource.ID {
		t.Fatalf("creation attachments = %+v", creation)
	}

	executorProfileID := registerOfficeProjectExecutor(t, ctx, h, "exec-office-creation-attachments", models.ExecutorTypeLocal)
	_, adapter := newOfficeProjectLifecycleAdapter(t, ctx, h, filepath.Join(t.TempDir(), "creation-tasks"))
	h.agentMgr.launchDelegate = adapter
	starter, ok := newOfficeTaskStarter(h.orchestrator).(officeservice.TaskStarterWithLaunchContextSession)
	if !ok {
		t.Fatal("production Office task starter does not return the launched session")
	}
	sessionID, err := starter.StartTaskWithLaunchContextReturningSession(
		ctx, created.Task.ID, "mock-profile", officeservice.LaunchContext{
			ExecutorID: "exec-office-creation-attachments", ExecutorProfileID: executorProfileID,
			Prompt:      "Review the creation files.",
			Attachments: creation,
			Env: map[string]string{
				"KANDEV_CLI":          "kandev",
				"KANDEV_API_URL":      "http://localhost:7400/api/v1",
				"KANDEV_API_KEY":      "test-api-key",
				"KANDEV_AGENT_ID":     "office-agent-test",
				"KANDEV_WORKSPACE_ID": h.workspaceID,
				"KANDEV_RUN_ID":       "office-creation-attachments",
				"KANDEV_TASK_ID":      created.Task.ID,
			},
		},
	)
	if err != nil {
		t.Fatalf("launch Office task with creation attachments: %v", err)
	}
	if sessionID == "" {
		t.Fatal("launch returned no session ID")
	}
	launch := h.agentMgr.awaitLaunch(t)
	if len(launch.Attachments) != 2 ||
		launch.Attachments[0].AttachmentID != stagedPNG.ID ||
		launch.Attachments[0].Type != "image" ||
		launch.Attachments[1].AttachmentID != stagedResource.ID ||
		launch.Attachments[1].DeliveryMode != "path" {
		t.Fatalf("launch attachments = %+v", launch.Attachments)
	}
	assertClaimedBytes(t, ctx, attachmentService, created.Task.ID, sessionID, stagedPNG.ID, pngBytes)
	assertClaimedBytes(t, ctx, attachmentService, created.Task.ID, sessionID, stagedResource.ID, resourceBytes)
}

func assertClaimedBytes(
	t *testing.T, ctx context.Context,
	attachmentService *taskservice.AttachmentService,
	taskID, sessionID, attachmentID string, want []byte,
) {
	t.Helper()
	file, _, _, size, err := attachmentService.OpenClaimed(ctx, attachmentID, taskID, sessionID)
	if err != nil {
		t.Fatalf("open claimed attachment %s: %v", attachmentID, err)
	}
	defer func() { _ = file.Close() }()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read claimed attachment %s: %v", attachmentID, err)
	}
	if size != int64(len(want)) || !bytes.Equal(got, want) {
		t.Fatalf("attachment %s bytes = %q (size %d), want %q", attachmentID, got, size, want)
	}
}
