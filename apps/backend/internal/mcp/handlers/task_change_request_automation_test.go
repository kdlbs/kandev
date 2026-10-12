package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type taskChangeRequestAutomationRecorder struct {
	called  int
	taskID  string
	request TaskChangeRequestAutomationRequest
	result  TaskChangeRequestAutomationResult
}

func (r *taskChangeRequestAutomationRecorder) UpdateTaskChangeRequestAutomation(
	_ context.Context,
	taskID string,
	request TaskChangeRequestAutomationRequest,
) (TaskChangeRequestAutomationResult, error) {
	r.called++
	r.taskID = taskID
	r.request = request
	if r.result.TaskID == "" {
		r.result.TaskID = taskID
	}
	return r.result, nil
}

func automationHandlerContext() context.Context {
	return mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID: "workspace-1", CallerTaskID: "task-current", CallerSessionID: "session-1",
		Surface: mcpprofile.SurfaceKanbanTask,
	})
}

func TestParseTaskChangeRequestAutomationPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]interface{}
		valid   bool
	}{
		{
			name: "association accepts false",
			payload: map[string]interface{}{
				"target": map[string]interface{}{"scope": "association", "provider": "github", "repository_id": "repo-gh", "number": 8},
				"patch":  map[string]interface{}{"auto_fix_enabled": false},
			}, valid: true,
		},
		{
			name: "task accepts empty prompt",
			payload: map[string]interface{}{
				"target": map[string]interface{}{"scope": "task", "providers": []string{"gitlab"}},
				"patch":  map[string]interface{}{"auto_fix_prompt_override": ""},
			}, valid: true,
		},
		{
			name: "association rejects prompt",
			payload: map[string]interface{}{
				"target": map[string]interface{}{"scope": "association", "provider": "github", "repository_id": "repo-gh", "number": 8},
				"patch":  map[string]interface{}{"auto_fix_prompt_override": "custom"},
			}, valid: false,
		},
		{
			name: "task rejects duplicate providers",
			payload: map[string]interface{}{
				"target": map[string]interface{}{"scope": "task", "providers": []string{"github", "github"}},
				"patch":  map[string]interface{}{"auto_fix_enabled": true},
			}, valid: false,
		},
		{
			name: "rejects injected task identity",
			payload: map[string]interface{}{
				"task_id": "other-task",
				"target":  map[string]interface{}{"scope": "task", "providers": []string{"github"}},
				"patch":   map[string]interface{}{"auto_fix_enabled": true},
			}, valid: false,
		},
		{
			name: "rejects null false distinction",
			payload: map[string]interface{}{
				"target": map[string]interface{}{"scope": "association", "provider": "github", "repository_id": "repo-gh", "number": 8},
				"patch":  map[string]interface{}{"auto_fix_enabled": nil},
			}, valid: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := makeWSMessage(t, ws.ActionMCPUpdateTaskChangeRequestAutomation, test.payload)
			_, response, err := parseTaskChangeRequestAutomationPayload(msg)
			if test.valid {
				require.NoError(t, err)
				assert.Nil(t, response)
				return
			}
			assert.NoError(t, err)
			require.NotNil(t, response)
			assert.Equal(t, ws.MessageTypeError, response.Type)
		})
	}
}

func TestUpdateTaskChangeRequestAutomationUsesBoundPrincipal(t *testing.T) {
	svc, repo := newTestTaskService(t)
	now := timeNowForAutomationHandlerTest()
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{
		ID: "workspace-1", Name: "Workspace", CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(context.Background(), &models.Task{
		ID: "task-current", WorkspaceID: "workspace-1", Title: "Current", State: v1.TaskStateTODO,
		CreatedAt: now, UpdatedAt: now,
	}))
	recorder := &taskChangeRequestAutomationRecorder{}
	h := NewHandlers(svc, nil, nil, nil, nil, repo, repo, nil, nil, nil, nil, nil, testLogger(t))
	h.SetTaskChangeRequestAutomationService(recorder)

	msg := makeWSMessage(t, ws.ActionMCPUpdateTaskChangeRequestAutomation, map[string]interface{}{
		"target": map[string]interface{}{"scope": "association", "provider": "github", "repository_id": "repo-gh", "number": 8},
		"patch":  map[string]interface{}{"auto_fix_enabled": false},
	})
	response, err := h.handleUpdateTaskChangeRequestAutomation(automationHandlerContext(), msg)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	assert.Equal(t, 1, recorder.called)
	assert.Equal(t, "task-current", recorder.taskID)
	assert.Equal(t, false, *recorder.request.Patch.AutoFixEnabled)
}

func TestUpdateTaskChangeRequestAutomationRejectsInjectedIdentity(t *testing.T) {
	msg, err := ws.NewRequest("id", ws.ActionMCPUpdateTaskChangeRequestAutomation, map[string]interface{}{
		"task_id": "other-task",
		"target":  map[string]interface{}{"scope": "task", "providers": []string{"github"}},
		"patch":   map[string]interface{}{"auto_fix_enabled": true},
	})
	require.NoError(t, err)
	_, response, parseErr := parseTaskChangeRequestAutomationPayload(msg)
	require.NoError(t, parseErr)
	require.NotNil(t, response)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	assert.Equal(t, ws.ErrorCodeValidation, payload.Code)
}

// Kept local so this file does not depend on wall-clock behavior in the
// handler authorization tests.
func timeNowForAutomationHandlerTest() time.Time {
	return time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
}
