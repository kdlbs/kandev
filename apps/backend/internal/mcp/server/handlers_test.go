package mcp

import (
	"encoding/json"
	"errors"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTaskModeServer(t *testing.T, backend BackendClient, taskID string) *Server {
	t.Helper()
	log := newTestLogger(t)
	return New(backend, "test-session", taskID, 10005, log, "", false, ModeTask)
}

func TestCreateTask_ToolSchema_HasParentID(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	toolsMap := s.mcpServer.ListTools()
	tool, ok := toolsMap["create_task_kandev"]
	require.True(t, ok, "create_task tool not registered")

	schema, err := json.Marshal(tool.Tool.InputSchema)
	require.NoError(t, err)

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	props, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "schema should have properties")
	assert.Contains(t, props, "parent_id", "create_task schema must expose parent_id")
	assert.Contains(t, props, "title")
	assert.Contains(t, props, "workspace_id")
	assert.Contains(t, props, "workflow_id")
	assert.Contains(t, props, "workflow_step_id")
	assert.Contains(t, props, "workspace_mode")
	assert.Contains(t, tool.Tool.Description, "explicit agent_profile_id > current/source task or parent task > workflow defaults > workspace default")
	assert.Contains(t, tool.Tool.Description, "Do not rely on workspace defaults for follow-up work from an active task")
	assert.Contains(t, tool.Tool.Description, "workspace_mode='new_workspace'")

	agentProfileProp, ok := props["agent_profile_id"].(map[string]interface{})
	require.True(t, ok, "agent_profile_id schema should be an object")
	agentProfileDesc, ok := agentProfileProp["description"].(string)
	require.True(t, ok, "agent_profile_id should have a description")
	assert.Contains(t, agentProfileDesc, "explicit value > current/source task or parent task > workflow defaults > workspace default")

	workflowProp, ok := props["workflow_id"].(map[string]interface{})
	require.True(t, ok, "workflow_id schema should be an object")
	workflowDesc, ok := workflowProp["description"].(string)
	require.True(t, ok, "workflow_id should have a description")
	assert.Contains(t, workflowDesc, "workspace_id is also omitted")
	assert.Contains(t, workflowDesc, "must belong to the effective workspace_id")

	workflowStepProp, ok := props["workflow_step_id"].(map[string]interface{})
	require.True(t, ok, "workflow_step_id schema should be an object")
	workflowStepDesc, ok := workflowStepProp["description"].(string)
	require.True(t, ok, "workflow_step_id should have a description")
	assert.Contains(t, workflowStepDesc, "pass only with an explicit workflow_id")

	workspaceModeProp, ok := props["workspace_mode"].(map[string]interface{})
	require.True(t, ok, "workspace_mode schema should be an object")
	workspaceModeDesc, ok := workspaceModeProp["description"].(string)
	require.True(t, ok, "workspace_mode should have a description")
	assert.Contains(t, workspaceModeDesc, "inherit_parent")
	assert.Contains(t, workspaceModeDesc, "new_workspace")

	// parent_id, workspace_id, workflow_id, workflow_step_id should NOT be required
	required, _ := parsed["required"].([]interface{})
	requiredSet := make(map[string]bool)
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	assert.True(t, requiredSet["title"], "title should be required")
	assert.False(t, requiredSet["parent_id"], "parent_id should not be required")
	assert.False(t, requiredSet["workspace_id"], "workspace_id should not be required")
	assert.False(t, requiredSet["workflow_id"], "workflow_id should not be required")
}

func TestCreateTask_SelfResolvesToTaskID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "subtask-1", "parent_id": "task-current"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Write tests",
		"parent_id": "self",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPCreateTask, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["parent_id"], "self should resolve to current task ID")
	assert.Equal(t, "Write tests", payload["title"])
	assert.Equal(t, "task-current", payload["source_task_id"], "source_task_id should be set to current task")
	assert.Equal(t, true, payload["start_agent"], "start_agent should default to true")
}

func TestCreateTask_SelfWithNoTaskContext_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Write tests",
		"parent_id": "self",
	})

	assert.True(t, result.IsError)
}

func TestCreateTask_ExplicitParentID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "subtask-1", "parent_id": "task-abc"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":     "Fix bug",
		"parent_id": "task-abc",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-abc", payload["parent_id"])
}

