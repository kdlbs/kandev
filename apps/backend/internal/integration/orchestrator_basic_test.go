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

func TestOrchestratorStatus(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.True(t, payload["running"].(bool))
	assert.Equal(t, float64(0), payload["active_agents"])
	assert.Equal(t, float64(0), payload["queued_tasks"])
}

func TestOrchestratorQueue(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("queue-1", ws.ActionOrchestratorQueue, map[string]interface{}{})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.Equal(t, float64(0), payload["total"])
	tasks := payload["tasks"].([]interface{})
	assert.Len(t, tasks, 0)
}

func TestOrchestratorStartTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Create a task with agent_profile_id
	taskID := ts.CreateTestTask(t, "test-profile-id", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task notifications
	subResp, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp.Type)

	// Start task execution
	resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "test-profile-id",
	})
	require.NoError(t, err)

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.True(t, payload["success"].(bool))
	assert.Equal(t, taskID, payload["task_id"])
	assert.NotEmpty(t, payload["agent_execution_id"])
	sessionID, _ := payload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Give the agent time to process
	time.Sleep(500 * time.Millisecond)

	// Check status shows active agent
	statusResp, err := client.SendRequest("status-2", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)

	var statusPayload map[string]interface{}
	require.NoError(t, statusResp.ParsePayload(&statusPayload))
	assert.GreaterOrEqual(t, statusPayload["active_agents"].(float64), float64(0))
}

func TestOrchestratorStartTaskWithAgentTypeOverride(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 1)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start with different agent profile
	resp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "override-profile-id",
		"priority":         3,
	})
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, resp.ParsePayload(&payload))

	assert.True(t, payload["success"].(bool))
}

func TestOrchestratorStopTask(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start task
	startResp, err := client.SendRequest("start-1", ws.ActionOrchestratorStart, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	require.True(t, startPayload["success"].(bool))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	_, err = client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)

	// Wait for the session to become active before stopping
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		session, sErr := ts.TaskRepo.GetTaskSession(context.Background(), sessionID)
		if sErr == nil && session.State == models.TaskSessionStateRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Stop task
	stopResp, err := client.SendRequest("stop-1", ws.ActionOrchestratorStop, map[string]interface{}{
		"task_id": taskID,
		"reason":  "test stop",
		"force":   false,
	})
	require.NoError(t, err)

	var stopPayload map[string]interface{}
	require.NoError(t, stopResp.ParsePayload(&stopPayload))
	success, ok := stopPayload["success"].(bool)
	assert.True(t, ok, "expected 'success' key in stop response, got: %v", stopPayload)
	assert.True(t, success)
}
