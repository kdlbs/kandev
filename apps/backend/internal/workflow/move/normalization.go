package move

import (
	"strings"

	"github.com/kandev/kandev/internal/workflow/models"
)

// NormalizeEntryOptions trims all string fields, folds the legacy top-level
// prompt into instructions, and returns nil for an empty options object.
//
// The legacy prompt is accepted only when the nested instructions field is
// absent. Supplying both non-blank values is rejected, even when they match,
// so callers use one unambiguous request shape.
func NormalizeEntryOptions(options *EntryOptions, legacyPrompt string) (*EntryOptions, error) {
	legacyPrompt = strings.TrimSpace(legacyPrompt)
	normalized := normalizedCopy(options)
	if normalized == nil {
		normalized = &EntryOptions{}
	}

	if normalized.Instructions != "" && legacyPrompt != "" {
		return nil, &ConflictingInstructionsError{}
	}
	if normalized.Instructions == "" {
		normalized.Instructions = legacyPrompt
	}
	if *normalized == (EntryOptions{}) {
		return nil, nil
	}
	return normalized, nil
}

// ValidateEntryOptions rejects agent-facing options for a move that does not
// actually change workflow step. Empty options are always a no-op and remain
// valid for position-only or no-step moves.
func ValidateEntryOptions(options *EntryOptions, change MoveChange) error {
	if normalizedCopy(options) == nil {
		return nil
	}
	if change != MoveChangeStep {
		return &EntryOptionsNotAllowedError{Change: change}
	}
	return nil
}

// ValidateEntryTarget rejects agent-facing entry options when the target step is
// a manual step: it does not start work on entry, so a one-shot hand-off has no
// recipient and accepting the options would silently drop them. It reuses
// models.StepRunsOnEntry, the single repository answer for whether entering a
// step starts work (auto_start_agent, queue_run, queue_run_for_each_participant,
// run_code_review, or a pull source), instead of re-deriving a narrower set. A
// nil step is treated as unknown and does not reject, so the move path reports
// its own canonical not-found error instead.
func ValidateEntryTarget(step *models.WorkflowStep) error {
	if step == nil || models.StepRunsOnEntry(step) {
		return nil
	}
	return ErrEntryTargetIsManual
}
