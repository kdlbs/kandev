package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-CLARIFICATION-LIFECYCLE-001.10
func TestPeerMessageClarificationAnswerDrainsExactlyOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session1", "task1", "execution1")
	seedPendingClarificationMessage(t, repo, "task1", "session1")
	manager := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, newMockStepGetter(), manager)
	svc.turnService = &repoTurnService{repo: repo}
	completed := make(chan struct{}, 2)
	svc.onQueuedMessageExecutionComplete = func() { completed <- struct{}{} }
	identity, err := svc.messageQueue.ResolveSessionIdentity(ctx, "task1", "session1")
	require.NoError(t, err)
	_, err = svc.messageQueue.QueueMessageWithMetadataForSession(ctx, identity,
		"peer report after answer", "", messagequeue.QueuedByAgent, false, nil,
		map[string]interface{}{"sender_task_id": "peer-task"})
	require.NoError(t, err)

	svc.CheckQueueAdmissionReadiness(ctx, identity)
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session1").Count)
	manager.mu.Lock()
	before := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Empty(t, before, "the live question must hold the peer prompt")

	_, claimed, err := repo.CompleteActiveClarificationBundle(ctx, "pending-session1", "answered",
		map[string]interface{}{"q1": map[string]interface{}{"custom_text": "Use PostgreSQL"}})
	require.NoError(t, err)
	require.True(t, claimed, "the actual durable question must accept the answer")
	require.NoError(t, svc.handleClarificationAnswered(ctx, bus.NewEvent(events.ClarificationAnswered, "test",
		map[string]interface{}{"session_id": "session1", "task_id": "task1",
			"pending_id": "pending-session1", "question": "Which approach?", "answer_text": "Use PostgreSQL"})))
	manager.mu.Lock()
	answered := append([]string(nil), manager.capturedPrompts...)
	manager.mu.Unlock()
	require.Len(t, answered, 1)
	require.Contains(t, answered[0], "Use PostgreSQL")
	require.Equal(t, 1, svc.messageQueue.GetStatus(ctx, "session1").Count,
		"the answer starts a turn; the peer waits for that turn's ready event")
	turn, err := repo.GetActiveTurnBySessionID(ctx, "session1")
	require.NoError(t, err)
	require.NotNil(t, turn)
	ready := watcher.AgentEventData{TaskID: "task1", SessionID: "session1", AgentExecutionID: "execution1", TurnID: turn.ID}
	svc.handleAgentReady(ctx, ready)
	select {
	case <-completed:
	case <-time.After(5 * time.Second):
		t.Fatal("answer ready event did not deliver the queued peer message")
	}
	require.Zero(t, svc.messageQueue.GetStatus(ctx, "session1").Count)
	successor, err := repo.GetActiveTurnBySessionID(ctx, "session1")
	require.NoError(t, err)
	require.NotNil(t, successor)
	require.NotEqual(t, turn.ID, successor.ID)
	svc.handleAgentReady(ctx, ready)
	stillActive, err := repo.GetActiveTurnBySessionID(ctx, "session1")
	require.NoError(t, err)
	require.NotNil(t, stillActive, "a delayed ready event must leave the peer's successor turn open")
	require.Equal(t, successor.ID, stillActive.ID)
	svc.CheckQueueAdmissionReadiness(ctx, identity)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	require.Equal(t, []string{answered[0], "peer report after answer"}, manager.capturedPrompts,
		"duplicate predecessor ready and admission checks must not repeat peer delivery")
}
