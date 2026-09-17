package messagequeue

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssistantContextQueueCannotMergeRevisions(t *testing.T) {
	first := QueuedMessage{ID: "first", SessionID: "s", TaskID: "task", Position: 0, Content: "old", QueuedBy: QueuedByUser,
		Metadata: map[string]interface{}{"orchestration_context_ref": "old"}}
	second := first
	second.ID, second.Position, second.Content = "second", 1, "new"
	second.Metadata = map[string]interface{}{"orchestration_context_ref": "new"}
	require.False(t, mergeAllowed(&second, &first, QueuedByUser), "manual merge must not discard the new revision")
	_, err := BuildSendNowEnvelope([]QueuedMessage{first, second})
	require.Error(t, err, "bulk send-now must not mix authority snapshots")
	_, ok := buildAutoMergedEntry(&first, &second)
	require.False(t, ok)
	second.Metadata = first.Metadata
	require.True(t, mergeAllowed(&second, &first, QueuedByUser))
	_, err = BuildSendNowEnvelope([]QueuedMessage{first, second})
	require.NoError(t, err)
}
