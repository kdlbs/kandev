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
	return systemTagRegex.ReplaceAllString(text, "")
}

// Wrap wraps content in <kandev-system> tags to mark it as system-injected.
func Wrap(content string) string {
	return TagStart + content + TagEnd
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
- list_boards_kandev: List boards in a workspace. Required params: workspace_id.
- list_tasks_kandev: List tasks on a board. Required params: board_id.
- create_task_kandev: Create a new task. Required params: workspace_id, board_id, workflow_step_id, title.
- update_task_kandev: Update a task. Required params: task_id.

IMPORTANT: You MUST use these MCP tools when instructed to create plans, ask questions, or interact with the Kandev platform. Do not skip them.`

// FormatKandevContext returns the Kandev context prompt with task and session IDs injected.
func FormatKandevContext(taskID, sessionID string) string {
	return fmt.Sprintf(KandevContext, taskID, sessionID)
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

// InterpolatePlaceholders replaces placeholders in prompt templates with actual values.
// Supported placeholders:
//   - {task_id} - the task ID
func InterpolatePlaceholders(template string, taskID string) string {
	result := template
	result = strings.ReplaceAll(result, "{task_id}", taskID)
	return result
}
