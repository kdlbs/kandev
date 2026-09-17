package models

import (
	"strings"
	"testing"
)

func TestDelegationContextPreservesRoutingAndLimitsInput(t *testing.T) {
	a := &AgentInstance{Settings: `{"routing":{"execution_profile_id":"work"}}`}
	if err := SetDelegationContext(a, "Use for work backend tasks only."); err != nil {
		t.Fatal(err)
	}
	if DelegationContext(a) != "Use for work backend tasks only." || !strings.Contains(a.Settings, "execution_profile_id") {
		t.Fatal(a.Settings)
	}
	previous := a.Settings
	if err := SetDelegationContext(a, strings.Repeat("a", 2001)); err == nil {
		t.Fatal("accepted unbounded context")
	}
	if a.Settings != previous {
		t.Fatal("failed update mutated settings")
	}
	if err := SetDelegationContext(a, ""); err != nil || DelegationContext(a) != "" {
		t.Fatal("cannot clear context")
	}
}
