package sysprompt

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test constants to avoid repeated string literals.
const (
	testConfigPrompt = "Configure my workflow"
	testPlanPrompt   = "Plan this task"
	testTaskID       = "task-123"
	testSessionID    = "session-123"
)

// --- ConfigContext tests ---

func TestConfigContext_ContainsAllTools(t *testing.T) {
	expectedTools := []string{
		"list_workspaces_kandev",
		"list_workflows_kandev",
		"create_workflow_kandev",
		"update_workflow_kandev",
		"delete_workflow_kandev",
		"list_workflow_steps_kandev",
		"create_workflow_step_kandev",
		"update_workflow_step_kandev",
		"delete_workflow_step_kandev",
		"reorder_workflow_steps_kandev",
		"list_agents_kandev",
		"update_agent_kandev",
		"create_agent_profile_kandev",
		"delete_agent_profile_kandev",
		"list_executors_kandev",
		"list_executor_profiles_kandev",
		"create_executor_profile_kandev",
		"update_executor_profile_kandev",
		"delete_executor_profile_kandev",
		"list_agent_profiles_kandev",
		"update_agent_profile_kandev",
		"get_mcp_config_kandev",
		"update_mcp_config_kandev",
		"list_tasks_kandev",
		"move_task_kandev",
		"delete_task_kandev",
		"archive_task_kandev",
		"update_task_state_kandev",
		"ask_user_question_kandev",
	}

	for _, tool := range expectedTools {
		assert.Contains(t, ConfigContext(), tool, "ConfigContext should contain tool: %s", tool)
	}
}

func TestConfigContext_ContainsSections(t *testing.T) {
	assert.Contains(t, ConfigContext(), "WORKFLOW TOOLS:")
	assert.Contains(t, ConfigContext(), "AGENT TOOLS:")
	assert.Contains(t, ConfigContext(), "EXECUTOR PROFILE TOOLS:")
	assert.Contains(t, ConfigContext(), "MCP CONFIG TOOLS:")
	assert.Contains(t, ConfigContext(), "TASK TOOLS:")
	assert.Contains(t, ConfigContext(), "INTERACTION:")
	assert.Contains(t, ConfigContext(), "EXAMPLE REQUESTS")
}

func TestConfigContext_HasExactlyOneSessionIDPlaceholder(t *testing.T) {
	count := strings.Count(ConfigContext(), "{session_id}")
	assert.Equal(t, 1, count, "ConfigContext should have exactly 1 {session_id} placeholder")
}

func TestFormatConfigContext_InjectsSessionID(t *testing.T) {
	result := FormatConfigContext("session-abc-123")
	assert.Contains(t, result, "Session ID: session-abc-123")
	assert.NotContains(t, result, "{session_id}")
}

func TestInjectConfigContext_WrapsInSystemTags(t *testing.T) {
	result := InjectConfigContext(testSessionID, testConfigPrompt)
	assert.True(t, strings.HasPrefix(result, TagStart))
	assert.Contains(t, result, TagEnd)
	assert.Contains(t, result, testConfigPrompt)
	assert.Contains(t, result, testSessionID)
}

func TestInjectConfigContext_SystemContentStrippable(t *testing.T) {
	result := InjectConfigContext(testSessionID, testConfigPrompt)
	stripped := StripSystemContent(result)
	assert.Equal(t, testConfigPrompt, stripped)
	assert.NotContains(t, stripped, "KANDEV CONFIG MCP TOOLS")
}

// --- KandevContext tests (existing, verify not broken) ---

func TestKandevContext_HasExactlyTwoPlaceholders(t *testing.T) {
	ctx := KandevContext()
	taskCount := strings.Count(ctx, "{task_id}")
	sessionCount := strings.Count(ctx, "{session_id}")
	assert.Equal(t, 1, taskCount, "KandevContext should have exactly 1 {task_id} placeholder")
	assert.Equal(t, 1, sessionCount, "KandevContext should have exactly 1 {session_id} placeholder")
}

