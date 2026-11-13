package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/auth"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.18
func TestManagedRuntimeOwnerModeWiring(t *testing.T) {
	if _, allowed := singleUserManagedRuntimeOwner(nil)(); allowed {
		t.Fatal("unavailable auth service supplied a single-user repair owner")
	}
	if _, allowed := singleUserManagedRuntimeOwner(&auth.Service{})(); !allowed {
		t.Fatal("disabled auth service did not supply the established single-user owner")
	}
	owner, allowed := singleUserManagedRuntimeOwner(&auth.Service{})()
	if !allowed || owner != userstore.DefaultUserID {
		t.Fatalf("disabled auth owner = (%q, %v), want default user", owner, allowed)
	}
}
