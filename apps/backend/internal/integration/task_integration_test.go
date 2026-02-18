// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestTaskCRUD(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create workflow (workflow steps are created automatically)
	workflowResp, err := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Task Test Workflow",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var workflowPayload map[string]interface{}
	err = workflowResp.ParsePayload(&workflowPayload)
	require.NoError(t, err)
	workflowID := workflowPayload["id"].(string)

	// Get first workflow step
	stepResp, err := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	require.NoError(t, err)

	var stepListPayload map[string]interface{}
	err = stepResp.ParsePayload(&stepListPayload)
	require.NoError(t, err)
	steps := stepListPayload["steps"].([]interface{})
	require.NotEmpty(t, steps)
	workflowStepID := steps[0].(map[string]interface{})["id"].(string)

	var taskID string

	// Create task
	t.Run("CreateTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-create-1", ws.ActionTaskCreate, map[string]interface{}{
			"workspace_id":     workspaceID,
			"workflow_id":      workflowID,
			"workflow_step_id": workflowStepID,
			"title":            "Test Task",
			"description":      "A test task for integration testing",
			"priority":         3, // HIGH priority (1=LOW, 2=MEDIUM, 3=HIGH)
			"repository_id":    repositoryID,
			"base_branch":      "main",
		})
		require.NoError(t, err)

		// Debug: print error if there is one
		if resp.Type == ws.MessageTypeError {
			var errPayload ws.ErrorPayload
			if err := resp.ParsePayload(&errPayload); err != nil {
				t.Fatalf("failed to parse error payload: %v", err)
			}
			t.Logf("Error response: %+v", errPayload)
		}

		require.Equal(t, ws.MessageTypeResponse, resp.Type, "Expected response but got %s", resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		taskID = payload["id"].(string)
		assert.NotEmpty(t, taskID)
		assert.Equal(t, "Test Task", payload["title"])
		assert.Equal(t, "CREATED", payload["state"])
	})

	// Get task
	t.Run("GetTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-get-1", ws.ActionTaskGet, map[string]interface{}{
			"id": taskID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, taskID, payload["id"])
		assert.Equal(t, "Test Task", payload["title"])
	})

	// Update task
	t.Run("UpdateTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-update-1", ws.ActionTaskUpdate, map[string]interface{}{
			"id":    taskID,
			"title": "Updated Task Title",
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, "Updated Task Title", payload["title"])
	})

	// List tasks
	t.Run("ListTasks", func(t *testing.T) {
		resp, err := client.SendRequest("task-list-1", ws.ActionTaskList, map[string]interface{}{
			"workflow_id": workflowID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		tasks, ok := payload["tasks"].([]interface{})
		require.True(t, ok)
		assert.Len(t, tasks, 1)
	})

	// Delete task
	t.Run("DeleteTask", func(t *testing.T) {
		resp, err := client.SendRequest("task-delete-1", ws.ActionTaskDelete, map[string]interface{}{
			"id": taskID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, true, payload["success"])

		// Verify task is deleted
		listResp, err := client.SendRequest("task-list-2", ws.ActionTaskList, map[string]interface{}{
			"workflow_id": workflowID,
		})
		require.NoError(t, err)

		var listPayload map[string]interface{}
		err = listResp.ParsePayload(&listPayload)
		require.NoError(t, err)

		tasks, ok := listPayload["tasks"].([]interface{})
		require.True(t, ok)
		assert.Len(t, tasks, 0)
	})
}

func TestTaskStateTransitions(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create workflow and task (workflow steps are created automatically)
	workflowResp, _ := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "State Test Workflow",
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
		"title":            "State Test Task",
		"description":      "Test state transitions",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	var taskPayload map[string]interface{}
	if err := taskResp.ParsePayload(&taskPayload); err != nil {
		t.Fatalf("failed to parse task payload: %v", err)
	}
	taskID := taskPayload["id"].(string)

	// Test state transitions
	stateTests := []struct {
		name     string
		newState string
	}{
		{"ToInProgress", "IN_PROGRESS"},
		{"ToCompleted", "COMPLETED"},
		{"BackToTodo", "TODO"},
		{"ToBlocked", "BLOCKED"},
	}

	for i, tc := range stateTests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := client.SendRequest(
				"state-"+string(rune('0'+i)),
				ws.ActionTaskState,
				map[string]interface{}{
					"id":    taskID,
					"state": tc.newState,
				},
			)
			require.NoError(t, err)
			assert.Equal(t, ws.MessageTypeResponse, resp.Type)

			var payload map[string]interface{}
			err = resp.ParsePayload(&payload)
			require.NoError(t, err)

			assert.Equal(t, tc.newState, payload["state"])
		})
	}
}

func TestTaskMove(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)
	repositoryID := createRepository(t, client, workspaceID)

	// Create workflow (workflow steps are created automatically)
	workflowResp, _ := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Move Test Workflow",
		"workflow_template_id": "simple",
	})
	var workflowPayload map[string]interface{}
	if err := workflowResp.ParsePayload(&workflowPayload); err != nil {
		t.Fatalf("failed to parse workflow payload: %v", err)
	}
	workflowID := workflowPayload["id"].(string)

	// Get workflow steps (at least 2 should exist from default template)
	stepResp, _ := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
		"workflow_id": workflowID,
	})
	var stepListPayload map[string]interface{}
	if err := stepResp.ParsePayload(&stepListPayload); err != nil {
		t.Fatalf("failed to parse step list payload: %v", err)
	}
	steps := stepListPayload["steps"].([]interface{})
	require.GreaterOrEqual(t, len(steps), 2, "Expected at least 2 workflow steps")
	step1ID := steps[0].(map[string]interface{})["id"].(string)
	step2ID := steps[1].(map[string]interface{})["id"].(string)

	// Create task in first step
	taskResp, _ := client.SendRequest("task-1", ws.ActionTaskCreate, map[string]interface{}{
		"workspace_id":     workspaceID,
		"workflow_id":      workflowID,
		"workflow_step_id": step1ID,
		"title":            "Movable Task",
		"repository_id":    repositoryID,
		"base_branch":      "main",
	})
	var taskPayload map[string]interface{}
	if err := taskResp.ParsePayload(&taskPayload); err != nil {
		t.Fatalf("failed to parse task payload: %v", err)
	}
	taskID := taskPayload["id"].(string)

	// Move task to second step
	t.Run("MoveToStep2", func(t *testing.T) {
		resp, err := client.SendRequest("move-1", ws.ActionTaskMove, map[string]interface{}{
			"id":               taskID,
			"workflow_id":      workflowID,
			"workflow_step_id": step2ID,
			"position":         0,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		// Response is {"task": {...}, "workflow_step": {...}}
		taskData := payload["task"].(map[string]interface{})
		assert.Equal(t, step2ID, taskData["workflow_step_id"])
	})

	// Verify task is now in step 2
	t.Run("VerifyMove", func(t *testing.T) {
		resp, err := client.SendRequest("get-1", ws.ActionTaskGet, map[string]interface{}{
			"id": taskID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, step2ID, payload["workflow_step_id"])
	})
}
