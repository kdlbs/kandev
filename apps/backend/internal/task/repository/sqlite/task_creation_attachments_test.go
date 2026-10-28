package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func stagedCreationAttachment(id, owner, workspace string) *models.TaskMessageAttachment {
	return &models.TaskMessageAttachment{
		ID: id, OwnerID: owner, WorkspaceID: workspace,
		Name: id + ".png", MimeType: "image/png", Kind: "image",
		DeliveryMode: "prompt", SizeBytes: 8, StorageKey: id,
		State:     models.AttachmentStateStaged,
		ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC(),
	}
}

func TestTaskCreationAttachmentsAtomic(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-creation-atomic")
	first := stagedCreationAttachment("creation-first", "owner-1", "workspace-creation-atomic")
	second := stagedCreationAttachment("creation-second", "owner-1", "workspace-creation-atomic")
	for _, attachment := range []*models.TaskMessageAttachment{first, second} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	task := &models.Task{ID: "task-creation-atomic", WorkspaceID: "workspace-creation-atomic", Title: "Atomic"}
	task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-1", WorkspaceID: "workspace-creation-atomic",
		IDs: []string{second.ID, first.ID},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	attachments, err := repo.ListTaskCreationAttachments(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 2 || attachments[0].ID != second.ID || attachments[1].ID != first.ID {
		t.Fatalf("creation order = %+v, want [%s %s]", attachments, second.ID, first.ID)
	}
	for _, attachment := range attachments {
		if attachment.State != models.AttachmentStateClaimed ||
			attachment.TaskID != task.ID || attachment.SessionID != "" {
			t.Fatalf("claimed row = %+v", attachment)
		}
	}
}

func TestTaskCreationAttachmentsMixedInvalidRollsBack(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-creation-rollback")
	valid := stagedCreationAttachment("creation-valid", "owner-1", "workspace-creation-rollback")
	foreign := stagedCreationAttachment("creation-foreign", "owner-2", "workspace-creation-rollback")
	expired := stagedCreationAttachment("creation-expired", "owner-1", "workspace-creation-rollback")
	expired.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	for _, attachment := range []*models.TaskMessageAttachment{valid, foreign, expired} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name string
		ids  []string
		want error
	}{
		{name: "foreign", ids: []string{valid.ID, foreign.ID}, want: models.ErrAttachmentClaimConflict},
		{name: "expired", ids: []string{valid.ID, expired.ID}, want: models.ErrAttachmentClaimConflict},
	}
	for index, tc := range cases {
		task := &models.Task{
			ID: "task-creation-rollback-" + tc.name, WorkspaceID: "workspace-creation-rollback",
			Title: tc.name,
		}
		task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
			OwnerID: "owner-1", WorkspaceID: "workspace-creation-rollback", IDs: tc.ids,
		}
		err := repo.CreateTask(ctx, task)
		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
		if _, err := repo.GetTask(ctx, task.ID); err == nil {
			t.Fatalf("%s: task survived a rejected attachment set", tc.name)
		}
		stored, err := repo.GetMessageAttachment(ctx, valid.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.State != models.AttachmentStateStaged || stored.TaskID != "" {
			t.Fatalf("%s: valid sibling changed after rollback: %+v", tc.name, stored)
		}
		_ = index
	}
}

func TestTaskCreationAttachmentsCleanup(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-creation-cleanup")
	attachment := stagedCreationAttachment("creation-cleanup", "owner-1", "workspace-creation-cleanup")
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{ID: "task-creation-cleanup", WorkspaceID: "workspace-creation-cleanup", Title: "Cleanup"}
	task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-1", WorkspaceID: "workspace-creation-cleanup", IDs: []string{attachment.ID},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.DeleteMessageAttachmentsBySession(ctx, task.ID, "session-other"); err != nil {
		t.Fatal(err)
	}
	attachments, err := repo.ListTaskCreationAttachments(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].ID != attachment.ID {
		t.Fatalf("session cleanup removed creation attachments: %+v", attachments)
	}

	if err := repo.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	attachments, err = repo.ListTaskCreationAttachments(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 0 {
		t.Fatalf("task deletion left associations: %+v", attachments)
	}
}

func TestTaskCreationAttachmentsWorkspaceCleanup(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-creation-workspace-delete")
	attachment := stagedCreationAttachment(
		"creation-workspace-delete", "owner-1", "workspace-creation-workspace-delete",
	)
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{
		ID: "task-creation-workspace-delete", WorkspaceID: "workspace-creation-workspace-delete",
		Title: "Workspace cleanup",
	}
	task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-1", WorkspaceID: "workspace-creation-workspace-delete",
		IDs: []string{attachment.ID},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DeleteMessageAttachmentsByWorkspaceTx(
		ctx, tx, "workspace-creation-workspace-delete",
	); err != nil {
		_ = tx.Rollback()
		t.Fatalf("delete workspace attachments: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var associations int
	if err := repo.db.GetContext(ctx, &associations,
		`SELECT COUNT(*) FROM task_creation_attachments WHERE task_id = ?`, task.ID); err != nil {
		t.Fatal(err)
	}
	if associations != 0 {
		t.Fatalf("workspace cleanup left %d associations", associations)
	}
}

func TestTaskCreationAttachmentsRestartAndOrder(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-creation-restart")
	first := stagedCreationAttachment("creation-restart-first", "owner-1", "workspace-creation-restart")
	second := stagedCreationAttachment("creation-restart-second", "owner-1", "workspace-creation-restart")
	for _, attachment := range []*models.TaskMessageAttachment{first, second} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatal(err)
		}
	}
	task := &models.Task{ID: "task-creation-restart", WorkspaceID: "workspace-creation-restart", Title: "Restart"}
	task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-1", WorkspaceID: "workspace-creation-restart",
		IDs: []string{second.ID, first.ID},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewWithDB(repo.db, repo.db, nil)
	if err != nil {
		t.Fatal(err)
	}
	attachments, err := reopened.ListTaskCreationAttachments(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 2 || attachments[0].ID != second.ID || attachments[1].ID != first.ID {
		t.Fatalf("restart order = %+v", attachments)
	}
}

func TestTaskCreationAttachmentsDuplicateIDRejected(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-creation-duplicate")
	attachment := stagedCreationAttachment("creation-duplicate", "owner-1", "workspace-creation-duplicate")
	if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
		t.Fatal(err)
	}
	task := &models.Task{ID: "task-creation-duplicate", WorkspaceID: "workspace-creation-duplicate", Title: "Duplicate"}
	task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-1", WorkspaceID: "workspace-creation-duplicate",
		IDs: []string{attachment.ID, attachment.ID},
	}
	if err := repo.CreateTask(ctx, task); !errors.Is(err, models.ErrAttachmentClaimConflict) {
		t.Fatalf("duplicate error = %v", err)
	}
}
