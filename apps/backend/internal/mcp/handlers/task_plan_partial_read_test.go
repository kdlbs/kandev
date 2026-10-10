package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PLAN-READ-001.1 AC-TASKS-PLAN-READ-001.3 AC-TASKS-PLAN-READ-001.4
func TestPlanPartialReadHandlerExactRange(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	content := "before\r\n猫😀e\u0301\r\nafter"
	created, err := h.planService.CreatePlan(context.Background(), service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: content,
	})
	require.NoError(t, err)
	out, err := h.handleGetTaskPlan(context.Background(), mcpPlanMsg(t, ws.ActionMCPGetTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","offset":8,"limit":4}`))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, out.Type)
	payload := decodeMCPPlanPayload(t, out)
	require.Equal(t, "猫😀e\u0301", payload["content"])
	require.Equal(t, true, payload["partial"])
	require.Equal(t, float64(8), payload["offset"])
	require.Equal(t, float64(4), payload["returned_characters"])
	require.Equal(t, float64(12), payload["next_offset"])
	require.Equal(t, float64(19), payload["total_characters"])
	require.Equal(t, float64(len(content)), payload["total_content_bytes"])
	require.Equal(t, created.Plan.WriteVersion, payload["version"])
}

// @covers AC-TASKS-PLAN-READ-001.2
func TestPlanPartialReadHandlerRejectsMalformedArguments(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	_, err := h.planService.CreatePlan(context.Background(), service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "do not return this document",
	})
	require.NoError(t, err)
	for _, args := range []string{
		`"offset":null`, `"offset":"0"`, `"offset":true`, `"offset":0.5`,
		`"offset":-1`, `"offset":9007199254740992`, `"limit":null`,
		`"limit":0`, `"limit":8193`, `"limit":1.5`, `"limit":"1"`,
		`"expected_version":null`, `"expected_version":""`, `"expected_version":123`,
	} {
		t.Run(args, func(t *testing.T) {
			out, err := h.handleGetTaskPlan(context.Background(), mcpPlanMsg(t, ws.ActionMCPGetTaskPlan,
				`{"task_id":"`+mcpPlanTaskID+`",`+args+`}`))
			require.NoError(t, err)
			require.Equal(t, ws.MessageTypeError, out.Type)
			var payload ws.ErrorPayload
			require.NoError(t, json.Unmarshal(out.Payload, &payload))
			require.Equal(t, ws.ErrorCodeValidation, payload.Code)
			require.NotContains(t, string(out.Payload), "do not return this document")
		})
	}
}

// @covers AC-TASKS-PLAN-READ-002.1
func TestPlanPartialReadHandlerVersionConflict(t *testing.T) {
	h := newMCPPlanTestHandlers(t)
	_, err := h.planService.CreatePlan(context.Background(), service.CreatePlanRequest{
		TaskID: mcpPlanTaskID, Content: "private content",
	})
	require.NoError(t, err)
	out, err := h.handleGetTaskPlan(context.Background(), mcpPlanMsg(t, ws.ActionMCPGetTaskPlan,
		`{"task_id":"`+mcpPlanTaskID+`","limit":10,"expected_version":"stale"}`))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, out.Type)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(out.Payload, &payload))
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, "plan_version_conflict", payload.Details["reason"])
	require.NotContains(t, string(out.Payload), "private content")
}
