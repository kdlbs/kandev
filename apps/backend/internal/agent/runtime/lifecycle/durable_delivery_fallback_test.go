package lifecycle

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDurableDeliveryFallbackProjectionRetry(t *testing.T) {
	repo := newDurableStreamSQLiteRepository(t)
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "task", WorkspaceID: "workspace", WorkflowStepID: "step", Title: "Task"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "session-1", TaskID: "task", State: models.TaskSessionStateRunning, QueueIncarnationID: "owner"}))
	event := agentctl.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "wake", DeliveryStreamID: "stream", DeliveryIncarnationID: "owner", DeliveryHarnessGeneration: 1, DeliverySequence: 1}
	callbacks := 0
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(*AgentExecution, agentctl.AgentEvent) { callbacks++ }}, nil, nil)
	ack := &recordingAgentDeliveryAcknowledger{}
	execution, client, acked := deliveryProjectionFixture(t, event, ack)
	execution.TaskID = "task"
	require.NoError(t, repo.UpsertExecutorRunning(t.Context(), &models.ExecutorRunning{ID: "session-1", SessionID: "session-1", TaskID: "task", AgentExecutionID: execution.ID, Status: models.ExecutorRunningStatusRunning}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(t.Context(), &models.HarnessSessionGeneration{SessionID: "session-1", IncarnationID: "owner", Generation: 1, NativeSessionID: "native", CreationReason: "initial"}))
	defer sm.Wait()
	_, err := repo.DB().Exec(`CREATE TRIGGER fail_fallback BEFORE INSERT ON task_session_turns BEGIN SELECT RAISE(ABORT,'projection failure'); END`)
	require.NoError(t, err)
	require.Error(t, sm.processAgentEvent(context.Background(), execution, client, repo, event, 0))
	require.Zero(t, callbacks)
	select {
	case <-acked:
		t.Fatal("failed projection acknowledged")
	default:
	}
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), "stream")
	require.NoError(t, err)
	require.Zero(t, cursor.ProjectedSequence)
	_, err = repo.DB().Exec(`DROP TRIGGER fail_fallback`)
	require.NoError(t, err)
	require.NoError(t, sm.processAgentEvent(t.Context(), execution, client, repo, event, 0))
	select {
	case <-acked:
	case <-time.After(5 * time.Second):
		t.Fatal("retry was not acknowledged")
	}
	require.Equal(t, 1, callbacks)
	turn, err := repo.GetTurn(t.Context(), "agent-delivery-turn-stream-1")
	require.NoError(t, err)
	require.Equal(t, "step", turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart])
	cursor, err = repo.GetAgentDeliveryCursor(t.Context(), "stream")
	require.NoError(t, err)
	require.EqualValues(t, 1, cursor.ProjectedSequence)
}
