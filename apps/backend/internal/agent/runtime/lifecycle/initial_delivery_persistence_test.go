package lifecycle

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestInitialPromptPersistsCanonicalSubmissionBeforeDispatch(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Initial delivery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", QueueIncarnationID: "incarnation"}))
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	require.NoError(t, client.StreamUpdates(ctx, nil, nil, nil))
	waitForWSConnected(t, mock)
	stop := newTestStopCh(t)
	sm := NewSessionManager(newSessionTestLogger(), stop)
	stream := NewStreamManager(newSessionTestLogger(), StreamCallbacks{}, nil, stop)
	cleanupStreamManager(t, stop, stream)
	stream.setAgentDeliveryRepository(repo)
	sm.SetDependencies(nil, stream, nil, nil)
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", Status: v1.AgentStatusReady, DeliveryMode: DurableDeliveryV1, DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, agentctl: client, promptDoneCh: make(chan PromptCompletionSignal, 1)}
	id := initialPromptDeliverySubmissionID(execution)
	reached := false
	sm.beforePromptDispatchHook = func() {
		reached = true
		stored, err := repo.GetAgentDeliverySubmission(ctx, id)
		require.NoError(t, err, "canonical identity must exist before provider dispatch")
		require.Equal(t, models.DeliverySubmissionDispatching, stored.State)
		require.EqualValues(t, 2, stored.HarnessGeneration)
		var payload struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.Unmarshal(stored.Payload, &payload))
		require.Equal(t, "initial instruction", payload.Text)
		require.Equal(t, journal.SubmissionHash(stored.Payload), stored.PayloadHash)
	}
	_, err := sm.sendPrompt(ctx, execution, "initial instruction", false, nil, true, sendPromptCallbacks{deliverySubmissionID: id}, false)
	require.NoError(t, err)
	require.True(t, reached)
}
