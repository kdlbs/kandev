package models

import "testing"

func reviewStepEvents(config map[string]any) StepEvents {
	return StepEvents{
		OnEnter: []OnEnterAction{
			{Type: OnEnterAutoStartAgent},
			{Type: OnEnterRunCodeReview, Config: config},
		},
	}
}

func reviewActionConfig(t *testing.T, events StepEvents) map[string]any {
	t.Helper()
	for _, a := range events.OnEnter {
		if a.Type == OnEnterRunCodeReview {
			return a.Config
		}
	}
	t.Fatal("run_code_review action missing from on_enter")
	return nil
}

func TestConvertReviewProfileToPortable_ReplacesInstanceID(t *testing.T) {
	resolve := func(id string) *AgentProfilePortable {
		if id != "profile-local" {
			return nil
		}
		return &AgentProfilePortable{AgentName: "codex", Model: "gpt-5.3", Mode: "review"}
	}

	events := ConvertReviewProfileToPortable(
		reviewStepEvents(map[string]any{ReviewAgentProfileConfigKey: "profile-local"}), resolve)

	config := reviewActionConfig(t, events)
	if _, leaked := config[ReviewAgentProfileConfigKey]; leaked {
		t.Fatal("the instance profile id must not survive export")
	}
	descriptor, ok := config[ReviewAgentProfilePortableKey].(map[string]any)
	if !ok {
		t.Fatalf("expected a portable descriptor, got %#v", config[ReviewAgentProfilePortableKey])
	}
	if descriptor["agent_name"] != "codex" || descriptor["model"] != "gpt-5.3" || descriptor["mode"] != "review" {
		t.Fatalf("unexpected descriptor: %#v", descriptor)
	}
	if len(events.OnEnter) != 2 || events.OnEnter[0].Type != OnEnterAutoStartAgent {
		t.Fatalf("other on_enter actions must be preserved: %+v", events.OnEnter)
	}
}

func TestConvertReviewProfileToPortable_DropsUnresolvableProfile(t *testing.T) {
	// A dangling id would import as a reference to a profile that does not
	// exist; dropping it makes the action fall back to the code-review utility
	// agent instead.
	events := ConvertReviewProfileToPortable(
		reviewStepEvents(map[string]any{ReviewAgentProfileConfigKey: "gone"}),
		func(string) *AgentProfilePortable { return nil })

	config := reviewActionConfig(t, events)
	if _, leaked := config[ReviewAgentProfileConfigKey]; leaked {
		t.Fatal("an unresolvable profile id must be dropped, not exported")
	}
	if _, present := config[ReviewAgentProfilePortableKey]; present {
		t.Fatal("no portable descriptor should be written for an unresolvable profile")
	}
}

func TestConvertReviewProfileToPortable_NilResolverAndEmptyConfig(t *testing.T) {
	events := ConvertReviewProfileToPortable(
		reviewStepEvents(map[string]any{ReviewAgentProfileConfigKey: "profile-local"}), nil)
	if _, leaked := reviewActionConfig(t, events)[ReviewAgentProfileConfigKey]; leaked {
		t.Fatal("without a resolver the instance id must still be dropped")
	}

	// A profile-less action is the common case and must round-trip untouched.
	bare := ConvertReviewProfileToPortable(reviewStepEvents(nil), nil)
	if cfg := reviewActionConfig(t, bare); cfg != nil {
		t.Fatalf("expected nil config preserved, got %#v", cfg)
	}
}

func TestConvertReviewProfileToID_MatchesLocalProfile(t *testing.T) {
	match := func(agentName, model, mode string) string {
		if agentName == "codex" && model == "gpt-5.3" && mode == "review" {
			return "profile-imported"
		}
		return ""
	}

	events := ConvertReviewProfileToID(reviewStepEvents(map[string]any{
		ReviewAgentProfilePortableKey: map[string]any{
			"agent_name": "codex", "model": "gpt-5.3", "mode": "review",
		},
	}), match)

	config := reviewActionConfig(t, events)
	if config[ReviewAgentProfileConfigKey] != "profile-imported" {
		t.Fatalf("expected the matched profile id, got %#v", config[ReviewAgentProfileConfigKey])
	}
	if _, leaked := config[ReviewAgentProfilePortableKey]; leaked {
		t.Fatal("the portable descriptor must be consumed on import")
	}
}

func TestConvertReviewProfileToID_DropsUnmatchedDescriptor(t *testing.T) {
	events := ConvertReviewProfileToID(reviewStepEvents(map[string]any{
		ReviewAgentProfilePortableKey: map[string]any{"agent_name": "unknown"},
	}), func(string, string, string) string { return "" })

	config := reviewActionConfig(t, events)
	if _, present := config[ReviewAgentProfileConfigKey]; present {
		t.Fatal("an unmatched descriptor must not produce a profile id")
	}
	if _, present := config[ReviewAgentProfilePortableKey]; present {
		t.Fatal("an unmatched descriptor must be dropped so no dangling reference survives")
	}
}

func TestReviewProfileRoundTripAcrossWorkspaces(t *testing.T) {
	resolve := func(string) *AgentProfilePortable {
		return &AgentProfilePortable{AgentName: "claude", Model: "haiku"}
	}
	match := func(agentName, model, _ string) string {
		if agentName == "claude" && model == "haiku" {
			return "profile-in-target-workspace"
		}
		return ""
	}

	exported := ConvertReviewProfileToPortable(
		reviewStepEvents(map[string]any{ReviewAgentProfileConfigKey: "profile-in-source-workspace"}), resolve)
	imported := ConvertReviewProfileToID(exported, match)

	if got := reviewActionConfig(t, imported)[ReviewAgentProfileConfigKey]; got != "profile-in-target-workspace" {
		t.Fatalf("round trip should land on the target workspace's profile, got %#v", got)
	}
}

func TestReviewActionSurvivesExportValidation(t *testing.T) {
	export := &WorkflowExport{
		Version: ExportVersion,
		Type:    ExportType,
		Workflows: []WorkflowPortable{{
			Name: "With review",
			Steps: []StepPortable{{
				Name:     "Review",
				Position: 0,
				Events:   reviewStepEvents(map[string]any{ReviewAgentProfilePortableKey: map[string]any{"agent_name": "claude"}}),
			}},
		}},
	}
	if err := export.Validate(); err != nil {
		t.Fatalf("a run_code_review action must pass export validation: %v", err)
	}
}
