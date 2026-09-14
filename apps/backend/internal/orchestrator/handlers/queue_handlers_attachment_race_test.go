package handlers

import (
	"context"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

func TestWsUpdateMessageDoesNotReleaseAttachmentReclaimedByConcurrentEdit(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	handlers.eventBus = nil
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session-race", "task-race", "original", "", messagequeue.QueuedByUser, false, []messagequeue.MessageAttachment{{
		Type:         "resource",
		AttachmentID: "attachment-a",
		Name:         "a.txt",
		MimeType:     "text/plain",
	}})
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection-a")
	require.NoError(t, err)
	claimer := newBlockingAttachmentClaimer()
	handlers.SetAttachmentClaimer(claimer)

	firstDone := make(chan updateResult, 1)
	go func() {
		response, callErr := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-a"), createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id":               entry.SessionID,
			"entry_id":                 entry.ID,
			"lease_id":                 lease.LeaseID,
			"operation_id":             "operation-a",
			"expected_target_revision": lease.TargetRevision,
			"content":                  "first edit",
			"attachments":              []messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "attachment-b", Name: "b.txt", MimeType: "text/plain"}},
		}))
		firstDone <- updateResult{response: response, err: callErr}
	}()
	<-claimer.firstReleaseStarted
	secondDone := make(chan updateResult, 1)
	go func() {
		response, callErr := handlers.wsUpdateMessage(ws.WithConnectionID(ctx, "connection-a"), createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id":               entry.SessionID,
			"entry_id":                 entry.ID,
			"lease_id":                 lease.LeaseID,
			"operation_id":             "operation-b",
			"expected_target_revision": int64(1),
			"content":                  "second edit",
			"attachments":              []messagequeue.MessageAttachment{{Type: "resource", AttachmentID: "attachment-a", Name: "a.txt", MimeType: "text/plain"}},
		}))
		secondDone <- updateResult{response: response, err: callErr}
	}()
	select {
	case <-secondDone:
		t.Fatal("concurrent edit bypassed the first edit's attachment cleanup")
	case <-time.After(100 * time.Millisecond):
	}
	close(claimer.releaseFirst)
	first := <-firstDone
	second := <-secondDone
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Equal(t, ws.MessageTypeResponse, first.response.Type)
	require.Equal(t, ws.MessageTypeResponse, second.response.Type)
	require.True(t, claimer.isClaimed("attachment-a"), "attachment reclaimed by the second edit must remain claimed")
}

type updateResult struct {
	response *ws.Message
	err      error
}

type blockingAttachmentClaimer struct {
	mu                  sync.Mutex
	claimed             map[string]bool
	releaseFirst        chan struct{}
	firstReleaseStarted chan struct{}
	releaseCount        int
}

func newBlockingAttachmentClaimer() *blockingAttachmentClaimer {
	return &blockingAttachmentClaimer{
		claimed:             make(map[string]bool),
		releaseFirst:        make(chan struct{}),
		firstReleaseStarted: make(chan struct{}),
	}
}

func (c *blockingAttachmentClaimer) ClaimMessageAttachments(_ context.Context, _ string, _ string, attachments []v1.MessageAttachment) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, attachment := range attachments {
		c.claimed[attachment.AttachmentID] = true
	}
	return nil
}

func (c *blockingAttachmentClaimer) ReleaseMessageAttachments(_ context.Context, _ string, _ string, attachments []v1.MessageAttachment) error {
	c.mu.Lock()
	c.releaseCount++
	first := c.releaseCount == 1
	c.mu.Unlock()
	if first {
		close(c.firstReleaseStarted)
		<-c.releaseFirst
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, attachment := range attachments {
		delete(c.claimed, attachment.AttachmentID)
	}
	return nil
}

func (c *blockingAttachmentClaimer) isClaimed(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.claimed[id]
}
