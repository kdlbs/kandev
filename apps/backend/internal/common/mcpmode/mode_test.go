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
