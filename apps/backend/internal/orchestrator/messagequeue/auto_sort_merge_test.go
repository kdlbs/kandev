package messagequeue

import (
	"testing"

	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.7
// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.8
func TestSortedFoldTargetLatestAndBarriers(t *testing.T) {
	first, latest, other := sortTestEntry("A1", "A"), sortTestEntry("A2", "A"), sortTestEntry("B1", "B")
	latest.Model = "incompatible"
	source := sortTestEntry("A3", "A")
	require.Same(t, latest, testSortedFoldTarget([]*QueuedMessage{first, latest, other}, source), "never skip an incompatible latest sender entry")
	user := &QueuedMessage{QueuedBy: QueuedByUser}
	require.Same(t, first, testSortedFoldTarget([]*QueuedMessage{first, user, other}, source))
	barriers := map[string]*QueuedMessage{
		"reserved":     {QueuedBy: QueuedByAgent, Metadata: map[string]any{MetadataLifecycleReserved: true}},
		"lifecycle":    {QueuedBy: QueuedByAgent, Metadata: map[string]any{MetadataLifecycleDurable: true}},
		"plan-comment": {QueuedBy: QueuedByAgent, Metadata: map[string]any{plancomments.MetadataClientQueueID: "id", plancomments.MetadataRequestFingerprint: "hash"}},
		"managed":      {QueuedBy: QueuedByAgent, Metadata: map[string]any{MetadataManagedInput: true}},
		"workflow":     {QueuedBy: QueuedByWorkflow}, "server": {QueuedBy: QueuedByServer}, "move": {QueuedBy: QueuedByMoveTask},
	}
	for name, barrier := range barriers {
		t.Run(name, func(t *testing.T) {
			require.Nil(t, testSortedFoldTarget([]*QueuedMessage{first, barrier, other}, source))
		})
	}
	for _, by := range []string{QueuedByUser, QueuedByWorkflow, QueuedByServer, QueuedByMoveTask, "clarification"} {
		invalid := sortTestEntry("source", "A")
		invalid.QueuedBy = by
		require.Nil(t, testSortedFoldTarget([]*QueuedMessage{first}, invalid))
	}
	require.Nil(t, testSortedFoldTarget([]*QueuedMessage{first}, sortTestEntry("source", "")))
}

func testSortedFoldTarget(rows []*QueuedMessage, source *QueuedMessage) *QueuedMessage {
	target, _ := sortedFoldTarget(rows, source)
	return target
}
