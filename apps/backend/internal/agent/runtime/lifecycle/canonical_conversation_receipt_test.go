package lifecycle

import (
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCanonicalReceiptCoalescing(t *testing.T) {
	var published []coalescedStreamChunk
	coalescer := newStreamCoalescer(time.Hour, func(chunk coalescedStreamChunk) { published = append(published, chunk) })
	defer coalescer.close()
	for i := int64(0); i < 4; i++ {
		coalescer.add(coalescedStreamChunk{
			eventType: "message_streaming", messageID: "m", content: "x", isAppend: i > 0, canonicalProjection: true,
			conversationReceipt: canonicalTestReceipt(i, i+1),
		})
	}
	coalescer.flush()
	require.Len(t, published, 2)
	receipt := published[1].conversationReceipt
	require.NotNil(t, receipt)
	require.True(t, receipt.Complete)
	require.Equal(t, int64(1), receipt.BaseRevision)
	require.Equal(t, int64(4), receipt.Revision)
	require.Len(t, receipt.Operations, 1)
	require.Equal(t, "snapshot-4", receipt.Operations[0].Message.Content)
}

func TestCanonicalReceiptMergeRejectsMissingCoverage(t *testing.T) {
	for _, next := range []*models.ConversationMutationReceipt{nil, canonicalTestReceipt(3, 4), {SessionID: "other", BaseRevision: 2, Revision: 3, Complete: true}, {SessionID: "s", BaseRevision: 2, Revision: 3, Complete: false}} {
		require.Nil(t, mergeCanonicalConversationReceipts(canonicalTestReceipt(1, 2), next))
	}
	require.Nil(t, mergeCanonicalConversationReceipts(nil, canonicalTestReceipt(1, 2)))
}

func TestCanonicalReceiptMergePreservesCreatedTurn(t *testing.T) {
	first := canonicalTestReceipt(0, 2)
	first.Operations = append([]models.ConversationMutationOperation{{Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityTurn, ID: "turn", SessionID: "s", Turn: &models.Turn{ID: "turn"}}}, first.Operations...)
	merged := mergeCanonicalConversationReceipts(first, canonicalTestReceipt(2, 3))
	require.NotNil(t, merged)
	require.Len(t, merged.Operations, 2)
	require.Equal(t, "turn", merged.Operations[0].ID)
	require.Equal(t, "snapshot-3", merged.Operations[1].Message.Content)
	require.Equal(t, "snapshot-2", first.Operations[1].Message.Content)
}

func canonicalTestReceipt(base, revision int64) *models.ConversationMutationReceipt {
	return &models.ConversationMutationReceipt{
		SessionID: "s", BaseRevision: base, Revision: revision, Complete: true,
		Operations: []models.ConversationMutationOperation{{
			Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityMessage, ID: "m", SessionID: "s",
			Message: &models.Message{ID: "m", Content: fmt.Sprintf("snapshot-%d", revision)},
		}},
	}
}

func TestCanonicalReceiptCannotCrossAgentWire(t *testing.T) {
	event := streams.AgentEvent{CanonicalConversationReceipt: encodeCanonicalConversationReceipt(canonicalTestReceipt(1, 2))}
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "snapshot")
	var decoded streams.AgentEvent
	require.NoError(t, json.Unmarshal([]byte(`{"canonical_conversation_receipt":{"complete":true},"CanonicalConversationReceipt":{"complete":true}}`), &decoded))
	require.Nil(t, decoded.CanonicalConversationReceipt)
}
