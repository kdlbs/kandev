package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestTaskCreationAttachmentsPostgres covers the same atomic claim and ordered
// association contract on the PostgreSQL dialect. Skips unless
// KANDEV_TEST_POSTGRES_DSN is set.
func TestTaskCreationAttachmentsPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-creation-pg", Name: "Creation PG"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	now := time.Now().UTC()
	first := &models.TaskMessageAttachment{
		ID: "creation-pg-first", OwnerID: "owner-pg", WorkspaceID: "workspace-creation-pg",
		Name: "first.png", MimeType: "image/png", Kind: "image", DeliveryMode: "prompt",
		SizeBytes: 4, StorageKey: "creation-pg-first", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	second := &models.TaskMessageAttachment{
		ID: "creation-pg-second", OwnerID: "owner-pg", WorkspaceID: "workspace-creation-pg",
		Name: "second.zip", MimeType: "application/zip", Kind: "resource", DeliveryMode: "path",
		SizeBytes: 4, StorageKey: "creation-pg-second", State: models.AttachmentStateStaged,
		ExpiresAt: now.Add(time.Hour), CreatedAt: now,
	}
	for _, attachment := range []*models.TaskMessageAttachment{first, second} {
		if err := repo.CreateMessageAttachment(ctx, attachment); err != nil {
			t.Fatalf("create attachment: %v", err)
		}
	}

	task := &models.Task{ID: "task-creation-pg", WorkspaceID: "workspace-creation-pg", Title: "Creation PG"}
	task.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-pg", WorkspaceID: "workspace-creation-pg",
		IDs: []string{second.ID, first.ID},
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	attachments, err := repo.ListTaskCreationAttachments(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 2 || attachments[0].ID != second.ID || attachments[1].ID != first.ID {
		t.Fatalf("creation order = %+v", attachments)
	}

	rollbackTask := &models.Task{ID: "task-creation-pg-rollback", WorkspaceID: "workspace-creation-pg", Title: "Rollback"}
	rollbackTask.CreationAttachmentClaim = &models.TaskCreationAttachmentClaim{
		OwnerID: "owner-pg", WorkspaceID: "workspace-creation-pg",
		IDs: []string{first.ID, first.ID},
	}
	if err := repo.CreateTask(ctx, rollbackTask); !errors.Is(err, models.ErrAttachmentClaimConflict) {
		t.Fatalf("duplicate creation set error = %v", err)
	}
	if _, err := repo.GetTask(ctx, rollbackTask.ID); err == nil {
		t.Fatal("rolled-back task row survived")
	}
}
