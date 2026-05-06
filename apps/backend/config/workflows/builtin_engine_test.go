package workflows

import (
	"testing"

	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestOfficeDefault_TriggersCompileThroughEngine verifies that every step in
// the embedded office-default template can be compiled into engine.StepSpec
// without panic and that the new event-driven triggers surface as compiled
// engine.Action lists. This pins the template's JSON shape against the
// engine's compile path so future schema drift is caught at build time.
func TestOfficeDefault_TriggersCompileThroughEngine(t *testing.T) {
	tmpl := loadTemplateForTest(t, "office-default")

	for _, def := range tmpl.Steps {
		step := stepFromDefinition(def)
		spec := engine.CompileStep(step)

		// Every compiled spec must carry the seven new trigger keys, even
		// if the slice is empty — Engine.HandleTrigger reads them by key.
		for _, trig := range []engine.Trigger{
			engine.TriggerOnComment,
			engine.TriggerOnBlockerResolved,
			engine.TriggerOnChildrenCompleted,
			engine.TriggerOnApprovalResolved,
			engine.TriggerOnHeartbeat,
			engine.TriggerOnBudgetAlert,
			engine.TriggerOnAgentError,
		} {
			if _, ok := spec.Events[trig]; !ok {
				t.Errorf("step %q: compiled spec missing trigger %q", def.Name, trig)
			}
		}
	}

	work := findStep(tmpl.Steps, "Work")
	if work == nil {
		t.Fatal("Work step not present in template")
	}
	workSpec := engine.CompileStep(stepFromDefinition(*work))
	commentActions := workSpec.Events[engine.TriggerOnComment]
	if len(commentActions) != 1 {
		t.Fatalf("Work.on_comment compiled to %d actions, want 1", len(commentActions))
	}
	if commentActions[0].Kind != engine.ActionQueueRun {
		t.Errorf("Work.on_comment[0].Kind = %q, want queue_run", commentActions[0].Kind)
	}
	if commentActions[0].QueueRun == nil {
		t.Fatal("Work.on_comment[0].QueueRun is nil")
	}
	if commentActions[0].QueueRun.Target != engine.TargetPrimary {
		t.Errorf("Work.on_comment[0] target = %q, want primary", commentActions[0].QueueRun.Target)
	}

	review := findStep(tmpl.Steps, "Review")
	if review == nil {
		t.Fatal("Review step not present in template")
	}
	reviewSpec := engine.CompileStep(stepFromDefinition(*review))
	enterActions := reviewSpec.Events[engine.TriggerOnEnter]
	if len(enterActions) != 2 {
		t.Fatalf("Review.on_enter compiled to %d actions, want 2", len(enterActions))
	}
	if enterActions[0].Kind != engine.ActionClearDecisions {
		t.Errorf("Review.on_enter[0].Kind = %q, want clear_decisions", enterActions[0].Kind)
	}
	if enterActions[1].Kind != engine.ActionQueueRunForEachParticipant {
		t.Errorf("Review.on_enter[1].Kind = %q, want queue_run_for_each_participant", enterActions[1].Kind)
	}
	if enterActions[1].QueueRunForEachParticipant == nil {
		t.Fatal("Review.on_enter[1].QueueRunForEachParticipant is nil")
	}
	if enterActions[1].QueueRunForEachParticipant.Role != "reviewer" {
		t.Errorf("Review.on_enter[1] role = %q, want reviewer", enterActions[1].QueueRunForEachParticipant.Role)
	}

	completeActions := reviewSpec.Events[engine.TriggerOnTurnComplete]
	if len(completeActions) != 2 {
		t.Fatalf("Review.on_turn_complete compiled to %d actions, want 2", len(completeActions))
	}
	for i, action := range completeActions {
		if action.Guard == nil || action.Guard.WaitForQuorum == nil {
			t.Errorf("Review.on_turn_complete[%d] missing wait_for_quorum guard", i)
		}
	}
}

func loadTemplateForTest(t *testing.T, id string) *wfmodels.WorkflowTemplate {
	t.Helper()
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	for _, tmpl := range templates {
		if tmpl.ID == id {
			return tmpl
		}
	}
	t.Fatalf("template %q not found", id)
	return nil
}

func findStep(steps []wfmodels.StepDefinition, name string) *wfmodels.StepDefinition {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}
	}
	return nil
}

// stepFromDefinition copies the StepDefinition fields used by engine.CompileStep
// onto a WorkflowStep. The compiler only reads Name, Position, Prompt, Events.
func stepFromDefinition(def wfmodels.StepDefinition) *wfmodels.WorkflowStep {
	return &wfmodels.WorkflowStep{
		ID:        def.ID,
		Name:      def.Name,
		Position:  def.Position,
		Prompt:    def.Prompt,
		Events:    def.Events,
		StageType: def.StageType,
	}
}
