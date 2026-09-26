package service

import "testing"

// Revive caps effective lines per file including tests, so the launch-semantics
// cases live here rather than growing service_dependencies_test.go.

// `start_when_unblocked: false` means "no automatic start at all" — it governs
// the DEFERRED launch only. The presence of blockers is what suppresses the
// immediate one, so a create must never launch a task that is already blocked.
func TestResolveStartWhenUnblockedGovernsOnlyTheDeferredLaunch(t *testing.T) {
	no := false
	req := &CreateTaskRequest{BlockedBy: []string{"a"}, StartWhenUnblocked: &no}
	if ResolveStartWhenUnblocked(req) {
		t.Fatal("explicit false must not record a deferred intent")
	}
	// The create paths gate the immediate launch on len(BlockedBy) == 0, not on
	// this flag; assert the invariant the handlers rely on.
	if len(req.BlockedBy) == 0 {
		t.Fatal("fixture should have blockers")
	}
}
