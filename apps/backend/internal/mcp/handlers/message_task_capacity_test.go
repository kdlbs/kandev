package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// capacityQueueOrchestrator reports a full ceiling so the agent-message path
// must keep its prompt in the existing queue and report the queued status.
type capacityQueueOrchestrator struct {
	*fakeOrchestrator

	reserveCalls      int
	promptAttempts    int
	registeredTaskID  string
	registeredSession string
	registeredEntryID string
}

func (o *capacityQueueOrchestrator) ReserveMessageTurn(
	context.Context, string, string, string,
) (*orchestrator.MessageTurnReservation, bool) {
	o.reserveCalls++
	return nil, false
}

func (o *capacityQueueOrchestrator) PromptTask(
	context.Context, string, string, string, string, bool, []v1.MessageAttachment, bool,
) (*orchestrator.PromptResult, error) {
	o.promptAttempts++
	return nil, orchestrator.ErrMessageTurnCapacityExceeded
}

func (o *capacityQueueOrchestrator) RegisterMessageCapacityWait(
	_ context.Context, taskID, sessionID, entryID string,
) {
	o.registeredTaskID = taskID
	o.registeredSession = sessionID
	o.registeredEntryID = entryID
}

// TestMessageTaskCapacity_ReportsQueuedAtFullCapacity verifies the inter-task
// message contract: a capacity refusal keeps the recorded message, persists a
// queue receipt, and returns the existing queued status with its entry ID.
func TestMessageTaskCapacity_ReportsQueuedAtFullCapacity(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateWaitingForInput)

	h, fake := newMessageTaskHandler(t, svc, repo)
	orch := &capacityQueueOrchestrator{fakeOrchestrator: fake}
	h.sessionLauncher = orch

	payload := map[string]interface{}{
		"task_id":           target.ID,
		"session_id":        primary.ID,
		"prompt":            "wait for capacity",
		"sender_task_id":    sender.ID,
		"sender_session_id": "sender-sess-1",
	}
	resp, err := h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	require.Equal(t, "queued", out["status"])
	require.Equal(t, primary.ID, out["session_id"])
	require.Equal(t, 1, orch.promptAttempts)
	require.Zero(t, orch.reserveCalls, "the idle prompt path refuses before a request reservation")

	queue := fake.GetMessageQueue()
	identity, err := queue.ResolveSessionIdentity(context.Background(), target.ID, primary.ID)
	require.NoError(t, err)
	status, err := queue.Snapshot(context.Background(), identity)
	require.NoError(t, err)
	require.Equal(t, 1, status.Count)
	require.Equal(t, true, status.Entries[0].Metadata["user_message_recorded"])
	require.NotEmpty(t, orch.registeredEntryID)
}

// TestMessageTaskCapacity_QueuesPinnedCompletedFollowUp keeps the explicit
// completed-session permission but defers its runtime launch until capacity is
// available. Raw queue metadata never grants the intent.
func TestMessageTaskCapacity_QueuesPinnedCompletedFollowUp(t *testing.T) {
	svc, repo := newTestTaskService(t)
	sender, target, primary := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCompleted)
	require.Equal(t, models.TaskSessionStateCompleted, primary.State)

	h, fake := newMessageTaskHandler(t, svc, repo)
	orch := &capacityQueueOrchestrator{fakeOrchestrator: fake}
	h.sessionLauncher = orch

	payload := map[string]interface{}{
		"task_id":           target.ID,
		"session_id":        primary.ID,
		"prompt":            "completed follow-up waits",
		"sender_task_id":    sender.ID,
		"sender_session_id": "sender-sess-1",
	}
	resp, err := h.handleMessageTask(context.Background(), makeWSMessage(t, ws.ActionMCPMessageTask, payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, ws.MessageTypeResponse, resp.Type, string(resp.Payload))

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &out))
	require.Equal(t, "queued", out["status"])
	require.Equal(t, 1, orch.reserveCalls)
	require.Zero(t, orch.promptAttempts, "a completed follow-up must not launch during a capacity refusal")
	require.Equal(t, target.ID, orch.registeredTaskID)
	require.Equal(t, primary.ID, orch.registeredSession)

	queue := fake.GetMessageQueue()
	identity, err := queue.ResolveSessionIdentity(context.Background(), target.ID, primary.ID)
	require.NoError(t, err)
	status, err := queue.Snapshot(context.Background(), identity)
	require.NoError(t, err)
	require.Equal(t, 1, status.Count)
	require.Equal(t, true, status.Entries[0].Metadata[orchestrator.MetaKeyCompletedFollowUpIntent])
	require.NotEqual(t, true, status.Entries[0].Metadata["user_message_recorded"],
		"the deferred transcript must be recorded when the queued turn dispatches")
}