func TestCreateTask_ForwardsWorkspaceMode(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "subtask-1", "parent_id": "task-current"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":          "Own workspace",
		"parent_id":      "self",
		"workspace_mode": "new_workspace",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["parent_id"])
	assert.Equal(t, "new_workspace", payload["workspace_mode"])
}

func TestCreateTask_NoParentID_WithIDs_CreatesTopLevelTask(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Standalone"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "Standalone",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "", payload["parent_id"])
	assert.Equal(t, "ws-1", payload["workspace_id"])
	assert.Equal(t, "wf-1", payload["workflow_id"])
	assert.Equal(t, "task-current", payload["source_task_id"])
}

func TestCreateTask_SourceTaskID_AlwaysSet(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new"},
	}
	s := newTaskModeServer(t, backend, "my-task-123")

	callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "New task",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "my-task-123", payload["source_task_id"])
}

func TestCreateTask_SourceTaskID_EmptyWhenNoTaskContext(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new"},
	}
	s := newTaskModeServer(t, backend, "")

	callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "New task",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
	})

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "", payload["source_task_id"])
}

func TestCreateTask_StartAgentFalse_DoesNotAutoStart(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Plan task"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "Plan task",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
		"start_agent":  false,
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, false, payload["start_agent"], "start_agent should be false when explicitly set")
}

func TestCreateTask_WithRepositoryID(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Task with repo"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":         "Task with repo",
		"workspace_id":  "ws-1",
		"workflow_id":   "wf-1",
		"repository_id": "repo-123",
		"base_branch":   "main",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "repo-123", repos[0]["repository_id"])
	assert.Equal(t, "main", repos[0]["base_branch"])
}

func TestCreateTask_WithLocalPath(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Task with local path"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":        "Task with local path",
		"workspace_id": "ws-1",
		"workflow_id":  "wf-1",
		"local_path":   "/Users/me/projects/myrepo",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "/Users/me/projects/myrepo", repos[0]["local_path"])
}

func TestCreateTask_WithRepositoryURL(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Task with URL"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":          "Task with URL",
		"workspace_id":   "ws-1",
		"workflow_id":    "wf-1",
		"repository_url": "https://github.com/acme/widgets",
		"base_branch":    "main",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://github.com/acme/widgets", repos[0]["github_url"])
	assert.Equal(t, "main", repos[0]["base_branch"])
}

// TestCreateTask_BaseBranchOnly_ForwardsTopLevel pins the bug-fix wiring:
// when the caller passes only base_branch (no repository_id / local_path /
// repository_url), the MCP server forwards it at the top level of the WS
// payload so the backend can apply it as an override on inherited
// subtask repos. Previously base_branch was silently dropped when no
// repo identifier was passed.
func TestCreateTask_BaseBranchOnly_ForwardsTopLevel(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "subtask-1", "parent_id": "task-current"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":       "Stacked PR child",
		"parent_id":   "self",
		"description": "branch off the parent's PR branch",
		"base_branch": "feature/create-new-page-endp-05z",
	})

	assert.False(t, result.IsError)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "feature/create-new-page-endp-05z", payload["base_branch"],
		"base_branch should be forwarded at the top level even when no repo identifier is supplied")
	_, hasRepos := payload["repositories"]
	assert.False(t, hasRepos, "no repositories slice should be produced when only base_branch is supplied")
}

func TestCreateTask_RepositoryURL_AllowedForSubtasks(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Subtask with URL"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":          "Subtask with URL",
		"parent_id":      "self",
		"description":    "Fix the upstream review-eligibility check",
		"repository_url": "https://github.com/acme/widgets",
		"base_branch":    "main",
	})

	assert.False(t, result.IsError, "repository_url should be accepted for subtasks (cross-repo subtask)")

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["parent_id"], "self resolves to current task id")

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok, "repositories should be a slice")
	require.Len(t, repos, 1)
	assert.Equal(t, "https://github.com/acme/widgets", repos[0]["github_url"])
	assert.Equal(t, "main", repos[0]["base_branch"])
}

func TestCreateTask_LocalPath_AllowedForSubtasks(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "task-new", "title": "Subtask with local path"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "create_task_kandev", map[string]interface{}{
		"title":       "Subtask with local path",
		"parent_id":   "self",
		"description": "Patch the sibling repo",
		"local_path":  "/Users/me/projects/sibling",
	})

	assert.False(t, result.IsError, "local_path should be accepted for subtasks (cross-repo subtask)")

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["parent_id"])

	repos, ok := payload["repositories"].([]map[string]string)
	require.True(t, ok)
	require.Len(t, repos, 1)
	assert.Equal(t, "/Users/me/projects/sibling", repos[0]["local_path"])
}

