// Package integration provides end-to-end integration tests for the Kandev backend.
package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestErrorHandling(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	client := NewWSClient(t, ts.Server.URL)
	defer client.Close()

	t.Run("UnknownAction", func(t *testing.T) {
		resp, err := client.SendRequest("err-1", "unknown.action", map[string]interface{}{})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)

		var payload ws.ErrorPayload
		err = resp.ParsePayload(&payload)
		require.NoError(t, err)

		assert.Equal(t, ws.ErrorCodeUnknownAction, payload.Code)
	})

	t.Run("GetNonExistentWorkflow", func(t *testing.T) {
		resp, err := client.SendRequest("err-2", ws.ActionWorkflowGet, map[string]interface{}{
			"id": "non-existent-workflow-id",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("GetNonExistentTask", func(t *testing.T) {
		resp, err := client.SendRequest("err-3", ws.ActionTaskGet, map[string]interface{}{
			"id": "non-existent-task-id",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("CreateTaskWithoutWorkflow", func(t *testing.T) {
		resp, err := client.SendRequest("err-4", ws.ActionTaskCreate, map[string]interface{}{
			"title": "Orphan Task",
		})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})

	t.Run("ListWorkflowStepsWithoutWorkflow", func(t *testing.T) {
		resp, err := client.SendRequest("err-5", ws.ActionWorkflowStepList, map[string]interface{}{})
		require.NoError(t, err)

		assert.Equal(t, ws.MessageTypeError, resp.Type)
	})
}
