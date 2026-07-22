package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/sysprompt"
)

// AppendReferenceExpansions resolves any "@name" saved-prompt references in
// prompt and, when at least one resolves, appends a hidden
// <kandev-system>-wrapped block containing the expanded content while leaving
// the original @mentions in place in the visible prompt body.
//
// It returns prompt unchanged when: prompt contains no "@", when resolution
// fails (the failure is logged via log, when non-nil, and treated as
// non-fatal), or when no references resolve to a known prompt.
func (s *Service) AppendReferenceExpansions(ctx context.Context, prompt string, log *zap.Logger) string {
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
	b.WriteString("EXPANDED PROMPT REFERENCES: The message above references saved prompts by @name. ")
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
