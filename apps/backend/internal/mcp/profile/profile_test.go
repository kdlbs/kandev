package profile

import "testing"

func TestLegacyTaskTitleProfile(t *testing.T) {
	ctx := Legacy("task-title-pending", false, []string{"gitlab", "gitlab", "github"})
	if ctx.Surface != SurfaceKanbanTask {
		t.Fatalf("surface = %q, want %q", ctx.Surface, SurfaceKanbanTask)
	}
	if !ctx.HasCapability(CapabilityTaskTitle) || !ctx.HasCapability(CapabilityUserQuestion) {
		t.Fatalf("capabilities = %#v, want title and user question", ctx.Capabilities)
	}
	if got, want := ctx.Providers, []string{"github", "gitlab"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("providers = %#v, want %#v", got, want)
	}
}

func TestAutopilotQuestionCapabilitiesCanBeSwapped(t *testing.T) {
	ctx := New(SurfaceKanbanTask, []Capability{CapabilityParentQuestion}, nil)
	if !ctx.HasCapability(CapabilityParentQuestion) {
		t.Fatal("parent-question capability missing")
	}
	ctx = ctx.WithoutCapability(CapabilityParentQuestion).WithCapability(CapabilityUserQuestion)
	if ctx.HasCapability(CapabilityParentQuestion) || !ctx.HasCapability(CapabilityUserQuestion) {
		t.Fatalf("capabilities = %#v, want only user-question", ctx.Capabilities)
	}
}

func TestWithoutCapabilityDoesNotMutateSourceProfile(t *testing.T) {
	source := New(SurfaceKanbanTask, []Capability{CapabilityParentQuestion, CapabilityTaskTitle}, nil)
	derived := source.WithoutCapability(CapabilityParentQuestion)

	if !source.HasCapability(CapabilityParentQuestion) || !source.HasCapability(CapabilityTaskTitle) {
		t.Fatalf("source capabilities = %#v, want both capabilities preserved", source.Capabilities)
	}
	if derived.HasCapability(CapabilityParentQuestion) || !derived.HasCapability(CapabilityTaskTitle) {
		t.Fatalf("derived capabilities = %#v, want only task title", derived.Capabilities)
	}
}

func TestLegacyExternalHasNoQuestionCapability(t *testing.T) {
	ctx := Legacy("external", false, nil)
	if ctx.HasCapability(CapabilityUserQuestion) || ctx.HasCapability(CapabilityParentQuestion) {
		t.Fatalf("external capabilities = %#v, want no question capability", ctx.Capabilities)
	}
}

func TestLegacyAutomationHasNoQuestionCapability(t *testing.T) {
	ctx := Legacy("automation", false, nil)
	if ctx.Surface != SurfaceAutomation {
		t.Fatalf("surface = %q, want %q", ctx.Surface, SurfaceAutomation)
	}
	if ctx.HasCapability(CapabilityUserQuestion) || ctx.HasCapability(CapabilityParentQuestion) {
		t.Fatalf("automation capabilities = %#v, want no question capability", ctx.Capabilities)
	}
}
