package coordinator

import (
	"slices"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestPhaseOneToolNamesMatchTheProfilePackage(t *testing.T) {
	want := ToolNames(PhaseOnePolicy(), false, false)
	got := mcpprofile.BoundCoordinatorToolNames(mcpprofile.Context{})
	if !slices.Equal(want, got) {
		t.Fatalf("mcpprofile phase-1 names %v differ from ToolNames %v", got, want)
	}
}
