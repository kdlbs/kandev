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

// kandevContextMarker is a stable string from kandev-context.md that lets
// callers detect a prompt that has already been wrapped with the Kandev MCP
// system block. Used by [HasKandevContext] to make the wrap step idempotent
// across the multiple call sites that record first-turn prompts (WS handler,
// workflow auto-start, orchestrator).
const kandevContextMarker = "KANDEV MCP TOOLS"

// HasKandevContext reports whether the prompt already contains the Kandev MCP
// system block produced by [InjectKandevContext]. Use this to gate a wrap at
// any call site so the same prompt never gets double-wrapped on its way down
// to the agent or the DB.
//
// The marker is matched only inside a <kandev-system>...</kandev-system>
// block — a user message body that happens to mention "KANDEV MCP TOOLS"
// would not falsely signal that the wrap is already applied.
func HasKandevContext(text string) bool {
	for _, block := range systemTagRegex.FindAllString(text, -1) {
		if strings.Contains(block, kandevContextMarker) {
			return true
		}
	}
	return false
}

// PlanMode returns the system prompt prepended when plan mode is enabled.
// It instructs agents to collaborate on the plan without implementing changes.
func PlanMode() string { return prompts.Get("plan-mode") }

// KandevContext returns the system prompt template that provides Kandev-specific
// instructions and session context to agents. Contains {task_id}, {session_id},
// and {step_complete_section} placeholders — use [FormatKandevContext] to inject values.
func KandevContext() string { return prompts.Get("kandev-context") }

// stepCompleteSection is the description + instruction block for the
// step_complete_kandev MCP tool. Only injected when the current workflow step
// has `auto_advance_requires_signal = true` (ADR 0015). Agents on legacy
// auto-advance steps never see the tool so they cannot fire false transitions.
//
// MUST end with "\n": the template inlines {step_complete_section}
// immediately before the next bullet (`- create_task_plan_kandev:`), so the
// trailing newline is what separates the two list items in the enabled case
// without forcing the template to add its own. Dropping the "\n" silently
// merges the two bullets onto one line; the omit path (empty string) is
// unaffected since the next line in the template already starts the bullet.
const stepCompleteSection = "- step_complete_kandev: Signal that every user-stated requirement for the CURRENT workflow step is satisfied. " +
	"Call this as the LAST action of the step (after the final tool call / commit / answer). " +
	"Idempotent — a second call within the same step is a no-op. " +
	"Do NOT call when asking a question, mid-conversation, or on partial progress. " +
	"Required params: summary (one-paragraph plain text). Optional: handoff, blockers.\n"

// FormatKandevContext returns the Kandev context prompt with task and session IDs injected.
// When requiresCompletionSignal is true, the step_complete_kandev tool description is
// included; otherwise the placeholder is collapsed to an empty string.
func FormatKandevContext(taskID, sessionID string, requiresCompletionSignal bool) string {
	section := ""
	if requiresCompletionSignal {
		section = stepCompleteSection
	}
	return Resolve("kandev-context", map[string]string{
		"task_id":               taskID,
		"session_id":            sessionID,
		"step_complete_section": section,
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
// The system content is wrapped in <kandev-system> tags. Pass requiresCompletionSignal=true
// when the current workflow step has `auto_advance_requires_signal` enabled (ADR 0015) so the
// step_complete_kandev tool description is exposed; otherwise the tool is hidden from the agent.
func InjectKandevContext(taskID, sessionID, prompt string, requiresCompletionSignal bool) string {
	return Wrap(FormatKandevContext(taskID, sessionID, requiresCompletionSignal)) + "\n\n" + prompt
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
