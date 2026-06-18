package workflows

import "testing"

// TestKanbanInProgress_GatesOnCompletionSignal pins that the default ("simple")
// Kanban workflow advances In Progress -> Review only on an explicit completion
// signal, not on the agent's first bare turn-end.
//
// The template's own description states the agent "runs automatically, then
// move to Review when done". Without auto_advance_requires_signal the
// on_turn_complete move fires on the first turn-end (e.g. an auto-started
// agent that has barely read its prompt), promoting the task to Review before
// any real work — which contradicts "when done". Gating on the
// step_complete_kandev signal makes the behaviour match the description.
func TestKanbanInProgress_GatesOnCompletionSignal(t *testing.T) {
	tmpl := loadTemplateForTest(t, "simple")
	step := findStep(tmpl.Steps, "In Progress")
	if step == nil {
		t.Fatal("In Progress step not present in kanban template")
	}
	if !step.AutoAdvanceRequiresSignal {
		t.Error("In Progress must set auto_advance_requires_signal=true so it advances on the completion signal, not on the first bare turn-end")
	}
}
