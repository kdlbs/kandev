package mcp

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestFreshCIRunToolInjectsCallerAndKeepsAuthorityFieldsClosed(t *testing.T) {
	backend := &testBackend{response: map[string]any{
		"request_id": "request-1", "task_id": "target-1", "run_id": float64(100),
	}}
	server := newTaskModeServer(t, backend, "coordinator-1")
	result := callTool(t, server, "request_fresh_ci_run_kandev", map[string]any{
		"task_id": "target-1", "repository_id": "repository-1", "pr_number": 42,
		"expected_head_sha":         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"expected_workflow_step_id": "ci-fixup", "source_run_id": 100,
		"expected_source_attempt": 1, "evidence_kind": "pr_head",
		"idempotency_key": "consumer-42-attempt-1",
	})
	require.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPRequestFreshCIRun, backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "coordinator-1", payload["actor_task_id"])
	assert.Equal(t, "test-session", payload["actor_session_id"])
	properties := toolInputProperties(t, server, "request_fresh_ci_run_kandev")
	for _, forbidden := range []string{
		"actor_task_id", "actor_session_id", "owner", "repo", "ref", "workflow_id",
		"workflow_path", "inputs", "token", "authorization",
	} {
		assert.NotContains(t, properties, forbidden)
	}
}

func TestRequestFreshCIRunToolRejectsExtraAuthorityFields(t *testing.T) {
	backend := &testBackend{}
	server := newTaskModeServer(t, backend, "coordinator-1")
	result := callTool(t, server, "request_fresh_ci_run_kandev", map[string]any{
		"task_id": "target-1", "repository_id": "repository-1", "pr_number": 42,
		"expected_head_sha":         "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"expected_workflow_step_id": "ci-fixup", "source_run_id": 100,
		"expected_source_attempt": 1, "evidence_kind": "pr_head",
		"idempotency_key": "consumer-42-attempt-1", "ref": "main",
	})
	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
}

func TestRequestFreshCIRunToolOnlyRegisteredForGitHubTaskContext(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	noProvider := NewWithProfile(backend, "session", "task", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
	_ = noProvider.Close(context.Background())
	assert.NotContains(t, getRegisteredToolNames(noProvider), "request_fresh_ci_run_kandev")

	gitHubTask := NewWithProfile(backend, "session", "task", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, []string{"github"}))
	_ = gitHubTask.Close(context.Background())
	assert.Contains(t, getRegisteredToolNames(gitHubTask), "request_fresh_ci_run_kandev")
}
