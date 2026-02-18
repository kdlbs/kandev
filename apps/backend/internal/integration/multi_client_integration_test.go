// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestMultipleClients(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client1 := NewWSClient(t, ts.Server.URL)
	defer client1.Close()

	client2 := NewWSClient(t, ts.Server.URL)
	defer client2.Close()

	workspaceID := createWorkspace(t, client1)
	repositoryID := createRepository(t, client1, workspaceID)

	// Client 1 creates a workflow
	workflowResp, err := client1.SendRequest("c1-workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Shared Workflow",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var workflowPayload map[string]interface{}
	err = workflowResp.ParsePayload(&workflowPayload)
	require.NoError(t, err)
	workflowID := workflowPayload["id"].(string)

	// Client 2 can see the workflow (filter by workspace to avoid default workflow)
	listResp, err := client2.SendRequest("c2-list-1", ws.ActionWorkflowList, map[string]interface{}{
		"workspace_id": workspaceID,
	})
	require.NoError(t, err)

	var listPayload map[string]interface{}
	err = listResp.ParsePayload(&listPayload)
	require.NoError(t, err)

	workflows, ok := listPayload["workflows"].([]interface{})
	require.True(t, ok)
	assert.Len(t, workflows, 1)

	// Client 2 lists workflow steps (created automatically with workflow)
	stepResp, err := client2.SendRequest("c2-step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)

	var stepPayload map[string]interface{}
	err = stepResp.ParsePayload(&stepPayload)
	require.NoError(t, err)
	steps := stepPayload["steps"].([]interface{})
	require.NotEmpty(t, steps)
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// Client 1 can also see the workflow steps
	stepListResp, err := client1.SendRequest("c1-step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)

	var stepListPayload map[string]interface{}
	err = stepListResp.ParsePayload(&stepListPayload)
	require.NoError(t, err)

	stepsList, ok := stepListPayload["steps"].([]interface{})
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(stepsList), 1)

	// Client 1 creates a task in the workflow step
	taskResp, err := client1.SendRequest("c1-task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"workflow_step_id": workflowStepID,
		"title":            "Task by Client 1",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	require.NoError(t, err)

	var taskPayload map[string]interface{}
	err = taskResp.ParsePayload(&taskPayload)
	require.NoError(t, err)

	// Client 2 can see the task
	taskListResp, err := client2.SendRequest("c2-task-list-1", ws.ActionTaskList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)

	var taskListPayload map[string]interface{}
	err = taskListResp.ParsePayload(&taskListPayload)
	require.NoError(t, err)

	tasks, ok := taskListPayload["tasks"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tasks, 1)

	_ = taskPayload // suppress unused variable warning
}

func TestConcurrentRequests(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create workflow (workflow steps are created automatically)
	workflowResp, _ := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Concurrent Test Workflow",
		"workflow_template_id": "simple",
	})
	var workflowPayload map[string]interface{}
	if err := workflowResp.ParsePayload(&workflowPayload); err != nil {
		t.Fatalf("failed to parse workflow payload: %v", err)
	}
	workflowID := workflowPayload["id"].(string)

	// Get first workflow step
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	// Create multiple tasks concurrently
	numTasks := 10
	var wg sync.WaitGroup
	results := make(chan error, numTasks)

	for i := 0; i < numTasks; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Create separate client for concurrency
			c := NewWSClient(t, ts.Server.URL)
			defer c.Close()

			_, err := c.SendRequest(
				"task-"+string(rune('0'+idx)),
				ws.ActionTaskCreate,
				map[string]interface{}{
					"workspace_id":     workspaceID,
					"workflow_id":      workflowID,
					"workflow_step_id": workflowStepID,
					"title":            "Concurrent Task " + string(rune('0'+idx)),
					"repository_id":    repositoryID,
					"base_branch":      "main",
				},
			)
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	// Verify all tasks were created
	for err := range results {
		assert.NoError(t, err)
	}

	// Verify task count
	listResp, err := client.SendRequest("list-1", ws.ActionTaskList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)

	var listPayload map[string]interface{}
	err = listResp.ParsePayload(&listPayload)
	require.NoError(t, err)

	tasks, ok := listPayload["tasks"].([]interface{})
	require.True(t, ok)
	assert.Len(t, tasks, numTasks)
}

func TestTaskSubscription(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create a task first (workflow steps are created automatically with workflow)
	workflowResp, _ := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Sub Test Workflow",
		"workflow_template_id": "simple",
	})
	var workflowPayload map[string]interface{}
	if err := workflowResp.ParsePayload(&workflowPayload); err != nil {
		t.Fatalf("failed to parse workflow payload: %v", err)
	}
	workflowID := workflowPayload["id"].(string)

	// Get first workflow step
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	taskResp, _ := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"workflow_step_id": workflowStepID,
		"title":            "Subscribable Task",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	var taskPayload map[string]interface{}
	if err := taskResp.ParsePayload(&taskPayload); err != nil {
		t.Fatalf("failed to parse task payload: %v", err)
	}
	taskID := taskPayload["id"].(string)

	// Subscribe to the task
	t.Run("Subscribe", func(t *testing.T) {
		resp, err := client.SendRequest("sub-1", ws.ActionTaskSubscribe, map[string]interface{}{
			"task_id": taskID,
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, true, payload["success"])
		assert.Equal(t, taskID, payload["task_id"])
	})

	// Unsubscribe from the task
	t.Run("Unsubscribe", func(t *testing.T) {
		resp, err := client.SendRequest("unsub-1", ws.ActionTaskUnsubscribe, map[string]interface{}{
			"task_id": taskID,
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, true, payload["success"])
	})
}
