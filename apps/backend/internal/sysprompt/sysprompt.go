// Package sysprompt provides centralized system prompts and utilities for
// injecting system-level instructions into agent conversations.
//
// All system prompts are wrapped in <kandev-system> tags to mark them as
// system-injected content that can be stripped when displaying to users.
package sysprompt

import (
	"fmt"
	"regexp"
	"strings"
)

// System tag constants for marking system-injected content.
const (
	// TagStart marks the beginning of system-injected content.
	TagStart = "<kandev-system>"
	// TagEnd marks the end of system-injected content.
	TagEnd = "</kandev-system>"
)

// systemTagRegex matches <kandev-system>...</kandev-system> content including the tags.
var systemTagRegex = regexp.MustCompile(`<kandev-system>[\s\S]*?</kandev-system>\s*`)

// StripSystemContent removes all <kandev-system>...</kandev-system> blocks from text.
// This is used to hide system-injected content from the frontend UI.
func StripSystemContent(text string) string {
	return strings.TrimSpace(systemTagRegex.ReplaceAllString(text, ""))
}

// Wrap wraps content in <kandev-system> tags to mark it as system-injected.
func Wrap(content string) string {
	return TagStart + content + TagEnd
}

// HasSystemContent checks whether the text contains any <kandev-system> tags.
func HasSystemContent(text string) bool {
	return systemTagRegex.MatchString(text)
}

// PlanMode is the system prompt prepended when plan mode is enabled.
// It instructs agents to collaborate on the plan without implementing changes.
// Used for both initial plan creation and follow-up plan refinement messages.
const PlanMode = `PLAN MODE ACTIVE:
You are in planning mode. Do not implement anything — focus on the plan.

The plan is a shared document that both you and the user can edit. The user may have already written parts of the plan or made edits to your previous version.
Ask the user clarifying questions if anything is unclear or if you need guidance on how to proceed.

WORKFLOW:
1. Read the current plan using the get_task_plan_kandev MCP tool.
2. Build on what already exists. Only replace or discard the user content if it is clearly irrelevant or incorrect.
3. If you need more context to make specific additions, explore the codebase - search for relevant files, read existing code, and understand the patterns in use. Ask questions if needed.
4. Make your additions specific to this project — reference actual file paths, function names, types, and architectural patterns. Avoid adding generic or boilerplate content.
5. Save your changes using the update_task_plan_kandev MCP tool (or create_task_plan_kandev if no plan exists yet).
6. After saving, STOP and wait for the user to review.

This instruction applies to THIS PROMPT ONLY.`

// KandevContext is the system prompt that provides Kandev-specific instructions
// and session context to agents. Use FormatKandevContext to inject task/session IDs.
const KandevContext = `KANDEV MCP TOOLS — You have access to the following MCP tools from the "kandev" server.
Always use the exact tool names shown below (they include the _kandev suffix).

Kandev Task ID: %s
Kandev Session ID: %s
Use these IDs when calling tools that require task_id or session_id.

Available tools:
- ask_user_question_kandev: Ask the user a clarifying question with multiple-choice options. Use this whenever you need user input before proceeding. Required params: prompt (string), options (array of {label, description}).
- create_task_plan_kandev: Save an implementation plan for the current task. Required params: task_id, content (markdown). Optional: title.
- get_task_plan_kandev: Retrieve the current plan for a task (includes any user edits). Required params: task_id.
- update_task_plan_kandev: Update an existing plan. Required params: task_id, content (markdown). Optional: title.
- delete_task_plan_kandev: Delete a task plan. Required params: task_id.
- list_workspaces_kandev: List all workspaces.
- list_workflows_kandev: List workflows in a workspace. Required params: workspace_id.
- list_tasks_kandev: List tasks in a workflow. Required params: workflow_id.
- create_task_kandev: Create a new task. Required params: workspace_id, workflow_id, workflow_step_id, title.
- update_task_kandev: Update a task. Required params: task_id.

IMPORTANT: You MUST use these MCP tools when instructed to create plans, ask questions, or interact with the Kandev platform. Do not skip them.`

