package engine

import (
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func compiledOnEnterReviewAction(t *testing.T, config map[string]any) (Action, bool) {
	t.Helper()
	step := &wfmodels.WorkflowStep{
		ID:   "step-1",
		Name: "Review",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterRunCodeReview, Config: config},
			},
		},
	}
	for _, action := range CompileStep(step).Events[TriggerOnEnter] {
		if action.Kind == ActionRunCodeReview {
			return action, true
		}
	}
	return Action{}, false
}

func TestCompileStep_RunCodeReviewWithProfile(t *testing.T) {
	action, ok := compiledOnEnterReviewAction(t, map[string]any{
		wfmodels.ReviewAgentProfileConfigKey: "profile-7",
	})
	if !ok {
		t.Fatal("expected a run_code_review action to compile")
	}
	if action.RunCodeReview == nil {
		t.Fatal("expected the typed RunCodeReview payload to be set")
	}
	if action.RunCodeReview.AgentProfileID != "profile-7" {
		t.Fatalf("expected the configured profile, got %q", action.RunCodeReview.AgentProfileID)
	}
}

func TestCompileStep_RunCodeReviewWithoutProfileIsStillCompiled(t *testing.T) {
	// No profile means "use the configured code-review utility agent", which is
	// the default on-demand behaviour — the action must not be dropped.
	for name, config := range map[string]map[string]any{
		"nil config":     nil,
		"empty config":   {},
		"blank profile":  {wfmodels.ReviewAgentProfileConfigKey: ""},
		"wrong type":     {wfmodels.ReviewAgentProfileConfigKey: 42},
		"unrelated keys": {"something": "else"},
	} {
		t.Run(name, func(t *testing.T) {
			action, ok := compiledOnEnterReviewAction(t, config)
			if !ok {
				t.Fatal("expected a run_code_review action to compile")
			}
			if action.RunCodeReview == nil {
				t.Fatal("expected the typed RunCodeReview payload to be set")
			}
			if action.RunCodeReview.AgentProfileID != "" {
				t.Fatalf("expected no profile, got %q", action.RunCodeReview.AgentProfileID)
			}
		})
	}
}

func TestCompileStep_RunCodeReviewCoexistsWithOtherEntryActions(t *testing.T) {
	step := &wfmodels.WorkflowStep{
		ID:   "step-1",
		Name: "Review",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{
				{Type: wfmodels.OnEnterResetAgentContext},
				{Type: wfmodels.OnEnterRunCodeReview},
				{Type: wfmodels.OnEnterAutoStartAgent},
			},
		},
	}
	kinds := make([]ActionKind, 0, 3)
	for _, action := range CompileStep(step).Events[TriggerOnEnter] {
		kinds = append(kinds, action.Kind)
	}
	want := []ActionKind{ActionResetAgentContext, ActionRunCodeReview, ActionAutoStartAgent}
	if len(kinds) != len(want) {
		t.Fatalf("expected %d actions, got %v", len(want), kinds)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("action order changed: got %v want %v", kinds, want)
		}
	}
}

func TestRunCodeReviewUnwiredIsANoOp(t *testing.T) {
	// A deployment without the review runner has no callback for the kind. The
	// engine must treat it as a no-op rather than failing step entry, so a task
	// never gets stuck because the feature is unconfigured.
	action, ok := compiledOnEnterReviewAction(t, nil)
	if !ok {
		t.Fatal("expected a run_code_review action to compile")
	}
	registry := MapRegistry{}
	if _, found := registry.Get(action.Kind); found {
		t.Fatal("expected no callback for run_code_review in an unwired registry")
	}
}
