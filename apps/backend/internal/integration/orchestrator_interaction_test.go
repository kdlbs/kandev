// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/pkg/acp/protocol"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestOrchestratorConcurrentTasks(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Create multiple tasks
	taskIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		taskIDs[i] = ts.CreateTestTask(t, "augment-agent", i+1)
	}

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Start all tasks concurrently
	var wg sync.WaitGroup
	results := make(chan map[string]interface{}, 3)

	for i, taskID := range taskIDs {
		wg.Add(1)
		go func(idx int, tid string) {
			defer wg.Done()

			resp, err := client.SendRequest(fmt.Sprintf("start-%d", idx), ws.ActionSessionLaunch, map[string]interface{}{
				"task_id":          tid,
				"agent_profile_id": "augment-agent",
			})
			if err != nil {
				t.Logf("Error starting task %d: %v", idx, err)
				return
			}

			var payload map[string]interface{}
			if err := resp.ParsePayload(&payload); err != nil {
				t.Logf("failed to parse start response: %v", err)
				return
			}
			results <- payload
		}(i, taskID)
	}

	wg.Wait()
	close(results)

	// Count successful launches
	successCount := 0
	for result := range results {
		if success, ok := result["success"].(bool); ok && success {
			successCount++
		}
	}

	assert.Equal(t, 3, successCount, "All tasks should start successfully")

	// Check status shows multiple active agents
	statusResp, err := client.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)

	var statusPayload map[string]interface{}
	require.NoError(t, statusResp.ParsePayload(&statusPayload))
	t.Logf("Active agents: %v", statusPayload["active_agents"])
}

func TestOrchestratorACPStreaming(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure simulated agent to emit specific ACP messages
	ts.AgentManager.SetACPMessageFn(func(taskID, executionID string) []protocol.Message {
		return []protocol.Message{
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 10,
					"stage":    "analyzing",
					"message":  "Analyzing codebase...",
				},
			},
			{
				Type:      protocol.MessageTypeLog,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"level":   "info",
					"message": "Found 5 files to modify",
				},
			},
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 50,
					"stage":    "modifying",
					"message":  "Modifying files...",
				},
			},
			{
				Type:      protocol.MessageTypeProgress,
				TaskID:    taskID,
				Timestamp: time.Now(),
				Data: map[string]interface{}{
					"progress": 100,
					"stage":    "completed",
					"message":  "All modifications complete",
				},
			},
		}
	})

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	// Subscribe to task
	_, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Start task
	startResp, err := client.SendRequest("start-1", ws.ActionSessionLaunch, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Extract session_id and subscribe to session for agent stream events
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	subResp, err := client.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp.Type, "Session subscription should succeed")

	// Wait for agent to process
	time.Sleep(500 * time.Millisecond)

	// Verify orchestrator status shows active processing
	statusResp, err := client.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, statusResp.Type)
}

func TestOrchestratorMultipleClients(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	// Create two clients
	client1 := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client1.Close()

	client2 := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client2.Close()

	// Both clients subscribe to the same task
	_, err := client1.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	_, err = client2.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]string{"task_id": taskID})
	require.NoError(t, err)

	// Client 1 starts the task
	startResp, err := client1.SendRequest("start-1", ws.ActionSessionLaunch, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)
	var startPayload map[string]interface{}
	require.NoError(t, startResp.ParsePayload(&startPayload))
	sessionID, _ := startPayload["session_id"].(string)
	require.NotEmpty(t, sessionID)

	// Both clients subscribe to the session
	subResp1, err := client1.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp1.Type, "Client 1 subscription should succeed")

	subResp2, err := client2.SendRequest("session-sub-1", ws.ActionSessionSubscribe, map[string]string{"session_id": sessionID})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, subResp2.Type, "Client 2 subscription should succeed")

	// Verify both clients can query orchestrator status
	status1, err := client1.SendRequest("status-1", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, status1.Type)

	status2, err := client2.SendRequest("status-2", ws.ActionOrchestratorStatus, map[string]interface{}{})
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, status2.Type)
}

func TestOrchestratorErrorHandling(t *testing.T) {
	t.Run("StartTaskMissingTaskID", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		resp, err := client.SendRequest("start-1", ws.ActionSessionLaunch, map[string]interface{}{})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)

		var errPayload ws.ErrorPayload
		require.NoError(t, resp.ParsePayload(&errPayload))
		assert.Equal(t, ws.ErrorCodeValidation, errPayload.Code)
		assert.Contains(t, errPayload.Message, "task_id")
	})

	t.Run("StartTaskNotFound", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		resp, err := client.SendRequest("start-1", ws.ActionSessionLaunch, map[string]interface{}{
			"task_id":          "non-existent-task",
			"agent_profile_id": "augment-agent",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("StopTaskNotRunning", func(t *testing.T) {
		ts := NewOrchestratorTestServer(t)
		defer ts.Close()

		taskID := ts.CreateTestTask(t, "augment-agent", 2)

		client := NewOrchestratorWSClient(t, ts.Server.URL)
		defer client.Close()

		// Try to stop a task that was never started
		resp, err := client.SendRequest("stop-1", ws.ActionOrchestratorStop, map[string]interface{}{
			"task_id": taskID,
		})
		require.NoError(t, err)

		// Should error because task is not running
		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})
}

func TestOrchestratorAgentFailure(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	defer ts.Close()

	// Configure agent manager to fail launches
	ts.AgentManager.SetShouldFail(true)

	taskID := ts.CreateTestTask(t, "augment-agent", 2)

	client := NewOrchestratorWSClient(t, ts.Server.URL)
	defer client.Close()

	resp, err := client.SendRequest("start-1", ws.ActionSessionLaunch, map[string]interface{}{
		"task_id":          taskID,
		"agent_profile_id": "augment-agent",
	})
	require.NoError(t, err)

	// Should get error response
	assert.Equal(t, ws.MessageTypeError, resp.Type)

	var errPayload ws.ErrorPayload
	require.NoError(t, resp.ParsePayload(&errPayload))
	assert.Contains(t, errPayload.Message, "Failed to launch session")
}
