package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-PLAN-READ-001.1 AC-TASKS-PLAN-READ-003.1
func TestPlanPartialReadSchemaAndForwarding(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"task_id": "task-A", "content": "猫", "partial": true, "version": "v1",
	}}
	s := newTaskModeServer(t, backend, "task-A")
	props := toolInputProperties(t, s, "get_task_plan_kandev")
	for _, field := range []string{"offset", "limit", "expected_version"} {
		require.Contains(t, props, field)
		schema := props[field].(map[string]interface{})
		require.NotContains(t, schema, "default", "defaults must not silently enable pagination")
	}
	result := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{
		"offset": 0, "limit": 1, "expected_version": "v1",
	})
	require.False(t, result.IsError)
	encoded, err := json.Marshal(backend.lastPayload)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &payload))
	require.Equal(t, float64(0), payload["offset"])
	require.Equal(t, float64(1), payload["limit"])
	require.Equal(t, "v1", payload["expected_version"])
	require.Equal(t, "猫", planContentFromRead(t, result))
	require.NotContains(t, firstText(t, result), "猫")
}

func TestPlanPartialReadRejectsInvalidArgumentsWithoutBackendCall(t *testing.T) {
	for _, args := range []map[string]interface{}{
		{"offset": nil}, {"offset": "0"}, {"offset": true}, {"offset": 0.5},
		{"offset": -1}, {"offset": 9007199254740992.0}, {"limit": nil},
		{"limit": 0}, {"limit": 8193}, {"limit": 1.5}, {"limit": "1"},
		{"expected_version": nil}, {"expected_version": ""}, {"expected_version": 123},
	} {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-A")
		result := callTool(t, s, "get_task_plan_kandev", args)
		require.True(t, result.IsError, "%v", args)
		require.Empty(t, backend.lastAction)
	}
}

func TestPlanPartialReadProfileExposure(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeTaskTitlePending, ModeOffice, ModeConfig, ModeExternal, ModeAutomation} {
		s := New(&testBackend{}, "session", "task", 10005, newTestLogger(t), "", false, mode)
		tools := getRegisteredToolNames(s)
		if mode == ModeTask || mode == ModeTaskTitlePending || mode == ModeOffice {
			require.Contains(t, tools, "get_task_plan_kandev")
		} else {
			require.NotContains(t, tools, "get_task_plan_kandev")
		}
	}
}
