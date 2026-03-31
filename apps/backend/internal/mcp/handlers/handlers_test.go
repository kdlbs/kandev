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
	mu     sync.Mutex
	req    *orchestrator.LaunchSessionRequest
	called chan struct{}
}

func newMockSessionLauncher() *mockSessionLauncher {
	return &mockSessionLauncher{called: make(chan struct{})}
}

func (m *mockSessionLauncher) LaunchSession(_ context.Context, req *orchestrator.LaunchSessionRequest) (*orchestrator.LaunchSessionResponse, error) {
	m.mu.Lock()
	m.req = req
	m.mu.Unlock()
	close(m.called)
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
	launcher := newMockSessionLauncher()
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
	h.autoStartTask(task, "agent-profile-1", "", "")

	select {
	case <-launcher.called:
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchSession was not called within timeout")
	}

	req := launcher.getRequest()
	assert.Equal(t, models.ExecutorIDWorktree, req.ExecutorID, "should default to exec-worktree")
	assert.Equal(t, "", req.ExecutorProfileID)
	assert.Equal(t, "agent-profile-1", req.AgentProfileID)
}

func TestAutoStartTask_ExplicitExecutorProfilePreserved(t *testing.T) {
	launcher := newMockSessionLauncher()
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
	h.autoStartTask(task, "agent-profile-1", "exec-profile-docker", "")

	select {
	case <-launcher.called:
	case <-time.After(2 * time.Second):
		t.Fatal("LaunchSession was not called within timeout")
	}

	req := launcher.getRequest()
	assert.Equal(t, "exec-profile-docker", req.ExecutorProfileID, "explicit executor profile should be preserved")
	assert.Equal(t, "", req.ExecutorID, "executorID should be empty when profile is set")
}

func TestResolveTaskRepositories_ExplicitRepos(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	explicit := []mcpRepositoryInput{
		{RepositoryID: "repo-1", BaseBranch: "main"},
		{LocalPath: "/tmp/myrepo"},
	}
	result, err := h.resolveTaskRepositories(context.Background(), "", "", explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 2)
	assert.Equal(t, "repo-1", result.Repos[0].RepositoryID)
	assert.Equal(t, "main", result.Repos[0].BaseBranch)
	assert.Equal(t, "/tmp/myrepo", result.Repos[1].LocalPath)
	assert.Empty(t, result.WorkspaceID, "workspace should not be set for explicit repos")
	assert.Empty(t, result.WorkflowID, "workflow should not be set for explicit repos")
}

func TestResolveTaskRepositories_NoInputs_ReturnsEmpty(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	result, err := h.resolveTaskRepositories(context.Background(), "", "", nil)
	require.NoError(t, err)
	assert.Empty(t, result.Repos)
}

func TestResolveTaskRepositories_ExplicitRepos_SkipsParentLookup(t *testing.T) {
	log := testLogger(t)
	h := &Handlers{logger: log.WithFields()}

	// Even with a parentID, explicit repos should be used without looking up the parent
	// (taskSvc is nil — if it tried to call GetTask it would panic).
	explicit := []mcpRepositoryInput{{RepositoryID: "repo-explicit"}}
	result, err := h.resolveTaskRepositories(context.Background(), "some-parent", "some-source", explicit)
	require.NoError(t, err)
	require.Len(t, result.Repos, 1)
	assert.Equal(t, "repo-explicit", result.Repos[0].RepositoryID)
}
