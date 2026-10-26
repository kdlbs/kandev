package api

import (
	"testing"

	"github.com/kandev/kandev/internal/common/subproc"
)

func TestIdentityAdvertisesManagedGitPolicyWithTheFlagOffAndOn(t *testing.T) {
	previous := subproc.ManagedGitIsolation()
	t.Cleanup(func() { subproc.SetManagedGitIsolation(previous) })
	for _, enabled := range []bool{false, true} {
		subproc.SetManagedGitIsolation(enabled)
		found := false
		for _, capability := range SurvivalCapabilities {
			found = found || capability == "managed-git-policy.v1"
		}
		if !found {
			t.Fatalf("flag=%v: agentctl does not advertise managed-git-policy.v1: %v", enabled, SurvivalCapabilities)
		}
	}
}
