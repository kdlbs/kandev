package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type canonicalNotificationReader struct {
	reads   int
	message *models.Message
}

func (r *canonicalNotificationReader) GetMessage(context.Context, string) (*models.Message, error) {
	r.reads++
	return r.message, nil
}

func TestCanonicalMessageNotificationUsesCommittedSnapshot(t *testing.T) {
	committed := &models.Message{ID: "m", TaskSessionID: "s", Content: "committed"}
	reader := &canonicalNotificationReader{message: &models.Message{ID: "m", TaskSessionID: "s", Content: "later writer"}}
	receipt := &models.ConversationMutationReceipt{SessionID: "s", BaseRevision: 3, Revision: 4, Complete: true,
		Operations: []models.ConversationMutationOperation{{Entity: models.ConversationEntityMessage, Kind: models.ConversationMutationUpsert, ID: "m", Message: committed}},
	}
	payload := &lifecycle.AgentStreamEventPayload{SessionID: "s", Data: &lifecycle.AgentStreamEventData{MessageID: "m", ConversationReceipt: receipt}}
	message, evidence, err := canonicalMessageNotification(t.Context(), reader, payload)
	require.NoError(t, err)
	require.Same(t, committed, message)
	require.Same(t, receipt, evidence)
	require.Zero(t, reader.reads)
}

func TestCanonicalMessageNotificationRetainsRecoveryFallback(t *testing.T) {
	for _, receipt := range []*models.ConversationMutationReceipt{nil, {SessionID: "other"}, {SessionID: "s", Complete: true}} {
		reader := &canonicalNotificationReader{message: &models.Message{ID: "m", TaskSessionID: "s", Content: "source"}}
		payload := &lifecycle.AgentStreamEventPayload{SessionID: "s", Data: &lifecycle.AgentStreamEventData{MessageID: "m", ConversationReceipt: receipt}}
		message, evidence, err := canonicalMessageNotification(t.Context(), reader, payload)
		require.NoError(t, err)
		require.Same(t, reader.message, message)
		require.Nil(t, evidence)
		require.Equal(t, 1, reader.reads)
	}
}
