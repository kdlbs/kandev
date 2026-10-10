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
	sm, execution, repo := initialDeliveryPersistenceFixture(t)
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

func TestInitialPromptUsesCanonicalSubmissionAdmittedByItsCallback(t *testing.T) {
	ctx := context.Background()
	sm, execution, repo := initialDeliveryPersistenceFixture(t)
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first-message")
	payload := []byte(`{"text":"initial instruction"}`)
	reached := false
	sm.beforePromptDispatchHook = func() { reached = true }
	admit := func() error {
		_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
			ID: "prompt:first-message", SessionID: "session", IncarnationID: "incarnation",
			HarnessGeneration: 2, OwnerGeneration: 2, DispatchAttemptID: "first-message",
			Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionDispatching,
		})
		return err
	}
	_, err := sm.sendPrompt(ctx, execution, "initial instruction", false, nil, true,
		sendPromptCallbacks{beforeAdmission: admit, deliverySubmissionID: "first-message"}, false)
	require.NoError(t, err)
	require.True(t, reached, "the admitted first instruction must reach native dispatch")
	stored, err := repo.GetAgentDeliverySubmission(ctx, "prompt:first-message")
	require.NoError(t, err)
	require.Equal(t, "first-message", stored.DispatchAttemptID)
	execution.promptDoneCh <- PromptCompletionSignal{StopReason: "end_turn", PromptGeneration: execution.promptGenerationSnapshot()}
	reached = false
	_, err = sm.sendPrompt(ctx, execution, "initial instruction", false, nil, true,
		sendPromptCallbacks{beforeAdmission: admit, deliverySubmissionID: "first-message"}, false)
	require.Error(t, err, "a caller callback cannot authorize a repeated runtime dispatch")
	require.False(t, reached)
}

func TestInitialPromptNeverReusesUnownedCanonicalSubmission(t *testing.T) {
	for _, test := range []struct {
		name        string
		incarnation string
		attempt     string
		callback    bool
	}{
		{name: "existing runtime attempt", incarnation: "incarnation", attempt: "execution"},
		{name: "foreign incarnation", incarnation: "other", attempt: "first-message", callback: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			sm, execution, repo := initialDeliveryPersistenceFixture(t)
			execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first-message")
			payload := []byte(`{"text":"initial instruction"}`)
			_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
				ID: "prompt:first-message", SessionID: "session", IncarnationID: test.incarnation,
				HarnessGeneration: 2, OwnerGeneration: 2, DispatchAttemptID: test.attempt,
				Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionDispatching,
			})
			require.NoError(t, err)
			reached := false
			sm.beforePromptDispatchHook = func() { reached = true }
			callbacks := sendPromptCallbacks{deliverySubmissionID: "first-message"}
			if test.callback {
				callbacks.beforeAdmission = func() error { return nil }
			}
			_, err = sm.sendPrompt(ctx, execution, "initial instruction", false, nil, true, callbacks, false)
			require.Error(t, err)
			require.False(t, reached)
		})
	}
}

func TestInitialPromptUsesCanonicalSubmissionFromLaunchHandoff(t *testing.T) {
	ctx := context.Background()
	sm, execution, repo := initialDeliveryPersistenceFixture(t)
	execution.setMetadataValue(initialDeliverySubmissionIDMetadataKey, "first-message")
	payload := []byte(`{"text":"initial instruction"}`)
	_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "prompt:first-message", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 2, OwnerGeneration: 2, DispatchAttemptID: "first-message",
		Payload: payload, PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionDispatching,
	})
	require.NoError(t, err)
	reached := false
	sm.beforePromptDispatchHook = func() { reached = true }
	_, err = sm.sendPrompt(ctx, execution, "initial instruction", false, nil, true,
		sendPromptCallbacks{deliverySubmissionID: "first-message"}, false)
	require.NoError(t, err)
	require.True(t, reached)
}

func initialDeliveryPersistenceFixture(t *testing.T) (*SessionManager, *AgentExecution, initialDeliveryAdmissionStore) {
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
	return sm, execution, repo
}
