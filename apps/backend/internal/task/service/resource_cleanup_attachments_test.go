package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

func TestDurableDeleteCleanupRemovesTaskAttachments(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	taskResult, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Attachment cleanup", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	taskID := taskResult.Task.ID

	storageRoot := t.TempDir()
	attachmentSvc, err := NewAttachmentService(repo, storageRoot, nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	taskSvc.attachmentSvc = attachmentSvc
	attachment, err := attachmentSvc.Stage(
		ctx, "owner", "ws-1", "cleanup.txt", "text/plain", "resource", "",
		strings.NewReader("attachment"),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if err := attachmentSvc.Claim(ctx, "owner", "ws-1", taskID, "", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	attachmentPath := filepath.Join(storageRoot, "attachments", attachment.StorageKey)
	if _, err := os.Stat(attachmentPath); err != nil {
		t.Fatalf("staged attachment bytes: %v", err)
	}
	if err := repo.DeleteTask(ctx, taskID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	job := &models.TaskResourceCleanupJob{
		ID: "job-attachment-cleanup", OperationID: "delete:attachment-cleanup",
		TaskID: taskID, Trigger: models.TaskResourceCleanupTriggerCascadeDelete,
		State: models.TaskResourceCleanupStateRunning,
	}
	if err := taskSvc.executeTaskResourceCleanupJob(ctx, job, &taskResourceCleanupSnapshot{
		ArchiveSourceManifestCaptured: true,
	}); err != nil {
		t.Fatalf("executeTaskResourceCleanupJob: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); !errors.Is(err, ErrAttachmentNotFound) {
		t.Fatalf("attachment row error = %v, want ErrAttachmentNotFound", err)
	}
	if _, err := os.Stat(attachmentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment bytes error = %v, want os.ErrNotExist", err)
	}
}

func TestWorkspaceDeleteCleanupSnapshotsAttachments(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	taskResult, err := taskSvc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", Title: "Workspace attachment", ProjectID: "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	storageRoot := t.TempDir()
	attachmentSvc, err := NewAttachmentService(repo, storageRoot, nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	taskSvc.attachmentSvc = attachmentSvc
	attachment, err := attachmentSvc.Stage(
		ctx, "owner", "ws-1", "workspace.txt", "text/plain", "resource", "",
		strings.NewReader("workspace attachment"),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	path := filepath.Join(storageRoot, "attachments", attachment.StorageKey)

	if err := attachmentSvc.Claim(ctx, "owner", "ws-1", taskResult.Task.ID, "", []string{attachment.ID}); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	cleanup, err := taskSvc.prepareWorkspaceDeleteTaskCleanup(ctx, taskResult.Task)
	if err != nil {
		t.Fatalf("prepareWorkspaceDeleteTaskCleanup: %v", err)
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(cleanup.cleanupJob.ResourceSnapshot), &snapshot); err != nil {
		t.Fatalf("decode cleanup snapshot: %v", err)
	}
	if snapshot.WorkspaceID != taskResult.Task.WorkspaceID {
		t.Fatalf("snapshot workspace ID = %q, want %q", snapshot.WorkspaceID, taskResult.Task.WorkspaceID)
	}
	if len(snapshot.Attachments) != 1 ||
		snapshot.Attachments[0].ID != attachment.ID ||
		snapshot.Attachments[0].StorageKey != attachment.StorageKey {
		t.Fatalf("snapshot attachments = %+v, want attachment %s with storage key", snapshot.Attachments, attachment.ID)
	}
	if err := repo.DeleteMessageAttachment(ctx, attachment.ID, attachment.OwnerID); err != nil {
		t.Fatalf("delete attachment registry row: %v", err)
	}
	descriptor := &models.TaskMessageAttachment{
		ID: snapshot.Attachments[0].ID, OwnerID: snapshot.Attachments[0].OwnerID,
		StorageKey: snapshot.Attachments[0].StorageKey,
	}
	if err := attachmentSvc.DeleteDescriptors(ctx, []*models.TaskMessageAttachment{descriptor}); err != nil {
		t.Fatalf("DeleteDescriptors: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment bytes error = %v, want os.ErrNotExist", err)
	}
}

func TestWorkspaceDeleteCleanupSnapshotsUnclaimedAttachments(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	storageRoot := t.TempDir()
	attachmentSvc, err := NewAttachmentService(repo, storageRoot, nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	taskSvc.attachmentSvc = attachmentSvc
	attachment, err := attachmentSvc.Stage(
		ctx, "owner", "ws-1", "staged.txt", "text/plain", "resource", "",
		strings.NewReader("unclaimed workspace attachment"),
	)
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	path := filepath.Join(storageRoot, "attachments", attachment.StorageKey)

	job, err := taskSvc.prepareWorkspaceAttachmentCleanup(ctx, "ws-1")
	if err != nil {
		t.Fatalf("prepareWorkspaceAttachmentCleanup: %v", err)
	}
	if job == nil {
		t.Fatal("workspace attachment cleanup job = nil, want durable snapshot")
	}
	var snapshot taskResourceCleanupSnapshot
	if err := json.Unmarshal([]byte(job.ResourceSnapshot), &snapshot); err != nil {
		t.Fatalf("decode cleanup snapshot: %v", err)
	}
	if len(snapshot.Attachments) != 1 ||
		snapshot.Attachments[0].ID != attachment.ID ||
		snapshot.Attachments[0].StorageKey != attachment.StorageKey {
		t.Fatalf("snapshot attachments = %+v, want unclaimed attachment", snapshot.Attachments)
	}
	if err := repo.DeleteMessageAttachment(ctx, attachment.ID, attachment.OwnerID); err != nil {
		t.Fatalf("delete attachment registry row: %v", err)
	}
	descriptor := &models.TaskMessageAttachment{
		ID: attachment.ID, OwnerID: attachment.OwnerID, StorageKey: attachment.StorageKey,
	}
	if err := attachmentSvc.DeleteDescriptors(ctx, []*models.TaskMessageAttachment{descriptor}); err != nil {
		t.Fatalf("DeleteDescriptors: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attachment bytes error = %v, want os.ErrNotExist", err)
	}
}

type workspaceAttachmentListingUnsupportedRepo struct {
	taskrepo.AttachmentRepository
}

func TestWorkspaceDeleteCleanupFailsClosedWithoutAttachmentListing(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	wrappedRepo := workspaceAttachmentListingUnsupportedRepo{AttachmentRepository: repo}
	attachmentSvc, err := NewAttachmentService(wrappedRepo, t.TempDir(), nil, accessTestLogger(t))
	if err != nil {
		t.Fatalf("NewAttachmentService: %v", err)
	}
	taskSvc.attachments = wrappedRepo
	taskSvc.attachmentSvc = attachmentSvc

	if _, err := taskSvc.prepareWorkspaceAttachmentCleanup(ctx, "ws-1"); err == nil {
		t.Fatal("prepareWorkspaceAttachmentCleanup succeeded without workspace attachment listing")
	}
}

func TestWorkspaceDeleteCleanupFailsClosedWithoutPersistence(t *testing.T) {
	taskSvc, _ := setupOfficeTest(t)
	taskSvc.resourceCleanups = nil
	taskSvc.attachmentSvc = &AttachmentService{}

	if _, err := taskSvc.prepareWorkspaceAttachmentCleanup(context.Background(), "ws-1"); err == nil {
		t.Fatal("prepareWorkspaceAttachmentCleanup succeeded without cleanup persistence")
	}
}

func TestWorkspaceDeleteCleanupFailsClosedWithoutExecutor(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	taskSvc.attachmentSvc = nil
	taskSvc.attachments = repo
	taskSvc.resourceCleanups = nil

	if _, err := taskSvc.prepareWorkspaceAttachmentCleanup(context.Background(), "ws-1"); err == nil {
		t.Fatal("prepareWorkspaceAttachmentCleanup succeeded without cleanup executor")
	}
}

func TestWorkspaceAttachmentCleanupRequiresCommittedWorkspaceDeletion(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	const workspaceID = "ws-prepared-attachment-cleanup"
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Prepared cleanup"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID:          "workspace-attachment-prepared",
		OperationID: "workspace_delete:workspace-attachments:" + workspaceID,
		Trigger:     models.TaskResourceCleanupTriggerWorkspaceDelete,
		State:       models.TaskResourceCleanupStatePrepared,
		ResourceSnapshot: `{"workspace_id":"` + workspaceID +
			`","attachments":[{"id":"attachment-1","owner_id":"owner","storage_key":"key"}]}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}

	committed, err := taskSvc.preparedTaskCleanupMutationCommitted(ctx, job)
	if err != nil {
		t.Fatalf("preparedTaskCleanupMutationCommitted before deletion: %v", err)
	}
	if committed {
		t.Fatal("workspace attachment cleanup committed before workspace deletion")
	}
	if err := repo.DeleteWorkspace(ctx, workspaceID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	committed, err = taskSvc.preparedTaskCleanupMutationCommitted(ctx, job)
	if err != nil {
		t.Fatalf("preparedTaskCleanupMutationCommitted after deletion: %v", err)
	}
	if !committed {
		t.Fatal("workspace attachment cleanup not committed after workspace deletion")
	}
}

func TestRetryTaskResourceCleanupPersistsAfterDeadlineExpiry(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	job := &models.TaskResourceCleanupJob{
		ID: "job-expired-retry", OperationID: "delete:expired-retry",
		TaskID: "task-expired-retry", Trigger: models.TaskResourceCleanupTriggerDelete,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("CreateTaskResourceCleanupJob: %v", err)
	}
	claimed, err := repo.MarkTaskResourceCleanupJobRunning(ctx, job.ID)
	if err != nil || !claimed {
		t.Fatalf("MarkTaskResourceCleanupJobRunning = %v, %v", claimed, err)
	}
	job, err = repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}

	expiredCtx, cancel := context.WithDeadline(ctx, time.Now().Add(-time.Second))
	defer cancel()
	cleanupErr := errors.New("cleanup deadline exceeded")
	if err := taskSvc.retryTaskResourceCleanupJob(expiredCtx, job, cleanupErr); !errors.Is(err, cleanupErr) {
		t.Fatalf("retryTaskResourceCleanupJob error = %v, want cleanup error", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("reload cleanup job: %v", err)
	}
	if got.State != models.TaskResourceCleanupStateRetryWait {
		t.Fatalf("cleanup state = %q, want retry_wait", got.State)
	}
}
