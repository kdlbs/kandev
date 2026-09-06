package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/stretchr/testify/require"
)

func TestCompleteStreamForwardsActingAgentProfileIdentity(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-stream-identity", "session-stream-identity", "")

	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	svc.turnService = &repoTurnService{repo: repo}
	firstTurn, err := svc.turnService.StartTurn(ctx, "session-stream-identity")
	require.NoError(t, err)
	svc.markExecutionCompleted("session-stream-identity", "execution-stream-identity")
	svc.completeTurnForSession(ctx, "session-stream-identity")

	svc.handleAgentStreamEvent(ctx, &lifecycle.AgentStreamEventPayload{
		TaskID:         "task-stream-identity",
		SessionID:      "session-stream-identity",
		ExecutionID:    "execution-stream-identity",
		AgentProfileID: "reviewer",
		Data:           &lifecycle.AgentStreamEventData{Type: agentEventComplete},
	})

	require.Len(t, eb.events, 1)
	require.Equal(t, events.AgentTurnMessageSaved, eb.events[0].subject)
	data, ok := eb.events[0].event.Data.(map[string]string)
	require.True(t, ok)
	require.Equal(t, firstTurn.ID, data["turn_id"])
	require.Equal(t, "reviewer", data[metaKeyAgentProfileID])
}
