package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestReleaseMessageAttachmentsKeepsTranscriptReferences(t *testing.T) {
	taskService, _, repo := createTestService(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	seedTask(t, ctx, repo, "task-attachments")
	seedSession(t, ctx, repo, "task-attachments", "session-attachments")
	seedTurn(t, repo, "turn-attachments", "session-attachments", "task-attachments")

	attachmentService, err := NewAttachmentService(repo, t.TempDir(), nil, logger.Default())
	if err != nil {
		t.Fatalf("create attachment service: %v", err)
	}
	taskService.SetAttachmentService(attachmentService)

	stage := func(name string) *models.TaskMessageAttachment {
		t.Helper()
		attachment, stageErr := attachmentService.Stage(
			ctx, "owner", "ws-plan", name, "text/plain", "resource", "path", strings.NewReader(name),
		)
		if stageErr != nil {
			t.Fatalf("stage %s: %v", name, stageErr)
		}
		return attachment
	}
	referenced := stage("referenced.txt")
	orphaned := stage("orphaned.txt")
	if err := attachmentService.Claim(
		ctx, "owner", "ws-plan", "task-attachments", "session-attachments", []string{referenced.ID, orphaned.ID},
	); err != nil {
		t.Fatalf("claim attachments: %v", err)
	}
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "message-attachments", TaskSessionID: "session-attachments", TaskID: "task-attachments",
		TurnID: "turn-attachments", AuthorType: models.MessageAuthorUser, Content: "kept",
		Metadata: map[string]interface{}{"attachments": []v1.MessageAttachment{{AttachmentID: referenced.ID}}},
	}); err != nil {
		t.Fatalf("create transcript message: %v", err)
	}

	candidates := []v1.MessageAttachment{{AttachmentID: referenced.ID}, {AttachmentID: orphaned.ID}}
	if err := taskService.ReleaseMessageAttachments(ctx, "task-attachments", "session-attachments", candidates); err != nil {
		t.Fatalf("release attachments: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, referenced.ID); err != nil {
		t.Fatalf("transcript attachment was released: %v", err)
	}
	if _, err := repo.GetMessageAttachment(ctx, orphaned.ID); !errors.Is(err, models.ErrAttachmentNotFound) {
		t.Fatalf("orphaned attachment error = %v, want not found", err)
	}
}
