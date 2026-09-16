package messagequeue_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type taskFeedbackQueueWriter interface {
	InsertWithTaskFeedback(
		context.Context,
		messagequeue.QueueSessionIdentity,
		*messagequeue.QueuedMessage,
		[]models.TaskPlanCommentRef,
		[]models.TaskPreviewFeedbackRef,
		string,
		bool,
		*messagequeue.QueueAttachmentClaim,
		int,
	) (*models.TaskPlanCommentSnapshot, *models.TaskPreviewFeedbackSnapshot, bool, error)
}

func TestSQLiteRepositoryInsertWithTaskFeedbackQueuesAndConsumesMixedContext(t *testing.T) {
	taskRepo, queueRepo := newPlanCommentQueueRepos(t)
	ctx := context.Background()
	seedQueuePlanComment(t, ctx, taskRepo, "preview")
	preview := seedQueuePreviewFeedback(t, ctx, taskRepo, "preview")
	queued := planCommentQueuedMessage(
		"preview", "queue-preview", "fingerprint-preview",
		[]models.TaskPlanCommentRef{{ID: "comment-preview", Version: 1}},
	)
	queued.Metadata["preview_feedback_refs"] = []models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}}
	identity := resolvePlanCommentQueueIdentity(t, ctx, queueRepo, queued)
	writer, ok := queueRepo.(taskFeedbackQueueWriter)
	if !ok {
		t.Fatal("queue repository does not implement task feedback admission")
	}

	planSnapshot, previewSnapshot, replay, err := writer.InsertWithTaskFeedback(
		ctx, identity, queued,
		[]models.TaskPlanCommentRef{{ID: "comment-preview", Version: 1}},
		[]models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}},
		"fingerprint-preview",
		true, nil, 10,
	)
	if err != nil || replay {
		t.Fatalf("InsertWithTaskFeedback replay=%v err=%v", replay, err)
	}
	if planSnapshot == nil || len(planSnapshot.Comments) != 0 ||
		previewSnapshot == nil || len(previewSnapshot.Items) != 0 {
		t.Fatalf("snapshots plan=%#v preview=%#v", planSnapshot, previewSnapshot)
	}
	for _, want := range []string{
		"### Plan Comments", "typed content", "### Web Preview Feedback",
		"Runtime queue copy", "node_path",
	} {
		if !strings.Contains(queued.Content, want) {
			t.Fatalf("queued content missing %q:\n%s", want, queued.Content)
		}
	}
	entries, err := queueRepo.ListBySession(ctx, queued.SessionID)
	if err != nil || len(entries) != 1 || entries[0].Content != queued.Content {
		t.Fatalf("stored queue=%#v err=%v", entries, err)
	}
}

func TestSQLiteRepositoryTaskFeedbackReplaySurvivesQueueDrain(t *testing.T) {
	taskRepo, queueRepo := newPlanCommentQueueRepos(t)
	ctx := context.Background()
	seedQueuePlanComment(t, ctx, taskRepo, "preview-drained")
	preview := seedQueuePreviewFeedback(t, ctx, taskRepo, "preview-drained")
	refs := []models.TaskPreviewFeedbackRef{{ID: preview.ID, Version: 1}}
	queued := planCommentQueuedMessage("preview-drained", "queue-preview-drained", "fingerprint-preview-drained", nil)
	queued.Metadata["preview_feedback_refs"] = refs
	identity := resolvePlanCommentQueueIdentity(t, ctx, queueRepo, queued)
	writer, ok := queueRepo.(taskFeedbackQueueWriter)
	if !ok {
		t.Fatal("queue repository does not implement task feedback admission")
	}

	_, _, replay, err := writer.InsertWithTaskFeedback(
		ctx, identity, queued, nil, refs, "fingerprint-preview-drained", false, nil, 10,
	)
	if err != nil || replay {
		t.Fatalf("first insert replay=%v err=%v", replay, err)
	}
	reserved, autoRun, err := queueRepo.ReserveHeadIfAutoRunForSession(ctx, identity)
	if err != nil || !autoRun || reserved == nil {
		t.Fatalf("reserve accepted feedback = %#v auto_run=%v err=%v", reserved, autoRun, err)
	}
	if err := queueRepo.AcknowledgeByIDForSession(ctx, identity, reserved); err != nil {
		t.Fatalf("acknowledge accepted feedback: %v", err)
	}

	retry := planCommentQueuedMessage("preview-drained", queued.ID, "fingerprint-preview-drained", nil)
	retry.Metadata["preview_feedback_refs"] = refs
	planSnapshot, previewSnapshot, replay, err := writer.InsertWithTaskFeedback(
		ctx, identity, retry, nil, refs, "fingerprint-preview-drained", false, nil, 10,
	)
	if err != nil || !replay || planSnapshot != nil || previewSnapshot != nil {
		t.Fatalf("drained replay plan=%#v preview=%#v replay=%v err=%v", planSnapshot, previewSnapshot, replay, err)
	}
	if retry.Content != queued.Content {
		t.Fatalf("replayed content = %q, want %q", retry.Content, queued.Content)
	}
	conflict := planCommentQueuedMessage("preview-drained", queued.ID, "different-fingerprint", nil)
	conflict.Metadata["preview_feedback_refs"] = refs
	if _, _, _, err := writer.InsertWithTaskFeedback(
		ctx, identity, conflict, nil, refs, "different-fingerprint", false, nil, 10,
	); !errors.Is(err, messagequeue.ErrQueueIDConflict) {
		t.Fatalf("conflicting drained replay error = %v, want ErrQueueIDConflict", err)
	}
	entries, err := queueRepo.ListBySession(ctx, queued.SessionID)
	if err != nil || len(entries) != 0 {
		t.Fatalf("queue after replay = %#v, err=%v", entries, err)
	}
}

func seedQueuePreviewFeedback(
	t *testing.T,
	ctx context.Context,
	repo *tasksqlite.Repository,
	suffix string,
) *models.TaskPreviewFeedback {
	t.Helper()
	item := &models.TaskPreviewFeedback{
		ID: "preview-queue-" + suffix, TaskID: "task-" + suffix,
		Kind: models.TaskPreviewFeedbackText, Comment: "Runtime queue copy",
		SourceKind: models.TaskPreviewFeedbackBrowser, SourceLabel: "Local app",
		PageRoute: "/queue", SelectedText: "Generated",
		TextAnchor: json.RawMessage(`{"start":{"node_path":[0],"offset":0},"end":{"node_path":[0],"offset":9},"containing_element":{"tag":"span","outer_html":"<span>Generated</span>"}}`),
	}
	if _, err := repo.CreateTaskPreviewFeedback(ctx, item, "", ""); err != nil {
		t.Fatal(err)
	}
	return item
}