// TestAddBranchToTask_ForwardsRepositoryURL verifies the agent-facing alias:
// repository_url on the MCP tool surface translates to github_url on the WS
// payload — mirroring create_task_kandev's wire format so the backend handler
// can resolve through the same code path.
func TestAddBranchToTask_ForwardsRepositoryURL(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "tr-1", "task_id": "task-current"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "add_branch_to_task_kandev", map[string]interface{}{
		"repository_url":  "https://github.com/acme/widgets",
		"checkout_branch": "feature/x",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPAddBranchToTask, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["task_id"], "task_id should default to current task")
	assert.Equal(t, "https://github.com/acme/widgets", payload["github_url"],
		"repository_url should be forwarded as github_url to match create_task wire format")
	assert.Equal(t, "feature/x", payload["checkout_branch"])
	assert.Equal(t, "", payload["repository_id"])
}

// TestAddBranchToTask_ForwardsLocalPath verifies local_path is plumbed through
// to the WS payload so the backend can find-or-create the repo in the task's
// workspace.
func TestAddBranchToTask_ForwardsLocalPath(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{"id": "tr-1", "task_id": "task-current"},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "add_branch_to_task_kandev", map[string]interface{}{
		"local_path":      "/Users/me/projects/sibling",
		"checkout_branch": "feature/y",
	})

	assert.False(t, result.IsError)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-current", payload["task_id"], "task_id should default to current task")
	assert.Equal(t, "/Users/me/projects/sibling", payload["local_path"])
	assert.Equal(t, "feature/y", payload["checkout_branch"])
	assert.Equal(t, "", payload["repository_id"])
}

// TestAddBranchToTask_RejectsMultipleLocators verifies the MCP-tier
// mutual-exclusion check fires before the request hits the WS handler, so
// the error names the agent-facing alias (repository_url) instead of the
// wire field (github_url).
func TestAddBranchToTask_RejectsMultipleLocators(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "add_branch_to_task_kandev", map[string]interface{}{
		"repository_url": "https://github.com/acme/widgets",
		"local_path":     "/Users/me/projects/sibling",
	})

	assert.True(t, result.IsError, "passing both repository_url and local_path should error at the MCP tier")
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "repository_url",
		"MCP-tier error should name the agent-facing alias, not the wire key")
	assert.Nil(t, backend.lastPayload, "request must not be forwarded to the backend")
}

func TestMessageTask_ForwardsToBackend(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"task_id":    "task-target",
			"session_id": "sess-1",
			"status":     "queued",
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "message_task_kandev", map[string]interface{}{
		"task_id":           "task-target",
		"session_id":        "sess-target",
		"prompt":            "follow up",
		"delivery_mode":     "interrupt",
		"sender_task_id":    "spoofed-task",
		"sender_session_id": "spoofed-session",
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPMessageTask, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-target", payload["task_id"])
	assert.Equal(t, "sess-target", payload["session_id"])
	assert.Equal(t, "follow up", payload["prompt"])
	assert.Equal(t, "interrupt", payload["delivery_mode"])
	assert.Equal(t, "task-current", payload["sender_task_id"])
	assert.Equal(t, "test-session", payload["sender_session_id"])
}

func TestMessageTask_DescriptionExplainsQueueInterruptAndStop(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	tools := s.mcpServer.ListTools()
	messageTool, ok := tools["message_task_kandev"]
	require.True(t, ok)
	description := messageTool.Tool.Description

	assert.Contains(t, description, `delivery_mode="queued"`)
	assert.Contains(t, description, `delivery_mode="interrupt"`)
	assert.Contains(t, description, "stop_task_kandev")
	assert.Contains(t, description, `safely falls back to "queued"`)
}

func TestMessageTask_MissingTaskID_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "message_task_kandev", map[string]interface{}{
		"prompt": "follow up",
	})

	assert.True(t, result.IsError)
}

func TestMessageTask_MissingPrompt_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "message_task_kandev", map[string]interface{}{
		"task_id": "task-target",
	})

	assert.True(t, result.IsError)
}

