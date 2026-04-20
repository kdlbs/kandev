// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestOrchestratorCancelAfterAgentCrash_UnsticksSession reproduces the "stuck session" bug.
//
// Scenario: an agent subprocess crashes while the task's session is in RUNNING state. The
// lifecycle manager no longer tracks an execution for the session, but the DB still says
// RUNNING. The user clicks "pause"/"end turn" in the UI, which sends `agent.cancel`. The
// expected UX is that the session unsticks — transitions to WAITING_FOR_INPUT — so the user
// can send a new prompt. Today the cancel propagates the lifecycle's "no execution for session"
// error, leaving the session stuck forever.
func TestOrchestratorCancelAfterAgentCrash_UnsticksSession(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start the task so a session is created and an execution is registered in the simulator.
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, startResp.Type, "start should succeed")

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	// Pin the session to RUNNING to model the stuck state observed in production
	// (task_session_state=RUNNING, is_agent_working=true, agentctl_execution_id=null).
	// We write directly to the repo because the simulator only emits AgentReady, not
	// the AgentRunning event that would normally drive the transition.
	require.NoError(t, ts.TaskRepo.UpdateTaskSessionState(
		context.Background(), sessionID, models.TaskSessionStateRunning, "",
	))

	// Kill the simulated agent mid-turn. The execution is dropped from the manager's store
	// so subsequent CancelAgent lookups return ErrNoExecutionForSession, exactly like the
	// real lifecycle manager after a subprocess crash.
	ts.AgentManager.CrashAgentForSession(sessionID)

	// User clicks "pause" / "end turn" — the WS path the frontend actually uses.
	cancelResp, err := client.SendRequest("cancel-1", ws.ActionAgentCancel, map[string]interface{}{
		"session_id": sessionID,
	})
	require.NoError(t, err)

	// 1. The cancel must succeed (not error out), so the frontend can clear its "working" spinner.
	assert.Equal(t, ws.MessageTypeResponse, cancelResp.Type,
		"cancel should succeed even when no execution exists for the session; got: %+v", cancelResp)

	// 2. The session must transition out of RUNNING so the user can send a new prompt.
	// Poll briefly because the state update may happen asynchronously after the WS response.
	deadline := time.Now().Add(2 * time.Second)
	var finalState models.TaskSessionState
	for time.Now().Before(deadline) {
		session, gErr := ts.TaskRepo.GetTaskSession(context.Background(), sessionID)
		require.NoError(t, gErr)
		finalState = session.State
		if finalState == models.TaskSessionStateWaitingForInput {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	assert.Equal(t, models.TaskSessionStateWaitingForInput, finalState,
		"session should unstick to WAITING_FOR_INPUT after cancel when no execution exists")

	// 3. A status message documenting the cancel should exist so there's a breadcrumb in chat.
	msgs, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)
	foundCancel := false
	for _, m := range msgs {
		if m.Type == models.MessageTypeStatus && m.Content == "Turn cancelled by user" {
			foundCancel = true
			break
		}
	}
	assert.True(t, foundCancel,
		"expected a 'Turn cancelled by user' status message to be recorded")
}

// TestOrchestratorCancelWhenAgentHangs_UnsticksSession covers the second path to a stuck
// session: the agent subprocess is alive and acknowledges the ACP cancel, but never
// publishes a completion event. The lifecycle manager's CancelAgent escalates and returns
// ErrCancelEscalated. The orchestrator service must treat that as a soft-fail (reconcile
// DB state) — otherwise the UI shows "agent is running" forever and cancel stays broken.
func TestOrchestratorCancelWhenAgentHangs_UnsticksSession(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, startResp.Type, "start should succeed")

	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	require.NoError(t, ts.TaskRepo.UpdateTaskSessionState(
		context.Background(), sessionID, models.TaskSessionStateRunning, "",
	))

	// Mark the simulated agent as hung: it's tracked, but CancelAgent will return
	// ErrCancelEscalated to model the real lifecycle escalation.
	ts.AgentManager.MarkAgentHungForSession(sessionID)

	cancelResp, err := client.SendRequest("cancel-1", ws.ActionAgentCancel, map[string]interface{}{
		"session_id": sessionID,
	})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, cancelResp.Type,
		"cancel should succeed even when lifecycle escalated after an unresponsive agent; got: %+v", cancelResp)

	deadline := time.Now().Add(2 * time.Second)
	var finalState models.TaskSessionState
	for time.Now().Before(deadline) {
		session, gErr := ts.TaskRepo.GetTaskSession(context.Background(), sessionID)
		require.NoError(t, gErr)
		finalState = session.State
		if finalState == models.TaskSessionStateWaitingForInput {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	assert.Equal(t, models.TaskSessionStateWaitingForInput, finalState,
		"session should unstick to WAITING_FOR_INPUT after cancel escalation")

	msgs, err := ts.TaskRepo.ListMessages(context.Background(), sessionID)
	require.NoError(t, err)
	foundCancel := false
	for _, m := range msgs {
		if m.Type == models.MessageTypeStatus && m.Content == "Turn cancelled by user" {
			foundCancel = true
			break
		}
	}
	assert.True(t, foundCancel,
		"expected a 'Turn cancelled by user' status message to be recorded")
}
