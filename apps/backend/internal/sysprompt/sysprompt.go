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

// These markers distinguish task and Office context from other system blocks.
const kandevContextMarker = "KANDEV MCP TOOLS"

const officeContextMarker = "KANDEV OFFICE MCP TOOLS"

type contextKind uint8

const (
	contextUnknown contextKind = iota
	contextTask
	contextOffice
	contextTrusted
)

func contextKindForBlock(block string, trustedContents []string) contextKind {
	if strings.Contains(block, officeContextMarker) {
		return contextOffice
	}
	if strings.Contains(block, kandevContextMarker) {
		return contextTask
	}
	trimmedBlock := strings.TrimSpace(block)
	for _, content := range trustedContents {
		if trimmedBlock == strings.TrimSpace(Wrap(content)) {
			return contextTrusted
		}
	}
	return contextUnknown
}

// OfficeContext returns the restricted first-turn prompt used by Office runs.
// Its tool inventory must stay exactly aligned with MCP ModeOffice.
func OfficeContext() string { return prompts.Get("office-context") }

// FormatOfficeContext injects the active task and session IDs into the Office context.
func FormatOfficeContext(taskID, sessionID string) string {
	return Resolve("office-context", map[string]string{
		"task_id":    taskID,
		"session_id": sessionID,
	})
}

// InjectOfficeContext ensures a first-turn prompt has the restricted Office context.
func InjectOfficeContext(taskID, sessionID, prompt string) string {
	return canonicalizeKandevContext(
		FormatOfficeContext(taskID, sessionID),
		prompt,
		trustedContextContents(sessionID),
	)
}

// PlanMode returns the system prompt prepended when plan mode is enabled.
// It instructs agents to collaborate on the plan without implementing changes.
func PlanMode() string { return prompts.Get("plan-mode") }

// KandevContext returns the task-mode system prompt template that provides
// Kandev-specific instructions and session context to agents. Contains
// {task_id}, {session_id}, and {step_complete_section} placeholders — use
// [FormatKandevContext] to inject values.
func KandevContext() string {
	return Resolve("kandev-context", map[string]string{
		"coordinator_task_control_section": coordinatorTaskControlSection,
	})
}

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
	"This is the canonical MCP protocol name; a client registry may display the client-specific alias mcp__kandev__step_complete_kandev for the same tool. " +
	"If the tool is not already visible, use the client's tool search/discovery with the canonical name. " +
	"Call this as the LAST action of the step (after the final tool call / commit / answer). " +
	"Idempotent — a second call within the same step is a no-op. " +
	"Do NOT call when asking a question, mid-conversation, or on partial progress. " +
	"Required params: summary (one-paragraph plain text). Optional: handoff, blockers.\n"

// coordinatorTaskControlSection documents task-mode-only parent/child controls.
// Restricted MCP modes omit the section because neither message_task_kandev nor
// stop_task_kandev is registered there. The baseline message-tool sentence stays
// in the template to avoid broadening this feature into a cleanup of older mode
// mismatches.
const coordinatorTaskControlSection = " Optional: session_id, delivery_mode. " +
	"Use delivery_mode=\"queued\" or omit it for information that can wait. " +
	"Use delivery_mode=\"interrupt\" for urgent replacement work on a running direct child; " +
	"if immediate cancel-and-dispatch cannot be confirmed safely, the message remains queued. " +
	"For halt-only work, use stop_task_kandev.\n" +
	"- stop_task_kandev: Halt all live sessions observed for a direct child, with no prompt and no replacement turn. " +
	"Only the target task's direct parent may call it. Required params: task_id."

// KandevContextOptions controls capability-dependent sections in the first-turn
// Kandev context.
type KandevContextOptions struct {
	RequiresCompletionSignal       bool
	IncludeCoordinatorTaskControls bool
}

// FormatKandevContext returns the Kandev context prompt with task and session IDs injected.
// When requiresCompletionSignal is true, the step_complete_kandev tool description is
// included; otherwise the placeholder is collapsed to an empty string.
func FormatKandevContext(taskID, sessionID string, requiresCompletionSignal bool) string {
	return FormatKandevContextWithOptions(taskID, sessionID, KandevContextOptions{
		RequiresCompletionSignal:       requiresCompletionSignal,
		IncludeCoordinatorTaskControls: true,
	})
}

// FormatKandevContextWithOptions returns capability-aware Kandev context.
func FormatKandevContextWithOptions(taskID, sessionID string, options KandevContextOptions) string {
	section := ""
	if options.RequiresCompletionSignal {
		section = stepCompleteSection
	}
	coordinatorControls := ""
	if options.IncludeCoordinatorTaskControls {
		coordinatorControls = coordinatorTaskControlSection
	}
	return Resolve("kandev-context", map[string]string{
		"task_id":                          taskID,
		"session_id":                       sessionID,
		"step_complete_section":            section,
		"coordinator_task_control_section": coordinatorControls,
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
	return InjectKandevContextWithOptions(taskID, sessionID, prompt, KandevContextOptions{
		RequiresCompletionSignal:       requiresCompletionSignal,
		IncludeCoordinatorTaskControls: true,
	})
}

// InjectKandevContextWithOptions prepends capability-aware Kandev context.
func InjectKandevContextWithOptions(taskID, sessionID, prompt string, options KandevContextOptions) string {
	return canonicalizeKandevContext(
		FormatKandevContextWithOptions(taskID, sessionID, options),
		prompt,
		trustedContextContents(sessionID),
	)
}

func trustedContextContents(sessionID string) []string {
	return []string{FormatConfigContext(sessionID), PlanMode(), DefaultPlanPrefix()}
}

func canonicalizeKandevContext(content, prompt string, trustedContents []string) string {
	replaced := false
	result := systemTagRegex.ReplaceAllStringFunc(prompt, func(block string) string {
		end := strings.Index(block, TagEnd) + len(TagEnd)
		suffix := block[end:]
		switch contextKindForBlock(block, trustedContents) {
		case contextUnknown:
			return suffix
		case contextTrusted:
			return block
		}
		if !replaced {
			replaced = true
			return Wrap(content) + suffix
		}
		return suffix
	})
	if replaced {
		return result
	}
	return Wrap(content) + "\n\n" + result
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