func TestStopTask_ToolSchemaIsMinimalAndDescriptionIsAccurate(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	tools := s.mcpServer.ListTools()
	stopTool, ok := tools["stop_task_kandev"]
	require.True(t, ok, "stop_task_kandev must be registered in task mode")

	schema, err := json.Marshal(stopTool.Tool.InputSchema)
	require.NoError(t, err)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(schema, &parsed))

	properties, ok := parsed["properties"].(map[string]interface{})
	require.True(t, ok, "stop schema must declare properties")
	require.Len(t, properties, 1, "stop schema must not expose sender, session, reason, or force controls")
	assert.Contains(t, properties, "task_id")
	for _, forbidden := range []string{"sender_task_id", "sender_session_id", "session_id", "reason", "force"} {
		assert.NotContains(t, properties, forbidden)
	}

	required, ok := parsed["required"].([]interface{})
	require.True(t, ok, "stop schema must declare task_id as required")
	assert.Equal(t, []interface{}{"task_id"}, required)

	description := stopTool.Tool.Description
	for _, phrase := range []string{
		"direct child",
		"all live sessions",
		"halt-only",
		"does not send a prompt or start a replacement turn",
		"CANCELLED",
		"REVIEW",
		"asynchronously",
		"not_running",
		"message_task_kandev",
		`delivery_mode="interrupt"`,
	} {
		assert.Contains(t, description, phrase)
	}
}

func TestStopTask_ForwardsTrustedSenderToBackend(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"task_id": "task-target",
			"status":  "stopped",
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "stop_task_kandev", map[string]interface{}{
		"task_id":           "task-target",
		"sender_task_id":    "spoofed-task",
		"sender_session_id": "spoofed-session",
		"session_id":        "spoofed-target-session",
		"reason":            "unsafe caller-controlled reason",
		"force":             true,
	})

	assert.False(t, result.IsError)
	assert.Equal(t, "mcp.stop_task", backend.lastAction)
	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	require.Len(t, payload, 2, "forwarder must build a fresh trusted payload")
	assert.Equal(t, "task-target", payload["task_id"])
	assert.Equal(t, "task-current", payload["sender_task_id"])
	assert.NotContains(t, payload, "sender_session_id")
	assert.NotContains(t, payload, "session_id")
	assert.NotContains(t, payload, "reason")
	assert.NotContains(t, payload, "force")
}

func TestStopTask_MissingTaskIDReturnsErrorWithoutForwarding(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "stop_task_kandev", map[string]interface{}{})

	assert.True(t, result.IsError)
	assert.Empty(t, backend.lastAction)
	assert.Nil(t, backend.lastPayload)
}

func TestStopTask_BackendErrorReturnsToolError(t *testing.T) {
	backend := &testBackend{err: errors.New("stop refused")}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "stop_task_kandev", map[string]interface{}{
		"task_id": "task-target",
	})

	assert.True(t, result.IsError)
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	assert.Contains(t, text.Text, "stop refused")
}

func TestGetTaskConversation_ForwardsToBackend(t *testing.T) {
	backend := &testBackend{
		response: map[string]interface{}{
			"task_id":    "task-target",
			"session_id": "sess-1",
			"messages":   []interface{}{},
			"total":      0,
			"has_more":   false,
			"cursor":     "",
		},
	}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "get_task_conversation_kandev", map[string]interface{}{
		"task_id":       "task-target",
		"session_id":    "sess-1",
		"limit":         25,
		"sort":          "desc",
		"message_types": []interface{}{"message", "tool_call"},
	})

	assert.False(t, result.IsError)
	assert.Equal(t, ws.ActionMCPGetTaskConversation, backend.lastAction)

	payload, ok := backend.lastPayload.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-target", payload["task_id"])
	assert.Equal(t, "sess-1", payload["session_id"])
	assert.Equal(t, 25, payload["limit"])
	assert.Equal(t, "desc", payload["sort"])
	assert.Equal(t, []string{"message", "tool_call"}, payload["message_types"])
}

func TestGetTaskConversation_MissingTaskID_ReturnsError(t *testing.T) {
	backend := &testBackend{}
	s := newTaskModeServer(t, backend, "task-current")

	result := callTool(t, s, "get_task_conversation_kandev", map[string]interface{}{})

	assert.True(t, result.IsError)
}
