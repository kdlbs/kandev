package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type combinedReceiptRepository struct{ combinedBatchRepository }

func (r *combinedReceiptRepository) ReceiveAndProjectCanonicalAgentDeliveryEventsWithReceipt(ctx context.Context, events []*models.AgentDeliveryEvent, effects []*models.AgentDeliveryEffect, high int64) ([]models.CanonicalAgentDeliveryProjectionResult, *models.ConversationMutationReceipt, error) {
	results, err := r.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, high)
	return results, canonicalTestReceipt(1, 3), err
}

func TestCombinedConversationReceiptReachesNotification(t *testing.T) {
	repository := &combinedReceiptRepository{}
	events := combinedProcessorEvents()
	execution, client, _ := deliveryProjectionFixture(t, events[1], &recordingAgentDeliveryAcknowledger{})
	var notifications []agentctl.AgentEvent
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(_ *AgentExecution, event agentctl.AgentEvent) { notifications = append(notifications, event) }}, nil, nil)
	defer sm.Wait()
	processor := newStreamEventProcessor(sm, context.Background(), execution, client, repository, 0, nil)
	for _, event := range events {
		require.True(t, processor.enqueue(event))
	}
	require.NoError(t, processor.close())
	require.Equal(t, 1, repository.combinedCalls)
	require.Zero(t, repository.receiveCalls)
	require.Len(t, notifications, 1)
	require.Equal(t, "firstsecond", notifications[0].Text)
	receipt := decodeCanonicalConversationReceipt(notifications[0].CanonicalConversationReceipt)
	require.NotNil(t, receipt, "combined delivery notification must retain the transaction snapshot")
	require.Equal(t, int64(1), receipt.BaseRevision)
	require.Equal(t, int64(3), receipt.Revision)
	require.Equal(t, "snapshot-3", receipt.Operations[0].Message.Content)
}
