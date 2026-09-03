package move

import (
	"strings"

	"github.com/kandev/kandev/internal/workflow/models"
)

const (
	InstructionsHeading = "## One-time workflow move instructions"
	InstructionsEnd     = "<!-- /one-time-workflow-move-instructions -->"
)

// OverlayStep returns a copy of step with one-shot move-entry options applied.
// The original step is never mutated. Reset is OR-ed onto the copy and never
// disables an existing reset_agent_context action.
//
// Without skip_step_prompt, instructions are appended after the durable prompt.
// With skip_step_prompt the durable prompt (and its task-description fallback)
// is suppressed for this entry: when instructions are present the copy's prompt
// becomes only those instructions, and when they are absent the copy's
// auto_start_agent action is dropped so no turn starts and the task lands idle.
func OverlayStep(step *models.WorkflowStep, opts *EntryOptions) *models.WorkflowStep {
	if step == nil {
		return nil
	}

	copy := *step
	copy.Events.OnEnter = append([]models.OnEnterAction(nil), step.Events.OnEnter...)
	if opts == nil {
		return &copy
	}

	wrapped := wrapMoveInstructions(opts.Instructions)
	switch {
	case opts.SkipStepPrompt && wrapped != "":
		// Skip the step prompt/task description; send only the one-time
		// instructions. The workflow-level instructions block, if any, is still
		// added downstream by buildWorkflowPrompt.
		copy.Prompt = wrapped
	case opts.SkipStepPrompt:
		// Skip with no instructions: suppress the auto-started turn entirely by
		// dropping auto_start_agent, so the on_enter path treats this like a
		// step without auto-start and the agent waits for manual input.
		copy.Events.OnEnter = withoutOnEnterAction(copy.Events.OnEnter, models.OnEnterAutoStartAgent)
	case wrapped != "":
		if copy.Prompt == "" {
			// Preserve empty-prompt composition: durable steps with no prompt
			// still carry the task description via {{task_prompt}}.
			copy.Prompt = "{{task_prompt}}\n\n" + wrapped
		} else {
			copy.Prompt = copy.Prompt + "\n\n" + wrapped
		}
	}
	if opts.ResetContext && !copy.HasOnEnterAction(models.OnEnterResetAgentContext) {
		copy.Events.OnEnter = append(copy.Events.OnEnter, models.OnEnterAction{
			Type: models.OnEnterResetAgentContext,
		})
	}
	return &copy
}

// withoutOnEnterAction returns actions with every entry of the given type
// removed. The input slice is never mutated.
func withoutOnEnterAction(actions []models.OnEnterAction, drop models.OnEnterActionType) []models.OnEnterAction {
	result := make([]models.OnEnterAction, 0, len(actions))
	for _, action := range actions {
		if action.Type == drop {
			continue
		}
		result = append(result, action)
	}
	return result
}

// WrapInstructions renders one-shot move instructions with the same sentinel
// markers OverlayStep uses, so the auto-start prompt path (which rebuilds the
// prompt from the durable step and cannot see a step overlay) can append the
// identical block. Empty or whitespace-only input yields an empty string.
func WrapInstructions(instructions string) string {
	return wrapMoveInstructions(instructions)
}

func wrapMoveInstructions(instructions string) string {
	instructions = strings.TrimSpace(instructions)
	if instructions == "" {
		return ""
	}
	return InstructionsHeading + "\n\n" + instructions + "\n\n" + InstructionsEnd
}

// StepCarriesMoveInstructions reports whether step.Prompt already contains
// one-shot move instructions (the overlay sentinel). Used by on-enter
// dispatch to decide whether to queue those instructions without
// re-appending them.
func StepCarriesMoveInstructions(step *models.WorkflowStep) bool {
	if step == nil {
		return false
	}
	return strings.Contains(step.Prompt, InstructionsEnd)
}

// ExtractInstructions returns the one-shot instructions block (including its
// sentinel markers) previously appended by OverlayStep/WrapInstructions, or ""
// when the prompt carries none. Used to re-deliver instructions to an existing
// session that will not receive the overlaid step prompt through a launch.
func ExtractInstructions(prompt string) string {
	start := strings.Index(prompt, InstructionsHeading)
	if start < 0 {
		return ""
	}
	end := strings.Index(prompt, InstructionsEnd)
	if end < 0 || end < start {
		return ""
	}
	return prompt[start : end+len(InstructionsEnd)]
}
