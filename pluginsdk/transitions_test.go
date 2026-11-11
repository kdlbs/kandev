package pluginsdk

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.4 AC-PLUGINS-WORKFLOW-HISTORY-002.1
func TestHostClientExposesOptionalTransitionHistory(t *testing.T) {
	if _, ok := reflect.TypeOf(&grpcHostClient{}).MethodByName("Transitions"); !ok {
		t.Fatal("Host client has no transition-history extension")
	}
}

type recordingTransitionHost struct {
	*dataRecordingHost
	reader *recordingTransitionReader
}

func (h *recordingTransitionHost) Transitions() TransitionHistoryReader { return h.reader }

type recordingTransitionReader struct {
	lastTaskID string
	lastPage   Page
	lastWFID   string
}

func (r *recordingTransitionReader) ListTask(_ context.Context, taskID string, page Page) ([]TaskStepTransition, *PageInfo, error) {
	r.lastTaskID, r.lastPage = taskID, page
	return []TaskStepTransition{{ID: "9223372036854775806", ToWorkflowStepID: strPtr("review"), Trigger: "test", OccurredAt: "2026-09-22T12:00:00Z"}}, &PageInfo{NextCursor: "next", HasMore: true}, nil
}

func (r *recordingTransitionReader) ListWorkflowGroups(_ context.Context, workflowID string, page Page) ([]WorkflowTransitionGroup, *PageInfo, error) {
	r.lastWFID, r.lastPage = workflowID, page
	return []WorkflowTransitionGroup{{Kind: "entry", ToStepID: strPtr("review"), Count: 3}}, &PageInfo{}, nil
}

// @covers AC-PLUGINS-WORKFLOW-HISTORY-001.1 AC-PLUGINS-WORKFLOW-HISTORY-002.1
func TestTransitionHistoryRoundTripsAcrossHostRPC(t *testing.T) {
	impl := &recordingTransitionHost{dataRecordingHost: &dataRecordingHost{}, reader: &recordingTransitionReader{}}
	host := dialHostOverBufconn(t, impl)
	reader, ok := TransitionHistory(host)
	require.True(t, ok)
	items, page, err := reader.ListTask(context.Background(), "task-1", Page{Limit: 2, Cursor: "before"})
	require.NoError(t, err)
	require.Equal(t, "task-1", impl.reader.lastTaskID)
	require.Equal(t, Page{Limit: 2, Cursor: "before"}, impl.reader.lastPage)
	require.Equal(t, "9223372036854775806", items[0].ID)
	require.Nil(t, items[0].FromWorkflowStepID)
	require.Equal(t, "review", *items[0].ToWorkflowStepID)
	require.Equal(t, &PageInfo{NextCursor: "next", HasMore: true}, page)
	groups, _, err := reader.ListWorkflowGroups(context.Background(), "wf-1", Page{Limit: 5})
	require.NoError(t, err)
	require.Equal(t, "wf-1", impl.reader.lastWFID)
	require.Equal(t, int64(3), groups[0].Count)
	require.Nil(t, groups[0].FromStepID)
}
