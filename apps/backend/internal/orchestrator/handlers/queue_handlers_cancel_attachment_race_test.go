package handlers

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type cancelAttachmentRaceClaimer struct {
	mu             sync.Mutex
	claimed        map[string]bool
	releaseStarted chan struct{}
	releaseDone    chan struct{}
	claimDone      chan struct{}
	onceRelease    sync.Once
	onceClaim      sync.Once
}

func newCancelAttachmentRaceClaimer() *cancelAttachmentRaceClaimer {
	return &cancelAttachmentRaceClaimer{
		claimed:        make(map[string]bool),
		releaseStarted: make(chan struct{}),
		releaseDone:    make(chan struct{}),
		claimDone:      make(chan struct{}),
	}
}

func (c *cancelAttachmentRaceClaimer) ClaimMessageAttachments(_ context.Context, _ string, _ string, attachments []v1.MessageAttachment) error {
	c.mu.Lock()
	for _, attachment := range attachments {
		c.claimed[attachment.AttachmentID] = true
	}
	c.mu.Unlock()
	c.onceClaim.Do(func() { close(c.claimDone) })
	return nil
}

func (c *cancelAttachmentRaceClaimer) ReleaseMessageAttachments(_ context.Context, _ string, _ string, attachments []v1.MessageAttachment) error {
	c.onceRelease.Do(func() {
		close(c.releaseStarted)
		<-c.releaseDone
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, attachment := range attachments {
		delete(c.claimed, attachment.AttachmentID)
	}
	return nil
}

func (c *cancelAttachmentRaceClaimer) isClaimed(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.claimed[id]
}

func TestWsCancelAllDoesNotReleaseAttachmentReclaimedByConcurrentQueue(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := newCancelAttachmentRaceClaimer()
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()

	entry, err := queue.QueueMessage(ctx, "session-cancel-race", "task-cancel-race", "old", "", messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{{
		Type: "resource", AttachmentID: "attachment-reused", Name: "old.txt", MimeType: "text/plain",
	}})
	require.NoError(t, err)
	claimer.claimed[entry.Attachments[0].AttachmentID] = true

	cancelDone := make(chan struct{})
	go func() {
		_, _ = handlers.wsCancelAll(ctx, createTestMessage(t, ws.ActionMessageQueueCancel, map[string]string{
			"session_id": entry.SessionID,
		}))
		close(cancelDone)
	}()
	<-claimer.releaseStarted

	queueDone := make(chan struct{})
	go func() {
		_, _ = handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": entry.SessionID,
			"task_id":    entry.TaskID,
			"content":    "new",
			"attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "attachment-reused", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
		close(queueDone)
	}()

	select {
	case <-claimer.claimDone:
	case <-time.After(100 * time.Millisecond):
	}
	close(claimer.releaseDone)
	<-cancelDone
	<-queueDone

	require.True(t, claimer.isClaimed("attachment-reused"), "cancellation cleanup released an attachment used by a new queue row")
	status := queue.GetStatus(ctx, entry.SessionID)
	require.Len(t, status.Entries, 1)
	require.Equal(t, "attachment-reused", status.Entries[0].Attachments[0].AttachmentID)
}

func TestWsRemoveEntryDoesNotReleaseAttachmentReclaimedByConcurrentQueue(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	claimer := newCancelAttachmentRaceClaimer()
	handlers.SetAttachmentClaimer(claimer)
	ctx := context.Background()

	entry, err := queue.QueueMessage(ctx, "session-remove-race", "task-remove-race", "old", "", messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{{
		Type: "resource", AttachmentID: "attachment-reused-remove", Name: "old.txt", MimeType: "text/plain",
	}})
	require.NoError(t, err)
	claimer.claimed[entry.Attachments[0].AttachmentID] = true

	removeDone := make(chan struct{})
	go func() {
		_, _ = handlers.wsRemoveEntry(ctx, createTestMessage(t, ws.ActionMessageQueueRemove, map[string]string{
			"session_id": entry.SessionID,
			"entry_id":   entry.ID,
		}))
		close(removeDone)
	}()
	<-claimer.releaseStarted

	queueDone := make(chan struct{})
	go func() {
		_, _ = handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": entry.SessionID,
			"task_id":    entry.TaskID,
			"content":    "new",
			"attachments": []messagequeue.MessageAttachment{{
				Type: "resource", AttachmentID: "attachment-reused-remove", Name: "new.txt", MimeType: "text/plain",
			}},
		}))
		close(queueDone)
	}()
	select {
	case <-claimer.claimDone:
	case <-time.After(100 * time.Millisecond):
	}
	close(claimer.releaseDone)
	<-removeDone
	<-queueDone

	require.True(t, claimer.isClaimed("attachment-reused-remove"), "remove cleanup released an attachment used by a new queue row")
}