// FormatKandevContext returns the Kandev context prompt with task and session IDs injected.
func FormatKandevContext(taskID, sessionID string) string {
	return fmt.Sprintf(KandevContext, taskID, sessionID)
}

// ConfigContext is the system prompt for config-mode MCP sessions.
// It provides instructions for agents performing configuration operations
// via the dedicated config chat on the settings page.
const ConfigContext = `KANDEV CONFIG MCP TOOLS — You are a configuration assistant for the Kandev platform.
You have access to the following MCP tools from the "kandev" server.
Always use the exact tool names shown below (they include the _kandev suffix).

Session ID: %s

WORKFLOW TOOLS:
- list_workspaces_kandev: List all workspaces to get workspace IDs.
- list_workflows_kandev: List workflows in a workspace. Required: workspace_id.
- create_workflow_kandev: Create a new workflow. Required: workspace_id, name. Optional: description.
- update_workflow_kandev: Update a workflow. Required: workflow_id. Optional: name, description.
- delete_workflow_kandev: Delete a workflow and all its steps (destructive). Required: workflow_id.
- list_workflow_steps_kandev: List workflow steps (columns) in a workflow. Required: workflow_id.
- create_workflow_step_kandev: Create a new workflow step. Required: workflow_id, name. Optional: position, color, prompt, is_start_step, allow_manual_move, show_in_command_panel, events.
- update_workflow_step_kandev: Update a workflow step. Required: step_id. Optional: name, color, prompt, is_start_step, allow_manual_move, show_in_command_panel, auto_archive_after_hours, events.
- delete_workflow_step_kandev: Delete a workflow step (destructive). Required: step_id.
- reorder_workflow_steps_kandev: Reorder workflow steps. Required: workflow_id, step_ids (ordered array of step IDs).

AGENT TOOLS:
- list_agents_kandev: List all configured agents and their profiles.
- update_agent_kandev: Update agent settings. Required: agent_id. Optional: supports_mcp, mcp_config_path.
- create_agent_profile_kandev: Create a new agent profile. Required: agent_id, name, model. Optional: auto_approve.
- delete_agent_profile_kandev: Delete an agent profile. Required: profile_id.

EXECUTOR PROFILE TOOLS:
Executors (local, worktree, local_docker, sprites) are pre-defined. Use list_executors to find executor IDs, then manage profiles.
- list_executors_kandev: List all executors with their IDs and types.
- list_executor_profiles_kandev: List profiles for an executor. Required: executor_id.
- create_executor_profile_kandev: Create an executor profile. Required: executor_id, name. Optional: mcp_policy, config, prepare_script, cleanup_script.
- update_executor_profile_kandev: Update an executor profile. Required: profile_id. Optional: name, mcp_policy, config, prepare_script, cleanup_script.
- delete_executor_profile_kandev: Delete an executor profile. Required: profile_id.

MCP CONFIG TOOLS:
- list_agent_profiles_kandev: List profiles for an agent. Required: agent_id.
- update_agent_profile_kandev: Update a profile. Required: profile_id. Optional: name, model, auto_approve.
- get_mcp_config_kandev: Get MCP server config for a profile. Required: profile_id.
- update_mcp_config_kandev: Update MCP config for a profile. Required: profile_id. Optional: enabled, servers.

TASK TOOLS:
- list_tasks_kandev: List all tasks in a workflow. Required: workflow_id.
- move_task_kandev: Move a task to a different workflow step. Required: task_id, workflow_step_id.
- delete_task_kandev: Delete a task. Required: task_id.
- archive_task_kandev: Archive a task. Required: task_id.
- update_task_state_kandev: Update task state. Required: task_id, state (TODO, CREATED, SCHEDULING, IN_PROGRESS, REVIEW, BLOCKED, WAITING_FOR_INPUT, COMPLETED, FAILED, CANCELLED).

INTERACTION:
- ask_user_question_kandev: Ask the user a clarifying question with multiple-choice options. Required: prompt, options.

EXAMPLE REQUESTS the user might ask:
- "Create a new workflow called 'Feature Development'"
- "Add a 'Code Review' step to my workflow"
- "Create a new agent profile for Claude with auto-approve enabled"
- "Show me the current workflow steps"
- "Update the MCP servers for the default agent profile"
- "Create a new executor profile for Docker with a prepare script"
- "Move all completed tasks to the 'Done' column"
- "Archive old tasks from last month"

IMPORTANT: Always list existing resources before creating or modifying. Confirm destructive operations (delete, archive) with the user first using ask_user_question_kandev.`

