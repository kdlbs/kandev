// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestWorkflowCRUD(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)

	// Create workflow
	t.Run("CreateWorkflow", func(t *testing.T) {
		resp, err := client.SendRequest("workflow-create-1", ws.ActionWorkflowCreate, map[string]interface{}{
			"workspace_id":         workspaceID,
			"name":                 "Test Workflow",
			"description":          "A test workflow for integration testing",
			"workflow_template_id": "simple",
		})
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.NotEmpty(t, payload["id"])
		assert.Equal(t, "Test Workflow", payload["name"])
	})

	// List workflows
	t.Run("ListWorkflows", func(t *testing.T) {
		resp, err := client.SendRequest("workflow-list-1", ws.ActionWorkflowList, map[string]interface{}{
			"workspace_id": workspaceID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		workflows, ok := payload["workflows"].([]interface{})
		require.True(t, ok)
		assert.Len(t, workflows, 1)
	})
}

func TestWorkflowStepList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	workspaceID := createWorkspace(t, client)

	// Create a workflow (workflow steps are created automatically with default template)
	workflowResp, err := client.SendRequest("workflow-1", ws.ActionWorkflowCreate, map[string]interface{}{
		"workspace_id":         workspaceID,
		"name":                 "Workflow Step Test Workflow",
		"workflow_template_id": "simple",
	})
	require.NoError(t, err)

	var workflowPayload map[string]interface{}
	err = workflowResp.ParsePayload(&workflowPayload)
	require.NoError(t, err)
	workflowID := workflowPayload["id"].(string)

	// List workflow steps
	t.Run("ListWorkflowSteps", func(t *testing.T) {
		resp, err := client.SendRequest("step-list-1", ws.ActionWorkflowStepList, map[string]interface{}{
			"workflow_id": workflowID,
		})
		require.NoError(t, err)

		var payload map[string]interface{}
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		steps, ok := payload["steps"].([]interface{})
		require.True(t, ok)
		// Default workflow template creates 4 steps: Todo, In Progress, Review, Done
		assert.GreaterOrEqual(t, len(steps), 1)

		// Verify first step has expected fields
		firstStep := steps[0].(map[string]interface{})
		assert.NotEmpty(t, firstStep["id"])
		assert.NotEmpty(t, firstStep["name"])
	})
}
