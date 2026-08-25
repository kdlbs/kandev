package mcp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// planText is the single text block a plan tool returns.
func planText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.False(t, result.IsError)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func TestUpdateTaskPlan_AcksWithoutEchoingContent(t *testing.T) {
	planBody := "# Plan\n" + strings.Repeat("step\n", 500)
	backend := &testBackend{response: map[string]interface{}{
		"id":         "plan-1",
		"task_id":    "task-A",
		"title":      "Implementation plan",
		"content":    planBody,
		"created_by": "agent",
		"updated_at": "2026-08-06T10:00:00Z",
	}}
	s := newTaskModeServer(t, backend, "task-A")

	text := planText(t, callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
		"content": planBody,
	}))

	assert.NotContains(t, text, strings.Repeat("step\n", 2), "the plan body must not be echoed back to its own author")
	assert.Contains(t, text, "Plan updated successfully")
	assert.Contains(t, text, "task_id=task-A")
	assert.Contains(t, text, `title="Implementation plan"`)
	assert.Contains(t, text, fmt.Sprintf("%d bytes", len(planBody)))
	assert.Contains(t, text, "updated_at=2026-08-06T10:00:00Z")
	assert.Contains(t, text, "get_task_plan_kandev", "the ack must point at how to read the plan back")
	assert.Less(t, len(text), len(planBody)/4, "the ack must be far smaller than the plan it acknowledges")
}

func TestCreateTaskPlan_AcksWithoutEchoingContent(t *testing.T) {
	planBody := "# Plan\n" + strings.Repeat("step\n", 500)
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-A",
		"title":   "Plan",
		"content": planBody,
	}}
	s := newTaskModeServer(t, backend, "task-A")

	text := planText(t, callTool(t, s, "create_task_plan_kandev", map[string]interface{}{
		"content": planBody,
	}))

	assert.NotContains(t, text, strings.Repeat("step\n", 2))
	assert.Contains(t, text, "Plan created successfully")
	assert.Contains(t, text, fmt.Sprintf("%d bytes", len(planBody)))
}

// The size reported is what the backend stored, so an agent can tell a
// truncated or normalized write apart from a verbatim one.
func TestPlanAck_ReportsStoredSizeNotSentSize(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-A",
		"title":   "Plan",
		"content": "stored",
	}}
	s := newTaskModeServer(t, backend, "task-A")

	text := planText(t, callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
		"content": "a much longer plan body than what came back",
	}))

	assert.Contains(t, text, "6 bytes")
}

// TestUpdateTaskPlan_SurfacesTruncationWarning pins that planWriteAck relays
// a plan_write_warning field from the backend response into the tool result
// text an agent actually reads. The ws handler is the layer that computes
// this warning (internal/mcp/handlers/task_plan_guard.go); this test only
// pins that the ack layer above it doesn't drop the field on the floor.
func TestUpdateTaskPlan_SurfacesTruncationWarning(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id":               "task-A",
		"title":                 "Plan",
		"content":               "y",
		"plan_write_warning":    "WARNING: this write replaced 40000 chars with 1 (dropped 39999 chars, 100%). Plan revision 3 has the prior content.",
		"prior_revision_number": float64(3),
	}}
	s := newTaskModeServer(t, backend, "task-A")

	text := planText(t, callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
		"content": "y",
	}))

	assert.Contains(t, text, "Plan updated successfully")
	assert.Contains(t, text, "WARNING")
	assert.Contains(t, text, "40000 chars with 1")
	assert.Contains(t, text, "revision 3")
}

// TestUpdateTaskPlan_NoWarningWhenAbsent pins that the ack stays exactly the
// pre-existing shape when the backend reports no truncation.
func TestUpdateTaskPlan_NoWarningWhenAbsent(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-A",
		"title":   "Plan",
		"content": "step two",
	}}
	s := newTaskModeServer(t, backend, "task-A")

	text := planText(t, callTool(t, s, "update_task_plan_kandev", map[string]interface{}{
		"content": "step two",
	}))

	assert.NotContains(t, text, "WARNING")
}

func TestCreateTask_ResponseOmitsDescriptionEcho(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"id":          "task-new",
		"title":       "Child task",
		"state":       "CREATED",
		"workflow_id": "wf-1",
		"description": "a very long prompt the caller just sent",
	}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":  "Child task",
		"prompt": "a very long prompt the caller just sent",
	})
	require.False(t, result.IsError)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	assert.NotContains(t, text.Text, "a very long prompt the caller just sent")
	assert.NotContains(t, text.Text, `"description"`)
	assert.Contains(t, text.Text, "task-new")
	assert.Contains(t, text.Text, "Child task")
	assert.Contains(t, text.Text, "CREATED")
	assert.Contains(t, text.Text, "wf-1")
}

func TestUpdateTask_ResponseOmitsDescriptionEcho(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"id":          "task-A",
		"title":       "Renamed task",
		"state":       "RUNNING",
		"workflow_id": "wf-1",
		"description": "a very long description the caller just sent",
	}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "update_task_kandev", map[string]interface{}{
		"task_id":     "task-A",
		"title":       "Renamed task",
		"description": "a very long description the caller just sent",
	})
	require.False(t, result.IsError)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	assert.NotContains(t, text.Text, "a very long description the caller just sent")
	assert.NotContains(t, text.Text, `"description"`)
	assert.Contains(t, text.Text, "task-A")
	assert.Contains(t, text.Text, "Renamed task")
	assert.Contains(t, text.Text, "RUNNING")
	assert.Contains(t, text.Text, "wf-1")
}

// Unlike create_task, update_task is routinely called without a description —
// a state-only move still returned the task's stored prose, which the caller
// never asked for and pays thousands of tokens to carry.
func TestUpdateTask_StateOnlyMove_StillOmitsStoredDescription(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"id":          "task-A",
		"title":       "Some task",
		"state":       "DONE",
		"description": "prose the caller never sent and did not ask for",
	}}
	s := newTaskModeServer(t, backend, "task-A")

	result := callTool(t, s, "update_task_kandev", map[string]interface{}{
		"task_id": "task-A",
		"state":   "DONE",
	})
	require.False(t, result.IsError)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)

	assert.NotContains(t, text.Text, "prose the caller never sent")
	assert.Contains(t, text.Text, "DONE")
}
