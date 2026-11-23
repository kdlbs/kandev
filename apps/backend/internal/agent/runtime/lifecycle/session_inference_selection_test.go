package lifecycle

import (
	"context"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestSessionInferenceUsesSelectedManagedRuntimeVersion(t *testing.T) {
	copilot := agents.NewCopilotACP()
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = &copilotSelectionReader{version: "1.0.104"}

	if _, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", copilot.ID(), "", "hi"); err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	recorded := harness.recorded()
	if len(recorded) != 1 {
		t.Fatalf("agentctl calls = %d, want 1", len(recorded))
	}
	version, ok := copilotplugins.RuntimeVersionFromCommand(recorded[0].InferenceConfig.Command)
	if !ok || version != "1.0.104" {
		t.Fatalf("session inference command = %v, want the selected @github/copilot@1.0.104", recorded[0].InferenceConfig.Command)
	}
}

type openCodeV2SelectionReader struct{ spec agents.ManagedNPMRuntimeSpec }

func (r openCodeV2SelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, nil
}

func (r openCodeV2SelectionReader) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	return managedruntime.OpenCodeSelection{
		SchemaVersion:         1,
		Family:                managedruntime.OpenCodeFamilyV2,
		Source:                managedruntime.OpenCodeSourceManaged,
		Package:               r.spec.Package,
		AppliedDefaultVersion: r.spec.DefaultVersionOrPinned(),
		Revision:              1,
	}, true, nil
}

func TestSessionInferenceUsesSelectedOpenCodeFamily(t *testing.T) {
	openCode := agents.NewOpenCodeACP()
	v2, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	if err != nil {
		t.Fatal(err)
	}
	harness := newInferenceHarness(t, nil)
	harness.manager.managedRuntimeSelections = openCodeV2SelectionReader{spec: v2}

	if _, err := harness.manager.ExecuteInferencePrompt(t.Context(), "session-1", openCode.ID(), "", "hi"); err != nil {
		t.Fatalf("ExecuteInferencePrompt: %v", err)
	}
	got := harness.recorded()[0].InferenceConfig.Command
	if !slices.Contains(got, v2.PackageSpec(v2.DefaultVersionOrPinned())) {
		t.Fatalf("session inference command = %v, want the selected v2 package %s", got, v2.PackageSpec(v2.DefaultVersionOrPinned()))
	}
}
