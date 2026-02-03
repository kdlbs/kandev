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
// It instructs agents to analyze and plan without using writing/destructive tools.
const PlanMode = `PLAN MODE ACTIVE - READ-ONLY RESTRICTIONS:
You are in plan mode. You MUST NOT use any writing, modifying, or destructive tools.
This includes but is not limited to: file writes, file deletes, git commits, shell commands that modify state.
You CAN use read-only tools (file reads, searches, code analysis) and the Kandev plan MCP tools (plan_get, plan_update, plan_item_update, etc.) if needed to create or update the task plan.
Focus on analyzing the request and creating a detailed plan. This restriction applies to THIS PROMPT ONLY.`

// KandevContext is the system prompt that provides Kandev-specific instructions
// and session context to agents. Use FormatKandevContext to inject task/session IDs.
const KandevContext = `IMPORTANT KANDEV INSTRUCTIONS:
- When you have questions for the user, use the ask_user_question_kandev MCP tool to ask them directly.
- When you need to create or update a plan for a task, use the Kandev MCP plan tools (plan_get, plan_update, etc.).
- Kandev Task ID: %s
- Kandev Session ID: %s
- Always use these IDs when calling Kandev MCP tools that require task_id or session_id parameters.`

// FormatKandevContext returns the Kandev context prompt with task and session IDs injected.
func FormatKandevContext(taskID, sessionID string) string {
	return fmt.Sprintf(KandevContext, taskID, sessionID)
}

// InjectKandevContext prepends the Kandev system prompt and session context to a user's prompt.
// The system content is wrapped in <kandev-system> tags.
func InjectKandevContext(taskID, sessionID, prompt string) string {
	return Wrap(FormatKandevContext(taskID, sessionID)) + "\n\n" + prompt
}

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

