package service

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSetUserMessageDeliveryStatusMarksExactSessionUserMessage(t *testing.T) {
	svc, bus, repo := newMessageTestService(t)
	ctx := context.Background()
	message := seedMessage(t, repo, &models.Message{
		ID: "message-not-delivered", Content: "Fix the button spacing",
		AuthorType: models.MessageAuthorUser,
		Metadata:   map[string]interface{}{"sender_task_id": "task-sender"},
	})
	bus.ClearEvents()

	marked, err := svc.SetUserMessageDeliveryStatus(
		ctx, message.ID, "sess-msg", models.MessageDeliveryStatusNotDelivered,
	)
	require.NoError(t, err)
	require.True(t, marked)

	stored, err := svc.GetMessage(ctx, message.ID)
	require.NoError(t, err)
	require.Equal(t, models.MessageDeliveryStatusNotDelivered,
		stored.Metadata[models.MessageMetaKeyDeliveryStatus])
	require.Equal(t, "task-sender", stored.Metadata["sender_task_id"],
		"unrelated metadata must be preserved")

	updated := 0
	for _, event := range bus.GetPublishedEvents() {
		if event.Type == events.MessageUpdated {
			updated++
		}
	}
	require.Equal(t, 1, updated)

	marked, err = svc.SetUserMessageDeliveryStatus(
		ctx, message.ID, "sess-msg", models.MessageDeliveryStatusNotDelivered,
	)
	require.NoError(t, err)
	require.True(t, marked)
	updated = 0
	for _, event := range bus.GetPublishedEvents() {
		if event.Type == events.MessageUpdated {
			updated++
		}
	}
	require.Equal(t, 1, updated, "reapplying the same status must not publish a duplicate update")
}

func TestCreateCompletedUserMessageIdempotentAvoidsActiveTurn(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	message, err := svc.CreateCompletedUserMessageIdempotent(
		ctx, "message-completed", &CreateMessageRequest{
			TaskSessionID: "sess-msg",
			TaskID:        "task-msg",
			Content:       "Fix the button spacing (2 notes)",
			AuthorType:    "user",
			Metadata: map[string]interface{}{
				models.MessageMetaKeyDeliveryStatus: models.MessageDeliveryStatusNotDelivered,
			},
		},
	)
	require.NoError(t, err)
	require.NotEmpty(t, message.TurnID)
	require.NotEqual(t, "turn-msg", message.TurnID,
		"a history-only message must not adopt the existing active turn")

	turn, err := repo.GetTurn(ctx, message.TurnID)
	require.NoError(t, err)
	require.NotNil(t, turn.CompletedAt)
	require.Equal(t, true, turn.Metadata[models.TurnMetaKeyLifecycleOnly])

	active, err := repo.GetActiveTurnBySessionID(ctx, "sess-msg")
	require.NoError(t, err)
	require.Equal(t, "turn-msg", active.ID, "the pre-existing active turn must stay untouched")

	replayed, err := svc.CreateCompletedUserMessageIdempotent(
		ctx, "message-completed", &CreateMessageRequest{
			TaskSessionID: "sess-msg",
			TaskID:        "task-msg",
			Content:       "Fix the button spacing (2 notes)",
			AuthorType:    "user",
		},
	)
	require.NoError(t, err)
	require.Equal(t, message.TurnID, replayed.TurnID)
	turns, err := repo.ListTurnsBySession(ctx, "sess-msg")
	require.NoError(t, err)
	require.Len(t, turns, 2, "replay must not create another turn")
}

func TestSetUserMessageDeliveryStatusSkipsAbsentOrForeignMessages(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	agentMessage := seedMessage(t, repo, &models.Message{
		ID: "agent-message", Content: "agent output", AuthorType: models.MessageAuthorAgent,
	})
	userMessage := seedMessage(t, repo, &models.Message{
		ID: "user-message", Content: "user input", AuthorType: models.MessageAuthorUser,
	})

	marked, err := svc.SetUserMessageDeliveryStatus(
		ctx, "missing-message", "sess-msg", models.MessageDeliveryStatusNotDelivered,
	)
	require.NoError(t, err)
	require.False(t, marked)

	marked, err = svc.SetUserMessageDeliveryStatus(
		ctx, agentMessage.ID, "sess-msg", models.MessageDeliveryStatusNotDelivered,
	)
	require.NoError(t, err)
	require.False(t, marked)

	marked, err = svc.SetUserMessageDeliveryStatus(
		ctx, userMessage.ID, "sess-other", models.MessageDeliveryStatusNotDelivered,
	)
	require.NoError(t, err)
	require.False(t, marked)

	agentStored, err := svc.GetMessage(ctx, agentMessage.ID)
	require.NoError(t, err)
	require.NotContains(t, agentStored.Metadata, models.MessageMetaKeyDeliveryStatus)
	userStored, err := svc.GetMessage(ctx, userMessage.ID)
	require.NoError(t, err)
	require.NotContains(t, userStored.Metadata, models.MessageMetaKeyDeliveryStatus)
}

func TestCreateCompletedUserMessageIdempotentConcurrentCreatesOneTurn(t *testing.T) {
	svc, _, repo := newMessageTestService(t)
	ctx := context.Background()
	const workers = 4
	start := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := svc.CreateCompletedUserMessageIdempotent(
				ctx, "message-completed-concurrent", &CreateMessageRequest{
					TaskSessionID: "sess-msg",
					TaskID:        "task-msg",
					Content:       "concurrent release retry",
					AuthorType:    "user",
				},
			)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	turns, err := repo.ListTurnsBySession(ctx, "sess-msg")
	require.NoError(t, err)
	require.Len(t, turns, 2, "only one completed lifecycle turn may be created")
	messages, err := repo.ListMessages(ctx, "sess-msg")
	require.NoError(t, err)
	require.Len(t, messages, 1, "only one message may be stored")
}
