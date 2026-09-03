package move

import (
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestOverlayStep_DoesNotMutateOriginal(t *testing.T) {
	step := &models.WorkflowStep{
		ID:             "step-qa",
		Prompt:         "review the diff",
		AgentProfileID: "profile-impl",
		Events: models.StepEvents{
			OnEnter: []models.OnEnterAction{{Type: models.OnEnterAutoStartAgent}},
		},
	}

	got := OverlayStep(step, &EntryOptions{
		ResetContext: true,
		Instructions: "reproduce the checkout failure",
	})
	if got == nil || got == step {
		t.Fatalf("OverlayStep() returned %v, want a distinct copy", got)
	}
	if step.Prompt != "review the diff" || step.AgentProfileID != "profile-impl" {
		t.Fatalf("OverlayStep() mutated original step: %+v", *step)
	}
	if step.HasOnEnterAction(models.OnEnterResetAgentContext) {
		t.Fatal("OverlayStep() added reset to the original step")
	}
	if len(step.Events.OnEnter) != 1 {
		t.Fatalf("original OnEnter len = %d, want 1", len(step.Events.OnEnter))
	}
}

func TestOverlayStep_AppendsInstructionsAndORsReset(t *testing.T) {
	step := &models.WorkflowStep{
		Prompt:         "review the diff",
		AgentProfileID: "profile-impl",
		Events: models.StepEvents{
			OnEnter: []models.OnEnterAction{{Type: models.OnEnterAutoStartAgent}},
		},
	}

	got := OverlayStep(step, &EntryOptions{
		ResetContext: true,
		Instructions: "reproduce the checkout failure",
	})
	wantPrompt := "review the diff\n\n" + InstructionsHeading + "\n\nreproduce the checkout failure\n\n" + InstructionsEnd
	if got.Prompt != wantPrompt {
		t.Fatalf("prompt = %q, want %q", got.Prompt, wantPrompt)
	}
	if got.AgentProfileID != "profile-impl" {
		t.Fatalf("agent profile = %q, want the durable profile-impl unchanged", got.AgentProfileID)
	}
	if !got.HasOnEnterAction(models.OnEnterResetAgentContext) {
		t.Fatal("reset_context overlay did not add reset_agent_context")
	}
	if !got.HasOnEnterAction(models.OnEnterAutoStartAgent) {
		t.Fatal("overlay dropped auto_start_agent")
	}
}

func TestOverlayStep_SkipStepPromptWithInstructionsReplacesPrompt(t *testing.T) {
	step := &models.WorkflowStep{
		Prompt: "review the diff",
		Events: models.StepEvents{
			OnEnter: []models.OnEnterAction{{Type: models.OnEnterAutoStartAgent}},
		},
	}

	got := OverlayStep(step, &EntryOptions{
		SkipStepPrompt: true,
		Instructions:   "reproduce the checkout failure",
	})
	wantPrompt := InstructionsHeading + "\n\nreproduce the checkout failure\n\n" + InstructionsEnd
	if got.Prompt != wantPrompt {
		t.Fatalf("prompt = %q, want only the instructions %q", got.Prompt, wantPrompt)
	}
	if !got.HasOnEnterAction(models.OnEnterAutoStartAgent) {
		t.Fatal("skip with instructions must keep auto_start_agent so the turn runs")
	}
	if step.Prompt != "review the diff" {
		t.Fatalf("OverlayStep() mutated original prompt: %q", step.Prompt)
	}
}

func TestOverlayStep_SkipStepPromptWithoutInstructionsDropsAutoStart(t *testing.T) {
	step := &models.WorkflowStep{
		Prompt: "review the diff",
		Events: models.StepEvents{
			OnEnter: []models.OnEnterAction{
				{Type: models.OnEnterEnablePlanMode},
				{Type: models.OnEnterAutoStartAgent},
			},
		},
	}

	got := OverlayStep(step, &EntryOptions{SkipStepPrompt: true, ResetContext: true})
	if got.HasOnEnterAction(models.OnEnterAutoStartAgent) {
		t.Fatal("skip without instructions must drop auto_start_agent so no turn starts")
	}
	if !got.HasOnEnterAction(models.OnEnterEnablePlanMode) {
		t.Fatal("skip must not drop other on_enter actions")
	}
	if !got.HasOnEnterAction(models.OnEnterResetAgentContext) {
		t.Fatal("reset_context stays independent of skip_step_prompt")
	}
	if len(step.Events.OnEnter) != 2 || !step.HasOnEnterAction(models.OnEnterAutoStartAgent) {
		t.Fatalf("OverlayStep() mutated original OnEnter: %+v", step.Events.OnEnter)
	}
}

func TestOverlayStep_ResetNeverDisablesExistingReset(t *testing.T) {
	step := &models.WorkflowStep{
		Events: models.StepEvents{
			OnEnter: []models.OnEnterAction{{Type: models.OnEnterResetAgentContext}},
		},
	}

	got := OverlayStep(step, &EntryOptions{Instructions: "handoff"})
	if !got.HasOnEnterAction(models.OnEnterResetAgentContext) {
		t.Fatal("overlay disabled the step's existing reset_agent_context")
	}
}

func TestOverlayStep_EmptyPromptKeepsTaskPlaceholder(t *testing.T) {
	step := &models.WorkflowStep{AgentProfileID: "profile-impl"}

	got := OverlayStep(step, &EntryOptions{Instructions: "handoff notes"})
	wantPrompt := "{{task_prompt}}\n\n" + InstructionsHeading + "\n\nhandoff notes\n\n" + InstructionsEnd
	if got.Prompt != wantPrompt {
		t.Fatalf("empty-prompt overlay = %q, want %q", got.Prompt, wantPrompt)
	}
}

func TestStepCarriesMoveInstructions(t *testing.T) {
	if StepCarriesMoveInstructions(nil) {
		t.Fatal("nil step should not carry move instructions")
	}
	plain := &models.WorkflowStep{Prompt: "review the diff"}
	if StepCarriesMoveInstructions(plain) {
		t.Fatal("plain prompt should not carry move instructions")
	}
	overlaid := OverlayStep(plain, &EntryOptions{Instructions: "handoff notes"})
	if !StepCarriesMoveInstructions(overlaid) {
		t.Fatal("overlaid step should carry move instructions")
	}
}

func TestExtractInstructions_RoundTripsWrappedBlock(t *testing.T) {
	if got := ExtractInstructions("review the diff"); got != "" {
		t.Fatalf("plain prompt should carry no instructions, got %q", got)
	}
	if got := WrapInstructions("   "); got != "" {
		t.Fatalf("whitespace-only instructions should wrap empty, got %q", got)
	}
	overlaid := OverlayStep(&models.WorkflowStep{Prompt: "review the diff"}, &EntryOptions{Instructions: "reproduce it"})
	block := ExtractInstructions(overlaid.Prompt)
	want := WrapInstructions("reproduce it")
	if block != want {
		t.Fatalf("extracted block = %q, want %q", block, want)
	}
	// The extracted block is self-contained: re-extracting it yields itself.
	if again := ExtractInstructions(block); again != block {
		t.Fatalf("re-extracted block = %q, want %q", again, block)
	}
}

func TestOverlayStep_EmptyOptionsCopiesWithoutChangingKnobs(t *testing.T) {
	step := &models.WorkflowStep{
		Prompt:         "review the diff",
		AgentProfileID: "profile-impl",
	}

	got := OverlayStep(step, nil)
	if got == nil || got == step {
		t.Fatalf("OverlayStep(nil opts) returned %v, want a distinct copy", got)
	}
	if got.Prompt != step.Prompt || got.AgentProfileID != step.AgentProfileID {
		t.Fatalf("empty overlay changed knobs: %+v", *got)
	}
	if OverlayStep(nil, &EntryOptions{ResetContext: true}) != nil {
		t.Fatal("OverlayStep(nil step) should return nil")
	}
}
