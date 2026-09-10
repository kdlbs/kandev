package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type contextRecordingReleaser struct {
	err   error
	calls int
}

func (r *contextRecordingReleaser) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (r *contextRecordingReleaser) ReleaseMessageAttachments(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment) error {
	r.calls++
	r.err = ctx.Err()
	return nil
}

func TestQueueAttachmentCleanupDetachesCancelledContext(t *testing.T) {
	handlers, _ := setupQueueHandlers(t)
	releaser := &contextRecordingReleaser{}
	handlers.SetAttachmentClaimer(releaser)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	entry := &messagequeue.QueuedMessage{
		ID:        "entry",
		SessionID: "session",
		TaskID:    "task",
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: "attachment",
		}},
	}
	pending, err := handlers.prepareEntryRemovalCleanup(ctx, entry, "remove")
	require.NoError(t, err)
	handlers.releaseQueuedAttachments(ctx, entry, pending)

	if releaser.err != nil {
		t.Fatalf("attachment cleanup context error = %v, want nil", releaser.err)
	}
}

func TestWsRemoveEntryDetachesCancelledCleanupContext(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	entry, err := queue.QueueMessage(context.Background(), "session", "task", "queued", "", "user", false, []messagequeue.MessageAttachment{{
		AttachmentID: "attachment",
	}})
	require.NoError(t, err)
	cleanupQueue := &cleanupContextQueueService{QueueService: queue, entry: entry}
	handlers.queueService = cleanupQueue
	claimer := &contextRecordingReleaser{}
	handlers.SetAttachmentClaimer(claimer)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	response, err := handlers.wsRemoveEntry(ctx, createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
		"session_id": entry.SessionID,
		"entry_id":   entry.ID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, claimer.err)
}

type cleanupContextQueueService struct {
	QueueService
	entry        *messagequeue.QueuedMessage
	getEntryErr  error
	referenceErr error
}

func (s *cleanupContextQueueService) GetEntry(ctx context.Context, _, _ string) (*messagequeue.QueuedMessage, error) {
	s.getEntryErr = ctx.Err()
	return s.entry, nil
}

func (s *cleanupContextQueueService) ReferencedQueueAttachmentIDs(ctx context.Context, _, _ string, _ []string) (map[string]struct{}, error) {
	s.referenceErr = ctx.Err()
	return nil, nil
}
func (s *cleanupContextQueueService) RemoveEntryWithEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error) {
	return s.entry, nil
}

func TestQueueAttachmentCleanupDetachesReadContext(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	current := &messagequeue.QueuedMessage{
		ID: "entry", SessionID: "session", TaskID: "task",
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: "replacement",
		}},
	}
	cleanupQueue := &cleanupContextQueueService{QueueService: queue, entry: current}
	handlers.queueService = cleanupQueue
	releaser := &contextRecordingReleaser{}
	previous := &messagequeue.QueuedMessage{
		ID: "entry", SessionID: "session", TaskID: "task",
		Attachments: []messagequeue.MessageAttachment{{
			AttachmentID: "original",
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handlers.releaseSupersededQueueAttachmentsAdmitted(ctx, wsUpdateMessageRequest{
		SessionID: "session", EntryID: "entry",
	}, previous, releaser)
	require.NoError(t, err)

	require.NoError(t, cleanupQueue.getEntryErr)
	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, releaser.err)
}

func TestQueueAttachmentCleanupDetachesCancelledReferenceLookup(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	cleanupQueue := &cleanupContextQueueService{QueueService: queue}
	handlers.queueService = cleanupQueue
	releaser := &contextRecordingReleaser{}
	previous := []messagequeue.MessageAttachment{{AttachmentID: "attachment"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := handlers.releaseQueueAttachmentCandidates(ctx, "task", wsUpdateMessageRequest{
		SessionID: "session", EntryID: "entry",
	}, previous, releaser)

	require.NoError(t, err)
	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, releaser.err)
	require.Equal(t, 1, releaser.calls)
}

func TestQueueAttachmentFailureCleanupDetachesReadContext(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	cleanupQueue := &cleanupContextQueueService{QueueService: queue}
	handlers.queueService = cleanupQueue
	releaser := &contextRecordingReleaser{}
	previous := &messagequeue.QueuedMessage{ID: "entry", TaskID: "task"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, handlers.releaseQueuedAttachmentUpdateFailure(ctx, previous, "session", []messagequeue.MessageAttachment{{
		AttachmentID: "claimed",
	}}, releaser))

	require.NoError(t, cleanupQueue.referenceErr)
	require.NoError(t, releaser.err)
}

type failingQueueAttachmentReleaser struct{}

func (failingQueueAttachmentReleaser) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return nil
}

func (failingQueueAttachmentReleaser) ReleaseMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	return errors.New("release unavailable")
}

func TestPendingAttachmentCleanupAdoptsExplicitLifecycleContext(t *testing.T) {
	handlers, queue := setupQueueHandlersWithoutStart(t, nil)
	current, err := queue.QueueMessage(
		context.Background(), "session", "task", "current", "", messagequeue.QueuedByUser, false,
		[]messagequeue.MessageAttachment{{AttachmentID: "current"}},
	)
	require.NoError(t, err)
	previous := *current
	previous.Attachments = []messagequeue.MessageAttachment{{AttachmentID: "previous"}}
	lifecycleCtx, cancelLifecycle := context.WithCancel(context.Background())

	pending, err := handlers.preparePendingAttachmentCleanup(
		context.Background(),
		wsUpdateMessageRequest{
			SessionID: "session", EntryID: current.ID, LeaseID: "lease", OperationID: "operation",
		},
		previous.TaskID,
		previous.Attachments,
		failingQueueAttachmentReleaser{},
	)
	require.NoError(t, err)
	handlers.queuePendingAttachmentCleanup(pending)
	handlers.Start(lifecycleCtx)
	cancelLifecycle()

	select {
	case <-handlers.attachmentCleanupCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("pending cleanup retained its lazy background context instead of the explicit lifecycle context")
	}
}
