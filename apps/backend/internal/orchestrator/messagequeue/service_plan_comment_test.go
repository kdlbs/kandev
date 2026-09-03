package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
)

type planCommentAdmissionRepositoryStub struct {
	Repository
	message        *QueuedMessage
	refs           []models.TaskPlanCommentRef
	requirePrimary bool
	max            int
	snapshot       *models.TaskPlanCommentSnapshot
	replay         bool
	err            error
}

func (r *planCommentAdmissionRepositoryStub) InsertWithPlanComments(
	_ context.Context,
	message *QueuedMessage,
	refs []models.TaskPlanCommentRef,
	requirePrimary bool,
	max int,
) (*models.TaskPlanCommentSnapshot, bool, error) {
	r.message = message
	r.refs = refs
	r.requirePrimary = requirePrimary
	r.max = max
	if r.err == nil {
		message.Content = "resolved content"
		message.Position = 3
	}
	return r.snapshot, r.replay, r.err
}

func TestQueueMessageWithPlanCommentsBuildsDurableNonMergeAdmission(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatal(err)
	}
	repo := &planCommentAdmissionRepositoryStub{
		Repository: NewMemoryRepository(),
		snapshot:   &models.TaskPlanCommentSnapshot{TaskID: "task", PlanID: "plan", Revision: 4},
	}
	service := NewService(repo, 7, log)
	refs := []models.TaskPlanCommentRef{{ID: "comment", Version: 2}}
	result, err := service.QueueMessageWithPlanComments(context.Background(), PlanCommentQueueRequest{
		ClientQueueID: "client-queue", SessionID: "session", TaskID: "task",
		Content: "typed content", Model: "model", UserID: QueuedByUser, PlanMode: true,
		Attachments: []MessageAttachment{{Type: "image", AttachmentID: "attachment"}},
		Metadata:    map[string]interface{}{"custom": "value"}, PlanCommentRefs: refs,
		RequirePrimarySession: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != repo.message || result.Snapshot != repo.snapshot || result.Replay {
		t.Fatalf("result = %#v", result)
	}
	if repo.message.ID != "client-queue" || repo.message.Content != "resolved content" || repo.max != 7 {
		t.Fatalf("admission message = %#v max=%d", repo.message, repo.max)
	}
	if !repo.requirePrimary || len(repo.refs) != 1 || repo.refs[0] != refs[0] {
		t.Fatalf("admission refs=%#v require_primary=%v", repo.refs, repo.requirePrimary)
	}
	if got, _ := repo.message.Metadata[plancomments.MetadataRequestFingerprint].(string); got == "" {
		t.Fatal("request fingerprint is missing")
	}
	if got, _ := repo.message.Metadata[plancomments.MetadataClientQueueID].(string); got != "client-queue" {
		t.Fatalf("client queue id metadata = %q", got)
	}
	if !plancomments.MetadataRefsMatch(repo.message.Metadata, refs) {
		t.Fatalf("metadata refs = %#v", repo.message.Metadata[plancomments.MetadataRefs])
	}
	if repo.message.Metadata["custom"] != "value" {
		t.Fatalf("custom metadata = %#v", repo.message.Metadata)
	}
}

func TestQueueMessageWithPlanCommentsRequiresAtomicRepository(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(NewMemoryRepository(), 10, log)
	result, err := service.QueueMessageWithPlanComments(context.Background(), PlanCommentQueueRequest{
		ClientQueueID: "client-queue", SessionID: "session", TaskID: "task", Content: "typed",
		PlanCommentRefs: []models.TaskPlanCommentRef{{ID: "comment", Version: 1}},
	})
	if err == nil || result != nil {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
