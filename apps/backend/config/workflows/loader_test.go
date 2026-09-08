package workflows

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
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

func TestConvertStep_PreservesAgentProfileAndSessionPolicies(t *testing.T) {
	var raw templateYAML
	if err := yaml.Unmarshal([]byte(`
id: test
name: Test
steps:
  - id: review
    name: Review
    agent_profile_id: profile-review
    profile_session_start_policy: new
    profile_session_end_policy: park
`), &raw); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	step, err := convertStep(raw.Steps[0])
	if err != nil {
		t.Fatalf("convertStep returned error: %v", err)
	}
	if step.AgentProfileID != "profile-review" {
		t.Fatalf("agent profile ID = %q, want profile-review", step.AgentProfileID)
	}
	if step.ProfileSessionStartPolicy != taskmodels.WorkflowProfileSessionStartPolicyNew {
		t.Fatalf("profile session start policy = %q, want new", step.ProfileSessionStartPolicy)
	}
	if step.ProfileSessionEndPolicy != taskmodels.WorkflowProfileSessionEndPolicyPark {
		t.Fatalf("profile session end policy = %q, want park", step.ProfileSessionEndPolicy)
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

// TestLoadTemplates_OfficeDefaultWorkStepRequiresSignal verifies that the
// office-default template's `work` step gates its turn-end auto-advance
// (Work -> Review) on the ADR 0015 declarative completion signal, now that
// step_complete_kandev is registered for the Office MCP surface. Without
// this flag the new signal would be decorative: the step would still
// advance on bare turn-end regardless of whether the agent called the tool.
func TestLoadTemplates_OfficeDefaultWorkStepRequiresSignal(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	var officeDefault *models.WorkflowTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "office-default" {
			officeDefault = tmpl
			break
		}
	}
	if officeDefault == nil {
		t.Fatal("office-default template not found")
	}

	var work *models.StepDefinition
	for i := range officeDefault.Steps {
		if officeDefault.Steps[i].ID == "work" {
			work = &officeDefault.Steps[i]
			break
		}
	}
	if work == nil {
		t.Fatal("office-default template step \"work\" not found")
	}

	if got := boolFieldForTest(t, work, "AutoAdvanceRequiresSignal"); !got {
		t.Error("office-default template step \"work\" must set auto_advance_requires_signal: true")
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

func TestLoadTemplates_ImproveKandevManagedPublicationPromptContract(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	var improve *models.WorkflowTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "improve-kandev" {
			improve = tmpl
			break
		}
	}
	if improve == nil {
		t.Fatal("improve-kandev template not found")
	}
	if len(improve.Steps) != 3 {
		t.Fatalf("improve-kandev steps = %d, want 3", len(improve.Steps))
	}

	prStep := improve.Steps[2]
	normalizedPrompt := strings.Join(strings.Fields(prStep.Prompt), " ")
	for _, required := range []string{
		"Managed workspace credentials",
		"origin` remains the canonical `kdlbs/kandev`",
		"ordinary `git push`",
		"gh pr create --repo kdlbs/kandev --base main",
		"<fork-owner>:<branch>",
		"Executor-owned credentials are a separate compatibility path",
		"Never select this path to recover from a managed preparation failure",
		"gh repo fork kdlbs/kandev",
		"executor-owned credentials",
		"--remote-name=origin",
	} {
		if !strings.Contains(normalizedPrompt, required) {
			t.Errorf("managed publication prompt must contain %q", required)
		}
	}
	managedPrompt := normalizedPrompt
	if executorSection := strings.Index(managedPrompt, "Executor-owned credentials"); executorSection >= 0 {
		managedPrompt = managedPrompt[:executorSection]
	}
	for _, forbidden := range []string{
		"gh repo fork",
		"remote-name=origin",
		"rename the existing `origin`",
	} {
		if strings.Contains(managedPrompt, forbidden) {
			t.Errorf("managed publication prompt must not contain %q", forbidden)
		}
	}
}

// TestLoadTemplates_PRReviewMRAutomationInstruction is AC30: the pr-review
// template's review step must instruct the agent to enable lifecycle
// notifications on whichever provider the task's linked review target is
// on — update_task_pr_automation_kandev for a GitHub PR,
// update_task_mr_automation_kandev for a GitLab MR.
func TestLoadTemplates_PRReviewMRAutomationInstruction(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates() returned error: %v", err)
	}

	var prReview *models.WorkflowTemplate
	for _, tmpl := range templates {
		if tmpl.ID == "pr-review" {
			prReview = tmpl
			break
		}
	}
	if prReview == nil {
		t.Fatal("pr-review template not found")
	}

	var review *models.StepDefinition
	for i := range prReview.Steps {
		if prReview.Steps[i].ID == "review" {
			review = &prReview.Steps[i]
			break
		}
	}
	if review == nil {
		t.Fatal("pr-review template has no review step")
	}
	for _, required := range []string{
		"update_task_pr_automation_kandev",
		"update_task_mr_automation_kandev",
		"prompt_on_review_requested",
		"prompt_on_merged",
		"prompt_on_closed",
	} {
		if !strings.Contains(review.Prompt, required) {
			t.Errorf("review prompt must contain %q", required)
		}
	}
}
