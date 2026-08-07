package mcp

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolArgumentValidationRejectsUnknownTopLevelArgument(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "list_workspaces_kandev", map[string]interface{}{
		"unexpected": "discarded today",
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	assert.NotEqual(t, ws.ActionMCPListWorkspaces, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Equal(t,
		"invalid arguments for list_workspaces_kandev: validation failed at $ (keyword: additionalProperties)",
		content.Text)
}

func TestToolArgumentValidationNamesMissingWalkthroughStepProperty(t *testing.T) {
	const rejectedValue = "private sibling value"
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "show_walkthrough_kandev", map[string]interface{}{
		"steps": []interface{}{
			map[string]interface{}{
				"file":  "example.go",
				"line":  1,
				"title": rejectedValue,
			},
		},
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, "/steps/0")
	assert.Contains(t, content.Text, "keyword: required")
	assert.Contains(t, content.Text, `missing: "text"`)
	assert.NotContains(t, content.Text, rejectedValue)
}

func TestToolArgumentValidationNamesEveryMissingWalkthroughStepProperty(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "show_walkthrough_kandev", map[string]interface{}{
		"steps": []interface{}{
			map[string]interface{}{"line": 1},
		},
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `missing: "file", "text"`)
}

func TestToolArgumentValidationSortsMissingProperties(t *testing.T) {
	const toolName = "unordered_required_properties_tool"
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			toolName,
			"Validates deterministic missing-property diagnostics.",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"alpha": {"type": "string"},
					"zeta": {"type": "string"}
				},
				"required": ["zeta", "alpha"]
			}`),
		),
		s.wrapHandler(toolName, s.listWorkspacesHandler()),
	)
	s.rebuildToolArgumentValidators()

	result := callTool(t, s, toolName, map[string]interface{}{})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, `missing: "alpha", "zeta"`)
}

func TestToolArgumentValidation(t *testing.T) {
	t.Run("accepts an empty object for a parameterless tool", func(t *testing.T) {
		backend := &testBackend{
			response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0},
		}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "list_workspaces_kandev", map[string]interface{}{})

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPListWorkspaces, backend.lastAction)
	})

	t.Run("accepts omitted arguments for a parameterless tool", func(t *testing.T) {
		backend := &testBackend{
			response: map[string]interface{}{"workspaces": []interface{}{}, "total": 0},
		}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "list_workspaces_kandev", nil)

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPListWorkspaces, backend.lastAction)
	})

	t.Run("rejects a missing required argument", func(t *testing.T) {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "list_workflows_kandev", map[string]interface{}{})

		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	})

	t.Run("rejects the wrong declared type", func(t *testing.T) {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "create_task_kandev", map[string]interface{}{
			"title":       "Typed arguments",
			"start_agent": "false",
		})

		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	})

	t.Run("rejects a declared enum violation", func(t *testing.T) {
		backend := &testBackend{}
		s := newTaskModeServer(t, backend, "task-current")

		result := callTool(t, s, "message_task_kandev", map[string]interface{}{
			"task_id":       "task-target",
			"prompt":        "Status?",
			"delivery_mode": "later",
		})

		assert.True(t, result.IsError)
		assert.Empty(t, backend.lastAction)
	})

	t.Run("keeps an intentionally open nested map", func(t *testing.T) {
		backend := &testBackend{response: map[string]interface{}{"id": "profile-1"}}
		s := newTestServer(t, backend)

		result := callTool(t, s, "create_executor_profile_kandev", map[string]interface{}{
			"executor_id": "exec-local",
			"name":        "Custom",
			"config": map[string]interface{}{
				"provider_specific_key": "allowed",
			},
		})

		assert.False(t, result.IsError)
		assert.Equal(t, ws.ActionMCPCreateExecutorProfile, backend.lastAction)
	})
}

func TestToolArgumentValidationDoesNotExposeRejectedValues(t *testing.T) {
	const (
		secret   = "api-key-super-secret-123"
		toolName = "secret_pattern_tool"
	)
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")
	s.mcpServer.AddTool(
		mcp.NewToolWithRawSchema(
			toolName,
			"Validates a secret without exposing it.",
			json.RawMessage(`{
				"type": "object",
				"properties": {
					"token": {"type": "string", "pattern": "^safe$"}
				},
				"required": ["token"]
			}`),
		),
		s.wrapHandler(toolName, s.listWorkspacesHandler()),
	)
	s.rebuildToolArgumentValidators()

	result := callTool(t, s, toolName, map[string]interface{}{
		"token": secret,
	})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	require.NotEmpty(t, result.Content)
	content, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, content.Text, "/token")
	assert.Contains(t, content.Text, "pattern")
	assert.NotContains(t, content.Text, secret)
}

func TestAllRegisteredToolSchemasCompile(t *testing.T) {
	for _, mode := range []string{ModeTask, ModeConfig, ModeExternal, ModeOffice} {
		t.Run(mode, func(t *testing.T) {
			log := newTestLogger(t)
			s := New(&testBackend{}, "session-1", "task-1", 10005, log, "", true, mode)
			tools := s.mcpServer.ListTools()

			s.validatorMu.RLock()
			defer s.validatorMu.RUnlock()
			require.Len(t, s.toolValidators, len(tools))
			for name := range tools {
				validator, ok := s.toolValidators[name]
				require.True(t, ok, "missing validator for %s", name)
				assert.NoError(t, validator.err, "schema for %s must compile", name)
				assert.NotNil(t, validator.schema, "schema for %s must compile", name)
			}
		})
	}
}

func TestSetModeRebuildsToolValidators(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"id": "workflow-1"}}
	s := newTaskModeServer(t, backend, "task-current")

	s.SetMode(ModeConfig)
	result := callTool(t, s, "create_workflow_kandev", map[string]interface{}{
		"workspace_id": "workspace-1",
		"name":         "Validated workflow",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateWorkflow, backend.lastAction)
}

func TestToolValidationCoordinatesWithModeChange(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	req := mcp.CallToolRequest{}
	req.Method = "tools/call"
	req.Params.Name = "list_workspaces_kandev"
	req.Params.Arguments = map[string]interface{}{}

	s.validatorMu.Lock()
	validatorLocked := true
	t.Cleanup(func() {
		if validatorLocked {
			s.validatorMu.Unlock()
		}
	})

	validationDone := make(chan error, 1)
	go func() {
		_, err := s.validateToolArguments("list_workspaces_kandev", req)
		validationDone <- err
	}()

	waitForCondition(t, "validation to acquire the mode read lock", func() bool {
		if s.mu.TryLock() {
			s.mu.Unlock()
			return false
		}
		return true
	})

	modeDone := make(chan struct{})
	go func() {
		s.SetMode(ModeConfig)
		close(modeDone)
	}()
	waitForCondition(t, "mode change to wait for the write lock", func() bool {
		if s.mu.TryRLock() {
			s.mu.RUnlock()
			return false
		}
		return true
	})

	select {
	case <-modeDone:
		t.Fatal("mode change completed while validation held the read lock")
	default:
	}
	s.validatorMu.Unlock()
	validatorLocked = false

	select {
	case err := <-validationDone:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for validation to complete")
	}
	select {
	case <-modeDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for mode change to complete")
	}
	assert.Equal(t, ModeConfig, s.mode)
}

func TestNormalizeCreateTaskArguments(t *testing.T) {
	t.Run("keeps canonical prompt unchanged", func(t *testing.T) {
		arguments := map[string]any{
			"title":  "Review lane",
			"prompt": "Detailed review instructions",
		}

		normalized, err := normalizeToolArguments("create_task_kandev", arguments)

		require.NoError(t, err)
		assert.Equal(t, arguments, normalized)
		assert.Contains(t, arguments, "prompt")
		assert.NotContains(t, arguments, "description")
	})

	t.Run("copies the legacy description alias without mutating the request", func(t *testing.T) {
		arguments := map[string]any{
			"title":       "Review lane",
			"description": "Detailed review instructions",
		}

		normalized, err := normalizeToolArguments("create_task_kandev", arguments)

		require.NoError(t, err)
		normalizedArguments, ok := normalized.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Detailed review instructions", normalizedArguments["prompt"])
		assert.NotContains(t, normalizedArguments, "description")
		assert.Equal(t, "Detailed review instructions", arguments["description"])
		assert.NotContains(t, arguments, "prompt")
	})
}

func waitForCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		runtime.Gosched()
	}
}
