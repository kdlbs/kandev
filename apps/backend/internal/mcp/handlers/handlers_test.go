package handlers

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCreateTask_MissingTitle(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateTask_SubtaskMissingDescription(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":     "Fix bug",
		"parent_id": "task-parent",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateTask_InvalidPayload(t *testing.T) {
	h := &Handlers{}
	msg := &ws.Message{
		ID:      "test-id",
		Type:    ws.MessageTypeRequest,
		Action:  ws.ActionMCPCreateTask,
		Payload: json.RawMessage(`{invalid`),
	}

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeBadRequest)
}

func TestHandleCreateTask_TopLevel_MissingWorkspaceID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":       "New task",
		"workflow_id": "wf-1",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

func TestHandleCreateTask_TopLevel_MissingWorkflowID(t *testing.T) {
	h := &Handlers{}
	msg := makeWSMessage(t, ws.ActionMCPCreateTask, map[string]interface{}{
		"title":        "New task",
		"workspace_id": "ws-1",
	})

	resp, err := h.handleCreateTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeValidation)
}

// mockSessionLauncher captures LaunchSession calls for testing autoStartTask.
type mockSessionLauncher struct {
	mu  sync.Mutex
	req *orchestrator.LaunchSessionRequest
}

func (m *mockSessionLauncher) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.req = req
	return &orchestrator.LaunchSessionResponse{
		Success:   true,
		TaskID:    req.TaskID,
		SessionID: "session-1",
	}, nil
}

func (m *mockSessionLauncher) getRequest() *orchestrator.LaunchSessionRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.req
}

func TestAutoStartTask_DefaultsToWorktreeExecutor(t *testing.T) {
	launcher := &mockSessionLauncher{}
	log := testLogger(t)
	h := &Handlers{
		sessionLauncher: launcher,
		logger:          log.WithFields(),
	}

	task := &models.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
	}

	// Call with agent profile but no executor info
	h.autoStartTask(task, "agent-profile-1", "")

	// autoStartTask runs in a goroutine; wait for it
	require.Eventually(t, func() bool {
		return launcher.getRequest() != nil
	}, 2*time.Second, 50*time.Millisecond)

	req := launcher.getRequest()
	assert.Equal(t, "exec-worktree", req.ExecutorID, "should default to exec-worktree")
	assert.Equal(t, "", req.ExecutorProfileID)
	assert.Equal(t, "agent-profile-1", req.AgentProfileID)
}

func TestAutoStartTask_ExplicitExecutorProfilePreserved(t *testing.T) {
	launcher := &mockSessionLauncher{}
	log := testLogger(t)
	h := &Handlers{
		sessionLauncher: launcher,
		logger:          log.WithFields(),
	}

	task := &models.Task{
		ID:          "task-1",
		WorkspaceID: "ws-1",
	}

	// Call with explicit executor profile
	h.autoStartTask(task, "agent-profile-1", "exec-profile-docker")

	require.Eventually(t, func() bool {
		return launcher.getRequest() != nil
	}, 2*time.Second, 50*time.Millisecond)

	req := launcher.getRequest()
	assert.Equal(t, "exec-profile-docker", req.ExecutorProfileID, "explicit executor profile should be preserved")
	assert.Equal(t, "", req.ExecutorID, "executorID should be empty when profile is set")
}
