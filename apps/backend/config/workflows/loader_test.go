package workflows

import (
	"fmt"
	"testing"
)

func TestLoadTemplates_AllValid(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}
	if len(templates) == 0 {
		t.Fatal("LoadTemplates() returned no templates")
	}
	for _, tmpl := range templates {
		if tmpl.ID == "" {
			t.Error("template has empty ID")
		}
		if tmpl.Name == "" {
			t.Errorf("template %q has empty name", tmpl.ID)
		}
		if len(tmpl.Steps) == 0 {
			t.Errorf("template %q has no steps", tmpl.ID)
		}
	}
}

func TestLoadTemplates_EachHasStartStep(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	for _, tmpl := range templates {
		startCount := 0
		for _, step := range tmpl.Steps {
			if step.IsStartStep {
				startCount++
			}
		}
		if startCount == 0 {
			t.Errorf("template %q has no step with is_start_step: true", tmpl.ID)
		}
		if startCount > 1 {
			t.Errorf("template %q has %d start steps (expected 1)", tmpl.ID, startCount)
		}
	}
}

func TestLoadTemplates_MoveToStepReferencesExist(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	for _, tmpl := range templates {
		stepIDs := make(map[string]bool)
		for _, step := range tmpl.Steps {
			stepIDs[step.ID] = true
		}

		for _, step := range tmpl.Steps {
			// Collect all move_to_step configs from all event types
			var configs []map[string]interface{}
			for _, a := range step.Events.OnTurnStart {
				if a.Config != nil {
					configs = append(configs, a.Config)
				}
			}
			for _, a := range step.Events.OnTurnComplete {
				if a.Config != nil {
					configs = append(configs, a.Config)
				}
			}

			for _, cfg := range configs {
				stepID, ok := cfg["step_id"]
				if !ok {
					continue
				}
				ref := fmt.Sprintf("%v", stepID)
				if !stepIDs[ref] {
					t.Errorf("template %q, step %q: move_to_step references %q which does not exist",
						tmpl.ID, step.ID, ref)
				}
			}
		}
	}
}

func TestLoadTemplates_StepPositionsUnique(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	for _, tmpl := range templates {
		positions := make(map[int]string)
		for _, step := range tmpl.Steps {
			if existing, ok := positions[step.Position]; ok {
				t.Errorf("template %q: steps %q and %q share position %d",
					tmpl.ID, existing, step.ID, step.Position)
			}
			positions[step.Position] = step.ID
		}
	}
}

func TestLoadTemplates_ExpectedTemplateIDs(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	expected := map[string]bool{
		"simple":       false,
		"standard":     false,
		"architecture": false,
		"pr-review":    false,
		"feature-dev":  false,
	}

	for _, tmpl := range templates {
		if _, ok := expected[tmpl.ID]; ok {
			expected[tmpl.ID] = true
		}
	}

	for id, found := range expected {
		if !found {
			t.Errorf("expected template %q not found", id)
		}
	}
}
