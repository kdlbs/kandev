package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func stageCreationAttachment(
	t *testing.T, svc *AttachmentService, ctx context.Context, workspaceID, name, mime, kind, mode string,
) *models.TaskMessageAttachment {
	t.Helper()
	attachment, err := svc.Stage(ctx, "owner-creation", workspaceID, name, mime, kind, mode, strings.NewReader(name))
	if err != nil {
		t.Fatalf("stage %s: %v", name, err)
	}
	return attachment
}

func creationDescriptor(attachment *models.TaskMessageAttachment) v1.MessageAttachment {
	return v1.MessageAttachment{
		Type: attachment.Kind, AttachmentID: attachment.ID, MimeType: attachment.MimeType,
		Name: attachment.Name, SizeBytes: attachment.SizeBytes, DeliveryMode: attachment.DeliveryMode,
	}
}

func TestCreateTaskClaimsCreationAttachmentsAtomically(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-creation"})
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-creation-service")
	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	taskService.SetAttachmentService(attachmentService)
	first := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-service", "first.png", "image/png", "image", "prompt")
	second := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-service", "second.zip", "application/zip", "resource", "path")

	result, err := taskService.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-creation-service", WorkflowID: wfID, Title: "Creation",
		Attachments: []v1.MessageAttachment{
			creationDescriptor(second), creationDescriptor(first),
		},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	attachments, err := repo.ListTaskCreationAttachments(ctx, result.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 2 || attachments[0].ID != second.ID || attachments[1].ID != first.ID {
		t.Fatalf("creation set = %+v", attachments)
	}
	for _, attachment := range attachments {
		if attachment.State != models.AttachmentStateClaimed || attachment.TaskID != result.Task.ID {
			t.Fatalf("claimed attachment = %+v", attachment)
		}
	}
	read, err := taskService.ReadTaskCreationAttachments(ctx, result.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 2 || read[0].AttachmentID != second.ID || read[1].AttachmentID != first.ID {
		t.Fatalf("read creation set = %+v", read)
	}
}

func TestRollbackPartialTaskReleasesCreationAttachments(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-creation"})
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-creation-rollback")
	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	taskService.SetAttachmentService(attachmentService)
	attachment := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-rollback", "rollback.png", "image/png", "image", "prompt")
	result, err := taskService.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-creation-rollback", WorkflowID: wfID, Title: "Rollback",
		Attachments: []v1.MessageAttachment{creationDescriptor(attachment)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := taskService.rollbackPartialTask(ctx, result.Task.ID, errors.New("later create step failed")); err == nil {
		t.Fatal("rollbackPartialTask returned no cause")
	}
	if _, err := repo.GetTask(ctx, result.Task.ID); err == nil {
		t.Fatal("task survived rollback")
	}
	if _, err := repo.GetMessageAttachment(ctx, attachment.ID); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("creation attachment survived rollback: %v", err)
	}
}

func TestCreationAttachmentClaimFencedAgainstDelete(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-creation"})
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-fence")
	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	taskService.SetAttachmentService(attachmentService)

	t.Run("delete wins before admission", func(t *testing.T) {
		staged := stageCreationAttachment(t, attachmentService, ctx, "ws-fence", "delete-first.png", "image/png", "image", "prompt")
		if err := attachmentService.Delete(ctx, "owner-creation", staged.ID); err != nil {
			t.Fatalf("delete staged attachment: %v", err)
		}
		_, err := taskService.CreateTask(ctx, &CreateTaskRequest{
			WorkspaceID: "ws-fence", WorkflowID: wfID, Title: "Delete first",
			Attachments: []v1.MessageAttachment{creationDescriptor(staged)},
		})
		if !errors.Is(err, models.ErrAttachmentNotFound) {
			t.Fatalf("create error = %v, want missing attachment", err)
		}
	})

	t.Run("create wins and delete cannot unlink claimed bytes", func(t *testing.T) {
		staged := stageCreationAttachment(t, attachmentService, ctx, "ws-fence", "create-first.png", "image/png", "image", "prompt")
		task := &models.Task{ID: "task-fence", WorkspaceID: "ws-fence", Title: "Create first"}
		task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
			OwnerID: "owner-creation", WorkspaceID: "ws-fence", IDs: []string{staged.ID},
		}
		started := make(chan struct{})
		release := make(chan struct{})
		claimDone := make(chan error, 1)
		go func() {
			claimDone <- attachmentService.withClaimAdmission(func() error {
				close(started)
				<-release
				return repo.CreateTask(ctx, task)
			})
		}()
		<-started
		deleteDone := make(chan error, 1)
		go func() { deleteDone <- attachmentService.Delete(ctx, "owner-creation", staged.ID) }()
		select {
		case err := <-deleteDone:
			t.Fatalf("delete bypassed the claim fence: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		if err := <-claimDone; err != nil {
			t.Fatalf("claim insert: %v", err)
		}
		if err := <-deleteDone; !errors.Is(err, models.ErrAttachmentClaimConflict) {
			t.Fatalf("delete after claim = %v, want claim conflict", err)
		}
		file, _, _, size, err := attachmentService.OpenClaimed(ctx, staged.ID, task.ID, "")
		if err != nil {
			t.Fatalf("claimed bytes were removed: %v", err)
		}
		_ = file.Close()
		if size != int64(len("create-first.png")) {
			t.Fatalf("claimed bytes size = %d", size)
		}
	})
}

func TestCreateTaskFoundExternalIDDoesNotClaimRetryAttachment(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-creation"})
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-creation-found")
	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	taskService.SetAttachmentService(attachmentService)

	first, err := taskService.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-creation-found", WorkflowID: wfID, Title: "Original", ExternalID: "ext-creation",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustSettle(t, ctx, repo, first.Task.ID, "ext-creation")
	retryAttachment := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-found", "retry.png", "image/png", "image", "prompt")

	retry, err := taskService.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-creation-found", WorkflowID: wfID, Title: "Retry", ExternalID: "ext-creation",
		Attachments: []v1.MessageAttachment{creationDescriptor(retryAttachment)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if retry.Outcome != CreateTaskOutcomeFoundSettled {
		t.Fatalf("outcome = %v", retry.Outcome)
	}
	stored, err := repo.GetMessageAttachment(ctx, retryAttachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != models.AttachmentStateStaged || stored.TaskID != "" {
		t.Fatalf("retry attachment was claimed: %+v", stored)
	}
	attachments, err := repo.ListTaskCreationAttachments(ctx, first.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 0 {
		t.Fatalf("retry attachment entered the existing task: %+v", attachments)
	}
}

func TestReadTaskCreationAttachmentsExcludesUnrelatedTaskClaims(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-creation"})
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-creation-scope")
	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	taskService.SetAttachmentService(attachmentService)
	creation := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-scope", "creation.png", "image/png", "image", "prompt")
	result, err := taskService.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-creation-scope", WorkflowID: wfID, Title: "Scope",
		Attachments: []v1.MessageAttachment{creationDescriptor(creation)},
	})
	if err != nil {
		t.Fatal(err)
	}

	chat := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-scope", "chat.txt", "text/plain", "resource", "path")
	if err := attachmentService.Claim(ctx, "owner-creation", "ws-creation-scope", result.Task.ID, "", []string{chat.ID}); err != nil {
		t.Fatal(err)
	}
	read, err := taskService.ReadTaskCreationAttachments(ctx, result.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(read) != 1 || read[0].AttachmentID != creation.ID {
		t.Fatalf("read set = %+v, want only the creation attachment", read)
	}
}

func TestCreateTaskRejectsUnclaimedCreationAttachment(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner-creation"})
	wfID := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "ws-creation-reject")
	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatal(err)
	}
	taskService.SetAttachmentService(attachmentService)
	valid := stageCreationAttachment(t, attachmentService, ctx, "ws-creation-reject", "valid.png", "image/png", "image", "prompt")
	foreign := &models.TaskMessageAttachment{
		ID: "foreign-creation", OwnerID: "other-owner", WorkspaceID: "ws-creation-reject",
		Name: "foreign.png", MimeType: "image/png", Kind: "image", DeliveryMode: "prompt",
		SizeBytes: 4, StorageKey: "foreign-creation", State: models.AttachmentStateStaged,
	}

	_, err = taskService.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-creation-reject", WorkflowID: wfID, Title: "Rejected",
		Attachments: []v1.MessageAttachment{creationDescriptor(valid), creationDescriptor(foreign)},
	})
	if !errors.Is(err, models.ErrAttachmentNotFound) && !errors.Is(err, models.ErrAttachmentClaimConflict) {
		t.Fatalf("error = %v, want a missing/foreign attachment rejection", err)
	}
	stored, getErr := repo.GetMessageAttachment(ctx, valid.ID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if stored.State != models.AttachmentStateStaged || stored.TaskID != "" {
		t.Fatalf("valid sibling changed after rejection: %+v", stored)
	}
}
