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
	assert.Contains(t, tools, "list_workspaces_kandev")
	assert.Contains(t, tools, "list_workflows_kandev")
	assert.Contains(t, tools, "list_workflow_steps_kandev")
	assert.Contains(t, tools, "list_tasks_kandev")
	assert.Contains(t, tools, "create_task_kandev")
	assert.Contains(t, tools, "update_task_kandev")
	assert.Contains(t, tools, "move_task_kandev")
	assert.Contains(t, tools, "message_task_kandev")
	assert.Contains(t, tools, "get_task_conversation_kandev")

	// Task mode should have plan tools
	assert.Contains(t, tools, "create_task_plan_kandev")
	assert.Contains(t, tools, "get_task_plan_kandev")
	assert.Contains(t, tools, "update_task_plan_kandev")
	assert.Contains(t, tools, "delete_task_plan_kandev")

	// Task mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question_kandev")

	// Task mode should have profile listing tools (needed for create_task)
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "list_executor_profiles_kandev")

	// Task mode should NOT have config/mutation tools
	assert.NotContains(t, tools, "create_workflow_kandev")
	assert.NotContains(t, tools, "update_workflow_kandev")
	assert.NotContains(t, tools, "delete_workflow_kandev")
	assert.NotContains(t, tools, "create_workflow_step_kandev")
	assert.NotContains(t, tools, "update_workflow_step_kandev")
	assert.NotContains(t, tools, "update_agent_kandev")
	assert.NotContains(t, tools, "create_agent_profile_kandev")
	assert.NotContains(t, tools, "delete_agent_profile_kandev")
	assert.NotContains(t, tools, "list_agent_profiles_kandev")
	assert.NotContains(t, tools, "update_agent_profile_kandev")
	assert.NotContains(t, tools, "get_mcp_config_kandev")
	assert.NotContains(t, tools, "update_mcp_config_kandev")
	assert.NotContains(t, tools, "delete_task_kandev")
	assert.NotContains(t, tools, "archive_task_kandev")
	assert.NotContains(t, tools, "list_executors_kandev")
	assert.NotContains(t, tools, "create_executor_profile_kandev")
	assert.NotContains(t, tools, "update_executor_profile_kandev")
	assert.NotContains(t, tools, "delete_executor_profile_kandev")
	assert.NotContains(t, tools, "update_task_state_kandev")
	assert.NotContains(t, tools, "delete_workflow_step_kandev")
	assert.NotContains(t, tools, "reorder_workflow_steps_kandev")
}

func TestServerModeConfig_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// Config mode should have workflow config tools
	assert.Contains(t, tools, "list_workspaces_kandev")
	assert.Contains(t, tools, "list_workflows_kandev")
	assert.Contains(t, tools, "list_repositories_kandev")
	assert.Contains(t, tools, "create_workflow_kandev")
	assert.Contains(t, tools, "update_workflow_kandev")
	assert.Contains(t, tools, "delete_workflow_kandev")
	assert.Contains(t, tools, "list_workflow_steps_kandev")
	assert.Contains(t, tools, "create_workflow_step_kandev")
	assert.Contains(t, tools, "update_workflow_step_kandev")
	assert.Contains(t, tools, "delete_workflow_step_kandev")
	assert.Contains(t, tools, "reorder_workflow_steps_kandev")

	// Config mode should have agent tools
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "update_agent_kandev")
	assert.Contains(t, tools, "create_agent_profile_kandev")
	assert.Contains(t, tools, "delete_agent_profile_kandev")

	// Config mode should have MCP config tools
	assert.Contains(t, tools, "list_agent_profiles_kandev")
	assert.Contains(t, tools, "update_agent_profile_kandev")
	assert.Contains(t, tools, "get_mcp_config_kandev")
	assert.Contains(t, tools, "update_mcp_config_kandev")

	// Config mode should have executor profile tools
	assert.Contains(t, tools, "list_executors_kandev")
	assert.Contains(t, tools, "list_executor_profiles_kandev")
	assert.Contains(t, tools, "create_executor_profile_kandev")
	assert.Contains(t, tools, "update_executor_profile_kandev")
	assert.Contains(t, tools, "delete_executor_profile_kandev")

	// Config mode should have task tools
	assert.Contains(t, tools, "list_tasks_kandev")
	assert.Contains(t, tools, "move_task_kandev")
	assert.Contains(t, tools, "delete_task_kandev")
	assert.Contains(t, tools, "archive_task_kandev")
	assert.Contains(t, tools, "update_task_state_kandev")
	assert.Contains(t, tools, "get_task_conversation_kandev")

	// Config mode should have interaction tools
	assert.Contains(t, tools, "ask_user_question_kandev")

	// Config mode should NOT have plan tools
	assert.NotContains(t, tools, "create_task_plan_kandev")
	assert.NotContains(t, tools, "get_task_plan_kandev")
	assert.NotContains(t, tools, "update_task_plan_kandev")
	assert.NotContains(t, tools, "delete_task_plan_kandev")

	// Config mode should NOT have task-mode kanban create/update tools
	assert.NotContains(t, tools, "create_task_kandev")
	assert.NotContains(t, tools, "update_task_kandev")
}