func TestFormatKandevContext_InjectsIDs(t *testing.T) {
	result := FormatKandevContext("task-abc", "session-xyz")
	assert.Contains(t, result, "Kandev Task ID: task-abc")
	assert.Contains(t, result, "Session ID: session-xyz")
	assert.NotContains(t, result, "{task_id}")
	assert.NotContains(t, result, "{session_id}")
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
	result := InjectPlanMode(testPlanPrompt)
	assert.True(t, strings.HasPrefix(result, TagStart))
	assert.Contains(t, result, "PLAN MODE ACTIVE")
	assert.Contains(t, result, testPlanPrompt)
}

func TestInjectPlanMode_SystemContentStrippable(t *testing.T) {
	result := InjectPlanMode(testPlanPrompt)
	stripped := StripSystemContent(result)
	assert.Equal(t, testPlanPrompt, stripped)
}

// --- SessionHandover tests ---

func TestSessionHandoverContext_HasPlaceholders(t *testing.T) {
	ctx := SessionHandoverContext()
	assert.Contains(t, ctx, "{session_count}")
	assert.Contains(t, ctx, "{plan_section}")
}

func TestFormatSessionHandover_InjectsValues(t *testing.T) {
	result := FormatSessionHandover(3, "PLAN: do the thing")
	assert.Contains(t, result, "3 previous session(s)")
	assert.Contains(t, result, "PLAN: do the thing")
	assert.NotContains(t, result, "{session_count}")
	assert.NotContains(t, result, "{plan_section}")
}

func TestInjectSessionHandover_WrapsInSystemTags(t *testing.T) {
	result := InjectSessionHandover(2, "", "Do the work")
	assert.True(t, strings.HasPrefix(result, TagStart))
	assert.Contains(t, result, "Do the work")
}

func TestInjectSessionHandover_SystemContentStrippable(t *testing.T) {
	result := InjectSessionHandover(2, "", "Do the work")
	stripped := StripSystemContent(result)
	assert.Equal(t, "Do the work", stripped)
}

func TestFormatSessionHandover_ValueWithPlaceholderLikeText(t *testing.T) {
	// Verify single-pass replacement: a plan section containing {session_count}
	// must not be re-processed.
	result := FormatSessionHandover(2, "Plan mentions {session_count} literally")
	assert.Contains(t, result, "2 previous session(s)")
	assert.Contains(t, result, "Plan mentions {session_count} literally")
}

// --- InterpolatePlaceholders tests ---

func TestInterpolatePlaceholders_TaskID(t *testing.T) {
	result := InterpolatePlaceholders("Check {task_id} status", testTaskID)
	assert.Equal(t, "Check task-123 status", result)
}

func TestInterpolatePlaceholders_NoPlaceholders(t *testing.T) {
	result := InterpolatePlaceholders("No placeholders here", testTaskID)
	assert.Equal(t, "No placeholders here", result)
}

func TestInterpolatePlaceholders_MultiplePlaceholders(t *testing.T) {
	result := InterpolatePlaceholders("{task_id} and {task_id}", testTaskID)
	assert.Equal(t, "task-123 and task-123", result)
}

// --- ConfigContext vs KandevContext distinction ---

func TestConfigContext_DoesNotContainPlanTools(t *testing.T) {
	assert.NotContains(t, ConfigContext(), "create_task_plan_kandev")
	assert.NotContains(t, ConfigContext(), "get_task_plan_kandev")
	assert.NotContains(t, ConfigContext(), "update_task_plan_kandev")
	assert.NotContains(t, ConfigContext(), "delete_task_plan_kandev")
}

func TestKandevContext_DoesNotContainConfigTools(t *testing.T) {
	assert.NotContains(t, KandevContext(), "create_workflow_step_kandev")
	assert.NotContains(t, KandevContext(), "update_workflow_step_kandev")
	assert.NotContains(t, KandevContext(), "list_agents_kandev")
	assert.NotContains(t, KandevContext(), "create_agent_kandev")
}
