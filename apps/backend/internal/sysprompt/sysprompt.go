// Package sysprompt provides centralized system prompts and utilities for
// injecting system-level instructions into agent conversations.
//
// All system prompts are wrapped in <kandev-system> tags to mark them as
// system-injected content that can be stripped when displaying to users.
//
// Prompt templates are stored as markdown files in config/prompts/ and loaded
// via the prompts package (go:embed). Placeholders use {key} syntax and are
// resolved by [Resolve].
package sysprompt

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kandev/kandev/config/prompts"
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

// placeholderRegex matches {key} placeholders in prompt templates.
var placeholderRegex = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}`)

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

// PlanMode returns the system prompt prepended when plan mode is enabled.
// It instructs agents to collaborate on the plan without implementing changes.
func PlanMode() string { return prompts.Get("plan-mode") }

// KandevContext returns the system prompt template that provides Kandev-specific
// instructions and session context to agents. Contains {task_id} and {session_id}
// placeholders — use [FormatKandevContext] to inject values.
func KandevContext() string { return prompts.Get("kandev-context") }

// FormatKandevContext returns the Kandev context prompt with task and session IDs injected.
func FormatKandevContext(taskID, sessionID string) string {
	return Resolve("kandev-context", map[string]string{
		"task_id":    taskID,
		"session_id": sessionID,
	})
}

// ConfigContext returns the system prompt for config-mode MCP sessions.
// Contains a {session_id} placeholder — use [FormatConfigContext] to inject values.
func ConfigContext() string { return prompts.Get("config-context") }

// FormatConfigContext returns the config context prompt with the session ID injected.
func FormatConfigContext(sessionID string) string {
	return Resolve("config-context", map[string]string{
		"session_id": sessionID,
	})
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

// DefaultPlanPrefix returns the planning instruction prompt used when plan mode
// is requested but no workflow step provides its own prompt prefix.
func DefaultPlanPrefix() string { return prompts.Get("default-plan-prefix") }

// InjectPlanMode prepends the plan mode system prompt to a user's prompt.
// The system content is wrapped in <kandev-system> tags.
func InjectPlanMode(prompt string) string {
	return Wrap(PlanMode()) + "\n\n" + prompt
}

// SessionHandoverContext returns the template injected when a new session starts
// for a task that already has previous sessions. Contains {session_count} and
// {plan_section} placeholders — use [FormatSessionHandover] to inject values.
func SessionHandoverContext() string { return prompts.Get("session-handover") }

// FormatSessionHandover formats the session handover context.
// planSection should be pre-formatted (empty string if no plan exists).
func FormatSessionHandover(sessionCount int, planSection string) string {
	return Resolve("session-handover", map[string]string{
		"session_count": strconv.Itoa(sessionCount),
		"plan_section":  planSection,
	})
}

// InjectSessionHandover prepends session handover context to a prompt, wrapped in system tags.
func InjectSessionHandover(sessionCount int, planSection, prompt string) string {
	return Wrap(FormatSessionHandover(sessionCount, planSection)) + "\n\n" + prompt
}

// Resolve loads a prompt template by name and replaces all {key} placeholders
// with the corresponding values from vars. Every placeholder in the template
// should have a corresponding entry in vars; unreplaced placeholders are left
// as-is and passed through to the caller.
//
// Replacement is single-pass: values that themselves contain placeholder-like
// text (e.g. a plan section containing "{session_count}") are never re-processed.
func Resolve(name string, vars map[string]string) string {
	template := prompts.Get(name)
	return placeholderRegex.ReplaceAllStringFunc(template, func(placeholder string) string {
		key := placeholder[1 : len(placeholder)-1]
		if value, ok := vars[key]; ok {
			return value
		}
		return placeholder
	})
}

// InterpolatePlaceholders replaces placeholders in prompt templates with actual values.
// Supported placeholders:
//   - {task_id} - the task ID
func InterpolatePlaceholders(template string, taskID string) string {
	result := template
	result = strings.ReplaceAll(result, "{task_id}", taskID)
	return result
}
