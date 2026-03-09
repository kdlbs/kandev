package sysprompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- ConfigContext tests ---

func TestConfigContext_ContainsAllTools(t *testing.T) {
	expectedTools := []string{
		"list_workspaces_kandev",
		"list_workflows_kandev",
		"list_workflow_steps_kandev",
		"create_workflow_step_kandev",
		"update_workflow_step_kandev",
		"list_agents_kandev",
		"create_agent_kandev",
		"update_agent_kandev",
		"delete_agent_kandev",
		"list_agent_profiles_kandev",
		"update_agent_profile_kandev",
		"get_mcp_config_kandev",
		"update_mcp_config_kandev",
		"list_tasks_kandev",
		"move_task_kandev",
		"delete_task_kandev",
		"archive_task_kandev",
		"ask_user_question_kandev",
	}

	for _, tool := range expectedTools {
		assert.Contains(t, ConfigContext, tool, "ConfigContext should contain tool: %s", tool)
	}
}

func TestConfigContext_ContainsSections(t *testing.T) {
	assert.Contains(t, ConfigContext, "WORKFLOW TOOLS:")
	assert.Contains(t, ConfigContext, "AGENT TOOLS:")
	assert.Contains(t, ConfigContext, "MCP CONFIG TOOLS:")
	assert.Contains(t, ConfigContext, "TASK TOOLS:")
	assert.Contains(t, ConfigContext, "INTERACTION:")
}

func TestConfigContext_HasSessionIDPlaceholder(t *testing.T) {
	assert.Contains(t, ConfigContext, "%s")
}

func TestFormatConfigContext_InjectsSessionID(t *testing.T) {
	result := FormatConfigContext("session-abc-123")
	assert.Contains(t, result, "session-abc-123")
	assert.NotContains(t, result, "%s")
}

func TestInjectConfigContext_WrapsInSystemTags(t *testing.T) {
	result := InjectConfigContext("session-123", "Configure my workflow")
	assert.True(t, strings.HasPrefix(result, TagStart))
	assert.Contains(t, result, TagEnd)
	assert.Contains(t, result, "Configure my workflow")
	assert.Contains(t, result, "session-123")
}

func TestInjectConfigContext_SystemContentStrippable(t *testing.T) {
	result := InjectConfigContext("session-123", "Configure my workflow")
	stripped := StripSystemContent(result)
	assert.Equal(t, "Configure my workflow", stripped)
	assert.NotContains(t, stripped, "KANDEV CONFIG MCP TOOLS")
}

// --- KandevContext tests (existing, verify not broken) ---

func TestKandevContext_ContainsTaskIDPlaceholder(t *testing.T) {
	assert.Contains(t, KandevContext, "%s")
}

func TestFormatKandevContext_InjectsIDs(t *testing.T) {
	result := FormatKandevContext("task-abc", "session-xyz")
	assert.Contains(t, result, "task-abc")
	assert.Contains(t, result, "session-xyz")
}

func TestInjectKandevContext_WrapsInSystemTags(t *testing.T) {
	result := InjectKandevContext("task-abc", "session-xyz", "Do something")
	assert.True(t, strings.HasPrefix(result, TagStart))
	assert.Contains(t, result, "Do something")
}

func TestInjectKandevContext_SystemContentStrippable(t *testing.T) {
	result := InjectKandevContext("task-abc", "session-xyz", "Do something")
	stripped := StripSystemContent(result)
	assert.Equal(t, "Do something", stripped)
}

// --- StripSystemContent tests ---

func TestStripSystemContent_NoTags(t *testing.T) {
	assert.Equal(t, "Hello world", StripSystemContent("Hello world"))
}

func TestStripSystemContent_OnlyTags(t *testing.T) {
	input := Wrap("system content only")
	assert.Equal(t, "", StripSystemContent(input))
}

func TestStripSystemContent_MixedContent(t *testing.T) {
	input := Wrap("hidden") + "\n\nvisible text"
	result := StripSystemContent(input)
	assert.Equal(t, "visible text", result)
}

func TestStripSystemContent_MultipleTags(t *testing.T) {
	input := Wrap("first") + " middle " + Wrap("second") + " end"
	result := StripSystemContent(input)
	// The regex replaces tags + trailing whitespace, so check both parts are present
	assert.Contains(t, result, "middle")
	assert.Contains(t, result, "end")
	assert.NotContains(t, result, "first")
	assert.NotContains(t, result, "second")
}

// --- Wrap and HasSystemContent tests ---

func TestWrap(t *testing.T) {
	result := Wrap("test content")
	assert.Equal(t, TagStart+"test content"+TagEnd, result)
}

func TestHasSystemContent(t *testing.T) {
	assert.True(t, HasSystemContent(Wrap("content")))
	assert.False(t, HasSystemContent("no tags"))
}

// --- PlanMode tests ---

func TestInjectPlanMode_WrapsInTags(t *testing.T) {
	result := InjectPlanMode("Plan this task")
	assert.True(t, strings.HasPrefix(result, TagStart))
	assert.Contains(t, result, "PLAN MODE ACTIVE")
	assert.Contains(t, result, "Plan this task")
}

func TestInjectPlanMode_SystemContentStrippable(t *testing.T) {
	result := InjectPlanMode("Plan this task")
	stripped := StripSystemContent(result)
	assert.Equal(t, "Plan this task", stripped)
}

// --- InterpolatePlaceholders tests ---

func TestInterpolatePlaceholders_TaskID(t *testing.T) {
	result := InterpolatePlaceholders("Check {task_id} status", "task-123")
	assert.Equal(t, "Check task-123 status", result)
}

func TestInterpolatePlaceholders_NoPlaceholders(t *testing.T) {
	result := InterpolatePlaceholders("No placeholders here", "task-123")
	assert.Equal(t, "No placeholders here", result)
}

func TestInterpolatePlaceholders_MultiplePlaceholders(t *testing.T) {
	result := InterpolatePlaceholders("{task_id} and {task_id}", "task-123")
	assert.Equal(t, "task-123 and task-123", result)
}

// --- ConfigContext vs KandevContext distinction ---

func TestConfigContext_DoesNotContainPlanTools(t *testing.T) {
	assert.NotContains(t, ConfigContext, "create_task_plan_kandev")
	assert.NotContains(t, ConfigContext, "get_task_plan_kandev")
	assert.NotContains(t, ConfigContext, "update_task_plan_kandev")
	assert.NotContains(t, ConfigContext, "delete_task_plan_kandev")
}

func TestKandevContext_DoesNotContainConfigTools(t *testing.T) {
	assert.NotContains(t, KandevContext, "create_workflow_step_kandev")
	assert.NotContains(t, KandevContext, "update_workflow_step_kandev")
	assert.NotContains(t, KandevContext, "list_agents_kandev")
	assert.NotContains(t, KandevContext, "create_agent_kandev")
	assert.NotContains(t, KandevContext, "move_task_kandev")
	assert.NotContains(t, KandevContext, "delete_task_kandev")
	assert.NotContains(t, KandevContext, "archive_task_kandev")
}
