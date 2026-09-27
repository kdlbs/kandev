package mcpmode

import "testing"

func TestInstanceModes(t *testing.T) {
	for _, mode := range InstanceModes() {
		if !IsInstanceMode(mode) {
			t.Errorf("IsInstanceMode(%q) = false, want true", mode)
		}
	}

	if IsInstanceMode(External) {
		t.Errorf("IsInstanceMode(%q) = true, want false", External)
	}
}

// TestCoordinator_IsAnInstanceMode proves the coordinator MCP mode value is
// accepted by the agentctl instance API, wired in by task 03.
func TestCoordinator_IsAnInstanceMode(t *testing.T) {
	if Coordinator != "coordinator" {
		t.Errorf("Coordinator = %q, want %q", Coordinator, "coordinator")
	}
	if !IsInstanceMode(Coordinator) {
		t.Errorf("IsInstanceMode(%q) = false, want true", Coordinator)
	}
}
