//go:build !darwin

package process

import "testing"

// TestWalkProcessTree_DarwinStartTimeSource is the non-Darwin sibling of the
// same-named test in probe_darwin_test.go (//go:build darwin, which cannot
// compile here — it uses Darwin-only x/sys/unix APIs). Per spec.md's
// "Start-time source and resolution" section, AC-80's Darwin instance is
// host-gated and MUST t.Skip with an explicit platform reason rather than
// vanish from the CI log via a build tag with no counterpart — this file is
// that counterpart, so the test name is always visible: PASS on a
// maintainer's Darwin machine, SKIP on ubuntu-latest/windows-latest.
func TestWalkProcessTree_DarwinStartTimeSource(t *testing.T) {
	t.Skip("AC-80 Darwin instance requires a darwin host")
}
