package orchestrator

import "testing"

func TestDefaultSessionCeilingIsDisabled(t *testing.T) {
	if got := DefaultServiceConfig().SessionCapacity; got != unlimitedSessionCeiling {
		t.Fatalf("default session capacity = %d, want disabled (%d)", got, unlimitedSessionCeiling)
	}
}
