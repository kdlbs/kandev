package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	return log
}

// getRegisteredToolNames returns the names of all tools registered on the MCP server.
func getRegisteredToolNames(s *Server) []string {
	toolsMap := s.mcpServer.ListTools()
	names := make([]string, 0, len(toolsMap))
	for name := range toolsMap {
		names = append(names, name)
	}
	return names
}

func TestServerModeTask_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// Task mode should have kanban tools
	assert.Contains(t, tools, "list_workspaces")
	assert.Contains(t, tools, "list_workflows")
	assert.Contains(t, tools, "list_workflow_steps")
	assert.Contains(t, tools, "list_tasks")
	assert.Contains(t, tools, "create_task")
	assert.Contains(t, tools, "update_task")

	// Task mode should have plan tools
	assert.Contains(t, tools, "create_task_plan")
	assert.Contains(t, tools, "get_task_plan")
	assert.Contains(t, tools, "update_task_plan")
	assert.Contains(t, tools, "delete_task_plan")

	// Task mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question")

	// Task mode should NOT have config tools
	assert.NotContains(t, tools, "create_workflow")
	assert.NotContains(t, tools, "update_workflow")
	assert.NotContains(t, tools, "delete_workflow")
	assert.NotContains(t, tools, "create_workflow_step")
	assert.NotContains(t, tools, "update_workflow_step")
	assert.NotContains(t, tools, "list_agents")
	assert.NotContains(t, tools, "update_agent")
	assert.NotContains(t, tools, "create_agent_profile")
	assert.NotContains(t, tools, "delete_agent_profile")
	assert.NotContains(t, tools, "list_agent_profiles")
	assert.NotContains(t, tools, "update_agent_profile")
	assert.NotContains(t, tools, "get_mcp_config")
	assert.NotContains(t, tools, "update_mcp_config")
	assert.NotContains(t, tools, "move_task")
	assert.NotContains(t, tools, "delete_task")
	assert.NotContains(t, tools, "archive_task")
	assert.NotContains(t, tools, "list_executors")
	assert.NotContains(t, tools, "list_executor_profiles")
	assert.NotContains(t, tools, "create_executor_profile")
	assert.NotContains(t, tools, "update_executor_profile")
	assert.NotContains(t, tools, "delete_executor_profile")
	assert.NotContains(t, tools, "update_task_state")
	assert.NotContains(t, tools, "delete_workflow_step")
	assert.NotContains(t, tools, "reorder_workflow_steps")
}

func TestServerModeConfig_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// Config mode should have workflow config tools
	assert.Contains(t, tools, "list_workspaces")
	assert.Contains(t, tools, "list_workflows")
	assert.Contains(t, tools, "create_workflow")
	assert.Contains(t, tools, "update_workflow")
	assert.Contains(t, tools, "delete_workflow")
	assert.Contains(t, tools, "list_workflow_steps")
	assert.Contains(t, tools, "create_workflow_step")
	assert.Contains(t, tools, "update_workflow_step")
	assert.Contains(t, tools, "delete_workflow_step")
	assert.Contains(t, tools, "reorder_workflow_steps")

	// Config mode should have agent tools
	assert.Contains(t, tools, "list_agents")
	assert.Contains(t, tools, "update_agent")
	assert.Contains(t, tools, "create_agent_profile")
	assert.Contains(t, tools, "delete_agent_profile")

	// Config mode should have MCP config tools
	assert.Contains(t, tools, "list_agent_profiles")
	assert.Contains(t, tools, "update_agent_profile")
	assert.Contains(t, tools, "get_mcp_config")
	assert.Contains(t, tools, "update_mcp_config")

	// Config mode should have executor profile tools
	assert.Contains(t, tools, "list_executors")
	assert.Contains(t, tools, "list_executor_profiles")
	assert.Contains(t, tools, "create_executor_profile")
	assert.Contains(t, tools, "update_executor_profile")
	assert.Contains(t, tools, "delete_executor_profile")

	// Config mode should have task tools
	assert.Contains(t, tools, "list_tasks")
	assert.Contains(t, tools, "move_task")
	assert.Contains(t, tools, "delete_task")
	assert.Contains(t, tools, "archive_task")
	assert.Contains(t, tools, "update_task_state")

	// Config mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question")

	// Config mode should NOT have plan tools
	assert.NotContains(t, tools, "create_task_plan")
	assert.NotContains(t, tools, "get_task_plan")
	assert.NotContains(t, tools, "update_task_plan")
	assert.NotContains(t, tools, "delete_task_plan")

	// Config mode should NOT have task-mode kanban create/update tools
	assert.NotContains(t, tools, "create_task")
	assert.NotContains(t, tools, "update_task")
}

func TestServerModeDefault_DefaultsToTask(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, "")
	require.NotNil(t, s)
	assert.Equal(t, ModeTask, s.mode)

	tools := getRegisteredToolNames(s)
	assert.Contains(t, tools, "create_task")
	assert.Contains(t, tools, "create_task_plan")
	assert.NotContains(t, tools, "create_workflow_step")
}

func TestServerModeConfig_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeConfig)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question")
	assert.Contains(t, tools, "list_agents")
	assert.Contains(t, tools, "create_workflow_step")
}

func TestServerModeTask_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeTask)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question")
	assert.Contains(t, tools, "create_task")
	assert.Contains(t, tools, "create_task_plan")
}

func TestServerModeTask_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	tools := getRegisteredToolNames(s)
	// 6 kanban + 1 interaction + 4 plan = 11
	assert.Equal(t, 11, len(tools))
}

func TestServerModeConfig_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	tools := getRegisteredToolNames(s)
	// 10 workflow + 4 agent + 4 mcp + 5 executor + 5 task + 1 interaction = 29
	assert.Equal(t, 29, len(tools))
}

func TestServerModeConfig_ToolDescriptions(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)

	toolsMap := s.mcpServer.ListTools()

	assert.Contains(t, toolsMap["create_workflow_step"].Tool.Description, "Create a new workflow step")
	assert.Contains(t, toolsMap["list_agents"].Tool.Description, "List all configured agents")
	assert.Contains(t, toolsMap["get_mcp_config"].Tool.Description, "Get MCP server configuration")
}

func TestServerModeConstants(t *testing.T) {
	assert.Equal(t, "task", ModeTask)
	assert.Equal(t, "config", ModeConfig)
}
