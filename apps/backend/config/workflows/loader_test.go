package workflows

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
	"gopkg.in/yaml.v3"
)

func boolFieldForTest(t *testing.T, value any, name string) bool {
	t.Helper()
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	field := v.FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("%T is missing %s", value, name)
	}
	if field.Kind() != reflect.Bool {
		t.Fatalf("%T.%s has kind %s, want bool", value, name, field.Kind())
	}
	return field.Bool()
}

func TestLoadTemplates_CancelTriggersTurnCompleteDefaults(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}
	var simple *models.WorkflowTemplate
	for _, template := range templates {
		if template.ID == "simple" {
			simple = template
			break
		}
	}
	if simple == nil {
		t.Fatal("simple template not found")
	}
	want := map[string]bool{"Backlog": true, "In Progress": true, "Review": false, "Done": false}
	seen := make(map[string]bool, len(want))
	for _, step := range simple.Steps {
		wantValue, ok := want[step.Name]
		if !ok {
			continue
		}
		seen[step.Name] = true
		if got := boolFieldForTest(t, step, "CancelTriggersTurnComplete"); got != wantValue {
			t.Errorf("simple template step %q cancel trigger = %t, want %t", step.Name, got, wantValue)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("simple template step %q not found", name)
		}
	}
}

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

// TestConvertEvents_SetSessionMode round-trips a set_session_mode on_enter
// action through the YAML loader: the action type passes the allow-list and its
// "mode" config survives into the typed StepEvents. See issue #1183.
func TestConvertEvents_SetSessionMode(t *testing.T) {
	const yamlDoc = `
on_enter:
  - type: set_session_mode
    config:
      mode: acceptEdits
`
	var e stepEventsYAML
	if err := yaml.Unmarshal([]byte(yamlDoc), &e); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	events, err := convertEvents(e)
	if err != nil {
		t.Fatalf("convertEvents returned error: %v", err)
	}
	if len(events.OnEnter) != 1 {
		t.Fatalf("expected 1 on_enter action, got %d", len(events.OnEnter))
	}
	if events.OnEnter[0].Type != models.OnEnterSetSessionMode {
		t.Fatalf("unexpected action type %q", events.OnEnter[0].Type)
	}
	if mode, _ := events.OnEnter[0].Config["mode"].(string); mode != "acceptEdits" {
		t.Fatalf("expected mode=acceptEdits, got %q", mode)
	}
}

// TestConvertEvents_SetSessionModeRejectsMissingMode verifies the loader fails
// fast when a set_session_mode action has no usable "mode" config, rather than
// silently dropping it at compile time. See issue #1183.
func TestConvertEvents_SetSessionModeRejectsMissingMode(t *testing.T) {
	cases := map[string]string{
		"no config":  "on_enter:\n  - type: set_session_mode\n",
		"empty mode": "on_enter:\n  - type: set_session_mode\n    config:\n      mode: \"\"\n",
		"non-string": "on_enter:\n  - type: set_session_mode\n    config:\n      mode: 3\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			var e stepEventsYAML
			if err := yaml.Unmarshal([]byte(doc), &e); err != nil {
				t.Fatalf("unmarshal yaml: %v", err)
			}
			if _, err := convertEvents(e); err == nil {
				t.Fatal("expected convertEvents to reject set_session_mode with no usable mode")
			}
		})
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
		"simple":              false,
		"standard":            false,
		"architecture":        false,
		"pr-review":           false,
		"feature-dev":         false,
		"improve-kandev":      false,
		"report-kandev-issue": false,
		"office-default":      false,
		"routine":             false,
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

// TestLoadTemplates_HiddenFlag verifies that the YAML loader propagates
// the `hidden` field into WorkflowTemplate.Hidden. System-only templates
// must be hidden while user-pickable templates remain visible.
func TestLoadTemplates_HiddenFlag(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	hiddenByID := map[string]bool{}
	for _, tmpl := range templates {
		hiddenByID[tmpl.ID] = tmpl.Hidden
	}

	for _, id := range []string{"improve-kandev", "report-kandev-issue", "office-default", "routine"} {
		if !hiddenByID[id] {
			t.Errorf("expected template %q to be hidden", id)
		}
	}
	for _, id := range []string{"simple", "standard", "architecture", "pr-review", "feature-dev"} {
		if hiddenByID[id] {
			t.Errorf("template %q must not be hidden", id)
		}
	}
}

func TestLoadTemplates_ReportKandevIssuePromptContract(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	var report *models.WorkflowTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "report-kandev-issue" {
			report = tmpl
			break
		}
	}
	if report == nil {
		t.Fatal("report-kandev-issue template not found")
	}
	if !report.Hidden {
		t.Error("report-kandev-issue must be hidden")
	}
	if len(report.Steps) != 1 {
		t.Fatalf("report-kandev-issue steps = %d, want 1", len(report.Steps))
	}

	step := report.Steps[0]
	if !step.IsStartStep {
		t.Error("issue step must be the start step")
	}
	if len(step.Events.OnEnter) != 1 ||
		step.Events.OnEnter[0].Type != models.OnEnterAutoStartAgent {
		t.Errorf("issue step on_enter = %+v, want auto_start_agent", step.Events.OnEnter)
	}
	for _, required := range []string{
		"ask_user_question_kandev",
		".github/ISSUE_TEMPLATE",
		"gh issue create",
		"sensitive",
		"duplicate",
		"skip STEP 4",
		"security-advisory",
	} {
		if !strings.Contains(step.Prompt, required) {
			t.Errorf("issue prompt must contain %q", required)
		}
	}
}
