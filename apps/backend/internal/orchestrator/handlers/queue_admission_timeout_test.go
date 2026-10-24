package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type timeoutMutationQueue struct {
	*messagequeue.Service
}

func TestQueueAdmissionTimeoutThroughSQLiteHandlers(t *testing.T) {
	h, queue, db := newPersistentCleanupQueue(t, filepath.Join(t.TempDir(), "queue.db"))
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	entry, err := queue.QueueMessage(ctx, "session", "task", "retain me", "", "user", false, nil)
	require.NoError(t, err)
	lease, err := queue.BeginEdit(ctx, entry.SessionID, entry.ID, "connection")
	require.NoError(t, err)
	held, release, holderDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		holderDone <- queue.WithSessionAdmission(ctx, entry.SessionID, func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer func() { close(release); require.NoError(t, <-holderDone) }()
	cases := []struct {
		action string
		handle func(context.Context, *ws.Message) (*ws.Message, error)
	}{
		{ws.ActionMessageQueueAdd, h.wsQueueMessage},
		{ws.ActionMessageQueueAppend, h.wsAppendToQueue},
		{ws.ActionMessageQueueUpdate, h.wsUpdateMessage},
		{ws.ActionMessageQueueEditBegin, h.wsBeginEdit},
		{ws.ActionMessageQueueEditRenew, h.wsRenewEdit},
		{ws.ActionMessageQueueEditEnd, h.wsEndEdit},
	}
	type result struct {
		action   string
		response *ws.Message
		err      error
	}
	results := make(chan result, len(cases))
	for _, tc := range cases {
		msg := createTestMessage(t, tc.action, map[string]interface{}{
			"task_id": "task", "session_id": "session", "content": "must not change",
			"entry_id": entry.ID, "lease_id": lease.LeaseID, "operation_id": "operation",
			"expected_target_revision": lease.TargetRevision,
		})
		operationCtx := ctx
		if tc.action != ws.ActionMessageQueueUpdate {
			operationCtx = ws.WithConnectionID(ctx, "connection")
		}
		go func() { response, err := tc.handle(operationCtx, msg); results <- result{tc.action, response, err} }()
	}
	for range cases {
		got := <-results
		require.NoError(t, got.err, got.action)
		var payload struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(got.response.Payload, &payload))
		require.Equal(t, ws.ErrorCodeUnavailable, payload.Code, got.action)
	}
}

func (*timeoutMutationQueue) CancelAllWithEntries(context.Context, string) ([]messagequeue.QueuedMessage, error) {
	return nil, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) CancelAllForSession(context.Context, messagequeue.QueueSessionIdentity) (*messagequeue.QueueRemovalResult, error) {
	return nil, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) MergeIntoAbove(context.Context, string, string, string) (*messagequeue.QueuedMessage, error) {
	return nil, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) MergeIntoAboveForSession(context.Context, messagequeue.QueueSessionIdentity, string, string) (*messagequeue.QueuedMessage, error) {
	return nil, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) ReorderEntries(context.Context, string, []string) error {
	return messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) ReorderEntriesForSession(context.Context, messagequeue.QueueSessionIdentity, []string) error {
	return messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) SetSessionAutoMerge(context.Context, messagequeue.QueueSessionIdentity, bool) (messagequeue.AutoMergePolicy, error) {
	return messagequeue.AutoMergePolicy{}, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) RemoveEntryWithEntry(context.Context, string, string) (*messagequeue.QueuedMessage, error) {
	return nil, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) RemoveEntryForSession(context.Context, messagequeue.QueueSessionIdentity, string) (*messagequeue.QueueRemovalResult, error) {
	return nil, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) DrainQueuedMessage(context.Context, string) (bool, error) {
	return false, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) DrainQueuedMessageForSession(context.Context, messagequeue.QueueSessionIdentity) (bool, error) {
	return false, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) SetQueueAutoRun(context.Context, string, bool) (bool, bool, error) {
	return false, false, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) SetQueueAutoRunForSession(context.Context, messagequeue.QueueSessionIdentity, bool) (bool, bool, error) {
	return false, false, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) SendQueuedNow(context.Context, string, string, string) (int, error) {
	return 0, messagequeue.ErrSessionAdmissionTimeout
}

func (*timeoutMutationQueue) SendQueuedNowForSession(context.Context, messagequeue.QueueSessionIdentity, string, string) (int, error) {
	return 0, messagequeue.ErrSessionAdmissionTimeout
}

func TestQueueMutationsReturnRetryableAdmissionTimeout(t *testing.T) {
	for _, identified := range []bool{false, true} {
		for _, action := range []string{
			ws.ActionMessageQueueCancel, ws.ActionMessageQueueMerge, ws.ActionMessageQueueReorder, ws.ActionMessageQueueAutoMergeSet,
			ws.ActionMessageQueueRemove, ws.ActionMessageQueueAutoRunSet, ws.ActionMessageQueueDrain, ws.ActionMessageQueueSendNow,
		} {
			t.Run(action+map[bool]string{false: "/legacy", true: "/identified"}[identified], func(t *testing.T) {
				queue := messagequeue.NewServiceMemory(logger.Default())
				queue.SetAutoMergeEnabled(false)
				entry, err := queue.QueueMessage(context.Background(), "session", "task", "retain me", "", "user", false, nil)
				require.NoError(t, err)
				var access QueueAccessAuthorizer = allowQueueAccess{}
				if identified {
					access = allowQueueIdentityAccess{}
				}
				timeoutQueue := &timeoutMutationQueue{Service: queue}
				h := NewQueueHandlers(timeoutQueue, &mockEventBus{}, logger.Default(), timeoutQueue, access, nil)
				msg := createTestMessage(t, action, map[string]interface{}{
					"task_id": "task", "session_id": "session", "session_incarnation_id": "memory:session",
					"entry_id": entry.ID, "ordered_ids": []string{entry.ID}, "enabled": false,
					"scope": "entry",
				})
				var response *ws.Message
				switch action {
				case ws.ActionMessageQueueCancel:
					response, err = h.wsCancelAll(context.Background(), msg)
				case ws.ActionMessageQueueMerge:
					response, err = h.wsMergeIntoAbove(context.Background(), msg)
				case ws.ActionMessageQueueReorder:
					response, err = h.wsReorder(context.Background(), msg)
				case ws.ActionMessageQueueAutoMergeSet:
					response, err = h.wsSetAutoMerge(context.Background(), msg)
				case ws.ActionMessageQueueRemove:
					response, err = h.wsRemoveEntry(context.Background(), msg)
				case ws.ActionMessageQueueAutoRunSet:
					response, err = h.wsSetAutoRun(context.Background(), msg)
				case ws.ActionMessageQueueDrain:
					response, err = h.wsDrainQueue(context.Background(), msg)
				case ws.ActionMessageQueueSendNow:
					response, err = h.wsSendNow(context.Background(), msg)
				}
				require.NoError(t, err)
				require.Equal(t, ws.MessageTypeError, response.Type)
				var payload struct {
					Code string `json:"code"`
				}
				require.NoError(t, json.Unmarshal(response.Payload, &payload))
				require.Equal(t, ws.ErrorCodeUnavailable, payload.Code)
				require.Equal(t, "retain me", queue.GetStatus(context.Background(), "session").Entries[0].Content)
			})
		}
	}
}
