package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/sysprompt"
)

// expansionMarker opens the hidden expansion block written by
// FormatPromptReferenceExpansions. AppendReferenceExpansions checks for its
// presence to stay idempotent: the block's own "### @name" headers are valid
// @mention syntax by the same matching rules used to resolve references, so
// without this guard a second call on an already-expanded prompt would
// re-scan the block and append a duplicate one below it.
const expansionMarker = "EXPANDED PROMPT REFERENCES:"

// expansionBlockPrefix is the exact, literal prefix that
// FormatPromptReferenceExpansions' output has once wrapped by sysprompt.Wrap:
// the opening <kandev-system> tag immediately followed by expansionMarker,
// with nothing in between. AppendReferenceExpansions checks for this exact
// concatenation (not just expansionMarker anywhere inside any
// <kandev-system> block) so that an unrelated system block produced by a
// different code path (e.g. a plan-mode prefix or MCP context wrap) that
// merely happens to mention this phrase somewhere in its content cannot
// false-positive the idempotency guard.
const expansionBlockPrefix = sysprompt.TagStart + expansionMarker

// AppendReferenceExpansions resolves any "@name" saved-prompt references in
// prompt and, when at least one resolves, appends a hidden
// <kandev-system>-wrapped block containing the expanded content while leaving
// the original @mentions in place in the visible prompt body.
//
// It returns prompt unchanged when: prompt already contains a previously
// appended expansion block (detected via expansionBlockPrefix, an exact
// match of "<kandev-system>" immediately followed by expansionMarker, not an
// unscoped substring match against expansionMarker alone), prompt contains
// no "@", when resolution fails (the failure is logged via log, when
// non-nil, and treated as non-fatal), or when no references resolve to a
// known prompt.
//
// AppendReferenceExpansions is deliberately idempotent: calling it a second
// time on a string it already expanded is a safe no-op, so callers do not
// need to track whether expansion already ran.
func (s *Service) AppendReferenceExpansions(ctx context.Context, prompt string, log *zap.Logger) string {
	if strings.Contains(prompt, expansionBlockPrefix) {
		return prompt
	}
	if !strings.Contains(prompt, "@") {
		return prompt
	}
	expansions, err := s.ResolvePromptReferences(ctx, prompt)
	if err != nil {
		if log != nil {
			log.Warn("failed to resolve prompt references", zap.Error(err))
		}
		return prompt
	}
	if len(expansions) == 0 {
		return prompt
	}
	return prompt + "\n\n" + sysprompt.Wrap(FormatPromptReferenceExpansions(expansions))
}

// FormatPromptReferenceExpansions renders resolved prompt-reference
// expansions into the hidden system-context block appended after a prompt.
// Both name and content are sanitized to strip any embedded
// sysprompt.TagEnd so a saved prompt cannot prematurely close the
// surrounding <kandev-system> wrapper.
func FormatPromptReferenceExpansions(expansions []PromptReferenceExpansion) string {
	var b strings.Builder
	b.WriteString(expansionMarker + " The message above references saved prompts by @name. ")
	b.WriteString("Use these expansions as hidden context while preserving the original @mentions.")
	for _, expansion := range expansions {
		b.WriteString("\n\n### @")
		b.WriteString(sanitizePromptExpansionSystemText(expansion.Name))
		b.WriteString("\n")
		b.WriteString(sanitizePromptExpansionSystemText(expansion.Content))
	}
	return b.String()
}

// sanitizePromptExpansionSystemText strips any embedded sysprompt.TagEnd from
// a value before it is written into a <kandev-system>-wrapped block.
func sanitizePromptExpansionSystemText(value string) string {
	return strings.ReplaceAll(value, sysprompt.TagEnd, "")
}
