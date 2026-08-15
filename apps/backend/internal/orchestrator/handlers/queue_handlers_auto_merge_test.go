package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestWsQueueMessage_AutoMergeClaimsAttachmentBeforeFold(t *testing.T) {
	handlers, service := setupQueueHandlers(t)
	service.SetAutoMergeEnabled(true)
	claimer := &recordingQueueAttachmentClaimer{}
	handlers.SetAttachmentClaimer(claimer)
	target, err := service.QueueMessage(context.Background(), "session", "task", "first", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	response, err := handlers.wsQueueMessage(context.Background(), autoMergeQueueRequest(t, "second", "source-file"))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var queued messagequeue.QueuedMessage
	require.NoError(t, json.Unmarshal(response.Payload, &queued))
	require.Equal(t, target.ID, queued.ID)
	require.Equal(t, "first\n\nsecond", queued.Content)
	require.Equal(t, []string{"source-file"}, claimer.claims)
	status := service.GetStatus(context.Background(), "session")
	require.Equal(t, 1, status.Count)
	require.Equal(t, "source-file", status.Entries[0].Attachments[0].AttachmentID)
}

func TestWsQueueMessage_AutoMergeClaimFailureRollsBackOnlySource(t *testing.T) {
	handlers, service := setupQueueHandlers(t)
	service.SetAutoMergeEnabled(true)
	claimer := &recordingQueueAttachmentClaimer{claimErr: errors.New("staged file expired")}
	handlers.SetAttachmentClaimer(claimer)
	target, err := service.QueueMessage(context.Background(), "session", "task", "first", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	before, err := json.Marshal(target)
	require.NoError(t, err)

	response, err := handlers.wsQueueMessage(context.Background(), autoMergeQueueRequest(t, "second", "expired-file"))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Equal(t, ws.ErrorCodeValidation, parseError(t, response).Code)
	status := service.GetStatus(context.Background(), "session")
	require.Equal(t, 1, status.Count)
	after, err := json.Marshal(status.Entries[0])
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestWsQueueMessage_AutoMergeBlocksLaterAdmissionUntilClaimFinalizes(t *testing.T) {
	handlers, service := setupQueueHandlers(t)
	service.SetAutoMergeEnabled(true)
	claimer := &blockingQueueAttachmentClaimer{started: make(chan struct{}), release: make(chan struct{})}
	handlers.SetAttachmentClaimer(claimer)
	_, err := service.QueueMessage(context.Background(), "session", "task", "first", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	handlerDone := make(chan error, 1)
	go func() {
		response, callErr := handlers.wsQueueMessage(context.Background(), autoMergeQueueRequest(t, "second", "source-file"))
		if callErr == nil && response.Type != ws.MessageTypeResponse {
			callErr = errors.New("queue handler returned an error response")
		}
		handlerDone <- callErr
	}()
	<-claimer.started

	laterDone := make(chan error, 1)
	go func() {
		_, queueErr := service.QueueMessage(context.Background(), "session", "task", "third", "", messagequeue.QueuedByUser, false, nil)
		laterDone <- queueErr
	}()
	select {
	case err := <-laterDone:
		t.Fatalf("later admission bypassed attachment claim: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(claimer.release)
	require.NoError(t, <-handlerDone)
	require.NoError(t, <-laterDone)
	status := service.GetStatus(context.Background(), "session")
	require.Equal(t, 1, status.Count)
	require.Equal(t, "first\n\nsecond\n\nthird", status.Entries[0].Content)
}

type blockingQueueAttachmentClaimer struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingQueueAttachmentClaimer) ClaimMessageAttachments(context.Context, string, string, []v1.MessageAttachment) error {
	close(c.started)
	<-c.release
	return nil
}

func autoMergeQueueRequest(t *testing.T, content, attachmentID string) *ws.Message {
	t.Helper()
	return createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "session",
		"task_id":    "task",
		"content":    content,
		"attachments": []messagequeue.MessageAttachment{{
			Type: "resource", AttachmentID: attachmentID, Name: attachmentID + ".txt", MimeType: "text/plain", SizeBytes: 1,
		}},
	})
}