// FormatConfigContext returns the config context prompt with the session ID injected.
func FormatConfigContext(sessionID string) string {
	return fmt.Sprintf(ConfigContext, sessionID)
}

// InjectConfigContext prepends the config system prompt to a user's prompt.
// The system content is wrapped in <kandev-system> tags.
func InjectConfigContext(sessionID, prompt string) string {
	return Wrap(FormatConfigContext(sessionID)) + "\n\n" + prompt
}

// InjectKandevContext prepends the Kandev system prompt and session context to a user's prompt.
// The system content is wrapped in <kandev-system> tags.
func InjectKandevContext(taskID, sessionID, prompt string) string {
	return Wrap(FormatKandevContext(taskID, sessionID)) + "\n\n" + prompt
}

// DefaultPlanPrefix is the planning instruction prompt used when plan mode is
// requested but no workflow step provides its own prompt prefix.
const DefaultPlanPrefix = `[PLANNING PHASE]
Analyze this task and create a detailed implementation plan.

Before creating the plan, ask the user clarifying questions if anything is unclear.
Use the ask_user_question_kandev MCP tool to get answers before proceeding.

First check if a plan already exists using the get_task_plan_kandev MCP tool.
If the user has already started writing the plan, build on their content — do not replace it.

IMPORTANT: Before writing the plan, explore the codebase thoroughly. Read relevant files, search for existing patterns, and understand the project's architecture. Your plan must reference actual file paths, function names, types, and patterns from this project — not generic advice.

The plan should include:
1. Understanding of the requirements
2. Specific files that need to be modified or created (with actual paths from the codebase)
3. Step-by-step implementation approach grounded in existing code patterns
4. Potential risks or considerations

When including diagrams (architecture, sequence, flowcharts), always use mermaid syntax in code blocks.

Save the plan using the create_task_plan_kandev or update_task_plan_kandev MCP tool.
After saving, STOP and wait for user review. The user may edit the plan before approving it.
Do not create any other files during this phase — only use the MCP tools to save the plan.`

// InjectPlanMode prepends the plan mode system prompt to a user's prompt.
// The system content is wrapped in <kandev-system> tags.
func InjectPlanMode(prompt string) string {
	return Wrap(PlanMode) + "\n\n" + prompt
}

// SessionHandoverContext is injected when a new session starts for a task that
// already has previous sessions. It gives the agent awareness of prior work.
const SessionHandoverContext = `CONTEXT FROM PREVIOUS SESSIONS:
This task has had %d previous session(s). You are starting a new session on the same workspace.
Any code changes from previous sessions are already present in the working directory.
%s
Review the current state of the codebase before making changes — previous sessions may have
already completed part of the work. Do not repeat work that is already done.`

// FormatSessionHandover formats the session handover context.
// planSection should be pre-formatted (empty string if no plan exists).
func FormatSessionHandover(sessionCount int, planSection string) string {
	return fmt.Sprintf(SessionHandoverContext, sessionCount, planSection)
}

// InjectSessionHandover prepends session handover context to a prompt, wrapped in system tags.
func InjectSessionHandover(sessionCount int, planSection, prompt string) string {
	return Wrap(FormatSessionHandover(sessionCount, planSection)) + "\n\n" + prompt
}

// InterpolatePlaceholders replaces placeholders in prompt templates with actual values.
// Supported placeholders:
//   - {task_id} - the task ID
func InterpolatePlaceholders(template string, taskID string) string {
	result := template
	result = strings.ReplaceAll(result, "{task_id}", taskID)
	return result
}
