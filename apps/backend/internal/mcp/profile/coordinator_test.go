package profile

import (
	"testing"

	"github.com/kandev/kandev/internal/common/mcpmode"
)

// TestSurfaceCoordinator_Value pins the coordinator MCP surface value used by
// the copilot's conversation profile.
func TestSurfaceCoordinator_Value(t *testing.T) {
	if SurfaceCoordinator != "coordinator" {
		t.Errorf("SurfaceCoordinator = %q, want %q", SurfaceCoordinator, "coordinator")
	}
}

// TestLegacyCoordinatorHasNoQuestionCapability proves Legacy(mcpmode.Coordinator, ...)
// resolves to the coordinator surface with no ask-question tool group: the
// coordinator's six-tool allowlist does not include ask_question_kandev.
func TestLegacyCoordinatorHasNoQuestionCapability(t *testing.T) {
	ctx := Legacy(mcpmode.Coordinator, false, nil)
	if ctx.Surface != SurfaceCoordinator {
		t.Fatalf("surface = %q, want %q", ctx.Surface, SurfaceCoordinator)
	}
	if ctx.HasCapability(CapabilityUserQuestion) || ctx.HasCapability(CapabilityParentQuestion) {
		t.Fatalf("coordinator capabilities = %#v, want no question capability", ctx.Capabilities)
	}
}

// TestNormalizeSurfaceKeepsCoordinator proves the coordinator surface is not
// silently downgraded to the Kanban default.
func TestNormalizeSurfaceKeepsCoordinator(t *testing.T) {
	ctx := New(SurfaceCoordinator, nil, nil)
	if ctx.Surface != SurfaceCoordinator {
		t.Fatalf("surface = %q, want %q", ctx.Surface, SurfaceCoordinator)
	}
}