func TestServerModeDefault_DefaultsToTask(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, "")
	require.NotNil(t, s)
	assert.Equal(t, ModeTask, s.mode)

	tools := getRegisteredToolNames(s)
	assert.Contains(t, tools, "create_task_kandev")
	assert.Contains(t, tools, "create_task_plan_kandev")
	assert.NotContains(t, tools, "create_workflow_step_kandev")
}

func TestServerModeConfig_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeConfig)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "create_workflow_step_kandev")
}

func TestServerModeTask_DisableAskQuestion(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", true, ModeTask)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.Contains(t, tools, "create_task_kandev")
	assert.Contains(t, tools, "create_task_plan_kandev")
}

func TestServerModeTask_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeTask)
	tools := getRegisteredToolNames(s)
	// 11 kanban + 1 interaction + 4 plan = 16
	assert.Equal(t, 16, len(tools))
}

func TestServerModeConfig_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)
	tools := getRegisteredToolNames(s)
	// 11 workflow (incl. list_repositories) + 4 agent + 4 mcp + 5 executor + 6 task + 1 interaction = 31
	assert.Equal(t, 31, len(tools))
}

func TestServerModeConfig_ToolDescriptions(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "test-session", "test-task", 10005, log, "", false, ModeConfig)

	toolsMap := s.mcpServer.ListTools()

	assert.Contains(t, toolsMap["create_workflow_step_kandev"].Tool.Description, "Create a new workflow step")
	assert.Contains(t, toolsMap["list_agents_kandev"].Tool.Description, "List all configured agents")
	assert.Contains(t, toolsMap["get_mcp_config_kandev"].Tool.Description, "Get MCP server configuration")
}

func TestServerModeConstants(t *testing.T) {
	assert.Equal(t, "task", ModeTask)
	assert.Equal(t, "config", ModeConfig)
	assert.Equal(t, "external", ModeExternal)
}

func TestServerModeExternal_RegistersCorrectTools(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "", "", 0, log, "", true, ModeExternal)
	require.NotNil(t, s)

	tools := getRegisteredToolNames(s)

	// External mode includes all config tools
	assert.Contains(t, tools, "list_workspaces_kandev")
	assert.Contains(t, tools, "list_repositories_kandev")
	assert.Contains(t, tools, "create_workflow_kandev")
	assert.Contains(t, tools, "list_agents_kandev")
	assert.Contains(t, tools, "get_mcp_config_kandev")
	assert.Contains(t, tools, "list_executors_kandev")
	assert.Contains(t, tools, "move_task_kandev")

	// External mode includes create_task_kandev so external agents can spawn tasks
	assert.Contains(t, tools, "create_task_kandev")

	// External mode does NOT include session-scoped tools
	assert.NotContains(t, tools, "ask_user_question_kandev")
	assert.NotContains(t, tools, "create_task_plan_kandev")
	assert.NotContains(t, tools, "get_task_plan_kandev")
	assert.NotContains(t, tools, "update_task_plan_kandev")
	assert.NotContains(t, tools, "delete_task_plan_kandev")

	// External mode does NOT include kanban update_task_kandev (config has its own update_task_state)
	assert.NotContains(t, tools, "update_task_kandev")

	// External mode does NOT include message_task_kandev (no live session context)
	assert.NotContains(t, tools, "message_task_kandev")
}

func TestServerModeExternal_ToolCount(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := New(backend, "", "", 0, log, "", true, ModeExternal)
	tools := getRegisteredToolNames(s)
	// 11 workflow (incl. list_repositories) + 4 agent + 4 mcp + 5 executor + 6 task + 1 create_task = 31
	assert.Equal(t, 31, len(tools))
}

func TestNewExternal_Constructs(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	defer backend.Close()

	s := NewExternal(backend, "http://localhost:38429", log, "")
	require.NotNil(t, s)
	assert.Equal(t, ModeExternal, s.mode)
	assert.True(t, s.disableAskQuestion)
	assert.Empty(t, s.sessionID)
	assert.Empty(t, s.taskID)
	assert.NotNil(t, s.sseServer)
	assert.NotNil(t, s.httpServer)
}
